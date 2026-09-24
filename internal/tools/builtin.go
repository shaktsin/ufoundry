package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shaktsin/ufoundry/internal/config"
	"github.com/shaktsin/ufoundry/internal/procutil"
)

const maxOutput = 64 << 10

// SkillEnvFunc returns the environment for running commands as part of a skill.
type SkillEnvFunc func(ctx context.Context, skill string) ([]string, error)

// RegisterBuiltins adds the built-in tools allowed by config. skillEnv may be nil.
func RegisterBuiltins(r *Registry, cfg *config.Config, ws *Workspaces, skillEnv SkillEnvFunc) {
	r.Add(&fileRead{ws})
	r.Add(&fileList{ws})
	r.Add(&fileWrite{ws})
	if cfg.Tools.ShellEnabled {
		r.Add(&shellRun{ws: ws, autoApprove: cfg.Policy.AutoApproveShellCommands, skillEnv: skillEnv})
	}
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

func clip(s string) string {
	if len(s) <= maxOutput {
		return s
	}
	cut := s[:maxOutput]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return cut + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-len(cut))
}

// resolvePath resolves a tool's path argument. Inside a project every path is
// resolved against its root; without a project the configured workspaces are
// used, and only for reading.
func resolvePath(ctx context.Context, ws *Workspaces, name, path string, op Operation) (string, error) {
	if scope := ScopeFrom(ctx); scope != nil {
		if name != "" && name != "project" {
			return "", fmt.Errorf("this chat works in the project %s; %q is not one of its folders", scope.ProjectName, name)
		}
		return scope.Resolve(path)
	}
	if op != OpRead {
		return "", ErrNoProject
	}
	w, err := ws.Get(name)
	if err != nil {
		return "", err
	}
	return w.Resolve(path, op)
}

// ---- file.read ----

type fileRead struct{ ws *Workspaces }

func (*fileRead) Name() string { return "file.read" }
func (*fileRead) Description() string {
	return "Read a text file from the open project. Paths are relative to the project root."
}
func (*fileRead) Schema() json.RawMessage {
	return schema(`{"type":"object","properties":{"path":{"type":"string","description":"File path, relative to the workspace root or absolute inside it"},"workspace":{"type":"string","description":"Workspace name; omit for the default"}},"required":["path"]}`)
}
func (*fileRead) Assess(args json.RawMessage) (Risk, string) {
	a, _ := decode[struct{ Path string }](args)
	return RiskGreen, "Read " + a.Path
}
func (t *fileRead) Call(ctx context.Context, args json.RawMessage) (string, error) {
	a, err := decode[struct{ Path, Workspace string }](args)
	if err != nil {
		return "", err
	}
	p, err := resolvePath(ctx, t.ws, a.Workspace, a.Path, OpRead)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", errors.New("file is not UTF-8 text")
	}
	return clip(string(b)), nil
}

// ---- file.list ----

type fileList struct{ ws *Workspaces }

func (*fileList) Name() string { return "file.list" }
func (*fileList) Description() string {
	return "List files in a directory of the open project."
}
func (*fileList) Schema() json.RawMessage {
	return schema(`{"type":"object","properties":{"path":{"type":"string","description":"Directory, relative to the workspace root; default is the root"},"workspace":{"type":"string"}}}`)
}
func (*fileList) Assess(args json.RawMessage) (Risk, string) {
	a, _ := decode[struct{ Path string }](args)
	return RiskGreen, "List " + a.Path
}
func (t *fileList) Call(ctx context.Context, args json.RawMessage) (string, error) {
	a, err := decode[struct{ Path, Workspace string }](args)
	if err != nil {
		return "", err
	}
	p, err := resolvePath(ctx, t.ws, a.Workspace, a.Path, OpRead)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var b strings.Builder
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		kind := "file"
		if e.IsDir() {
			kind = "dir"
		} else if info.Mode()&fs.ModeSymlink != 0 {
			kind = "link"
		}
		fmt.Fprintf(&b, "%s\t%s\t%d\n", kind, e.Name(), info.Size())
	}
	if b.Len() == 0 {
		return "(empty directory)", nil
	}
	return clip(b.String()), nil
}

// ---- file.write ----

type fileWrite struct{ ws *Workspaces }

func (*fileWrite) Name() string { return "file.write" }
func (*fileWrite) Description() string {
	return "Create or overwrite a text file inside the open project. The change is recorded and can be undone."
}
func (*fileWrite) Schema() json.RawMessage {
	return schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"workspace":{"type":"string"}},"required":["path","content"]}`)
}
func (*fileWrite) Assess(args json.RawMessage) (Risk, string) {
	a, _ := decode[struct {
		Path    string
		Content string
	}](args)
	return RiskYellow, fmt.Sprintf("Write %d bytes to %s", len(a.Content), a.Path)
}
func (t *fileWrite) Call(ctx context.Context, args json.RawMessage) (string, error) {
	a, err := decode[struct{ Path, Content, Workspace string }](args)
	if err != nil {
		return "", err
	}
	scope := ScopeFrom(ctx)
	if scope == nil {
		return "", ErrNoProject
	}
	p, err := resolvePath(ctx, t.ws, a.Workspace, a.Path, OpWrite)
	if err != nil {
		return "", err
	}
	before := Snapshot(p)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	if scope.Record != nil {
		scope.Record(ctx, p, before, false)
	}
	what := "wrote"
	if before == nil {
		what = "created"
	}
	return fmt.Sprintf("%s %s (%d bytes)", what, scope.Rel(p), len(a.Content)), nil
}

// ---- shell.run ----

type shellRun struct {
	ws          *Workspaces
	autoApprove []string
	skillEnv    SkillEnvFunc
}

func (*shellRun) Name() string { return "shell.run" }
func (*shellRun) Description() string {
	return "Run a shell command (sh -c) in the open project. Returns exit code, stdout and stderr."
}
func (*shellRun) Schema() json.RawMessage {
	return schema(`{"type":"object","properties":{"command":{"type":"string"},"workspace":{"type":"string","description":"Folder inside the project to run in; default the project root"},"timeout_seconds":{"type":"integer","description":"Default 60, max 600"},"skill":{"type":"string","description":"Run with this skill's environment (its venv, PATH and env vars; SKILL_DIR is set)"}},"required":["command"]}`)
}

func (t *shellRun) Assess(args json.RawMessage) (Risk, string) {
	a, _ := decode[struct{ Command string }](args)
	cmd := strings.TrimSpace(a.Command)
	for _, prefix := range t.autoApprove {
		if prefix != "" && (cmd == prefix || strings.HasPrefix(cmd, prefix+" ")) && !strings.ContainsAny(cmd, ";&|`$><") {
			return RiskYellow, "Run: " + cmd
		}
	}
	return RiskRed, "Run: " + cmd
}

// safeEnv is the environment passed to shell commands: no API keys or tokens.
func safeEnv() []string {
	keep := []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "TMPDIR", "SHELL", "TERM"}
	var env []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func (t *shellRun) Call(ctx context.Context, args json.RawMessage) (string, error) {
	a, err := decode[struct {
		Command        string
		Workspace      string
		Skill          string
		TimeoutSeconds int `json:"timeout_seconds"`
	}](args)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(a.Command) == "" {
		return "", errors.New("command is empty")
	}
	scope := ScopeFrom(ctx)
	if scope == nil {
		return "", ErrNoProject
	}
	if !scope.AllowShell {
		return "", fmt.Errorf("shell commands are switched off for the project %s", scope.ProjectName)
	}
	if !scope.AllowNet {
		if prog, yes := NeedsNetwork(a.Command); yes {
			return "", fmt.Errorf("%s needs the network, which is off for the project %s (turn it on in the project's settings)", prog, scope.ProjectName)
		}
	}
	dir := scope.Root
	if a.Workspace != "" {
		if d, err := scope.Resolve(a.Workspace); err == nil {
			dir = d
		} else {
			return "", err
		}
	}
	timeout := time.Duration(a.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "/bin/sh", "-c", a.Command)
	procutil.Prepare(cmd)
	cmd.Dir = dir
	cmd.Env = safeEnv()
	if a.Skill != "" {
		if t.skillEnv == nil {
			return "", errors.New("skills are not available")
		}
		env, err := t.skillEnv(ctx, a.Skill)
		if err != nil {
			return "", err
		}
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	code := 0
	var ee *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &ee):
		code = ee.ExitCode()
	case cctx.Err() != nil:
		return "", fmt.Errorf("command timed out after %s", timeout)
	default:
		return "", runErr
	}
	if cctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s", timeout)
	}
	out := fmt.Sprintf("exit_code: %d\n--- stdout ---\n%s", code, stdout.String())
	if stderr.Len() > 0 {
		out += "\n--- stderr ---\n" + stderr.String()
	}
	return clip(out), nil
}
