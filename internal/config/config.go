// Package config loads ~/.ufoundry/config.yaml. It reads the same keys as the
// Python app; unknown keys are ignored so existing files keep working.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// Config is the subset of config.yaml the Go engine understands today.
type Config struct {
	Version int           `yaml:"version"`
	LLM     LLMConfig     `yaml:"llm"`
	Agents  AgentsConfig  `yaml:"agents"`
	Tools   ToolsConfig   `yaml:"tools"`
	Policy  PolicyConfig  `yaml:"policy"`
	Storage StorageConfig `yaml:"storage"`
	Runtime RuntimeConfig `yaml:"runtime"`
	Models  ModelsConfig  `yaml:"models"`

	// SkillDirs are extra directories scanned for skills (each subfolder with a SKILL.md).
	SkillDirs []string `yaml:"skill_dirs"`
	// Skills holds runtime overrides: "defaults" plus one entry per skill name.
	Skills map[string]SkillRuntimeOverride `yaml:"skills"`
	// MCPServers are Model Context Protocol servers whose tools the agent can use.
	MCPServers []MCPServerConfig `yaml:"mcp_servers"`

	// Path is where the config was loaded from ("" if defaults only).
	Path string `yaml:"-"`
	// Home is the resolved UFoundry home directory.
	Home string `yaml:"-"`
}

type LLMConfig struct {
	Provider        string                       `yaml:"provider"`
	Model           string                       `yaml:"model"`
	APIKey          string                       `yaml:"api_key"` // legacy; migrated to Keychain
	ReasoningEffort string                       `yaml:"reasoning_effort"`
	Providers       map[string]LLMProviderConfig `yaml:"providers"`
}

type LLMProviderConfig struct {
	Enabled      *bool    `yaml:"enabled"`
	APIKey       string   `yaml:"api_key"` // legacy; migrated to Keychain
	BaseURL      string   `yaml:"base_url"`
	Models       []string `yaml:"models"`
	DefaultModel string   `yaml:"default_model"`
}

// IsEnabled defaults to true when unset.
func (p LLMProviderConfig) IsEnabled() bool { return p.Enabled == nil || *p.Enabled }

type AgentModelConfig struct {
	Provider        string `yaml:"provider"`
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
}

type AgentsConfig struct {
	Enabled                   bool             `yaml:"enabled"`
	ContextFile               string           `yaml:"context_file"`
	Orchestrator              AgentModelConfig `yaml:"orchestrator"`
	Worker                    AgentModelConfig `yaml:"worker"`
	MaxAgentIterations        int              `yaml:"max_agent_iterations"`
	MaxOrchestratorIterations int              `yaml:"max_orchestrator_iterations"`
	TokensPerMinute           int              `yaml:"tokens_per_minute"`
}

type WorkspaceACL struct {
	Read        *bool `yaml:"read"`
	Write       *bool `yaml:"write"`
	CreateFiles *bool `yaml:"create_files"`
	DeleteFiles *bool `yaml:"delete_files"`
	Shell       *bool `yaml:"shell"`
}

type WorkspaceConfig struct {
	Name    string       `yaml:"name"`
	Path    string       `yaml:"path"`
	ACL     WorkspaceACL `yaml:"acl"`
	Default bool         `yaml:"default"`
}

type ToolsConfig struct {
	ShellEnabled bool              `yaml:"shell_enabled"`
	Workspaces   []WorkspaceConfig `yaml:"workspaces"`
}

type PolicyConfig struct {
	ConfirmationStrictness   string   `yaml:"confirmation_strictness"` // normal | strict
	ApprovalMode             string   `yaml:"approval_mode"`           // normal | auto_approve_workspace
	AutoApproveTools         []string `yaml:"auto_approve_tools"`
	AutoApproveShellCommands []string `yaml:"auto_approve_shell_commands"`
	ApprovalTimeoutMinutes   int      `yaml:"approval_timeout_minutes"`
}

type StorageConfig struct {
	DBPath   string `yaml:"db_path"`
	VaultDir string `yaml:"vault_dir"`
}

type RuntimeConfig struct {
	LogDir  string `yaml:"log_dir"`
	WSHost  string `yaml:"ws_host"`
	WSPort  int    `yaml:"ws_port"`
	WSToken string `yaml:"ws_token"`
	// EngineWSPort is the Go engine's protocol WebSocket (0 disables it).
	// It is separate from ws_port so the Python gateway can run alongside
	// during the migration.
	EngineWSPort int `yaml:"engine_ws_port"`
	// SocketPath is the engine's Unix socket (new in the Go engine).
	SocketPath string `yaml:"socket_path"`
}

// ModelsConfig holds new model-selection settings.
type ModelsConfig struct {
	DefaultComplexity string                      `yaml:"default_complexity"`
	Complexity        map[string]ComplexityPreset `yaml:"complexity"`
	// Roles overrides the model per role: title, intent, orchestrator, worker.
	Roles map[string]AgentModelConfig `yaml:"roles"`
}

type ComplexityPreset struct {
	Reasoning       string `yaml:"reasoning"`
	MaxToolSteps    int    `yaml:"max_tool_steps"`
	MultiAgent      string `yaml:"multi_agent"`
	MaxOutputTokens int    `yaml:"max_output_tokens"`
}

// SkillRuntimeOverride overrides a skill's runtime (python/node binaries, PATH, env).
type SkillRuntimeOverride struct {
	PythonBin string            `yaml:"python_bin"`
	NodeBin   string            `yaml:"node_bin"`
	ExtraPath []string          `yaml:"extra_path"`
	Env       map[string]string `yaml:"env"`
	// Config is passed to the skill's scripts as the "config" object on stdin.
	Config map[string]any `yaml:"config"`
}

// MCPServerConfig is one MCP server (same keys as the Python app).
type MCPServerConfig struct {
	Name              string            `yaml:"name"`
	Transport         string            `yaml:"transport"` // stdio (default) | http
	Command           string            `yaml:"command"`
	Args              []string          `yaml:"args"`
	Env               map[string]string `yaml:"env"`
	EnvVars           []string          `yaml:"env_vars"`
	Cwd               string            `yaml:"cwd"`
	URL               string            `yaml:"url"`
	BearerTokenEnvVar string            `yaml:"bearer_token_env_var"`
	HTTPHeaders       map[string]string `yaml:"http_headers"`
	EnvHTTPHeaders    map[string]string `yaml:"env_http_headers"`
	Enabled           *bool             `yaml:"enabled"`
	Required          bool              `yaml:"required"`
	StartupTimeoutSec float64           `yaml:"startup_timeout_sec"`
	ToolTimeoutSec    float64           `yaml:"tool_timeout_sec"`
	EnabledTools      []string          `yaml:"enabled_tools"`
	DisabledTools     []string          `yaml:"disabled_tools"`
	// RiskLevel overrides the risk of every tool on this server: green | yellow | red.
	RiskLevel string `yaml:"risk_level"`
}

// IsEnabled defaults to true when unset.
func (m MCPServerConfig) IsEnabled() bool { return m.Enabled == nil || *m.Enabled }

// HomeDir returns the UFoundry home: $UFOUNDRY_HOME, else ~/.ufoundry.
// If only the pre-rename ~/.umabot exists it is moved to ~/.ufoundry.
func HomeDir() (string, error) {
	if h := os.Getenv("UFOUNDRY_HOME"); h != "" {
		return expand(h), nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home := filepath.Join(userHome, ".ufoundry")
	legacy := filepath.Join(userHome, ".umabot")
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		if st, err := os.Stat(legacy); err == nil && st.IsDir() {
			if err := os.Rename(legacy, home); err == nil {
				// Leave a symlink so explicit ~/.umabot paths in config keep working.
				_ = os.Symlink(home, legacy)
				slog.Info("migrated legacy home", "from", legacy, "to", home)
				migrateLegacyDB(home)
			}
		}
	}
	return home, nil
}

func migrateLegacyDB(home string) {
	oldDB := filepath.Join(home, "umabot.db")
	newDB := filepath.Join(home, "ufoundry.db")
	if _, err := os.Stat(newDB); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(oldDB); err == nil {
			if os.Rename(oldDB, newDB) == nil {
				_ = os.Symlink(newDB, oldDB)
			}
		}
	}
}

// Load reads config from path, or from the default locations when path is "".
func Load(path string) (*Config, error) {
	home, err := HomeDir()
	if err != nil {
		return nil, err
	}
	cfg := Default(home)

	candidates := []string{path}
	if path == "" {
		candidates = []string{filepath.Join(home, "config.yaml")}
		if wd, err := os.Getwd(); err == nil {
			candidates = append([]string{filepath.Join(wd, "config.yaml")}, candidates...)
		}
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		p = expand(p)
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			if path != "" {
				return nil, fmt.Errorf("config file %s not found", p)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		cfg.Path = p
		break
	}
	applyEnv(cfg)
	cfg.finalize()
	return cfg, cfg.Validate()
}

// Default returns a config with defaults rooted at home.
func Default(home string) *Config {
	return &Config{
		Version: 1,
		Home:    home,
		LLM:     LLMConfig{Provider: "claude"},
		Storage: StorageConfig{
			DBPath:   filepath.Join(home, "ufoundry.db"),
			VaultDir: filepath.Join(home, "vault"),
		},
		Runtime: RuntimeConfig{
			LogDir:       filepath.Join(home, "logs"),
			WSHost:       "127.0.0.1",
			WSPort:       8765,
			EngineWSPort: 8766,
			SocketPath:   filepath.Join(home, "run", "engine.sock"),
		},
		Policy: PolicyConfig{
			ConfirmationStrictness: "normal",
			ApprovalMode:           "normal",
			ApprovalTimeoutMinutes: 30,
		},
		Models: ModelsConfig{DefaultComplexity: "auto"},
	}
}

func (c *Config) finalize() {
	c.Storage.DBPath = expand(c.Storage.DBPath)
	c.Storage.VaultDir = expand(c.Storage.VaultDir)
	c.Runtime.LogDir = expand(c.Runtime.LogDir)
	c.Runtime.SocketPath = expand(c.Runtime.SocketPath)
	if c.Runtime.SocketPath == "" {
		c.Runtime.SocketPath = filepath.Join(c.Home, "run", "engine.sock")
	}
	for i := range c.Tools.Workspaces {
		c.Tools.Workspaces[i].Path = expand(c.Tools.Workspaces[i].Path)
	}
	for i := range c.SkillDirs {
		c.SkillDirs[i] = expand(c.SkillDirs[i])
	}
	for i := range c.MCPServers {
		c.MCPServers[i].Cwd = expand(c.MCPServers[i].Cwd)
	}
	if c.Agents.ContextFile != "" {
		c.Agents.ContextFile = expand(c.Agents.ContextFile)
	}
	if c.Policy.ApprovalTimeoutMinutes <= 0 {
		c.Policy.ApprovalTimeoutMinutes = 30
	}
	if c.Models.DefaultComplexity == "" {
		c.Models.DefaultComplexity = "auto"
	}
	c.LLM.Provider = NormalizeProvider(c.LLM.Provider)
	if c.LLM.Providers != nil {
		norm := make(map[string]LLMProviderConfig, len(c.LLM.Providers))
		for k, v := range c.LLM.Providers {
			norm[NormalizeProvider(k)] = v
		}
		c.LLM.Providers = norm
	}
}

// Validate checks values the engine depends on.
func (c *Config) Validate() error {
	switch c.Models.DefaultComplexity {
	case "auto", "quick", "standard", "deep":
	default:
		return fmt.Errorf("models.default_complexity: unknown value %q", c.Models.DefaultComplexity)
	}
	seen := map[string]bool{}
	for _, m := range c.MCPServers {
		if m.Name == "" {
			return fmt.Errorf("mcp_servers: every server needs a name")
		}
		if seen[m.Name] {
			return fmt.Errorf("mcp_servers: duplicate name %q", m.Name)
		}
		seen[m.Name] = true
		switch m.Transport {
		case "", "stdio":
			if m.Command == "" && m.IsEnabled() {
				return fmt.Errorf("mcp_servers.%s: command is required for stdio", m.Name)
			}
		case "http":
			if m.URL == "" && m.IsEnabled() {
				return fmt.Errorf("mcp_servers.%s: url is required for http", m.Name)
			}
		default:
			return fmt.Errorf("mcp_servers.%s: unknown transport %q", m.Name, m.Transport)
		}
	}
	if c.Runtime.EngineWSPort < 0 || c.Runtime.EngineWSPort > 65535 {
		return fmt.Errorf("runtime.engine_ws_port out of range: %d", c.Runtime.EngineWSPort)
	}
	return nil
}

// NormalizeProvider maps aliases to canonical provider ids.
func NormalizeProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "anthropic", "claude":
		return "claude"
	case "google", "gemini":
		return "gemini"
	case "openai":
		return "openai"
	case "openai_compatible", "openai-compatible", "ollama", "lmstudio", "openrouter":
		return "openai_compatible"
	}
	return strings.ToLower(strings.TrimSpace(p))
}

// envPrefixes are checked in order; UMABOT_ is a deprecated fallback.
var envPrefixes = []string{"UFOUNDRY_", "UMABOT_"}

func getenv(name string) (string, bool) {
	for _, p := range envPrefixes {
		if v, ok := os.LookupEnv(p + name); ok {
			if p == "UMABOT_" {
				slog.Warn("deprecated environment variable, rename to UFOUNDRY_"+name, "var", p+name)
			}
			return v, true
		}
	}
	return "", false
}

func applyEnv(c *Config) {
	if v, ok := getenv("LLM_PROVIDER"); ok {
		c.LLM.Provider = v
	}
	if v, ok := getenv("LLM_MODEL"); ok {
		c.LLM.Model = v
	}
	if v, ok := getenv("LLM_API_KEY"); ok {
		c.LLM.APIKey = v
	}
	if v, ok := getenv("DB_PATH"); ok {
		c.Storage.DBPath = v
	}
	if v, ok := getenv("LOG_DIR"); ok {
		c.Runtime.LogDir = v
	}
	if v, ok := getenv("WS_HOST"); ok {
		c.Runtime.WSHost = v
	}
	if v, ok := getenv("ENGINE_WS_PORT"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Runtime.EngineWSPort = n
		}
	}
	if v, ok := getenv("SHELL_TOOL"); ok {
		c.Tools.ShellEnabled = v == "1" || strings.EqualFold(v, "true")
	}
	if v, ok := getenv("APPROVAL_MODE"); ok {
		c.Policy.ApprovalMode = v
	}
}

func expand(p string) string {
	if p == "" {
		return p
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	return os.ExpandEnv(p)
}

// ReadDotenv parses KEY=VALUE lines from ~/.ufoundry/.env (the Python app's
// secret fallback). Missing files return an empty map.
func ReadDotenv(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}
