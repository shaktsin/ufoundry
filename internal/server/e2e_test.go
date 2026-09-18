package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/shaktsin/ufoundry/internal/client"
	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/engine"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/secrets"
	"github.com/shaktsin/ufoundry/internal/server"
	"github.com/shaktsin/ufoundry/internal/store"
)

// fakeProvider replays scripted responses. Each call pops the next script.
type fakeProvider struct {
	id      string
	mu      sync.Mutex
	scripts []func(req llm.Request, key string) ([]llm.Event, error)
	calls   []llm.Request
	keys    []string
}

func (f *fakeProvider) ID() string { return f.id }

func (f *fakeProvider) Stream(ctx context.Context, cred llm.Credential, req llm.Request) (<-chan llm.Event, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.keys = append(f.keys, cred.APIKey)
	var script func(llm.Request, string) ([]llm.Event, error)
	if len(f.scripts) > 0 {
		script, f.scripts = f.scripts[0], f.scripts[1:]
	}
	f.mu.Unlock()
	if script == nil {
		script = textReply("title words")
	}
	evs, err := script(req, cred.APIKey)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.Event, len(evs))
	for _, e := range evs {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (f *fakeProvider) ListModels(ctx context.Context, cred llm.Credential) ([]string, error) {
	return []string{"fake-large", "fake-small"}, nil
}

func (f *fakeProvider) push(s ...func(llm.Request, string) ([]llm.Event, error)) {
	f.mu.Lock()
	f.scripts = append(f.scripts, s...)
	f.mu.Unlock()
}

func textReply(text string) func(llm.Request, string) ([]llm.Event, error) {
	return func(llm.Request, string) ([]llm.Event, error) {
		return []llm.Event{
			{Type: llm.EventTextDelta, Text: text[:len(text)/2]},
			{Type: llm.EventTextDelta, Text: text[len(text)/2:]},
			{Type: llm.EventDone, Usage: llm.Usage{InputTokens: 1000, CachedInputTokens: 200, OutputTokens: 100, Reported: true}},
		}, nil
	}
}

func toolReply(name, args string) func(llm.Request, string) ([]llm.Event, error) {
	return func(llm.Request, string) ([]llm.Event, error) {
		return []llm.Event{
			{Type: llm.EventToolCall, ToolCall: &llm.ToolCall{ID: "call_1", Name: name, Args: json.RawMessage(args)}},
			{Type: llm.EventDone, Usage: llm.Usage{InputTokens: 500, OutputTokens: 20, Reported: true}},
		}, nil
	}
}

type harness struct {
	t    *testing.T
	fake *fakeProvider
	eng  *engine.Engine
	c    *client.Client
	ctx  context.Context
	ws   string
}

func newHarness(t *testing.T, mutate func(*config.Config)) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("UFOUNDRY_HOME", dir)
	cfg := config.Default(dir)
	cfg.Tools.ShellEnabled = true
	cfg.Runtime.SocketPath = filepath.Join(dir, "e.sock")
	cfg.Tools.Workspaces = []config.WorkspaceConfig{{Name: "w", Path: filepath.Join(dir, "ws"), Default: true}}
	if mutate != nil {
		mutate(cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st, err := store.Open(ctx, filepath.Join(dir, "ufoundry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := &fakeProvider{id: "claude"}
	reg := llm.NewRegistry()
	reg.Register(fake)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng, err := engine.New(ctx, engine.Options{Config: cfg, Store: st, Secrets: secrets.NewMemoryStore(), LLMs: reg, Logger: log,
		SchedulerPoll: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(eng, log)
	if err := srv.ListenUnix(cfg.Runtime.SocketPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srv.Close()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		eng.Shutdown(sctx)
	})
	c, err := client.Dial(ctx, cfg.Runtime.SocketPath, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &harness{t: t, fake: fake, eng: eng, c: c, ctx: ctx, ws: filepath.Join(dir, "ws")}
}

func (h *harness) call(method string, params, out any) {
	h.t.Helper()
	if err := h.c.Call(h.ctx, method, params, out); err != nil {
		h.t.Fatalf("%s: %v", method, err)
	}
}

// waitTurn consumes notifications until the turn completes. onApproval decides approvals.
func (h *harness) waitTurn(turnID string, onApproval func(protocol.Approval) bool) (protocol.Turn, string, []protocol.Item) {
	h.t.Helper()
	var text strings.Builder
	var tools []protocol.Item
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-timeout:
			h.t.Fatal("timed out waiting for turn")
		case n := <-h.c.Notifications():
			switch n.Method {
			case protocol.NotifyItemDelta:
				var d protocol.ItemDelta
				json.Unmarshal(n.Params, &d)
				if d.TurnID == turnID {
					text.WriteString(d.Text)
				}
			case protocol.NotifyItemCompleted:
				var ev protocol.ItemEvent
				json.Unmarshal(n.Params, &ev)
				if ev.Item.TurnID == turnID && ev.Item.Kind == protocol.ItemToolCall {
					tools = append(tools, ev.Item)
				}
			case protocol.NotifyApprovalRequest:
				var ev protocol.ApprovalEvent
				json.Unmarshal(n.Params, &ev)
				ok := onApproval != nil && onApproval(ev.Approval)
				go h.c.Call(h.ctx, protocol.MethodApprovalRespond, protocol.ApprovalRespondParams{ApprovalID: ev.Approval.ID, Approve: ok}, nil)
			case protocol.NotifyTurnCompleted:
				var ev protocol.TurnEvent
				json.Unmarshal(n.Params, &ev)
				if ev.Turn.ID == turnID {
					return ev.Turn, text.String(), tools
				}
			}
		}
	}
}

func (h *harness) addKey(provider, label, secret string) protocol.Credential {
	var c protocol.Credential
	h.call(protocol.MethodCredentialAdd, protocol.CredentialAddParams{Provider: provider, Label: label, Secret: secret}, &c)
	return c
}

func TestChatWithApprovedShellTool(t *testing.T) {
	h := newHarness(t, nil)
	key := h.addKey("claude", "personal", "sk-test-abcd1234")
	if key.Last4 != "1234" || !key.IsDefault {
		t.Fatalf("key = %+v", key)
	}
	h.fake.push(toolReply("shell__run", `{"command":"echo hello-from-shell"}`), textReply("The command printed hello."))

	var th protocol.Thread
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{}, &th)
	var res protocol.TurnStartResult
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "run echo please",
		Override: protocol.ModelSelection{Model: "claude-sonnet-4-6", Complexity: protocol.ComplexityDeep}}, &res)
	if res.Turn.Resolved.Provider != "claude" || res.Turn.Resolved.CredentialID != key.ID || res.Turn.Resolved.Complexity != "deep" {
		t.Fatalf("resolved = %+v", res.Turn.Resolved)
	}
	var approvals []protocol.Approval
	turn, text, tools := h.waitTurn(res.Turn.ID, func(a protocol.Approval) bool {
		approvals = append(approvals, a)
		return true
	})
	if turn.Status != protocol.TurnCompleted {
		t.Fatalf("turn = %+v", turn)
	}
	if text != "The command printed hello." {
		t.Fatalf("text = %q", text)
	}
	if len(approvals) != 1 || approvals[0].Risk != "red" || !strings.Contains(approvals[0].ActionSummary, "echo hello-from-shell") {
		t.Fatalf("approvals = %+v", approvals)
	}
	if len(tools) != 1 || !strings.Contains(tools[0].Tool.Output, "hello-from-shell") {
		t.Fatalf("tools = %+v", tools)
	}
	// The second model call saw the tool result; deep → high reasoning with adaptive style.
	h.fake.mu.Lock()
	second := h.fake.calls[1]
	h.fake.mu.Unlock()
	last := second.Messages[len(second.Messages)-1]
	if last.Role != llm.RoleTool || !strings.Contains(last.Result, "hello-from-shell") {
		t.Fatalf("tool result not sent back: %+v", last)
	}
	if second.Reasoning != llm.ReasoningHigh || second.ReasoningStyle != "adaptive" {
		t.Fatalf("reasoning = %q/%q", second.Reasoning, second.ReasoningStyle)
	}
	// Usage: 2 chat calls. Sonnet 4.6: (800*3 + 200*0.3 + 100*15)/1e6 + (500*3 + 20*15)/1e6
	want := (800*3.0+200*0.3+100*15.0)/1e6 + (500*3.0+20*15.0)/1e6
	if turn.Usage.Requests != 2 || turn.Usage.InputTokens != 1500 || abs(turn.Usage.CostUSD-want) > 1e-12 {
		t.Fatalf("turn usage = %+v (want cost %v)", turn.Usage, want)
	}
	// Per-key summary includes the background title request.
	time.Sleep(200 * time.Millisecond)
	var sum protocol.UsageSummaryResult
	h.call(protocol.MethodUsageSummary, protocol.UsageSummaryParams{GroupBy: "credential"}, &sum)
	if len(sum.Rows) != 1 || sum.Rows[0].Key != key.ID || sum.Rows[0].Usage.Requests < 2 || !strings.Contains(sum.Rows[0].Label, "personal") {
		t.Fatalf("summary = %+v", sum)
	}
	var byRole protocol.UsageSummaryResult
	h.call(protocol.MethodUsageSummary, protocol.UsageSummaryParams{GroupBy: "role"}, &byRole)
	roles := map[string]int64{}
	for _, r := range byRole.Rows {
		roles[r.Key] = r.Usage.Requests
	}
	if roles["chat"] != 2 || roles["title"] != 1 {
		t.Fatalf("roles = %v", roles)
	}
	// Title and history.
	var read protocol.ThreadReadResult
	h.call(protocol.MethodThreadRead, protocol.ThreadIDParams{ThreadID: th.ID}, &read)
	if read.Thread.Title != "title words" {
		t.Fatalf("title = %q", read.Thread.Title)
	}
	var hits protocol.ThreadSearchResult
	h.call(protocol.MethodThreadSearch, protocol.ThreadSearchParams{Query: "hello-from"}, &hits)
	if len(hits.Hits) == 0 || hits.Hits[0].ThreadID != th.ID {
		t.Fatalf("search = %+v", hits)
	}
	var exp protocol.ThreadExportResult
	h.call(protocol.MethodThreadExport, protocol.ThreadIDParams{ThreadID: th.ID}, &exp)
	if !strings.Contains(exp.Markdown, "## You\n\nrun echo please") {
		t.Fatalf("export = %s", exp.Markdown)
	}
}

func TestDeniedToolAndHistory(t *testing.T) {
	h := newHarness(t, nil)
	h.addKey("claude", "k", "sk-1")
	h.fake.push(toolReply("shell__run", `{"command":"rm -rf /tmp/x"}`), textReply("Okay, I will not."))
	var th protocol.Thread
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{Title: "cleanup"}, &th)
	var res protocol.TurnStartResult
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "delete it"}, &res)
	turn, _, tools := h.waitTurn(res.Turn.ID, func(protocol.Approval) bool { return false })
	if turn.Status != protocol.TurnCompleted || len(tools) != 1 || tools[0].Status != protocol.ItemDenied {
		t.Fatalf("turn=%+v tools=%+v", turn, tools)
	}
	// Second turn sends earlier text history and uses quick for a short message (auto).
	h.fake.push(textReply("hi!"))
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "hi"}, &res)
	if !res.Turn.AutoPicked || res.Turn.Resolved.Complexity != protocol.ComplexityQuick {
		t.Fatalf("auto: %+v", res.Turn)
	}
	h.waitTurn(res.Turn.ID, nil)
	h.fake.mu.Lock()
	req := h.fake.calls[len(h.fake.calls)-1]
	h.fake.mu.Unlock()
	if req.Messages[0].JoinedText() != "delete it" || req.Reasoning != "" {
		t.Fatalf("history/reasoning: %+v", req)
	}
}

func TestBudgetHardStopAndFallback(t *testing.T) {
	h := newHarness(t, nil)
	k1 := h.addKey("claude", "primary", "sk-primary")
	k2 := h.addKey("claude", "backup", "sk-backup")
	fb := true
	h.call(protocol.MethodCredentialUpdate, protocol.CredentialUpdateParams{CredentialID: k2.ID, Fallback: &fb}, nil)

	// Primary is rate limited → backup answers.
	h.fake.push(func(llm.Request, string) ([]llm.Event, error) {
		return nil, &llm.Error{Provider: "claude", Status: 429, Body: "rate limited"}
	}, textReply("from backup"))
	var th protocol.Thread
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{Title: "x"}, &th)
	var res protocol.TurnStartResult
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "hello there friend"}, &res)
	turn, text, _ := h.waitTurn(res.Turn.ID, nil)
	if turn.Status != protocol.TurnCompleted || text != "from backup" {
		t.Fatalf("fallback failed: %+v %q", turn, text)
	}
	h.fake.mu.Lock()
	keys := append([]string(nil), h.fake.keys...)
	h.fake.mu.Unlock()
	if keys[0] != "sk-primary" || keys[1] != "sk-backup" {
		t.Fatalf("keys used = %v", keys)
	}
	var sum protocol.UsageSummaryResult
	h.call(protocol.MethodUsageSummary, protocol.UsageSummaryParams{GroupBy: "credential"}, &sum)
	byKey := map[string]protocol.UsageTotals{}
	for _, r := range sum.Rows {
		byKey[r.Key] = r.Usage
	}
	if byKey[k1.ID].Requests != 1 || byKey[k1.ID].InputTokens != 0 || byKey[k2.ID].InputTokens != 1000 {
		t.Fatalf("per-key usage = %+v", byKey)
	}

	// Hard stop: a tiny budget already spent blocks the next turn.
	h.call(protocol.MethodUsageSetBudget, protocol.UsageSetBudgetParams{CredentialID: k2.ID, MonthlyBudgetUSD: 0.000001, HardStop: true}, nil)
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "again please now",
		Override: protocol.ModelSelection{CredentialID: k2.ID}}, &res)
	turn, _, _ = h.waitTurn(res.Turn.ID, nil)
	if turn.Status != protocol.TurnFailed || !strings.Contains(turn.Error, "budget") {
		t.Fatalf("budget not enforced: %+v", turn)
	}
}

func TestThreadManagementAndModels(t *testing.T) {
	h := newHarness(t, nil)
	k := h.addKey("claude", "k", "sk-xyz9")
	var a, b protocol.Thread
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{Title: "alpha"}, &a)
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{Title: "beta"}, &b)
	h.call(protocol.MethodThreadPin, protocol.ThreadFlagParams{ThreadID: a.ID, Value: true}, nil)
	h.call(protocol.MethodThreadArchive, protocol.ThreadFlagParams{ThreadID: b.ID, Value: true}, nil)
	var list protocol.ThreadListResult
	h.call(protocol.MethodThreadList, protocol.ThreadListParams{}, &list)
	if len(list.Threads) != 1 || list.Threads[0].ID != a.ID || !list.Threads[0].Pinned {
		t.Fatalf("list = %+v", list)
	}
	var set protocol.Thread
	h.call(protocol.MethodThreadSetSettings, protocol.ThreadSetSettingsParams{ThreadID: a.ID,
		Settings: protocol.ModelSelection{Model: "fake-large", Complexity: "standard", CredentialID: k.ID}}, &set)
	if set.Settings.Provider != "claude" || set.Settings.Model != "fake-large" {
		t.Fatalf("settings = %+v", set.Settings)
	}
	if err := h.c.Call(h.ctx, protocol.MethodThreadSetSettings, protocol.ThreadSetSettingsParams{ThreadID: a.ID,
		Settings: protocol.ModelSelection{Provider: "openai", CredentialID: k.ID}}, nil); err == nil {
		t.Fatal("expected provider/key mismatch error")
	}
	var test protocol.CredentialTestResult
	h.call(protocol.MethodCredentialTest, protocol.CredentialIDParams{CredentialID: k.ID}, &test)
	if !test.OK || len(test.Models) != 2 {
		t.Fatalf("test = %+v", test)
	}
	var models protocol.ModelListResult
	h.call(protocol.MethodModelList, protocol.ModelListParams{Provider: "claude"}, &models)
	found := false
	for _, m := range models.Models {
		found = found || (m.ID == "fake-large" && m.Source == "api")
	}
	if !found {
		t.Fatalf("api models not merged: %+v", models)
	}
	var d protocol.ComplexityDefaults
	h.call(protocol.MethodComplexityGetDefaults, nil, &d)
	d.Default = protocol.ComplexityStandard
	d.Presets[0].MaxToolSteps = 3
	h.call(protocol.MethodComplexitySetDefaults, d, &d)
	if d.Default != "standard" || d.Presets[0].MaxToolSteps != 3 {
		t.Fatalf("complexity = %+v", d)
	}
	var creds protocol.CredentialListResult
	h.call(protocol.MethodCredentialList, nil, &creds)
	raw, _ := json.Marshal(creds)
	if strings.Contains(string(raw), "sk-xyz9") {
		t.Fatal("secret leaked in credential/list")
	}
	h.call(protocol.MethodThreadDelete, protocol.ThreadIDParams{ThreadID: b.ID}, nil)
	if err := h.c.Call(h.ctx, protocol.MethodThreadRead, protocol.ThreadIDParams{ThreadID: b.ID}, nil); err == nil {
		t.Fatal("deleted thread still readable")
	}
}

func TestWorkspaceContainment(t *testing.T) {
	h := newHarness(t, nil)
	h.addKey("claude", "k", "sk-1")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("top secret"), 0o600)
	h.fake.push(toolReply("file__read", `{"path":"`+outside+`"}`), textReply("could not read it"))
	var th protocol.Thread
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{Title: "x"}, &th)
	var res protocol.TurnStartResult
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "read that file for me please"}, &res)
	_, _, tools := h.waitTurn(res.Turn.ID, nil)
	if len(tools) != 1 || tools[0].Status != protocol.ItemFailed || !strings.Contains(tools[0].Tool.Error, "outside workspace") {
		t.Fatalf("tools = %+v", tools)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestWebSocketTransport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UFOUNDRY_HOME", dir)
	cfg := config.Default(dir)
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng, err := engine.New(ctx, engine.Options{Config: cfg, Store: st, Secrets: secrets.NewMemoryStore(), Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(eng, log)
	tokenPath := filepath.Join(dir, "token")
	if err := srv.ListenWebSocket("127.0.0.1", 18767, tokenPath); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	tok, _ := os.ReadFile(tokenPath)

	// Wrong token is rejected.
	if _, _, err := websocket.Dial(ctx, "ws://127.0.0.1:18767/ws?token=nope", nil); err == nil {
		t.Fatal("expected unauthorized")
	}
	ws, _, err := websocket.Dial(ctx, "ws://127.0.0.1:18767/ws", &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Authorization": {"Bearer " + strings.TrimSpace(string(tok))}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	send := func(id int, method string, params any) map[string]any {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if err := ws.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatal(err)
		}
		_, data, err := ws.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		return m
	}
	if m := send(1, "engine/status", nil); m["error"] == nil {
		t.Fatal("expected initialize-first error")
	}
	if m := send(2, "initialize", protocol.InitializeParams{ClientName: "ws", ProtocolVersion: "2.0.0"}); m["error"] == nil {
		t.Fatal("expected version mismatch")
	}
	send(3, "initialize", protocol.InitializeParams{ClientName: "ws", ProtocolVersion: protocol.Version})
	m := send(4, "engine/status", nil)
	if res, _ := m["result"].(map[string]any); res == nil || res["protocolVersion"] != protocol.Version {
		t.Fatalf("status = %v", m)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestScheduledTasks(t *testing.T) {
	h := newHarness(t, nil)
	h.addKey("claude", "k", "sk-1")
	h.fake.push(textReply("Inbox: 3 new emails."))
	var task protocol.Task
	min := 7
	h.call(protocol.MethodTaskCreate, protocol.TaskCreateParams{Name: "inbox", Prompt: "summarise my inbox",
		TaskType: protocol.TaskPeriodic, Schedule: protocol.Schedule{Frequency: "hourly", Minute: &min}, Timezone: "UTC",
		Settings: protocol.ModelSelection{Complexity: protocol.ComplexityQuick}}, &task)
	if task.NextRunAt == nil || task.NextRunAt.Minute() != 7 || task.Status != "active" {
		t.Fatalf("task = %+v", task)
	}
	h.call(protocol.MethodTaskRunNow, protocol.TaskIDParams{TaskID: task.ID}, nil)
	var runs protocol.TaskRunsResult
	waitFor(t, "task run", func() bool {
		h.call(protocol.MethodTaskRuns, protocol.TaskIDParams{TaskID: task.ID}, &runs)
		return len(runs.Runs) == 1 && runs.Runs[0].Status != "running"
	})
	run := runs.Runs[0]
	if run.Status != "success" || run.Result != "Inbox: 3 new emails." || run.TurnID == "" {
		t.Fatalf("run = %+v", run)
	}
	var list protocol.TaskListResult
	h.call(protocol.MethodTaskList, protocol.TaskListParams{}, &list)
	got := list.Tasks[0]
	if got.Status != "active" || got.NextRunAt == nil || !got.NextRunAt.After(time.Now()) || got.ThreadID == "" || got.LastResult != run.Result {
		t.Fatalf("after run = %+v", got)
	}
	var read protocol.ThreadReadResult
	h.call(protocol.MethodThreadRead, protocol.ThreadIDParams{ThreadID: got.ThreadID}, &read)
	if read.Thread.Title != "Task: inbox" || read.Thread.Channel != "task" || read.Turns[0].Resolved.Complexity != "quick" {
		t.Fatalf("task thread = %+v turns=%+v", read.Thread, read.Turns)
	}

	// One-time task completes after its run.
	h.fake.push(textReply("done"))
	var once protocol.Task
	h.call(protocol.MethodTaskCreate, protocol.TaskCreateParams{Prompt: "remind me", TaskType: protocol.TaskOneTime,
		Schedule: protocol.Schedule{RunAt: time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)}}, &once)
	h.call(protocol.MethodTaskRunNow, protocol.TaskIDParams{TaskID: once.ID}, nil)
	waitFor(t, "one-time completion", func() bool {
		h.call(protocol.MethodTaskList, protocol.TaskListParams{Status: "completed"}, &list)
		return len(list.Tasks) == 1 && list.Tasks[0].ID == once.ID
	})
	if err := h.c.Call(h.ctx, protocol.MethodTaskCreate, protocol.TaskCreateParams{Prompt: "x", TaskType: protocol.TaskOneTime,
		Schedule: protocol.Schedule{RunAt: "2020-01-01T00:00"}}, nil); err == nil {
		t.Fatal("past one-time task accepted")
	}
	h.call(protocol.MethodTaskCancel, protocol.TaskIDParams{TaskID: task.ID}, &task)
	if task.Status != "cancelled" {
		t.Fatalf("cancel = %+v", task)
	}
}

func TestAgentUsesSkillAndCreatesTask(t *testing.T) {
	h := newHarness(t, nil)
	h.addKey("claude", "k", "sk-1")
	home := filepath.Dir(h.eng.Cfg.Runtime.SocketPath)
	dir := filepath.Join(home, "skills", "greeter")
	os.MkdirAll(filepath.Join(dir, "scripts"), 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: greeter\ndescription: Greet people by name\nrisk_level: green\nscripts:\n  greet:\n    path: scripts/greet.sh\n    description: greet\n---\nCall greet.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "scripts", "greet.sh"), []byte("#!/bin/bash\nread -r j\necho \"hello from skill $j\"\n"), 0o755)

	h.fake.push(
		toolReply("skill__run_script", `{"skill":"greeter","script":"greet","args":{"name":"Ada"}}`),
		toolReply("task__create", `{"name":"standup","prompt":"post standup notes","task_type":"periodic","frequency":"weekly","day_of_week":"mon","time":"09:30","timezone":"Europe/Berlin"}`),
		textReply("Greeted Ada and scheduled standup."),
	)
	var th protocol.Thread
	h.call(protocol.MethodThreadStart, protocol.ThreadStartParams{Title: "x"}, &th)
	var res protocol.TurnStartResult
	h.call(protocol.MethodTurnStart, protocol.TurnStartParams{ThreadID: th.ID, Text: "please greet Ada by name and schedule standup"}, &res)
	turn, _, tools := h.waitTurn(res.Turn.ID, nil)
	if turn.Status != protocol.TurnCompleted || len(tools) != 2 {
		t.Fatalf("turn=%+v tools=%+v", turn, tools)
	}
	if tools[0].Tool.Name != "skill.run_script" || !strings.Contains(tools[0].Tool.Output, `hello from skill {"config":{},"input":{"name":"Ada"}}`) {
		t.Fatalf("skill tool = %+v", tools[0].Tool)
	}
	if tools[1].Tool.Name != "task.create" || tools[1].Status != protocol.ItemCompleted {
		t.Fatalf("task tool = %+v", tools[1])
	}
	h.fake.mu.Lock()
	sys := h.fake.calls[0].System
	h.fake.mu.Unlock()
	if !strings.Contains(sys, "greeter: Greet people by name") || !strings.Contains(sys, `may match the "greeter" skill`) {
		t.Fatalf("system prompt lacks skills: %s", sys)
	}
	var list protocol.TaskListResult
	h.call(protocol.MethodTaskList, protocol.TaskListParams{}, &list)
	if len(list.Tasks) != 1 || list.Tasks[0].Timezone != "Europe/Berlin" || list.Tasks[0].CreatedBy != "agent" {
		t.Fatalf("tasks = %+v", list.Tasks)
	}
	var tl protocol.ToolListResult
	h.call(protocol.MethodToolList, nil, &tl)
	names := map[string]string{}
	for _, x := range tl.Tools {
		names[x.Name] = x.Source
	}
	if names["skill.run_script"] != "skill" || names["task.create"] != "task" || names["shell.run"] != "builtin" {
		t.Fatalf("tools = %v", names)
	}
	var sl protocol.SkillListResult
	h.call(protocol.MethodSkillList, nil, &sl)
	if len(sl.Skills) != 1 || !sl.Skills[0].Removable || sl.Skills[0].Scripts[0] != "greet" {
		t.Fatalf("skills = %+v", sl)
	}
	var ml protocol.MCPListResult
	h.call(protocol.MethodMCPList, nil, &ml)
	if len(ml.Servers) != 0 {
		t.Fatalf("mcp = %+v", ml)
	}
}
