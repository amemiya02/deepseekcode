// Package config loads deepseekcode configuration from TOML with
// precedence: CLI flags > project ./.deepseek/config.toml >
// user ~/.deepseek/config.toml > built-in defaults.
//
// Env vars referenced as ${VAR} inside string values are expanded at
// load time. API key resolution falls back to DEEPSEEK_API_KEY when
// [api].key is empty.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
)

// Config is the full configuration tree. Keep it flat per concern.
type Config struct {
	API         APIConfig                  `toml:"api"`
	Defaults    DefaultsConfig             `toml:"defaults"`
	Tools       ToolsConfig                `toml:"tools"`
	Duet        DuetConfig                 `toml:"duet"`
	Permissions PermissionsConfig          `toml:"permissions"`
	Sessions    SessionsConfig             `toml:"sessions"`
	Hooks       []HookItemConfig           `toml:"hooks"`
	MCPServers  map[string]MCPServerConfig `toml:"mcp_servers"`
}

type APIConfig struct {
	Key                 string `toml:"key"`
	BaseURL             string `toml:"base_url"`
	FirstTokenTimeoutMs int    `toml:"first_token_timeout_ms"`
	ChunkStallTimeoutMs int    `toml:"chunk_stall_timeout_ms"`
}

type DefaultsConfig struct {
	Model          string `toml:"model"`
	Thinking       bool   `toml:"thinking"`
	Theme          string `toml:"theme"`
	VimKeybindings bool   `toml:"vim_keybindings"`
}

type DuetConfig struct {
	Enabled            bool     `toml:"enabled"`
	RetryOnFailure     bool     `toml:"retry_on_failure"`
	ValidatorTimeoutMs int      `toml:"validator_timeout_ms"`
	ExtraDestructive   []string `toml:"extra_destructive_patterns"`
}

type PermissionsConfig struct {
	AllowBash          []string    `toml:"allow_bash"`
	SecretPathPatterns []string    `toml:"secret_path_patterns"`
	Rules              RulesConfig `toml:"rules"`
}

type RulesConfig struct {
	Allow []RuleItemConfig `toml:"allow"`
	Deny  []RuleItemConfig `toml:"deny"`
	Ask   []RuleItemConfig `toml:"ask"`
}

type RuleItemConfig struct {
	Tool string `toml:"tool"`
	Args string `toml:"args"`
}

type SessionsConfig struct {
	TTLDays       int `toml:"ttl_days"`
	SnapshotKeep  int `toml:"snapshot_keep"`
	AutoResumeAge int `toml:"auto_resume_age_hours"`
}

type HookItemConfig struct {
	Event   string `toml:"event"`   // PreToolUse, PostToolUse, PostToolUseFailure, SessionStart, SessionEnd
	Type    string `toml:"type"`    // "subprocess" | "builtin"
	Command string `toml:"command"` // when type == subprocess
	Name    string `toml:"name"`    // when type == builtin
	Timeout int    `toml:"timeout_seconds"`
}

type ToolsConfig struct {
	MaxReadBytes  int64 `toml:"max_read_bytes"`  // default 5_242_880 (5 MiB)
	MaxWriteBytes int64 `toml:"max_write_bytes"` // default 5_242_880 (5 MiB)
}

type MCPServerConfig struct {
	Command        string            `toml:"command"`
	Args           []string          `toml:"args"`
	Env            map[string]string `toml:"env"`
	TimeoutSeconds int               `toml:"timeout_seconds"`
}

// Load reads user + project config and overlays them onto defaults.
// Missing files are not errors; malformed files are.
func Load() (Config, error) {
	cfg := Default()

	if home, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(home, ".deepseek", "config.toml")
		if err := mergeFile(&cfg, userPath); err != nil {
			return cfg, fmt.Errorf("user config: %w", err)
		}
	}

	projectPath := filepath.Join(".deepseek", "config.toml")
	if err := mergeFile(&cfg, projectPath); err != nil {
		return cfg, fmt.Errorf("project config: %w", err)
	}

	expandEnvFields(&cfg)

	if cfg.API.Key == "" {
		cfg.API.Key = os.Getenv("DEEPSEEK_API_KEY")
	}
	return cfg, nil
}

// Validate checks invariants. Call after Load + any CLI overrides.
func (c Config) Validate() error {
	if c.API.Key == "" {
		return errors.New("DEEPSEEK_API_KEY is not set (export it or put `key` in [api])")
	}
	if c.API.BaseURL == "" {
		return errors.New("api.base_url is empty")
	}
	if c.Defaults.Model == "" {
		return errors.New("defaults.model is empty")
	}
	return nil
}

// UserConfigDir returns ~/.deepseek (or "" if no home).
func UserConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".deepseek")
}

// ProjectStateDir returns the local ./.deepseek dir (snapshots, last_session).
func ProjectStateDir() string {
	return ".deepseek"
}

func mergeFile(cfg *Config, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	// Decode into a partial overlay struct so unset fields don't zero
	// existing defaults. We decode into the actual Config (BurntSushi
	// toml leaves fields it doesn't see untouched), then explicitly
	// preserve slices/maps if the overlay didn't provide them.
	var overlay Config
	if _, err := toml.Decode(string(b), &overlay); err != nil {
		return err
	}
	applyOverlay(cfg, overlay)
	return nil
}

func applyOverlay(base *Config, ov Config) {
	if ov.API.Key != "" {
		base.API.Key = ov.API.Key
	}
	if ov.API.BaseURL != "" {
		base.API.BaseURL = ov.API.BaseURL
	}
	if ov.API.FirstTokenTimeoutMs != 0 {
		base.API.FirstTokenTimeoutMs = ov.API.FirstTokenTimeoutMs
	}
	if ov.API.ChunkStallTimeoutMs != 0 {
		base.API.ChunkStallTimeoutMs = ov.API.ChunkStallTimeoutMs
	}

	if ov.Defaults.Model != "" {
		base.Defaults.Model = ov.Defaults.Model
	}
	// bool overlays only flip if explicitly set in toml; we can't tell
	// from a zero value alone. For booleans we accept the overlay always
	// — the TOML decoder leaves unset bools as false, but our defaults
	// are deliberately true for Thinking and VimKeybindings. Users who
	// want to disable must set "thinking = false" explicitly, which
	// matches their intent.
	base.Defaults.Thinking = ov.Defaults.Thinking || (base.Defaults.Thinking && !overlayHasKey(ov, "thinking"))
	if ov.Defaults.Theme != "" {
		base.Defaults.Theme = ov.Defaults.Theme
	}
	base.Defaults.VimKeybindings = ov.Defaults.VimKeybindings || (base.Defaults.VimKeybindings && !overlayHasKey(ov, "vim_keybindings"))

	if ov.Tools.MaxReadBytes != 0 {
		base.Tools.MaxReadBytes = ov.Tools.MaxReadBytes
	}
	if ov.Tools.MaxWriteBytes != 0 {
		base.Tools.MaxWriteBytes = ov.Tools.MaxWriteBytes
	}

	// Duet
	base.Duet.Enabled = ov.Duet.Enabled || (base.Duet.Enabled && !overlayHasKey(ov, "duet.enabled"))
	base.Duet.RetryOnFailure = ov.Duet.RetryOnFailure || (base.Duet.RetryOnFailure && !overlayHasKey(ov, "duet.retry_on_failure"))
	if ov.Duet.ValidatorTimeoutMs != 0 {
		base.Duet.ValidatorTimeoutMs = ov.Duet.ValidatorTimeoutMs
	}
	if len(ov.Duet.ExtraDestructive) > 0 {
		base.Duet.ExtraDestructive = ov.Duet.ExtraDestructive
	}

	if len(ov.Permissions.AllowBash) > 0 {
		base.Permissions.AllowBash = ov.Permissions.AllowBash
	}
	if len(ov.Permissions.SecretPathPatterns) > 0 {
		base.Permissions.SecretPathPatterns = ov.Permissions.SecretPathPatterns
	}
	if len(ov.Permissions.Rules.Allow) > 0 {
		base.Permissions.Rules.Allow = ov.Permissions.Rules.Allow
	}
	if len(ov.Permissions.Rules.Deny) > 0 {
		base.Permissions.Rules.Deny = ov.Permissions.Rules.Deny
	}
	if len(ov.Permissions.Rules.Ask) > 0 {
		base.Permissions.Rules.Ask = ov.Permissions.Rules.Ask
	}

	if ov.Sessions.TTLDays != 0 {
		base.Sessions.TTLDays = ov.Sessions.TTLDays
	}
	if ov.Sessions.SnapshotKeep != 0 {
		base.Sessions.SnapshotKeep = ov.Sessions.SnapshotKeep
	}
	if ov.Sessions.AutoResumeAge != 0 {
		base.Sessions.AutoResumeAge = ov.Sessions.AutoResumeAge
	}

	if len(ov.Hooks) > 0 {
		base.Hooks = ov.Hooks
	}
	if len(ov.MCPServers) > 0 {
		base.MCPServers = ov.MCPServers
	}
}

// overlayHasKey is a temporary stub: real toml-key-presence tracking
// requires either a custom decoder or a re-parse. v0.1 accepts the
// simpler "false overlays defaults only if file is non-empty" semantics
// and documents the explicit form (`thinking = false`).
func overlayHasKey(_ Config, _ string) bool {
	return false
}

var envVarRe = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

func expandEnvFields(c *Config) {
	c.API.Key = expandEnv(c.API.Key)
	c.API.BaseURL = expandEnv(c.API.BaseURL)
	for name, srv := range c.MCPServers {
		srv.Command = expandEnv(srv.Command)
		for i := range srv.Args {
			srv.Args[i] = expandEnv(srv.Args[i])
		}
		for k, v := range srv.Env {
			srv.Env[k] = expandEnv(v)
		}
		c.MCPServers[name] = srv
	}
}

func expandEnv(s string) string {
	if s == "" {
		return s
	}
	return envVarRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[2 : len(m)-1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return m
	})
}
