// Package visualqa owns isolated, app-managed Chromium sessions used to test
// live previews as a user would. Project code remains in compute; only the
// preview's scoped loopback URL is opened by the browser.
package visualqa

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type Artifact struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	MimeType string `json:"mime_type"`
	Bytes    int64  `json:"bytes"`
}

type Report struct {
	Status      string     `json:"status"`
	Framework   string     `json:"framework"`
	DurationMS  int64      `json:"duration_ms"`
	Output      string     `json:"output,omitempty"`
	Diagnostics []string   `json:"diagnostics"`
	Artifacts   []Artifact `json:"artifacts"`
	Reason      string     `json:"reason,omitempty"`
	SessionID   string     `json:"session_id,omitempty"`
	Snapshot    string     `json:"snapshot,omitempty"`
}

type Manager struct {
	ctx      context.Context
	mu       sync.Mutex
	sessions map[string]*Session
}

type Session struct {
	id, threadID, root, artifactDir, profile string
	allowedOrigin                            string
	browserStderr                            func() string
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	cmd                                      *exec.Cmd
	conn                                     *websocket.Conn
	mu                                       sync.Mutex
	next                                     atomic.Int64
	pending                                  map[int64]chan response
	diagnostics                              []string
}

const (
	startupTimeout        = 30 * time.Second
	commandTimeout        = 5 * time.Second
	websocketMessageLimit = 12 << 20 // accommodates base64 screenshots up to the 8 MiB artifact cap
)

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}
type envelope struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func NewManager(ctx context.Context) *Manager {
	return &Manager{ctx: ctx, sessions: map[string]*Session{}}
}

func (m *Manager) Start(threadID, root, target string) (Report, error) {
	started := time.Now()
	startupCtx, cancelStartup := context.WithTimeout(m.ctx, startupTimeout)
	defer cancelStartup()
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Port() == "" {
		return Report{}, errors.New("visual QA may only open a scoped localhost preview URL")
	}
	browser, err := chromiumPath()
	if err != nil {
		return Report{Status: "not_run", Framework: "umcode-visual-qa", Diagnostics: []string{}, Artifacts: []Artifact{}, Reason: err.Error()}, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	profile, err := os.MkdirTemp("", "umcode-visual-qa-")
	if err != nil {
		cancel()
		return Report{}, err
	}
	cmd := exec.CommandContext(ctx, browser, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "--disable-sync", "--disable-background-networking",
		"--disable-component-update", "--disable-features=Translate", "about:blank")
	home, err := os.UserHomeDir()
	if err != nil {
		cancel()
		_ = os.RemoveAll(profile)
		return Report{}, fmt.Errorf("resolve home for isolated Chromium: %w", err)
	}
	cmd.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=" + home,
		"TMPDIR=" + os.TempDir(),
		"LANG=C.UTF-8",
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return Report{}, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return Report{}, fmt.Errorf("start bundled Chromium: %w", err)
	}
	var stderrMu sync.Mutex
	var stderrOutput bytes.Buffer
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrMu.Lock()
			if stderrOutput.Len() < 8<<10 {
				stderrOutput.WriteString(clip(scanner.Text(), 1200))
				stderrOutput.WriteByte('\n')
			}
			stderrMu.Unlock()
		}
	}()
	cleanup := func() { cancel(); _ = cmd.Wait(); _ = os.RemoveAll(profile) }
	port, err := waitDevTools(profile, cmd, 12*time.Second)
	if err != nil {
		cleanup()
		return Report{}, err
	}
	wsURL, err := createPage(port, "about:blank")
	if err != nil {
		cleanup()
		return Report{}, err
	}
	conn, _, err := websocket.Dial(startupCtx, wsURL, nil)
	if err != nil {
		cleanup()
		return Report{}, fmt.Errorf("connect to Chromium: %w", err)
	}
	id := "visual-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	s := &Session{id: id, threadID: threadID, root: root, artifactDir: filepath.Join(root, ".umcode", "artifacts", "visual-qa", id), profile: profile, allowedOrigin: u.Scheme + "://" + u.Host, ctx: ctx, cancel: cancel, cmd: cmd, conn: conn, pending: map[int64]chan response{}, browserStderr: func() string {
		stderrMu.Lock()
		defer stderrMu.Unlock()
		return strings.TrimSpace(stderrOutput.String())
	}}
	conn.SetReadLimit(websocketMessageLimit)
	go s.read(ctx)
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable"} {
		if _, err := s.call(startupCtx, method, map[string]any{}); err != nil {
			s.Close()
			return Report{}, err
		}
	}
	if _, err := s.call(startupCtx, "Fetch.enable", map[string]any{"patterns": []map[string]string{{"urlPattern": "*"}}}); err != nil {
		s.Close()
		return Report{}, err
	}
	if _, err := s.call(startupCtx, "Page.navigate", map[string]any{"url": target}); err != nil {
		s.Close()
		return Report{}, err
	}
	if err := s.waitReady(startupCtx, 15*time.Second); err != nil {
		s.Close()
		return Report{}, err
	}
	m.mu.Lock()
	if old := m.sessions[threadID]; old != nil {
		old.Close()
	}
	m.sessions[threadID] = s
	m.mu.Unlock()
	snapshot, _ := s.Snapshot(startupCtx)
	artifact, err := s.Screenshot(startupCtx, "initial.png")
	if err != nil {
		s.addDiagnostic("screenshot: " + err.Error())
	}
	report := s.Report("passed", snapshot)
	report.DurationMS = time.Since(started).Milliseconds()
	if artifact.Path != "" {
		report.Artifacts = append(report.Artifacts, artifact)
	}
	return report, nil
}

func (m *Manager) ForThread(threadID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[threadID]
}
func (m *Manager) Stop(threadID string) {
	m.mu.Lock()
	s := m.sessions[threadID]
	delete(m.sessions, threadID)
	m.mu.Unlock()
	if s != nil {
		s.Close()
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	all := m.sessions
	m.sessions = map[string]*Session{}
	m.mu.Unlock()
	for _, s := range all {
		s.Close()
	}
}

func chromiumPath() (string, error) {
	if p := os.Getenv("UMCODE_CHROMIUM_PATH"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	exe, _ := os.Executable()
	resources := filepath.Dir(exe)
	if filepath.Base(resources) == "MacOS" {
		resources = filepath.Join(filepath.Dir(resources), "Resources")
	}
	candidates := []string{filepath.Join(resources, "browser", "Chromium.app", "Contents", "MacOS", "Chromium"), filepath.Join(resources, "browser", "chromium")}
	candidates = append(candidates, filepath.Join(resources, "browser", "Chromium.app", "Contents", "MacOS", "Google Chrome"))
	if os.Getenv("UMCODE_VISUAL_QA_ALLOW_SYSTEM_BROWSER") == "1" && runtime.GOOS == "darwin" {
		candidates = append(candidates, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium")
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("the Visual QA plugin is enabled, but its pinned Chromium runtime is not installed")
}

func waitDevTools(profile string, cmd *exec.Cmd, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	path := filepath.Join(profile, "DevToolsActivePort")
	for time.Now().Before(deadline) {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		if b, err := os.ReadFile(path); err == nil {
			if line := strings.Split(string(b), "\n")[0]; line != "" {
				return line, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", errors.New("Chromium did not expose its automation endpoint")
}

func createPage(port, target string) (string, error) {
	req, _ := http.NewRequest(http.MethodPut, "http://127.0.0.1:"+port+"/json/new?"+url.QueryEscape(target), nil)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var page struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil || page.WebSocketDebuggerURL == "" {
		return "", errors.New("Chromium did not create a test page")
	}
	return page.WebSocketDebuggerURL, nil
}

func (s *Session) read(ctx context.Context) {
	for {
		_, b, err := s.conn.Read(ctx)
		if err != nil {
			diagnostic := "Chromium automation connection closed: " + err.Error()
			if stderr := s.browserStderr(); stderr != "" {
				diagnostic += "; Chromium stderr: " + clip(stderr, 2000)
			}
			s.addDiagnostic(diagnostic)
			s.mu.Lock()
			pending := s.pending
			s.pending = map[int64]chan response{}
			s.mu.Unlock()
			for _, ch := range pending {
				ch <- response{Error: &struct {
					Message string `json:"message"`
				}{Message: "Chromium automation connection closed"}}
			}
			return
		}
		var e envelope
		if json.Unmarshal(b, &e) != nil {
			continue
		}
		if e.ID != 0 {
			s.mu.Lock()
			ch := s.pending[e.ID]
			delete(s.pending, e.ID)
			s.mu.Unlock()
			if ch != nil {
				ch <- response{Result: e.Result, Error: e.Error}
			}
			continue
		}
		if e.Method == "Runtime.consoleAPICalled" || e.Method == "Runtime.exceptionThrown" || e.Method == "Network.loadingFailed" {
			s.addDiagnostic(e.Method + ": " + clip(string(e.Params), 1200))
		}
		if e.Method == "Fetch.requestPaused" {
			go s.handleRequest(e.Params)
		}
	}
}

func (s *Session) handleRequest(params json.RawMessage) {
	var event struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL string `json:"url"`
		} `json:"request"`
	}
	if json.Unmarshal(params, &event) != nil || event.RequestID == "" {
		return
	}
	u, err := url.Parse(event.Request.URL)
	allowed := err == nil && (u.Scheme == "data" || u.Scheme == "blob" || u.Scheme+"://"+u.Host == s.allowedOrigin)
	if allowed {
		_, _ = s.call(s.ctx, "Fetch.continueRequest", map[string]any{"requestId": event.RequestID})
		return
	}
	s.addDiagnostic("blocked external request: " + event.Request.URL)
	_, _ = s.call(s.ctx, "Fetch.failRequest", map[string]any{"requestId": event.RequestID, "errorReason": "BlockedByClient"})
}

func (s *Session) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	callCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	id := s.next.Add(1)
	ch := make(chan response, 1)
	s.mu.Lock()
	s.pending[id] = ch
	s.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := s.conn.Write(callCtx, websocket.MessageText, b); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case r := <-ch:
		if r.Error != nil {
			return nil, errors.New(r.Error.Message)
		}
		return r.Result, nil
	case <-callCtx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, callCtx.Err()
	}
}

func (s *Session) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, err := s.evaluate(ctx, "document.readyState")
		if err == nil && strings.Contains(string(raw), "complete") {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("preview did not finish loading in Chromium")
}

func (s *Session) evaluate(ctx context.Context, expression string) (json.RawMessage, error) {
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true})
	if err != nil {
		return nil, err
	}
	var out struct {
		Result struct {
			Value       json.RawMessage `json:"value"`
			Description string          `json:"description"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Exception) > 0 {
		return nil, fmt.Errorf("browser expression failed: %s", clip(string(out.Exception), 500))
	}
	return out.Result.Value, nil
}

func (s *Session) Snapshot(ctx context.Context) (string, error) {
	expr := `JSON.stringify({url:location.href,title:document.title,viewport:{width:innerWidth,height:innerHeight},text:(document.body?.innerText||'').slice(0,12000),controls:[...document.querySelectorAll('a,button,input,select,textarea,[role=button]')].slice(0,200).map((e,i)=>({i,tag:e.tagName.toLowerCase(),text:(e.innerText||e.value||e.getAttribute('aria-label')||'').trim().slice(0,200),type:e.type||'',disabled:!!e.disabled,href:e.href||''})),brokenImages:[...document.images].filter(i=>!i.complete||i.naturalWidth===0).map(i=>i.src),horizontalOverflow:document.documentElement.scrollWidth>innerWidth})`
	raw, err := s.evaluate(ctx, expr)
	if err != nil {
		return "", err
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) != nil {
		return string(raw), nil
	}
	return encoded, nil
}

func (s *Session) Act(ctx context.Context, action string, index int, value string) (string, error) {
	a, _ := json.Marshal(action)
	v, _ := json.Marshal(value)
	expr := fmt.Sprintf(`(()=>{const es=[...document.querySelectorAll('a,button,input,select,textarea,[role=button]')];const e=es[%d];if(!e)throw Error('control index not found');const a=%s,v=%s;if(a==='click')e.click();else if(a==='fill'){e.focus();e.value=v;e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}))}else if(a==='press')e.dispatchEvent(new KeyboardEvent('keydown',{key:v,bubbles:true}));else throw Error('unsupported action');return true})()`, index, a, v)
	if _, err := s.evaluate(ctx, expr); err != nil {
		return "", err
	}
	time.Sleep(250 * time.Millisecond)
	return s.Snapshot(ctx)
}

func (s *Session) Screenshot(ctx context.Context, name string) (Artifact, error) {
	if err := os.MkdirAll(s.artifactDir, 0o700); err != nil {
		return Artifact{}, err
	}
	raw, err := s.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": true})
	if err != nil {
		return Artifact{}, err
	}
	var data struct {
		Data string `json:"data"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return Artifact{}, errors.New("invalid screenshot response")
	}
	b, err := base64.StdEncoding.DecodeString(data.Data)
	if err != nil {
		return Artifact{}, err
	}
	path := filepath.Join(s.artifactDir, filepath.Base(name))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return Artifact{}, err
	}
	rel, _ := filepath.Rel(s.root, path)
	return Artifact{Path: filepath.ToSlash(rel), Kind: "screenshot", MimeType: "image/png", Bytes: int64(len(b))}, nil
}

func (s *Session) Report(status, snapshot string) Report {
	s.mu.Lock()
	d := append([]string(nil), s.diagnostics...)
	s.mu.Unlock()
	return Report{Status: status, Framework: "umcode-visual-qa", Diagnostics: d, Artifacts: []Artifact{}, SessionID: s.id, Snapshot: snapshot}
}
func (s *Session) addDiagnostic(v string) {
	s.mu.Lock()
	if len(s.diagnostics) < 100 {
		s.diagnostics = append(s.diagnostics, v)
	}
	s.mu.Unlock()
}
func (s *Session) Close() {
	_ = s.conn.Close(websocket.StatusNormalClosure, "done")
	s.cancel()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
	_ = os.RemoveAll(s.profile)
}
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
