package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/credentials"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/models"
	"github.com/shaktsin/ufoundry/internal/policy"
	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
	"github.com/shaktsin/ufoundry/internal/tools"
)

// historyLimit caps how many prior messages are sent with a turn.
const historyLimit = 60

// resolved is the concrete choice for a turn.
type resolved struct {
	sel    protocol.ModelSelection // what is used
	auto   bool                    // complexity picked by Auto
	preset protocol.ComplexityPreset
	meta   models.Meta
	cred   credentials.Resolved
}

// resolveSelection applies turn override > thread settings > defaults.
func (e *Engine) resolveSelection(ctx context.Context, th protocol.Thread, o protocol.ModelSelection, text string, attachments int) (resolved, error) {
	var r resolved
	o.Provider = config.NormalizeProvider(o.Provider)
	ts := th.Settings

	provider := firstNonEmpty(o.Provider, ts.Provider, e.Cfg.LLM.Provider)
	if provider == "" {
		provider = "claude"
	}
	if _, ok := e.LLMs.Get(provider); !ok {
		return r, fmt.Errorf("unknown provider %q", provider)
	}
	model := o.Model
	if model == "" && ts.Provider == provider {
		model = ts.Model
	}
	if model == "" && o.Provider == "" && ts.Provider == "" {
		model = ts.Model
	}
	if model == "" {
		model = e.defaultModel(provider)
	}
	if model == "" {
		return r, fmt.Errorf("no model selected for %s; pick one in the chat or set a default in Settings", provider)
	}
	credID := o.CredentialID
	if credID == "" && ts.Provider == provider {
		credID = ts.CredentialID
	}
	cred, err := e.Creds.Resolve(ctx, provider, credID)
	if err != nil {
		return r, err
	}

	e.mu.Lock()
	cplx := firstNonEmpty(string(o.Complexity), string(ts.Complexity), string(e.defaultCplx))
	presets := e.presets
	e.mu.Unlock()
	level := protocol.Complexity(cplx)
	if level == protocol.ComplexityAuto {
		level = models.Classify(text, attachments)
		r.auto = true
	}
	preset, ok := presets[level]
	if !ok {
		preset = presets[protocol.ComplexityStandard]
	}
	r.preset = preset
	r.meta = e.Catalog.Lookup(ctx, provider, model)
	r.cred = cred
	r.sel = protocol.ModelSelection{Provider: provider, Model: model, Complexity: level, CredentialID: cred.Record.ID}
	return r, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// StartTurn records the user's message and runs the agent loop in the background.
func (e *Engine) StartTurn(ctx context.Context, p protocol.TurnStartParams) (protocol.Turn, error) {
	return e.startTurn(ctx, turnRequest{TurnStartParams: p})
}

// turnRequest is a turn start with engine-internal options.
type turnRequest struct {
	protocol.TurnStartParams
	wait chan protocol.Turn // receives the finished turn (buffered)
}

func (e *Engine) startTurn(ctx context.Context, p turnRequest) (protocol.Turn, error) {
	if strings.TrimSpace(p.Text) == "" && len(p.Attachments) == 0 {
		return protocol.Turn{}, protocol.Errorf(protocol.CodeInvalidParams, "message is empty")
	}
	if err := validateSelection(p.Override); err != nil {
		return protocol.Turn{}, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}
	th, err := e.Store.GetThread(ctx, p.ThreadID)
	if err != nil {
		return protocol.Turn{}, err
	}
	res, err := e.resolveSelection(ctx, th, p.Override, p.Text, len(p.Attachments))
	if err != nil {
		return protocol.Turn{}, protocol.Errorf(protocol.CodeInvalidParams, "%v", err)
	}

	e.mu.Lock()
	if running, ok := e.threadTurns[th.ID]; ok {
		e.mu.Unlock()
		return protocol.Turn{}, protocol.Errorf(protocol.CodeConflict, "turn %s is still running in this thread", running)
	}
	turn := protocol.Turn{
		ID: store.NewID("trn"), ThreadID: th.ID, Status: protocol.TurnRunning,
		Selection: p.Override, Resolved: res.sel, AutoPicked: res.auto, StartedAt: time.Now().UTC(),
	}
	tctx, cancel := context.WithCancel(e.baseCtx)
	e.threadTurns[th.ID] = turn.ID
	e.activeTurns[turn.ID] = &activeTurn{cancel: cancel, turn: turn}
	if p.wait != nil {
		e.turnWaiters[turn.ID] = p.wait
	}
	e.mu.Unlock()

	var releaseOnce sync.Once
	abort := func() {
		e.mu.Lock()
		delete(e.turnWaiters, turn.ID)
		e.mu.Unlock()
	}
	release := func() {
		releaseOnce.Do(func() {
			e.mu.Lock()
			delete(e.threadTurns, th.ID)
			delete(e.activeTurns, turn.ID)
			e.mu.Unlock()
			cancel()
		})
	}
	if err := e.Store.CreateTurn(ctx, turn); err != nil {
		release()
		abort()
		return protocol.Turn{}, err
	}
	e.Bus.Publish(th.ID, protocol.NotifyTurnStarted, protocol.TurnEvent{Turn: turn})

	userItem, err := e.newItem(ctx, turn, protocol.ItemUserMessage)
	if err != nil {
		release()
		abort()
		return protocol.Turn{}, err
	}
	userItem.Text = p.Text
	userItem.Status = protocol.ItemCompleted
	if len(p.Attachments) > 0 {
		meta := make([]map[string]string, 0, len(p.Attachments))
		for _, a := range p.Attachments {
			meta = append(meta, map[string]string{"name": a.Name, "mimeType": a.MimeType})
		}
		userItem.Data, _ = json.Marshal(map[string]any{"attachments": meta})
	}
	if err := e.saveAndPublish(ctx, userItem, protocol.NotifyItemCompleted); err != nil {
		release()
		abort()
		return protocol.Turn{}, err
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer release()
		e.runTurn(tctx, th, turn, res, p.TurnStartParams, release)
	}()
	return turn, nil
}

// InterruptTurn cancels a running turn.
func (e *Engine) InterruptTurn(turnID string) error {
	e.mu.Lock()
	at, ok := e.activeTurns[turnID]
	e.mu.Unlock()
	if !ok {
		return protocol.Errorf(protocol.CodeNotFound, "turn %s is not running", turnID)
	}
	at.cancel()
	return nil
}

func (e *Engine) newItem(ctx context.Context, turn protocol.Turn, kind string) (protocol.Item, error) {
	seq, err := e.Store.NextSeq(ctx, turn.ThreadID)
	if err != nil {
		return protocol.Item{}, err
	}
	return protocol.Item{ID: store.NewID("itm"), ThreadID: turn.ThreadID, TurnID: turn.ID, Seq: seq,
		Kind: kind, Status: protocol.ItemInProgress, CreatedAt: time.Now().UTC()}, nil
}

func (e *Engine) saveAndPublish(ctx context.Context, it protocol.Item, method string) error {
	if err := e.Store.SaveItem(ctx, it); err != nil {
		return err
	}
	e.Bus.Publish(it.ThreadID, method, protocol.ItemEvent{Item: it})
	return nil
}

// runTurn is the agent loop.
func (e *Engine) runTurn(ctx context.Context, th protocol.Thread, turn protocol.Turn, res resolved, p protocol.TurnStartParams, release func()) {
	// Store writes use a context that survives interruption.
	sctx := context.WithoutCancel(ctx)
	log := e.Log.With("thread", th.ID, "turn", turn.ID)

	msgs, err := e.history(sctx, th.ID, turn.ID)
	if err != nil {
		e.finishTurn(sctx, th, turn, err, release)
		return
	}

	// The project is the sandbox for this turn: file and shell tools resolve
	// paths against its root, and every write becomes a fileChange item.
	var proj *protocol.Project
	if th.ProjectID != "" {
		p, err := e.Projects.Get(sctx, th.ProjectID)
		if err != nil {
			e.finishTurn(sctx, th, turn, fmt.Errorf("project %s could not be opened: %w", th.ProjectID, err), release)
			return
		}
		if p.Missing {
			e.finishTurn(sctx, th, turn, fmt.Errorf("the folder of project %s (%s) is gone", p.Name, p.Root), release)
			return
		}
		proj = &p
		rec := e.Projects.NewRecorder(p, th.ID, turn.ID, func(c protocol.FileChangeData) {
			e.publishFileChange(sctx, turn, c)
		})
		scope := &tools.Scope{
			ProjectID: p.ID, ProjectName: p.Name, Root: p.Root,
			AllowShell: boolOr(p.Tools.Shell, e.Cfg.Tools.ShellEnabled),
			AllowNet:   boolOr(p.Tools.Network, false),
			Record: func(c context.Context, abs string, before *string, deleted bool) {
				rec.Record(sctx, abs, before, deleted)
			},
		}
		ctx = tools.WithScope(ctx, scope)
		sctx = tools.WithScope(sctx, scope)
	}
	user := llm.Message{Role: llm.RoleUser}
	if p.Text != "" {
		user.Parts = append(user.Parts, llm.Part{Type: "text", Text: p.Text})
	}
	for _, a := range p.Attachments {
		if strings.HasPrefix(a.MimeType, "image/") {
			user.Parts = append(user.Parts, llm.Part{Type: "image", MimeType: a.MimeType, DataB64: a.DataB64})
		}
	}
	msgs = append(msgs, user)

	var specs []llm.ToolSpec
	if res.meta.Tools {
		for _, t := range e.Tools.All() {
			if !toolAllowed(t.Name(), proj) {
				continue
			}
			specs = append(specs, llm.ToolSpec{Name: tools.ToWire(t.Name()), Description: t.Description(), Schema: t.Schema()})
		}
	}
	req := llm.Request{
		Model: res.sel.Model, System: e.systemPrompt(sctx, p.Text, proj), Tools: specs, MaxTokens: res.preset.MaxOutputTokens,
	}
	if res.meta.Reasoning && res.preset.Reasoning != llm.ReasoningOff {
		req.Reasoning, req.ReasoningStyle = res.preset.Reasoning, res.meta.ReasoningStyle
	}

	cred := res.cred
	var turnErr error
	steps := 0
	for {
		if ctx.Err() != nil {
			turnErr = ctx.Err()
			break
		}
		req.Messages = msgs
		out, usedCred, err := e.callModel(ctx, sctx, turn, res, cred, req, "chat")
		cred = usedCred
		if err != nil {
			turnErr = err
			break
		}
		if len(out.calls) == 0 {
			break
		}
		steps++
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Parts: textParts(out.text), Thinking: out.thinking, ToolCalls: out.calls})
		for _, call := range out.calls {
			result, isErr := e.runTool(ctx, sctx, th, turn, call)
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, ToolName: call.Name, Result: result, IsError: isErr})
		}
		if steps >= res.preset.MaxToolSteps {
			it, err := e.newItem(sctx, turn, protocol.ItemError)
			if err == nil {
				it.Status = protocol.ItemCompleted
				it.Text = fmt.Sprintf("Stopped after %d tool steps (the %s limit). Ask me to continue, or re-run at a higher complexity.", steps, res.sel.Complexity)
				_ = e.saveAndPublish(sctx, it, protocol.NotifyItemCompleted)
			}
			break
		}
	}
	if turnErr != nil {
		log.Warn("turn failed", "err", turnErr)
	}
	e.finishTurn(sctx, th, turn, turnErr, release)
}

func textParts(s string) []llm.Part {
	if s == "" {
		return nil
	}
	return []llm.Part{{Type: "text", Text: s}}
}

type modelOutput struct {
	text     string
	thinking []llm.Thinking
	calls    []llm.ToolCall
}

// callModel streams one model call, publishing items and recording usage. On a
// rate-limit error it retries with fallback keys for the same provider.
func (e *Engine) callModel(ctx, sctx context.Context, turn protocol.Turn, res resolved, cred credentials.Resolved,
	req llm.Request, role string) (modelOutput, credentials.Resolved, error) {
	provider, _ := e.LLMs.Get(res.sel.Provider)
	tried := map[string]bool{}
	for {
		tried[cred.Record.ID] = true
		if err := e.checkBudget(sctx, cred.Record); err != nil {
			return modelOutput{}, cred, err
		}
		out, err := e.streamOnce(ctx, sctx, turn, res, cred, provider, req, role)
		if err == nil || !llm.IsRateLimit(err) {
			return out, cred, err
		}
		fbs, _ := e.Creds.Fallbacks(sctx, res.sel.Provider, cred.Record.ID)
		next := -1
		for i, fb := range fbs {
			if !tried[fb.Record.ID] {
				next = i
				break
			}
		}
		if next < 0 {
			return out, cred, err
		}
		e.Log.Info("rate limited; retrying with fallback key", "from", cred.Record.Label, "to", fbs[next].Record.Label)
		cred = fbs[next]
	}
}

func (e *Engine) streamOnce(ctx, sctx context.Context, turn protocol.Turn, res resolved, cred credentials.Resolved,
	provider llm.Provider, req llm.Request, role string) (modelOutput, error) {
	var out modelOutput
	start := time.Now()
	record := func(u llm.Usage, status string) {
		totals := protocol.UsageTotals{InputTokens: u.InputTokens, CachedInputTokens: u.CachedInputTokens,
			OutputTokens: u.OutputTokens, ReasoningTokens: u.ReasoningTokens, Requests: 1, Estimated: !u.Reported}
		if !u.Reported {
			totals.InputTokens = estimateInput(req)
			totals.OutputTokens = llm.EstimateTokens(out.text)
		}
		totals.CostUSD = models.Cost(res.meta, llm.Usage{InputTokens: totals.InputTokens,
			CachedInputTokens: totals.CachedInputTokens, OutputTokens: totals.OutputTokens})
		if err := e.Store.InsertUsage(sctx, store.UsageRecord{CredentialID: cred.Record.ID, Provider: res.sel.Provider,
			Model: res.sel.Model, ThreadID: turn.ThreadID, TurnID: turn.ID, Role: role, Usage: totals,
			LatencyMs: time.Since(start).Milliseconds(), Status: status}); err != nil {
			e.Log.Error("record usage", "err", err)
		}
	}

	ch, err := provider.Stream(ctx, cred.Material, req)
	if err != nil {
		status := "error"
		if llm.IsRateLimit(err) {
			status = "rate_limited"
		}
		record(llm.Usage{Reported: true}, status)
		return out, err
	}
	var msgItem, reasonItem *protocol.Item
	var text, reasoning strings.Builder
	var done *llm.Event
	var streamErr error
	for ev := range ch { // always drain so the adapter goroutine exits
		switch ev.Type {
		case llm.EventTextDelta:
			if msgItem == nil {
				it, err := e.newItem(sctx, turn, protocol.ItemAgentMessage)
				if err != nil {
					streamErr = err
					continue
				}
				msgItem = &it
				_ = e.saveAndPublish(sctx, it, protocol.NotifyItemStarted)
			}
			text.WriteString(ev.Text)
			e.Bus.Publish(turn.ThreadID, protocol.NotifyItemDelta, protocol.ItemDelta{ThreadID: turn.ThreadID, TurnID: turn.ID, ItemID: msgItem.ID, Text: ev.Text})
		case llm.EventReasoningDelta:
			if ev.Thinking != nil {
				out.thinking = append(out.thinking, *ev.Thinking)
				continue
			}
			if reasonItem == nil {
				it, err := e.newItem(sctx, turn, protocol.ItemReasoning)
				if err != nil {
					streamErr = err
					continue
				}
				reasonItem = &it
				_ = e.saveAndPublish(sctx, it, protocol.NotifyItemStarted)
			}
			reasoning.WriteString(ev.Text)
			e.Bus.Publish(turn.ThreadID, protocol.NotifyItemDelta, protocol.ItemDelta{ThreadID: turn.ThreadID, TurnID: turn.ID, ItemID: reasonItem.ID, Text: ev.Text})
		case llm.EventToolCall:
			tc := *ev.ToolCall
			out.calls = append(out.calls, tc)
		case llm.EventDone:
			d := ev
			done = &d
		case llm.EventError:
			streamErr = ev.Err
		}
	}
	out.text = text.String()
	finish := func(it *protocol.Item, body string, failed bool) {
		if it == nil {
			return
		}
		it.Text = body
		it.Status = protocol.ItemCompleted
		if failed {
			it.Status = protocol.ItemFailed
		}
		_ = e.saveAndPublish(sctx, *it, protocol.NotifyItemCompleted)
	}
	failed := streamErr != nil || ctx.Err() != nil
	finish(reasonItem, reasoning.String(), failed)
	finish(msgItem, out.text, failed)
	if done != nil {
		record(done.Usage, "ok")
	} else {
		record(llm.Usage{}, "error")
	}
	if streamErr == nil && ctx.Err() != nil {
		streamErr = ctx.Err()
	}
	if streamErr == nil && done == nil {
		streamErr = errors.New("model stream ended unexpectedly")
	}
	return out, streamErr
}

func estimateInput(req llm.Request) int64 {
	n := llm.EstimateTokens(req.System)
	for _, m := range req.Messages {
		n += llm.EstimateTokens(m.JoinedText()) + llm.EstimateTokens(m.Result)
		for _, c := range m.ToolCalls {
			n += llm.EstimateTokens(string(c.Args))
		}
	}
	return n
}

// checkBudget blocks calls on hard-stopped keys and warns once a month at 80%.
func (e *Engine) checkBudget(ctx context.Context, c protocol.Credential) error {
	if c.MonthlyBudgetUSD <= 0 {
		return nil
	}
	b, err := e.Creds.Budget(ctx, c)
	if err != nil {
		return err
	}
	if b.Blocked {
		return protocol.Errorf(protocol.CodeBudgetExceeded, "%v: %q has spent $%.2f of $%.2f this month",
			credentials.ErrBudgetExceeded, c.Label, b.SpentUSD, b.BudgetUSD)
	}
	if b.Percent >= 80 {
		month := time.Now().Format("2006-01")
		e.mu.Lock()
		warned := e.budgetWarned[c.ID] == month
		e.budgetWarned[c.ID] = month
		e.mu.Unlock()
		if !warned {
			e.Bus.PublishAdmin(protocol.NotifyBudgetWarning, protocol.BudgetWarning{
				CredentialID: c.ID, Label: c.Label, SpentUSD: b.SpentUSD, BudgetUSD: b.BudgetUSD, Percent: b.Percent})
		}
	}
	return nil
}

// runTool executes one tool call, asking for approval when policy requires it.
func (e *Engine) runTool(ctx, sctx context.Context, th protocol.Thread, turn protocol.Turn, call llm.ToolCall) (string, bool) {
	name := call.Name
	tool, ok := e.Tools.Get(call.Name)
	if ok {
		name = tool.Name()
	}
	it, err := e.newItem(sctx, turn, protocol.ItemToolCall)
	if err != nil {
		return "internal error: " + err.Error(), true
	}
	it.Tool = &protocol.ToolCallData{CallID: call.ID, Name: name, Args: call.Args}
	if !ok {
		it.Status, it.Tool.Error = protocol.ItemFailed, "unknown tool "+name
		_ = e.saveAndPublish(sctx, it, protocol.NotifyItemCompleted)
		return it.Tool.Error, true
	}
	risk, summary := tool.Assess(call.Args)
	it.Tool.Risk = string(risk)
	_ = e.saveAndPublish(sctx, it, protocol.NotifyItemStarted)

	decision, reason := e.gate.Check(name, risk, th.Channel == "listener")
	if decision == policy.Ask {
		approved, err := e.requestApproval(ctx, sctx, turn, it, name, call.Args, risk, reason, summary)
		if err != nil || !approved {
			it.Status = protocol.ItemDenied
			it.Tool.Error = "The user denied this action."
			if err != nil {
				it.Tool.Error = "Approval not granted: " + err.Error()
			}
			_ = e.saveAndPublish(sctx, it, protocol.NotifyItemCompleted)
			return it.Tool.Error, true
		}
	} else if decision == policy.Deny {
		it.Status, it.Tool.Error = protocol.ItemDenied, "Blocked by policy: "+reason
		_ = e.saveAndPublish(sctx, it, protocol.NotifyItemCompleted)
		return it.Tool.Error, true
	}

	output, err := tool.Call(ctx, call.Args)
	if err != nil {
		it.Status, it.Tool.Error = protocol.ItemFailed, err.Error()
		_ = e.saveAndPublish(sctx, it, protocol.NotifyItemCompleted)
		return "Error: " + err.Error(), true
	}
	it.Status, it.Tool.Output = protocol.ItemCompleted, output
	_ = e.saveAndPublish(sctx, it, protocol.NotifyItemCompleted)
	return output, false
}

// finishTurn records the outcome, publishes it and generates a title if needed.
// release frees the thread for the next turn; it runs before turn/completed is
// published so clients can start a new turn as soon as they see it.
func (e *Engine) finishTurn(ctx context.Context, th protocol.Thread, turn protocol.Turn, err error, release func()) {
	now := time.Now().UTC()
	turn.FinishedAt = &now
	switch {
	case err == nil:
		turn.Status = protocol.TurnCompleted
	case errors.Is(err, context.Canceled):
		turn.Status, turn.Error = protocol.TurnInterrupted, "interrupted"
	default:
		turn.Status, turn.Error = protocol.TurnFailed, err.Error()
		if it, ierr := e.newItem(ctx, turn, protocol.ItemError); ierr == nil {
			it.Status, it.Text = protocol.ItemCompleted, err.Error()
			_ = e.saveAndPublish(ctx, it, protocol.NotifyItemCompleted)
		}
	}
	if serr := e.Store.FinishTurn(ctx, turn); serr != nil {
		e.Log.Error("finish turn", "err", serr)
	}
	if turns, lerr := e.Store.ListTurns(ctx, th.ID); lerr == nil {
		for _, t := range turns {
			if t.ID == turn.ID {
				turn.Usage = t.Usage
			}
		}
	}
	_ = e.Store.TouchThread(ctx, th.ID)
	release()
	e.Bus.Publish(th.ID, protocol.NotifyTurnCompleted, protocol.TurnEvent{Turn: turn})
	e.mu.Lock()
	if ch, ok := e.turnWaiters[turn.ID]; ok {
		ch <- turn
		delete(e.turnWaiters, turn.ID)
	}
	e.mu.Unlock()
	e.publishThread(ctx, th.ID)
	if th.Title == "" && turn.Status == protocol.TurnCompleted {
		// Titles are generated in the background so the thread is free for the next turn.
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			e.generateTitle(ctx, th.ID, turn)
			e.publishThread(ctx, th.ID)
		}()
	}
}

// history converts earlier items into model messages (text only; tool traffic
// from earlier turns is summarised to keep context small and provider-neutral).
func (e *Engine) history(ctx context.Context, threadID, currentTurn string) ([]llm.Message, error) {
	items, err := e.Store.ListItems(ctx, threadID, 0)
	if err != nil {
		return nil, err
	}
	var msgs []llm.Message
	for _, it := range items {
		if it.TurnID == currentTurn || it.Status == protocol.ItemInProgress {
			continue
		}
		switch it.Kind {
		case protocol.ItemUserMessage, protocol.ItemInboundEvent:
			if it.Text != "" {
				msgs = appendText(msgs, llm.RoleUser, it.Text)
			}
		case protocol.ItemAgentMessage:
			if it.Text != "" && it.Status == protocol.ItemCompleted {
				msgs = appendText(msgs, llm.RoleAssistant, it.Text)
			}
		case protocol.ItemToolCall:
			if it.Tool != nil {
				note := fmt.Sprintf("[earlier tool call %s: %s]", it.Tool.Name, it.Status)
				msgs = appendText(msgs, llm.RoleAssistant, note)
			}
		case protocol.ItemFileChange:
			if it.Text != "" {
				msgs = appendText(msgs, llm.RoleAssistant, "["+it.Text+"]")
			}
		}
	}
	// Providers need the history to start with a user message.
	for len(msgs) > 0 && msgs[0].Role != llm.RoleUser {
		msgs = msgs[1:]
	}
	if len(msgs) > historyLimit {
		msgs = msgs[len(msgs)-historyLimit:]
		for len(msgs) > 0 && msgs[0].Role != llm.RoleUser {
			msgs = msgs[1:]
		}
	}
	// The new user message is appended by the caller; drop a trailing user
	// message so roles alternate.
	if n := len(msgs); n > 0 && msgs[n-1].Role == llm.RoleUser {
		msgs = appendText(msgs, llm.RoleAssistant, "(no reply)")
	}
	return msgs, nil
}

// appendText merges consecutive same-role text messages.
func appendText(msgs []llm.Message, role llm.Role, text string) []llm.Message {
	if n := len(msgs); n > 0 && msgs[n-1].Role == role {
		msgs[n-1].Parts[0].Text += "\n\n" + text
		return msgs
	}
	return append(msgs, llm.Text(role, text))
}

// boolOr returns *p when set, else def.
func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// publishFileChange records one edit as a fileChange item in the turn.
func (e *Engine) publishFileChange(ctx context.Context, turn protocol.Turn, c protocol.FileChangeData) {
	it, err := e.newItem(ctx, turn, protocol.ItemFileChange)
	if err != nil {
		return
	}
	it.Status = protocol.ItemCompleted
	verb := map[string]string{
		protocol.FileCreated: "Created", protocol.FileModified: "Edited", protocol.FileDeleted: "Deleted",
	}[c.Action]
	it.Text = fmt.Sprintf("%s %s (+%d −%d)", verb, c.Path, c.Additions, c.Deletions)
	it.Data, _ = json.Marshal(c)
	_ = e.saveAndPublish(ctx, it, protocol.NotifyItemCompleted)
}

// toolAllowed applies a project's tool settings: shell can be switched off,
// and the MCP servers a project may use can be limited to a list.
func toolAllowed(name string, proj *protocol.Project) bool {
	if proj == nil {
		return true
	}
	if strings.HasPrefix(name, "shell.") && !boolOr(proj.Tools.Shell, true) {
		return false
	}
	if proj.Tools.MCPServers == nil || !strings.HasPrefix(name, "mcp_") {
		return true
	}
	server := strings.SplitN(strings.TrimPrefix(name, "mcp_"), "_", 2)[0]
	for _, allowed := range proj.Tools.MCPServers {
		if allowed == server {
			return true
		}
	}
	return false
}
