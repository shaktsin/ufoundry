package preview

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shaktsin/umcode/internal/compute"
)

type fakeRunner struct {
	mu      sync.Mutex
	process *fakeProcess
	request compute.Request
	port    int
}

func (r *fakeRunner) Run(context.Context, compute.Request) (int, error) { return 0, nil }
func (r *fakeRunner) StartPreview(_ context.Context, req compute.Request, port int) (compute.PreviewProcess, error) {
	p := &fakeProcess{port: 32123, done: make(chan error, 1)}
	r.mu.Lock()
	r.process, r.request, r.port = p, req, port
	r.mu.Unlock()
	_, _ = io.WriteString(req.Stdout, "ready\n")
	return p, nil
}

type fakeProcess struct {
	port int
	done chan error
	once sync.Once
}

func (p *fakeProcess) HostPort() int { return p.port }
func (p *fakeProcess) Wait() error   { return <-p.done }
func (p *fakeProcess) Stop() {
	p.once.Do(func() {
		p.done <- context.Canceled
		close(p.done)
	})
}

func TestManagerStartsListsAndStopsPreview(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{}
	events := make(chan Event, 8)
	m := NewManager(ctx, runner, func(event Event) { events <- event })
	m.ready = func(string) bool { return true }
	session, err := m.Start(StartRequest{
		ThreadID: "thread-1", ProjectID: "project-1", Title: "Web",
		Root: t.TempDir(), Dir: t.TempDir(), Command: "npm run dev", GuestPort: 5173,
		Network: true, ReadyTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "ready" || session.URL == "" || session.Output != "ready\n" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if got := m.List(); len(got) != 1 || got[0].ID != session.ID {
		t.Fatalf("list = %+v", got)
	}
	if err := m.StopFor(session.ID, "another-thread"); err == nil {
		t.Fatal("cross-task preview stop unexpectedly succeeded")
	}
	if got := m.List(); len(got) != 1 {
		t.Fatal("cross-task stop removed the preview")
	}
	if err := m.StopFor(session.ID, "thread-1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Kind == "stopped" {
				if len(m.List()) != 0 {
					t.Fatal("stopped preview remains in active list")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for stopped event")
		}
	}
}

func TestOutputBufferCapsStoredAndStreamedOutput(t *testing.T) {
	var streamed int
	w := &outputBuffer{event: func(chunk string) { streamed += len(chunk) }}
	p := []byte(strings.Repeat("x", maxPreviewOutput+1024))
	if n, err := w.Write(p); err != nil || n != len(p) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if got := len(w.String()); got != maxPreviewOutput {
		t.Fatalf("stored output = %d bytes, want %d", got, maxPreviewOutput)
	}
	if streamed != maxPreviewOutput {
		t.Fatalf("streamed output = %d bytes, want %d", streamed, maxPreviewOutput)
	}
	if _, err := w.Write([]byte("ignored")); err != nil {
		t.Fatal(err)
	}
	if streamed != maxPreviewOutput {
		t.Fatal("output streamed after the cap")
	}
}

func TestManagerRejectsUnavailablePreview(t *testing.T) {
	m := NewManager(context.Background(), nil, nil)
	_, err := m.Start(StartRequest{})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
