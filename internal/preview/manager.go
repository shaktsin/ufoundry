// Package preview manages long-running development servers in isolated compute.
package preview

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/shaktsin/umcode/internal/compute"
	"github.com/shaktsin/umcode/internal/protocol"
)

const maxPreviewOutput = 256 << 10

type Event struct {
	Kind    string
	Session protocol.PreviewSession
	Output  string
	Error   string
}

type StartRequest struct {
	ThreadID     string
	ProjectID    string
	Title        string
	Root         string
	Dir          string
	Command      string
	GuestPort    int
	Network      bool
	VCPUs        int
	MemoryMiB    int
	DiskLimitMiB int
	ReadyTimeout time.Duration
	Progress     func(string)
}

type managed struct {
	mu      sync.Mutex
	public  protocol.PreviewSession
	process compute.PreviewProcess
	output  *outputBuffer
	done    chan error
}

type Manager struct {
	ctx      context.Context
	runner   compute.PreviewRunner
	onEvent  func(Event)
	mu       sync.RWMutex
	sessions map[string]*managed
	ready    func(string) bool
}

func NewManager(ctx context.Context, runner compute.PreviewRunner, onEvent func(Event)) *Manager {
	return &Manager{ctx: ctx, runner: runner, onEvent: onEvent, sessions: map[string]*managed{}, ready: portReady}
}

func (m *Manager) Start(req StartRequest) (protocol.PreviewSession, error) {
	if m.runner == nil {
		return protocol.PreviewSession{}, errors.New("isolated preview compute is unavailable")
	}
	if req.ThreadID == "" || req.ProjectID == "" {
		return protocol.PreviewSession{}, errors.New("live preview requires a project chat")
	}
	if req.GuestPort < 1 || req.GuestPort > 65535 {
		return protocol.PreviewSession{}, errors.New("preview port must be between 1 and 65535")
	}
	if req.ReadyTimeout <= 0 {
		req.ReadyTimeout = 30 * time.Second
	}
	id, err := newID()
	if err != nil {
		return protocol.PreviewSession{}, err
	}
	output := &outputBuffer{progress: req.Progress}
	process, err := m.runner.StartPreview(m.ctx, compute.Request{
		Root: req.Root, Dir: req.Dir, Command: req.Command, Network: req.Network,
		VCPUs: req.VCPUs, MemoryMiB: req.MemoryMiB, DiskLimitBytes: int64(req.DiskLimitMiB) << 20,
		Stdout: output, Stderr: output,
	}, req.GuestPort)
	if err != nil {
		return protocol.PreviewSession{}, err
	}
	title := req.Title
	if title == "" {
		title = "Live preview"
	}
	s := &managed{public: protocol.PreviewSession{
		ID: id, ThreadID: req.ThreadID, ProjectID: req.ProjectID, Title: title,
		URL: fmt.Sprintf("http://127.0.0.1:%d/", process.HostPort()), Status: "starting",
	}, process: process, output: output, done: make(chan error, 1)}
	output.event = func(chunk string) {
		m.emit(Event{Kind: "output", Session: s.metadata(), Output: chunk})
	}
	m.mu.Lock()
	for existingID, existing := range m.sessions {
		if existing.public.ThreadID == req.ThreadID {
			existing.process.Stop()
			delete(m.sessions, existingID)
		}
	}
	m.sessions[id] = s
	m.mu.Unlock()
	go m.wait(id, s)

	deadline := time.NewTimer(req.ReadyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	address := fmt.Sprintf("127.0.0.1:%d", process.HostPort())
	for {
		select {
		case err := <-s.done:
			if err == nil {
				err = errors.New("preview server exited before it became ready")
			}
			return protocol.PreviewSession{}, fmt.Errorf("preview server did not start: %w", err)
		case <-deadline.C:
			process.Stop()
			return protocol.PreviewSession{}, fmt.Errorf("preview server was not reachable on port %d within %s", req.GuestPort, req.ReadyTimeout)
		case <-tick.C:
			if !m.ready(address) {
				continue
			}
			m.mu.RLock()
			current := m.sessions[id]
			m.mu.RUnlock()
			if current != nil {
				current.setStatus("ready")
			}
			ready := s.snapshot()
			m.emit(Event{Kind: "started", Session: ready})
			return ready, nil
		}
	}
}

func portReady(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (m *Manager) wait(id string, s *managed) {
	err := s.process.Wait()
	s.done <- err
	close(s.done)
	m.mu.Lock()
	if m.sessions[id] == s {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	public := s.snapshot()
	public.Status = "stopped"
	public.Output = s.output.String()
	e := Event{Kind: "stopped", Session: public}
	if err != nil && !errors.Is(err, context.Canceled) {
		e.Error = err.Error()
	}
	m.emit(e)
}

func (m *Manager) Stop(id string) error {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s == nil {
		return errors.New("preview is not running")
	}
	s.process.Stop()
	return nil
}

// StopFor stops a preview only when it belongs to the calling task. This keeps
// agent tools scoped even if a preview ID is supplied by untrusted content.
func (m *Manager) StopFor(id, threadID string) error {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s == nil || threadID == "" || s.metadata().ThreadID != threadID {
		return errors.New("preview is not running for this task")
	}
	s.process.Stop()
	return nil
}

// GetFor returns a preview only when it belongs to the calling task.
func (m *Manager) GetFor(id, threadID string) (protocol.PreviewSession, error) {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s == nil || threadID == "" || s.metadata().ThreadID != threadID {
		return protocol.PreviewSession{}, errors.New("preview is not running for this task")
	}
	return s.snapshot(), nil
}

func (m *Manager) List() []protocol.PreviewSession {
	m.mu.RLock()
	out := make([]protocol.PreviewSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.snapshot())
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) Close() {
	m.mu.RLock()
	list := make([]*managed, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.RUnlock()
	for _, s := range list {
		s.process.Stop()
	}
}

func (m *Manager) emit(e Event) {
	if m.onEvent != nil {
		m.onEvent(e)
	}
}

func (s *managed) snapshot() protocol.PreviewSession {
	out := s.metadata()
	out.Output = s.output.String()
	return out
}

func (s *managed) metadata() protocol.PreviewSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.public
}

func (s *managed) setStatus(status string) {
	s.mu.Lock()
	s.public.Status = status
	s.mu.Unlock()
}

type outputBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	progress func(string)
	event    func(string)
}

func (w *outputBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	var accepted []byte
	if remain := maxPreviewOutput - w.buf.Len(); remain > 0 {
		accepted = p
		if len(accepted) > remain {
			accepted = accepted[:remain]
		}
		_, _ = w.buf.Write(accepted)
	}
	progress, event := w.progress, w.event
	w.mu.Unlock()
	if progress != nil && len(accepted) > 0 {
		progress(string(accepted))
	}
	if event != nil && len(accepted) > 0 {
		event(string(accepted))
	}
	return len(p), nil
}

func (w *outputBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func newID() (string, error) {
	var b [12]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return "prv_" + hex.EncodeToString(b[:]), nil
}
