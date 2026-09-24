// Package engine owns threads, turns, items and approvals, and runs the agent
// loop. Transports (Unix socket, WebSocket) call into it; it publishes events
// through the Bus.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/credentials"
	"github.com/shaktsin/ufoundry/internal/identity"
	"github.com/shaktsin/ufoundry/internal/llm"
	"github.com/shaktsin/ufoundry/internal/mcp"
	"github.com/shaktsin/ufoundry/internal/models"
	"github.com/shaktsin/ufoundry/internal/policy"
	"github.com/shaktsin/ufoundry/internal/projects"
	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/router"
	"github.com/shaktsin/ufoundry/internal/secrets"
	"github.com/shaktsin/ufoundry/internal/skills"
	"github.com/shaktsin/ufoundry/internal/store"
	"github.com/shaktsin/ufoundry/internal/tasks"
	"github.com/shaktsin/ufoundry/internal/tools"
	"github.com/shaktsin/ufoundry/internal/version"
)

// Engine is the UFoundry core.
type Engine struct {
	Cfg      *config.Config
	Store    *store.Store
	LLMs     *llm.Registry
	Catalog  *models.Catalog
	Creds    *credentials.Service
	Tools    *tools.Registry
	Skills   *skills.Registry
	MCP      *mcp.Manager
	Projects *projects.Service
	Router   *router.Router
	Tasks    *tasks.Service
	Bus      *Bus
	Log      *slog.Logger

	gate      *policy.Gate
	started   time.Time
	baseCtx   context.Context
	cancelAll context.CancelFunc

	mu           sync.Mutex
	activeTurns  map[string]*activeTurn // by turn id
	threadTurns  map[string]string      // thread id -> running turn id
	approvals    map[string]chan bool   // pending approval id -> decision
	budgetWarned map[string]string      // credential id -> month already warned
	routing      protocol.RoutingConfig // the models the user has approved
	presets      map[protocol.Complexity]protocol.ComplexityPreset
	defaultCplx  protocol.Complexity
	turnWaiters  map[string]chan protocol.Turn // turn id -> completion (task runs)
	wg           sync.WaitGroup
}

type activeTurn struct {
	cancel context.CancelFunc
	turn   protocol.Turn
}

// Options configures New.
type Options struct {
	Config  *config.Config
	Store   *store.Store
	Secrets secrets.Store
	LLMs    *llm.Registry // nil = built-in adapters
	Logger  *slog.Logger
	// Legacy finds API keys saved by the Python app (Keychain, .env); optional.
	Legacy credentials.LegacyLookup
	// DisableScheduler turns off scheduled task runs (tests, secondary engines).
	DisableScheduler bool
	// DisableMCP skips starting MCP servers.
	DisableMCP bool
	// SchedulerPoll overrides how often due tasks are checked (default 5s).
	SchedulerPoll time.Duration
}

// New builds an engine and runs startup housekeeping.
func New(ctx context.Context, o Options) (*Engine, error) {
	if o.LLMs == nil {
		o.LLMs = llm.NewRegistry()
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	cat, err := models.NewCatalog(o.Store)
	if err != nil {
		return nil, err
	}
	ws := tools.NewWorkspaces(o.Config)
	reg := tools.NewRegistry()
	sk := skills.NewRegistry(o.Config)
	tools.RegisterBuiltins(reg, o.Config, ws, sk.EnvFor)
	sk.Register(reg)

	mcpm := mcp.NewManager(o.Config.MCPServers, o.Logger)
	reg.AddSource(mcpm)

	base, cancel := context.WithCancel(context.Background())
	e := &Engine{
		Cfg: o.Config, Store: o.Store, LLMs: o.LLMs, Catalog: cat,
		Creds: credentials.New(o.Store, o.Secrets, o.LLMs), Tools: reg, Skills: sk, MCP: mcpm,
		Projects: projects.New(o.Store, o.Config),
		Bus:      NewBus(), Log: o.Logger,
		gate: policy.New(o.Config.Policy), started: time.Now(), baseCtx: base, cancelAll: cancel,
		activeTurns: map[string]*activeTurn{}, threadTurns: map[string]string{},
		approvals: map[string]chan bool{}, budgetWarned: map[string]string{},
		turnWaiters: map[string]chan protocol.Turn{},
	}
	e.Router = router.New(o.Store, e.Creds, cat, o.LLMs, o.Config, o.Logger)
	e.Tasks = tasks.NewService(o.Store, o.Logger, e.runTask, func(t protocol.Task) {
		e.Bus.PublishAdmin(protocol.NotifyTaskUpdated, protocol.TaskEvent{Task: t})
	})
	e.Tasks.Register(reg)
	if err := e.loadComplexity(ctx); err != nil {
		return nil, err
	}
	if err := e.loadRouting(ctx); err != nil {
		return nil, err
	}
	if n, err := o.Store.MarkStaleTurns(ctx); err != nil {
		return nil, err
	} else if n > 0 {
		e.Log.Warn("marked turns interrupted by previous shutdown", "count", n)
	}
	if err := o.Store.ExpirePendingApprovals(ctx); err != nil {
		return nil, err
	}
	if added, err := e.Creds.ImportConfigKeys(ctx, o.Config, o.Legacy); err != nil {
		e.Log.Warn("could not import API keys from config", "err", err)
	} else {
		for _, c := range added {
			e.Log.Info("imported an existing API key into secure storage (you can remove any api_key from config.yaml)",
				"provider", c.Provider, "credential", c.ID)
		}
	}
	if !o.DisableMCP {
		if err := mcpm.Start(false); err != nil {
			return nil, err
		}
	}
	if o.SchedulerPoll > 0 {
		e.Tasks.Poll = o.SchedulerPoll
	}
	if !o.DisableScheduler {
		e.Tasks.Start(base)
	}
	return e, nil
}

// Shutdown cancels running turns and waits for them to finish.
func (e *Engine) Shutdown(ctx context.Context) {
	e.cancelAll()
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		e.Tasks.Wait()
		e.MCP.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Status reports engine state.
func (e *Engine) Status(ctx context.Context) protocol.EngineStatus {
	e.mu.Lock()
	active := len(e.activeTurns)
	pending := len(e.approvals)
	e.mu.Unlock()
	return protocol.EngineStatus{
		EngineVersion: version.Version, ProtocolVersion: protocol.Version, StartedAt: e.started,
		Clients: e.Bus.Count(), ActiveTurns: active, PendingApprovals: pending, DBPath: e.Store.Path,
	}
}

// ---- threads ----

func (e *Engine) publishThread(ctx context.Context, id string) {
	if t, err := e.Store.GetThread(ctx, id); err == nil {
		e.Bus.Publish(id, protocol.NotifyThreadUpdated, protocol.ThreadEvent{Thread: t})
	}
}

// StartThread creates a thread.
func (e *Engine) StartThread(ctx context.Context, p protocol.ThreadStartParams) (protocol.Thread, error) {
	if err := validateSelection(p.Settings); err != nil {
		return protocol.Thread{}, err
	}
	p.Settings.Provider = config.NormalizeProvider(p.Settings.Provider)
	if p.ProjectID != "" {
		proj, err := e.Projects.Get(ctx, p.ProjectID)
		if err != nil {
			return protocol.Thread{}, protocol.Errorf(protocol.CodeInvalidParams, "project %s: %v", p.ProjectID, err)
		}
		if proj.Missing {
			return protocol.Thread{}, protocol.Errorf(protocol.CodeInvalidParams, "the folder of project %s (%s) is gone", proj.Name, proj.Root)
		}
		_ = e.Store.TouchProject(ctx, proj.ID)
	}
	if p.ParentThreadID != "" {
		parent, err := e.Store.GetThread(ctx, p.ParentThreadID)
		if err != nil {
			return protocol.Thread{}, protocol.Errorf(protocol.CodeInvalidParams, "parent chat %s: %v", p.ParentThreadID, err)
		}
		if p.ProjectID == "" {
			p.ProjectID = parent.ProjectID
		}
	}
	return e.Store.CreateThread(ctx, protocol.Thread{Title: strings.TrimSpace(p.Title), ProjectID: p.ProjectID,
		Channel: p.Channel, Settings: p.Settings, ForkedFrom: p.ParentThreadID})
}

// ReadThread returns a thread with its turns and items.
func (e *Engine) ReadThread(ctx context.Context, id string) (protocol.ThreadReadResult, error) {
	t, err := e.Store.GetThread(ctx, id)
	if err != nil {
		return protocol.ThreadReadResult{}, err
	}
	turns, err := e.Store.ListTurns(ctx, id)
	if err != nil {
		return protocol.ThreadReadResult{}, err
	}
	items, err := e.Store.ListItems(ctx, id, 0)
	if err != nil {
		return protocol.ThreadReadResult{}, err
	}
	return protocol.ThreadReadResult{Thread: t, Turns: turns, Items: items}, nil
}

// UpdateThread applies simple column changes and publishes the result.
func (e *Engine) UpdateThread(ctx context.Context, id string, cols map[string]any) (protocol.Thread, error) {
	if err := e.Store.UpdateThread(ctx, id, cols); err != nil {
		return protocol.Thread{}, err
	}
	e.publishThread(ctx, id)
	return e.Store.GetThread(ctx, id)
}

// SetThreadSettings stores a thread's provider/model/complexity/key choice.
func (e *Engine) SetThreadSettings(ctx context.Context, p protocol.ThreadSetSettingsParams) (protocol.Thread, error) {
	if err := validateSelection(p.Settings); err != nil {
		return protocol.Thread{}, err
	}
	s := p.Settings
	s.Provider = config.NormalizeProvider(s.Provider)
	if s.CredentialID != "" {
		c, err := e.Store.GetCredential(ctx, s.CredentialID)
		if err != nil {
			return protocol.Thread{}, fmt.Errorf("API key: %w", err)
		}
		if s.Provider == "" {
			s.Provider = c.Provider
		} else if c.Provider != s.Provider {
			return protocol.Thread{}, fmt.Errorf("API key %q is for %s, not %s", c.Label, c.Provider, s.Provider)
		}
	}
	return e.UpdateThread(ctx, p.ThreadID, map[string]any{
		"provider": s.Provider, "model": s.Model, "complexity": string(s.Complexity), "credential_id": s.CredentialID,
	})
}

// DeleteThread deletes a thread unless a turn is running in it.
func (e *Engine) DeleteThread(ctx context.Context, id string) error {
	e.mu.Lock()
	_, running := e.threadTurns[id]
	e.mu.Unlock()
	if running {
		return protocol.Errorf(protocol.CodeConflict, "a turn is running in this thread; interrupt it first")
	}
	return e.Store.DeleteThread(ctx, id)
}

// ForkThread copies a thread's items (optionally up to one item) into a new thread.
func (e *Engine) ForkThread(ctx context.Context, p protocol.ThreadForkParams) (protocol.Thread, error) {
	src, err := e.Store.GetThread(ctx, p.ThreadID)
	if err != nil {
		return protocol.Thread{}, err
	}
	var upTo int64
	if p.UpToItemID != "" {
		it, err := e.Store.GetItem(ctx, p.UpToItemID)
		if err != nil || it.ThreadID != src.ID {
			return protocol.Thread{}, fmt.Errorf("item %s not found in thread", p.UpToItemID)
		}
		upTo = it.Seq
	}
	items, err := e.Store.ListItems(ctx, src.ID, upTo)
	if err != nil {
		return protocol.Thread{}, err
	}
	title := src.Title
	channel := "app"
	if p.SideChat {
		title = "Side chat"
		channel = "side"
	} else if title != "" {
		title += " (fork)"
	}
	dst, err := e.Store.CreateThread(ctx, protocol.Thread{Title: title, ProjectID: src.ProjectID, Channel: channel, Settings: src.Settings, ForkedFrom: src.ID})
	if err != nil {
		return protocol.Thread{}, err
	}
	turnIDs := map[string]string{}
	for _, it := range items {
		if it.Status == protocol.ItemInProgress {
			continue
		}
		nt, ok := turnIDs[it.TurnID]
		if !ok {
			nt = store.NewID("trn")
			turnIDs[it.TurnID] = nt
			now := it.CreatedAt
			if err := e.Store.CreateTurn(ctx, protocol.Turn{ID: nt, ThreadID: dst.ID, Status: protocol.TurnCompleted, StartedAt: now, FinishedAt: &now}); err != nil {
				return protocol.Thread{}, err
			}
		}
		it.ID, it.ThreadID, it.TurnID = store.NewID("itm"), dst.ID, nt
		if err := e.Store.SaveItem(ctx, it); err != nil {
			return protocol.Thread{}, err
		}
	}
	return e.Store.GetThread(ctx, dst.ID)
}

// ExportThread renders a thread as Markdown.
func (e *Engine) ExportThread(ctx context.Context, id string) (string, error) {
	r, err := e.ReadThread(ctx, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	title := r.Thread.Title
	if title == "" {
		title = "Untitled chat"
	}
	fmt.Fprintf(&b, "# %s\n\n_Exported from UFoundry on %s_\n\n", title, time.Now().Format("2006-01-02 15:04"))
	for _, it := range r.Items {
		switch it.Kind {
		case protocol.ItemUserMessage:
			fmt.Fprintf(&b, "## You\n\n%s\n\n", it.Text)
		case protocol.ItemAgentMessage:
			fmt.Fprintf(&b, "## UFoundry\n\n%s\n\n", it.Text)
		case protocol.ItemInboundEvent:
			fmt.Fprintf(&b, "> **Inbound:** %s\n\n", it.Text)
		case protocol.ItemToolCall:
			if it.Tool != nil {
				fmt.Fprintf(&b, "<details><summary>Tool: %s (%s)</summary>\n\n```json\n%s\n```\n\n```\n%s%s\n```\n</details>\n\n",
					it.Tool.Name, it.Status, string(it.Tool.Args), it.Tool.Output, it.Tool.Error)
			}
		}
	}
	u := r.Thread.Usage
	fmt.Fprintf(&b, "---\n\nTokens: %d in (%d cached), %d out · Cost: $%.4f\n", u.InputTokens, u.CachedInputTokens, u.OutputTokens, u.CostUSD)
	return b.String(), nil
}

// ---- providers, models, complexity ----

// Providers lists providers with key counts and default models.
func (e *Engine) Providers(ctx context.Context) ([]protocol.Provider, error) {
	creds, err := e.Store.ListCredentials(ctx, "")
	if err != nil {
		return nil, err
	}
	count := map[string]int{}
	for _, c := range creds {
		count[c.Provider]++
	}
	names := map[string]string{"claude": "Anthropic Claude", "openai": "OpenAI", "gemini": "Google Gemini", "openai_compatible": "OpenAI-compatible (local)"}
	var out []protocol.Provider
	for _, id := range e.LLMs.IDs() {
		pc := e.Cfg.LLM.Providers[id]
		out = append(out, protocol.Provider{ID: id, DisplayName: names[id], Enabled: pc.IsEnabled(),
			DefaultModel: e.defaultModel(id), Credentials: count[id]})
	}
	return out, nil
}

// ProviderIdentities reports console subscription sessions without reading or
// returning the credentials owned by Codex or Claude Code.
func (e *Engine) ProviderIdentities(ctx context.Context) []protocol.ProviderIdentity {
	return identity.List(ctx)
}

func (e *Engine) defaultModel(provider string) string {
	if provider == e.Cfg.LLM.Provider && e.Cfg.LLM.Model != "" {
		return e.Cfg.LLM.Model
	}
	if pc, ok := e.Cfg.LLM.Providers[provider]; ok && pc.DefaultModel != "" {
		return pc.DefaultModel
	}
	return e.Catalog.DefaultModel(provider)
}

const complexitySettingKey = "complexity_defaults"

func (e *Engine) loadComplexity(ctx context.Context) error {
	e.presets = models.Presets(e.Cfg.Models)
	e.defaultCplx = protocol.Complexity(e.Cfg.Models.DefaultComplexity)
	var saved protocol.ComplexityDefaults
	if err := e.Store.GetSetting(ctx, complexitySettingKey, &saved); err == nil {
		if saved.Default.Valid() && saved.Default != "" {
			e.defaultCplx = saved.Default
		}
		for _, p := range saved.Presets {
			if _, ok := e.presets[p.Level]; ok {
				e.presets[p.Level] = p
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return nil
}

// ComplexityDefaults returns the default level and presets.
func (e *Engine) ComplexityDefaults() protocol.ComplexityDefaults {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := protocol.ComplexityDefaults{Default: e.defaultCplx}
	for _, lvl := range []protocol.Complexity{protocol.ComplexityQuick, protocol.ComplexityStandard, protocol.ComplexityDeep} {
		out.Presets = append(out.Presets, e.presets[lvl])
	}
	return out
}

// SetComplexityDefaults saves new defaults.
func (e *Engine) SetComplexityDefaults(ctx context.Context, d protocol.ComplexityDefaults) (protocol.ComplexityDefaults, error) {
	if !d.Default.Valid() || d.Default == "" {
		return protocol.ComplexityDefaults{}, fmt.Errorf("invalid default complexity %q", d.Default)
	}
	for _, p := range d.Presets {
		if p.Level == protocol.ComplexityAuto || !p.Level.Valid() || p.Level == "" {
			return protocol.ComplexityDefaults{}, fmt.Errorf("invalid preset level %q", p.Level)
		}
		if p.MaxToolSteps < 1 || p.MaxToolSteps > 200 {
			return protocol.ComplexityDefaults{}, fmt.Errorf("maxToolSteps must be 1–200")
		}
		switch p.Reasoning {
		case llm.ReasoningOff, llm.ReasoningLow, llm.ReasoningMedium, llm.ReasoningHigh:
		default:
			return protocol.ComplexityDefaults{}, fmt.Errorf("invalid reasoning %q", p.Reasoning)
		}
	}
	if err := e.Store.SetSetting(ctx, complexitySettingKey, d); err != nil {
		return protocol.ComplexityDefaults{}, err
	}
	e.mu.Lock()
	e.defaultCplx = d.Default
	for _, p := range d.Presets {
		e.presets[p.Level] = p
	}
	e.mu.Unlock()
	return e.ComplexityDefaults(), nil
}

// RefreshModels fetches the model list with a key (same as testing it).
func (e *Engine) RefreshModels(ctx context.Context, credentialID string) (protocol.CredentialTestResult, error) {
	return e.Creds.Test(ctx, credentialID)
}

// ---- usage ----

// UsageSummary aggregates usage and labels the rows.
func (e *Engine) UsageSummary(ctx context.Context, p protocol.UsageSummaryParams) (protocol.UsageSummaryResult, error) {
	if p.To.IsZero() {
		p.To = time.Now().Add(time.Minute)
	}
	if p.From.IsZero() {
		p.From = p.To.AddDate(0, 0, -30)
	}
	rows, total, err := e.Store.UsageGrouped(ctx, p)
	if err != nil {
		return protocol.UsageSummaryResult{}, err
	}
	switch p.GroupBy {
	case "credential", "":
		creds, _ := e.Store.ListCredentials(ctx, "")
		labels := map[string]string{}
		for _, c := range creds {
			labels[c.ID] = fmt.Sprintf("%s · %s (…%s)", c.Provider, c.Label, c.Last4)
		}
		for i := range rows {
			if l, ok := labels[rows[i].Key]; ok {
				rows[i].Label = l
			} else if rows[i].Key == "" {
				rows[i].Label = "(no key)"
			} else {
				rows[i].Label = rows[i].Key + " (deleted)"
			}
		}
	case "thread":
		for i := range rows {
			if t, err := e.Store.GetThread(ctx, rows[i].Key); err == nil {
				rows[i].Label = t.Title
			}
		}
	default:
		for i := range rows {
			rows[i].Label = rows[i].Key
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if p.GroupBy == "day" {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].Usage.CostUSD > rows[j].Usage.CostUSD
	})
	return protocol.UsageSummaryResult{Rows: rows, Total: total}, nil
}

// ---- helpers ----

func validateSelection(s protocol.ModelSelection) error {
	if !s.Complexity.Valid() {
		return fmt.Errorf("unknown complexity %q (use auto, quick, standard or deep)", s.Complexity)
	}
	return nil
}

// systemPrompt builds the system prompt from the built-in instructions, the
// skills catalog and AGENT.md. userText is used to point at a matching skill.
func (e *Engine) systemPrompt(ctx context.Context, userText string, proj *protocol.Project) string {
	var b strings.Builder
	b.WriteString("You are UFoundry, a personal AI assistant running on the user's own computer. ")
	b.WriteString("Be direct and concise. Use tools when they help; never invent tool results. ")
	b.WriteString("Risky actions (shell commands, writing files) may require the user's approval; if an action is denied, explain and suggest an alternative.\n")
	b.WriteString(fmt.Sprintf("Current time: %s (%s).\n", time.Now().Format(time.RFC1123), tasks.ZoneName(time.Local)))
	if cat := e.Skills.Catalog(); cat != "" {
		b.WriteString("\n" + cat)
		if sk, ok := e.Skills.Match(userText); ok && userText != "" {
			b.WriteString(fmt.Sprintf("The current request may match the %q skill; read its instructions before acting.\n", sk.Name))
		}
	}
	if proj != nil {
		fmt.Fprintf(&b, "\nYou are working in the project %q at %s. Every file you read or change must be inside that folder; "+
			"paths outside it are refused, and shell commands run there. Refer to files by their path relative to the project root.\n",
			proj.Name, proj.Root)
		if proj.VCS != nil && proj.VCS.Branch != "" {
			fmt.Fprintf(&b, "It is a git checkout on branch %s with %d changed files.\n", proj.VCS.Branch, proj.VCS.Dirty)
		}
		instructions, _ := e.Projects.InstructionsFor(ctx, *proj, "")
		b.WriteString(instructions)
		return b.String()
	}
	b.WriteString("\nThis chat is not attached to a project, so you can read and answer but not change files or run commands. " +
		"If the user asks for work on files, ask them to open a project first.\n")
	ctxFile := e.Cfg.Agents.ContextFile
	if ctxFile == "" {
		ctxFile = filepath.Join(e.Cfg.Home, "AGENT.md")
	}
	if data, err := os.ReadFile(ctxFile); err == nil && len(data) > 0 {
		b.WriteString("\n# User context (AGENT.md)\n")
		b.Write(data)
	}
	return b.String()
}
