package tools

import (
	"context"
	"encoding/json"
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
	out, err := sh.Call(context.Background(), json.RawMessage(`{"command":"echo $OPENAI_API_KEY; pwd; exit 3"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "must-not-leak") || !strings.Contains(out, "exit_code: 3") || !strings.Contains(out, "workspace") {
		t.Fatalf("out = %s", out)
	}
	if _, err := sh.Call(context.Background(), json.RawMessage(`{"command":"sleep 5","timeout_seconds":1}`)); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout: %v", err)
	}
}
