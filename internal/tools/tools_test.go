package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shaktsin/ufoundry/internal/config"
)

func TestWorkspaceACLAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "s.txt"), []byte("secret"), 0o600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	f := false
	cfg := config.Default(t.TempDir())
	cfg.Tools.Workspaces = []config.WorkspaceConfig{{Name: "ro", Path: root, ACL: config.WorkspaceACL{Write: &f, CreateFiles: &f}}}
	ws, _ := NewWorkspaces(cfg).Get("")
	if _, err := ws.Resolve("escape/s.txt", OpRead); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("symlink escape allowed: %v", err)
	}
	if _, err := ws.Resolve("../x", OpRead); err == nil {
		t.Fatal("dotdot escape allowed")
	}
	if _, err := ws.Resolve("new.txt", OpCreate); err == nil || !strings.Contains(err.Error(), "does not allow create") {
		t.Fatalf("ACL not enforced: %v", err)
	}
	if _, err := ws.Resolve("sub/new.txt", OpRead); err != nil {
		t.Fatalf("read inside denied: %v", err)
	}
}

func TestShellRunAndRisk(t *testing.T) {
	cfg := config.Default(t.TempDir())
	cfg.Tools.ShellEnabled = true
	cfg.Policy.AutoApproveShellCommands = []string{"ls", "git status"}
	t.Setenv("OPENAI_API_KEY", "must-not-leak")
	r := NewRegistry()
	RegisterBuiltins(r, cfg, NewWorkspaces(cfg), nil)
	project := t.TempDir()
	ctx := WithScope(context.Background(), &Scope{ProjectID: "prj_1", ProjectName: "demo", Root: project, AllowShell: true})
	sh, ok := r.Get("shell__run")
	if !ok {
		t.Fatal("shell.run not registered")
	}
	for cmd, want := range map[string]Risk{"ls -la": RiskYellow, "git status": RiskYellow, "ls; rm -rf /": RiskRed, "rm x": RiskRed} {
		args, _ := json.Marshal(map[string]string{"command": cmd})
		if got, _ := sh.Assess(args); got != want {
			t.Errorf("%q risk = %s, want %s", cmd, got, want)
		}
	}
	// Without a project there is no place to run.
	if _, err := sh.Call(context.Background(), json.RawMessage(`{"command":"ls"}`)); !errors.Is(err, ErrNoProject) {
		t.Fatalf("shell without a project: %v", err)
	}
	out, err := sh.Call(ctx, json.RawMessage(`{"command":"echo $OPENAI_API_KEY; pwd; exit 3"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "must-not-leak") || !strings.Contains(out, "exit_code: 3") || !strings.Contains(out, project) {
		t.Fatalf("out = %s", out)
	}
	if _, err := sh.Call(ctx, json.RawMessage(`{"command":"sleep 5","timeout_seconds":1}`)); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
}

func TestProjectScope(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("one\ntwo\n"), 0o644)

	var changed []string
	scope := &Scope{ProjectID: "prj_1", ProjectName: "demo", Root: root, AllowShell: true,
		Record: func(_ context.Context, abs string, before *string, deleted bool) {
			kind := "modified"
			if before == nil {
				kind = "created"
			}
			changed = append(changed, kind+" "+filepath.Base(abs))
		}}
	ctx := WithScope(context.Background(), scope)
	cfg := config.Default(t.TempDir())
	cfg.Tools.ShellEnabled = true
	r := NewRegistry()
	RegisterBuiltins(r, cfg, NewWorkspaces(cfg), nil)

	write, _ := r.Get("file__write")
	read, _ := r.Get("file__read")

	// Paths outside the project are refused, including through a symlink.
	for _, path := range []string{"../escape.txt", outside + "/secret.txt", "escape/secret.txt"} {
		args, _ := json.Marshal(map[string]string{"path": path, "content": "x"})
		if _, err := write.Call(ctx, args); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Fatalf("write to %s: %v", path, err)
		}
		args, _ = json.Marshal(map[string]string{"path": path})
		if _, err := read.Call(ctx, args); err == nil {
			t.Fatalf("read of %s was allowed", path)
		}
	}

	// Writing inside records the change.
	args, _ := json.Marshal(map[string]string{"path": "notes.txt", "content": "one\ntwo\nthree\n"})
	if _, err := write.Call(ctx, args); err != nil {
		t.Fatal(err)
	}
	args, _ = json.Marshal(map[string]string{"path": "sub/new.txt", "content": "hello"})
	if _, err := write.Call(ctx, args); err != nil {
		t.Fatal(err)
	}
	if strings.Join(changed, ", ") != "modified notes.txt, created new.txt" {
		t.Fatalf("recorded changes = %v", changed)
	}

	// Network commands are refused while the project has networking off.
	sh, _ := r.Get("shell__run")
	if _, err := sh.Call(ctx, json.RawMessage(`{"command":"curl https://example.com"}`)); err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("network guard: %v", err)
	}
	scope.AllowNet = true
	if _, err := sh.Call(ctx, json.RawMessage(`{"command":"echo ok"}`)); err != nil {
		t.Fatal(err)
	}
}
