package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPythonConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("UFOUNDRY_HOME", home)
	t.Setenv("UMABOT_LLM_MODEL", "legacy-env-model")
	cfg := filepath.Join(home, "config.yaml")
	os.WriteFile(cfg, []byte(`
llm:
  provider: anthropic
  model: claude-sonnet-4-6
  providers:
    Google: {default_model: gemini-2.5-pro}
control_panel: {enabled: true, ui_type: web}   # unknown to the Go engine: ignored
tools:
  shell_enabled: true
  workspaces:
    - {name: projects, path: ~/projects, default: true, acl: {delete_files: true}}
storage: {db_path: ~/.ufoundry/custom.db}
models:
  default_complexity: deep
  complexity: {quick: {max_tool_steps: 2}}
`), 0o600)
	c, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.LLM.Provider != "claude" || c.LLM.Model != "legacy-env-model" {
		t.Fatalf("llm = %+v", c.LLM)
	}
	if _, ok := c.LLM.Providers["gemini"]; !ok {
		t.Fatalf("providers not normalized: %v", c.LLM.Providers)
	}
	uh, _ := os.UserHomeDir()
	if c.Tools.Workspaces[0].Path != filepath.Join(uh, "projects") || !*c.Tools.Workspaces[0].ACL.DeleteFiles {
		t.Fatalf("workspace = %+v", c.Tools.Workspaces[0])
	}
	if c.Storage.DBPath != filepath.Join(uh, ".ufoundry", "custom.db") || c.Models.DefaultComplexity != "deep" {
		t.Fatalf("storage/models = %+v %+v", c.Storage, c.Models)
	}
	if c.Runtime.SocketPath != filepath.Join(home, "run", "engine.sock") || c.Runtime.EngineWSPort != 8766 {
		t.Fatalf("runtime = %+v", c.Runtime)
	}
	os.WriteFile(cfg, []byte("models: {default_complexity: extreme}\n"), 0o600)
	if _, err := Load(cfg); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLegacyHomeMigration(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("UFOUNDRY_HOME", "")
	os.MkdirAll(filepath.Join(user, ".umabot"), 0o700)
	os.WriteFile(filepath.Join(user, ".umabot", "umabot.db"), []byte("x"), 0o600)
	h, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if h != filepath.Join(user, ".ufoundry") {
		t.Fatalf("home = %s", h)
	}
	if _, err := os.Stat(filepath.Join(h, "ufoundry.db")); err != nil {
		t.Fatal("db not renamed")
	}
	if fi, err := os.Lstat(filepath.Join(user, ".umabot")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("legacy home symlink missing")
	}
}
