package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/shaktsin/ufoundry/internal/protocol"
	"github.com/shaktsin/ufoundry/internal/store"
	"github.com/shaktsin/ufoundry/internal/tools"
)

// RunFunc executes a task as a turn and returns the turn id and final text.
type RunFunc func(ctx context.Context, t protocol.Task) (turnID, result string, err error)

// Service creates, lists and runs scheduled tasks.
type Service struct {
	st       *store.Store
	log      *slog.Logger
	run      RunFunc
	notify   func(protocol.Task)
	Poll     time.Duration
	Lease    time.Duration
	Parallel int

	wg sync.WaitGroup
}

// NewService returns a task service. run executes due tasks; notify (optional)
// is called whenever a task changes.
func NewService(st *store.Store, log *slog.Logger, run RunFunc, notify func(protocol.Task)) *Service {
	return &Service{st: st, log: log, run: run, notify: notify, Poll: 5 * time.Second, Lease: 2 * time.Hour, Parallel: 4}
}

// Create validates and stores a new task.
func (s *Service) Create(ctx context.Context, p protocol.TaskCreateParams, createdBy string) (protocol.Task, error) {
	p.Name, p.Prompt = strings.TrimSpace(p.Name), strings.TrimSpace(p.Prompt)
	if p.Prompt == "" {
		return protocol.Task{}, errors.New("prompt is required")
	}
	if p.Name == "" {
		p.Name = p.Prompt
		if len([]rune(p.Name)) > 50 {
			p.Name = string([]rune(p.Name)[:50]) + "…"
		}
	}
	if p.TaskType == "" {
		if p.Schedule.RunAt != "" {
			p.TaskType = protocol.TaskOneTime
		} else {
			p.TaskType = protocol.TaskPeriodic
		}
	}
	if !p.Settings.Complexity.Valid() {
		return protocol.Task{}, fmt.Errorf("unknown complexity %q", p.Settings.Complexity)
	}
	loc, err := LoadZone(p.Timezone)
	if err != nil {
		return protocol.Task{}, fmt.Errorf("unknown timezone %q", p.Timezone)
	}
	zone := ZoneName(loc)
	if err := Validate(p.TaskType, p.Schedule, zone); err != nil {
		return protocol.Task{}, err
	}
	next, err := Next(p.TaskType, p.Schedule, zone, time.Now(), false)
	if err != nil {
		return protocol.Task{}, err
	}
	if next == nil || (p.TaskType == protocol.TaskOneTime && next.Before(time.Now().Add(-time.Minute))) {
		return protocol.Task{}, errors.New("that time is in the past")
	}
	t, err := s.st.CreateTask(ctx, protocol.Task{Name: p.Name, Prompt: p.Prompt, TaskType: p.TaskType,
		Schedule: p.Schedule, Timezone: zone, Settings: p.Settings, NextRunAt: next, CreatedBy: createdBy})
	if err != nil {
		return t, err
	}
	s.changed(ctx, t.ID)
	return t, nil
}

// Cancel stops a task.
func (s *Service) Cancel(ctx context.Context, id int64) (protocol.Task, error) {
	if err := s.st.CancelTask(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return protocol.Task{}, fmt.Errorf("task %d not found or already cancelled", id)
		}
		return protocol.Task{}, err
	}
	s.changed(ctx, id)
	return s.st.GetTask(ctx, id)
}

// RunNow makes an active task due immediately.
func (s *Service) RunNow(ctx context.Context, id int64) (protocol.Task, error) {
	if err := s.st.SetTaskNextRun(ctx, id, time.Now().Add(-time.Second)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return protocol.Task{}, fmt.Errorf("task %d is not active", id)
		}
		return protocol.Task{}, err
	}
	s.changed(ctx, id)
	return s.st.GetTask(ctx, id)
}

func (s *Service) changed(ctx context.Context, id int64) {
	if s.notify == nil {
		return
	}
	if t, err := s.st.GetTask(ctx, id); err == nil {
		s.notify(t)
	}
}

// Start runs the scheduler loop until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		sem := make(chan struct{}, s.Parallel)
		tick := time.NewTicker(s.Poll)
		defer tick.Stop()
		for {
			s.dispatch(ctx, sem)
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

// Wait blocks until the loop and running tasks have stopped.
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) dispatch(ctx context.Context, sem chan struct{}) {
	free := cap(sem) - len(sem)
	if free <= 0 || ctx.Err() != nil {
		return
	}
	due, err := s.st.LeaseDueTasks(context.WithoutCancel(ctx), time.Now(), s.Lease, free)
	if err != nil {
		s.log.Error("lease due tasks", "err", err)
		return
	}
	for _, t := range due {
		sem <- struct{}{}
		s.wg.Add(1)
		go func(t protocol.Task) {
			defer s.wg.Done()
			defer func() { <-sem }()
			s.execute(ctx, t)
		}(t)
	}
}

// execute runs one task and schedules its next run.
func (s *Service) execute(ctx context.Context, t protocol.Task) {
	sctx := context.WithoutCancel(ctx)
	log := s.log.With("task", t.ID, "name", t.Name)
	runID, err := s.st.StartTaskRun(sctx, t.ID, "")
	if err != nil {
		log.Error("start task run", "err", err)
		_ = s.st.ReleaseTaskLease(sctx, t.ID)
		return
	}
	log.Info("task started")
	turnID, result, runErr := s.run(ctx, t)
	if turnID != "" {
		_ = s.st.SetTaskRunTurn(sctx, runID, turnID)
	}
	var next *time.Time
	terminal := t.TaskType == protocol.TaskOneTime
	if !terminal {
		n, err := Next(t.TaskType, t.Schedule, t.Timezone, time.Now(), true)
		if err != nil || n == nil {
			terminal = true
		} else {
			next = n
		}
	}
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
		log.Warn("task failed", "err", runErr)
	} else {
		log.Info("task finished")
	}
	// One-time tasks are not retried after a failure (the Python app retried
	// forever); periodic tasks simply wait for their next slot.
	if err := s.st.FinishTaskRun(sctx, runID, t.ID, runErr == nil, result, errText, next, terminal); err != nil {
		log.Error("finish task run", "err", err)
	}
	s.changed(sctx, t.ID)
}

// ---- agent tools ----

// Register adds task.create, task.list and task.cancel to the tool registry.
func (s *Service) Register(reg *tools.Registry) {
	reg.Add(&createTool{s})
	reg.Add(&listTool{s})
	reg.Add(&cancelTool{s})
}

type createTool struct{ s *Service }

func (*createTool) Name() string { return "task.create" }
func (*createTool) Description() string {
	return "Schedule a task: a prompt UFoundry runs later, once (task_type one_time with run_at) or repeatedly (periodic: hourly with minute, daily/weekly with time HH:MM and day_of_week, or cron). Times are in the given IANA timezone, default the user's local zone."
}
func (*createTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{
"name":{"type":"string","description":"Short name"},
"prompt":{"type":"string","description":"What to do when the task runs, written as an instruction to yourself"},
"task_type":{"type":"string","enum":["one_time","periodic"]},
"run_at":{"type":"string","description":"one_time: local date-time like 2026-09-20T09:00"},
"frequency":{"type":"string","enum":["hourly","daily","weekly","cron"]},
"time":{"type":"string","description":"daily/weekly: HH:MM"},
"minute":{"type":"integer","description":"hourly: minute of the hour"},
"day_of_week":{"type":"string","description":"weekly: mon..sun"},
"cron":{"type":"string","description":"cron: 5-field expression, e.g. 0 9 * * mon-fri"},
"timezone":{"type":"string","description":"IANA zone, e.g. America/Los_Angeles"}},
"required":["prompt","task_type"]}`)
}

type createArgs struct {
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	TaskType  string `json:"task_type"`
	RunAt     string `json:"run_at"`
	Frequency string `json:"frequency"`
	Time      string `json:"time"`
	Minute    *int   `json:"minute"`
	DayOfWeek string `json:"day_of_week"`
	Cron      string `json:"cron"`
	Timezone  string `json:"timezone"`
}

func (a createArgs) params() protocol.TaskCreateParams {
	return protocol.TaskCreateParams{Name: a.Name, Prompt: a.Prompt, TaskType: a.TaskType, Timezone: a.Timezone,
		Schedule: protocol.Schedule{RunAt: a.RunAt, Frequency: a.Frequency, Time: a.Time, Minute: a.Minute, DayOfWeek: a.DayOfWeek, Cron: a.Cron}}
}

func (*createTool) Assess(args json.RawMessage) (tools.Risk, string) {
	var a createArgs
	_ = json.Unmarshal(args, &a)
	return tools.RiskYellow, "Schedule task: " + describe(a.params())
}
func (t *createTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var a createArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	task, err := t.s.Create(ctx, a.params(), "agent")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Scheduled task #%d %q; next run %s.", task.ID, task.Name, fmtNext(task)), nil
}

type listTool struct{ s *Service }

func (*listTool) Name() string        { return "task.list" }
func (*listTool) Description() string { return "List scheduled tasks (active by default)." }
func (*listTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["active","completed","cancelled","all"]}}}`)
}
func (*listTool) Assess(json.RawMessage) (tools.Risk, string) { return tools.RiskGreen, "List tasks" }
func (t *listTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(args, &a)
	status := a.Status
	if status == "" {
		status = protocol.TaskActive
	} else if status == "all" {
		status = ""
	}
	list, err := t.s.st.ListTasks(ctx, status)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No tasks.", nil
	}
	var b strings.Builder
	for _, task := range list {
		fmt.Fprintf(&b, "#%d %s [%s] %s — next %s\n", task.ID, task.Name, task.Status,
			describe(protocol.TaskCreateParams{TaskType: task.TaskType, Schedule: task.Schedule, Timezone: task.Timezone}), fmtNext(task))
	}
	return b.String(), nil
}

type cancelTool struct{ s *Service }

func (*cancelTool) Name() string        { return "task.cancel" }
func (*cancelTool) Description() string { return "Cancel a scheduled task by id." }
func (*cancelTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"task_id":{"type":"integer"}},"required":["task_id"]}`)
}
func (*cancelTool) Assess(args json.RawMessage) (tools.Risk, string) {
	var a struct {
		TaskID int64 `json:"task_id"`
	}
	_ = json.Unmarshal(args, &a)
	return tools.RiskYellow, fmt.Sprintf("Cancel task #%d", a.TaskID)
}
func (t *cancelTool) Call(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		TaskID int64 `json:"task_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	task, err := t.s.Cancel(ctx, a.TaskID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Cancelled task #%d %q.", task.ID, task.Name), nil
}

// Describe renders a schedule in words.
func Describe(p protocol.TaskCreateParams) string { return describe(p) }

func describe(p protocol.TaskCreateParams) string {
	s := p.Schedule
	var when string
	if p.TaskType == protocol.TaskOneTime {
		when = "once at " + s.RunAt
	} else {
		switch strings.ToLower(orDefault(s.Frequency, "daily")) {
		case "hourly":
			m := 0
			if s.Minute != nil {
				m = *s.Minute
			}
			when = fmt.Sprintf("hourly at :%02d", m)
		case "daily":
			when = "daily at " + orDefault(s.Time, "09:00")
		case "weekly":
			when = fmt.Sprintf("weekly on %s at %s", orDefault(s.DayOfWeek, "mon"), orDefault(s.Time, "09:00"))
		case "cron":
			when = "cron " + s.Cron
		default:
			when = s.Frequency
		}
	}
	if p.Timezone != "" {
		when += " (" + p.Timezone + ")"
	}
	if p.Name != "" {
		return fmt.Sprintf("%q %s", p.Name, when)
	}
	return when
}

func fmtNext(t protocol.Task) string {
	if t.NextRunAt == nil {
		return "none"
	}
	loc, err := LoadZone(t.Timezone)
	if err != nil {
		loc = time.Local
	}
	return t.NextRunAt.In(loc).Format("Mon Jan 2 15:04 MST")
}
