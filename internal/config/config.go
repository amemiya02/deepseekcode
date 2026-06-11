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
	API         APIConfig                     `toml:"api"`
	Defaults    DefaultsConfig                `toml:"defaults"`
	Tools       ToolsConfig                   `toml:"tools"`
	Duet        DuetConfig                    `toml:"duet"`
	Permissions PermissionsConfig             `toml:"permissions"`
	Sessions    SessionsConfig                `toml:"sessions"`
	Hooks       []HookItemConfig              `toml:"hooks"`
	MCPServers  map[string]MCPServerConfig    `toml:"mcp_servers"`
	Web         WebConfig                     `toml:"web"`
	Sandbox     SandboxConfig                 `toml:"sandbox"`
	Network     NetworkConfig                 `toml:"network"`
	Active      ActiveConfig                  `toml:"active"`
	Providers   map[string]ProviderConfigTOML `toml:"providers"`
	UI          UIConfig                      `toml:"ui"`
	Routing     RoutingConfig                 `toml:"routing"`
	Clarify     ClarifyConfig                 `toml:"clarify"`
	Cache       CacheConfig                   `toml:"cache"`

	LegacyAPIUsed         bool `toml:"-"`
	DefaultsModelExplicit bool `toml:"-"`
	providersExplicit     bool
}

// CacheConfig controls prompt-cache alignment and compaction behaviour.
// All fields default to the zero value (off), so omitting the [cache]
// section preserves today's behaviour exactly.
type CacheConfig struct {
	// AlignUnit is the measured DeepSeek cache block size in tokens.
	// When non-zero, the prefix assembler pads messages so they end on
	// an AlignUnit boundary, maximising cache hit rate. 128 is
	// empirically measured (bench/cmd/cacheprobe; docs/pricing.md) but
	// the hit-rate benefit is not yet validated by the head-to-head
	// benchmark — default off until proven. Set to 128 to opt in.
	AlignUnit int `toml:"align_unit"`
	// FIMFastPath enables a gated, conservative Fill-In-the-Middle
	// cheap-edit path for small single-hunk edits. Default false (off).
	FIMFastPath bool `toml:"fim_fastpath"`
}

// UIConfig holds presentational TUI options. Purely cosmetic — nothing
// here touches the wire / prompt-cache path.
type UIConfig struct {
	// TransparentBackground, when true, degrades the bg-tier panels (tool
	// results, diffs, reasoning) to left-bars / separators with no opaque
	// fills — a fully flat / fg-only look. The full-screen canvas is not
	// painted in either case (it bled through glamour/viewport ANSI resets;
	// see docs/adr/0002), so this is a "no backgrounds at all" toggle.
	// Default false (panels render their fills).
	TransparentBackground bool `toml:"transparent_background"`
	// Language overrides the UI message language for API errors. "" or "auto"
	// inspects LC_ALL/LC_MESSAGES/LANG; "en" forces English; "zh" forces Chinese.
	Language string `toml:"language"`
	// Accent is the colour accent token (e.g. "indigo", "terracotta") used by
	// the desktop SPA's ThemeProvider. Persisted so the Appearance section
	// round-trips correctly. Empty means "use the theme default".
	Accent string `toml:"accent"`
	// Density is the layout density token ("comfortable" | "compact") used by
	// the desktop SPA. Persisted so the Appearance section round-trips.
	// Empty means "use the theme default".
	Density string `toml:"density"`
}

// WebConfig configures web fetch and search tools.
type WebConfig struct {
	Enabled        bool   `toml:"enabled"`
	SearchProvider string `toml:"search_provider"`
	SearXNGBaseURL string `toml:"searxng_base_url"`
	AllowPrivate   bool   `toml:"allow_private"`
}

// SandboxConfig controls optional OS-native sandboxing for shell tools.
type SandboxConfig struct {
	Enabled         bool     `toml:"enabled"`
	AllowReadPaths  []string `toml:"allow_read_paths"`
	AllowWritePaths []string `toml:"allow_write_paths"`
	AllowNetwork    bool     `toml:"allow_network"`
}

type ActiveConfig struct {
	Provider string `toml:"provider"`
}

type ProviderConfigTOML struct {
	Type                string `toml:"type"`
	BaseURL             string `toml:"base_url"`
	APIKey              string `toml:"api_key"`
	EnvVar              string `toml:"env_var"`
	SecretsFileKey      string `toml:"secrets_file_key"`
	FirstTokenTimeoutMs int    `toml:"first_token_timeout_ms"`
	ChunkStallTimeoutMs int    `toml:"chunk_stall_timeout_ms"`
	DefaultModel        string   `toml:"default_model"`
	Models              []string `toml:"models"`
}

type APIConfig struct {
	Key                 string `toml:"key"`
	BaseURL             string `toml:"base_url"`
	UserID              string `toml:"user_id"`
	FirstTokenTimeoutMs int    `toml:"first_token_timeout_ms"`
	ChunkStallTimeoutMs int    `toml:"chunk_stall_timeout_ms"`
}

type DefaultsConfig struct {
	Model           string `toml:"model"`
	Thinking        bool   `toml:"thinking"`
	ReasoningEffort string `toml:"reasoning_effort"`
	Theme           string `toml:"theme"`
	VimKeybindings  bool   `toml:"vim_keybindings"`
	// AutoReasoning enables per-turn thinking selection based on the
	// user's last message. When true, SelectThinking decides whether
	// to enable/disable thinking each turn using keyword heuristics.
	// Default: false (opt-in).
	AutoReasoning bool `toml:"auto_reasoning"`
}

// RoutingConfig exposes the per-turn cost-aware router (internal/routing).
// All fields default to the zero value (off), so omitting the [routing]
// section preserves today's behavior exactly.
type RoutingConfig struct {
	// AutoRoute enables Flash-first routing: mechanical turns stay on the
	// cheap model + Non-think; reasoning/ambiguity/repeated-repair turns
	// escalate to EscalationModel at high/max effort. Default false (opt-in).
	AutoRoute bool `toml:"auto_route"`
	// EscalationModel is the pro-tier model AutoRoute escalates to. When
	// AutoRoute is true and this is empty, the wiring layer defaults it to
	// deepseek-v4-pro so escalation is actually reachable.
	EscalationModel string `toml:"escalation_model"`
}

// ClarifyConfig exposes the intent-clarify gate (internal/routing.NeedsClarification).
type ClarifyConfig struct {
	// AutoClarify asks one clarifying question on an under-specified prompt
	// before spending a (possibly pro/max) model turn. Default false (opt-in).
	AutoClarify bool `toml:"auto_clarify"`
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
	AllowSpecifiers    []string    `toml:"allow_specifiers"`
	DenySpecifiers     []string    `toml:"deny_specifiers"`
	AskSpecifiers      []string    `toml:"ask_specifiers"`
	// Default is the autonomy/permission level selected during onboarding.
	// Accepted values: "ask", "auto", "full". Empty means "ask" (safest).
	Default string `toml:"default"`
}

// NetworkConfig controls outbound proxy settings for the gateway HTTP client.
type NetworkConfig struct {
	// ProxyMode selects the proxy strategy: "auto" (platform default),
	// "env" (HTTPS_PROXY / NO_PROXY env vars), "custom" (explicit URL below),
	// "off" (bypass all proxies). Empty → "auto".
	ProxyMode string `toml:"proxy_mode"`
	// ProxyScheme is the scheme for the custom proxy: "http", "https",
	// "socks5", or "socks5h". Only used when ProxyMode == "custom".
	ProxyScheme string `toml:"proxy_scheme"`
	// ProxyURL is the host:port (no scheme) of the custom proxy.
	ProxyURL string `toml:"proxy_url"`
	// NoProxy is the comma-separated no-proxy list used when ProxyMode == "custom".
	NoProxy string `toml:"no_proxy"`
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
	Transport      string            `toml:"transport"` // "stdio" (default) | "sse" | "http" | "streamable-http"
	URL            string            `toml:"url"`       // required when transport="sse"
	Command        string            `toml:"command"`   // required when transport="stdio"
	Args           []string          `toml:"args"`
	Env            map[string]string `toml:"env"`
	TimeoutSeconds int               `toml:"timeout_seconds"`
	EnabledTools   []string          `toml:"enabled_tools"`      // allowlist: only these tools exposed
	DisabledTools  []string          `toml:"disabled_tools"`     // blocklist: these tools hidden
	Disabled       bool              `toml:"disabled,omitempty"` // configured but not connected
}

// ActiveMCPServers returns the subset of MCPServers that are not Disabled, so the
// runtime bridge and doctor connect only enabled servers. The result is a new map.
func (c Config) ActiveMCPServers() map[string]MCPServerConfig {
	out := make(map[string]MCPServerConfig, len(c.MCPServers))
	for name, srv := range c.MCPServers {
		if srv.Disabled {
			continue
		}
		out[name] = srv
	}
	return out
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

	// Merge .mcp.json (Claude/Cursor format) at project root — TOML wins on
	// name conflicts, so only names absent from the TOML map are added.
	if dotMCP, err := LoadDotMCPJSON(".mcp.json"); err == nil && len(dotMCP) > 0 {
		for name, srv := range dotMCP {
			if _, exists := cfg.MCPServers[name]; !exists {
				cfg.MCPServers[name] = srv
			}
		}
	}

	if cfg.API.Key == "" {
		cfg.API.Key = os.Getenv("DEEPSEEK_API_KEY")
	}
	applyLegacyAPICompat(&cfg)

	// Overlay state.toml [ui].theme over config.toml defaults.theme.
	if st, err := LoadState(); err == nil && st.UI.Theme != "" {
		cfg.Defaults.Theme = st.UI.Theme
	}

	return cfg, nil
}

// Validate checks invariants. Call after Load + any CLI overrides.
func (c Config) Validate() error {
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
	meta, err := toml.Decode(string(b), &overlay)
	if err != nil {
		return err
	}
	applyOverlay(cfg, overlay, meta)
	return nil
}

func applyOverlay(base *Config, ov Config, meta toml.MetaData) {
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
	if ov.API.UserID != "" {
		base.API.UserID = ov.API.UserID
	}
	if meta.IsDefined("api", "key") || meta.IsDefined("api", "base_url") {
		base.LegacyAPIUsed = true
	}

	if ov.Defaults.Model != "" {
		base.Defaults.Model = ov.Defaults.Model
	}
	if meta.IsDefined("defaults", "model") {
		base.DefaultsModelExplicit = true
	}
	// bool overlays only flip if explicitly set in toml; we can't tell
	// from a zero value alone. For booleans we accept the overlay always
	// — the TOML decoder leaves unset bools as false, but our defaults
	// are deliberately true for Thinking and VimKeybindings. Users who
	// want to disable must set "thinking = false" explicitly, which
	// matches their intent.
	base.Defaults.Thinking = ov.Defaults.Thinking || (base.Defaults.Thinking && !meta.IsDefined("defaults", "thinking"))
	if ov.Defaults.Theme != "" {
		base.Defaults.Theme = ov.Defaults.Theme
	}
	base.Defaults.VimKeybindings = ov.Defaults.VimKeybindings || (base.Defaults.VimKeybindings && !meta.IsDefined("defaults", "vim_keybindings"))
	// AutoReasoning defaults to false; opt-in semantics: either source
	// setting it to true is sufficient (OR merge). No need for the
	// overlayHasKey dance that Thinking/VimKeybindings require (those
	// default true and need "false means explicit disable" logic).
	base.Defaults.AutoReasoning = ov.Defaults.AutoReasoning || base.Defaults.AutoReasoning

	// Routing/Clarify: opt-in bools use OR-merge (either source enabling is
	// sufficient); EscalationModel is a non-empty-wins string. Mirrors the
	// AutoReasoning handling — these default false, so no "explicit false"
	// dance is needed.
	base.Routing.AutoRoute = ov.Routing.AutoRoute || base.Routing.AutoRoute
	if ov.Routing.EscalationModel != "" {
		base.Routing.EscalationModel = ov.Routing.EscalationModel
	}
	base.Clarify.AutoClarify = ov.Clarify.AutoClarify || base.Clarify.AutoClarify

	// Cache: AlignUnit is non-zero-wins (0 = alignment off / not yet measured).
	if ov.Cache.AlignUnit != 0 {
		base.Cache.AlignUnit = ov.Cache.AlignUnit
	}
	// FIMFastPath: bool opt-in, default false.
	if meta.IsDefined("cache", "fim_fastpath") {
		base.Cache.FIMFastPath = ov.Cache.FIMFastPath
	}

	if ov.Defaults.ReasoningEffort != "" {
		base.Defaults.ReasoningEffort = ov.Defaults.ReasoningEffort
	}

	if ov.Tools.MaxReadBytes != 0 {
		base.Tools.MaxReadBytes = ov.Tools.MaxReadBytes
	}
	if ov.Tools.MaxWriteBytes != 0 {
		base.Tools.MaxWriteBytes = ov.Tools.MaxWriteBytes
	}

	// Duet
	base.Duet.Enabled = ov.Duet.Enabled || (base.Duet.Enabled && !meta.IsDefined("duet", "enabled"))
	base.Duet.RetryOnFailure = ov.Duet.RetryOnFailure || (base.Duet.RetryOnFailure && !meta.IsDefined("duet", "retry_on_failure"))
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
	if ov.Permissions.Default != "" {
		base.Permissions.Default = ov.Permissions.Default
	}
	if len(ov.Permissions.AllowSpecifiers) > 0 {
		base.Permissions.AllowSpecifiers = ov.Permissions.AllowSpecifiers
	}
	if len(ov.Permissions.DenySpecifiers) > 0 {
		base.Permissions.DenySpecifiers = ov.Permissions.DenySpecifiers
	}
	if len(ov.Permissions.AskSpecifiers) > 0 {
		base.Permissions.AskSpecifiers = ov.Permissions.AskSpecifiers
	}

	// Network / proxy: copy non-empty values so users can clear fields by
	// setting an explicit empty in project config if needed. NoProxy may
	// legitimately be empty, so we always overlay it when the key is defined.
	if ov.Network.ProxyMode != "" {
		base.Network.ProxyMode = ov.Network.ProxyMode
	}
	if ov.Network.ProxyScheme != "" {
		base.Network.ProxyScheme = ov.Network.ProxyScheme
	}
	if ov.Network.ProxyURL != "" {
		base.Network.ProxyURL = ov.Network.ProxyURL
	}
	if meta.IsDefined("network", "no_proxy") {
		base.Network.NoProxy = ov.Network.NoProxy
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

	// Web config: Enabled defaults to true, so we need to check if explicitly set
	if meta.IsDefined("web", "enabled") {
		base.Web.Enabled = ov.Web.Enabled
	}
	if ov.Web.SearchProvider != "" {
		base.Web.SearchProvider = ov.Web.SearchProvider
	}
	if ov.Web.SearXNGBaseURL != "" {
		base.Web.SearXNGBaseURL = ov.Web.SearXNGBaseURL
	}
	// AllowPrivate defaults to false, so use OR semantics
	if ov.Web.AllowPrivate {
		base.Web.AllowPrivate = true
	}

	if meta.IsDefined("sandbox", "enabled") {
		base.Sandbox.Enabled = ov.Sandbox.Enabled
	}
	if len(ov.Sandbox.AllowReadPaths) > 0 {
		base.Sandbox.AllowReadPaths = ov.Sandbox.AllowReadPaths
	}
	if len(ov.Sandbox.AllowWritePaths) > 0 {
		base.Sandbox.AllowWritePaths = ov.Sandbox.AllowWritePaths
	}
	if meta.IsDefined("sandbox", "allow_network") {
		base.Sandbox.AllowNetwork = ov.Sandbox.AllowNetwork
	}

	// UI string fields: copy when the overlay explicitly sets them so that a
	// process restart (or second GET /v1/config) round-trips saved values.
	if ov.UI.Language != "" {
		base.UI.Language = ov.UI.Language
	}
	if ov.UI.Accent != "" {
		base.UI.Accent = ov.UI.Accent
	}
	if ov.UI.Density != "" {
		base.UI.Density = ov.UI.Density
	}

	// UI: TransparentBackground defaults to false, so only flip when the
	// overlay explicitly defined it (mirrors sandbox/web bool handling).
	if meta.IsDefined("ui", "transparent_background") {
		base.UI.TransparentBackground = ov.UI.TransparentBackground
	}

	if ov.Active.Provider != "" {
		base.Active.Provider = ov.Active.Provider
	}
	if len(ov.Providers) > 0 {
		base.Providers = ov.Providers
		base.providersExplicit = true
	}
}

var envVarRe = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

func expandEnvFields(c *Config) {
	c.API.Key = expandEnv(c.API.Key)
	c.API.BaseURL = expandEnv(c.API.BaseURL)
	for name, provider := range c.Providers {
		provider.BaseURL = expandEnv(provider.BaseURL)
		provider.APIKey = expandEnv(provider.APIKey)
		c.Providers[name] = provider
	}
	for i := range c.Sandbox.AllowReadPaths {
		c.Sandbox.AllowReadPaths[i] = expandEnv(c.Sandbox.AllowReadPaths[i])
	}
	for i := range c.Sandbox.AllowWritePaths {
		c.Sandbox.AllowWritePaths[i] = expandEnv(c.Sandbox.AllowWritePaths[i])
	}
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

func applyLegacyAPICompat(c *Config) {
	if c.Active.Provider == "" {
		c.Active.Provider = "deepseek"
	}
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfigTOML{}
	}
	p, ok := c.Providers["deepseek"]
	if !ok {
		p = ProviderConfigTOML{Type: "deepseek"}
	}
	if p.BaseURL == "" {
		p.BaseURL = c.API.BaseURL
	}
	if p.EnvVar == "" && !c.providersExplicit {
		p.EnvVar = "DEEPSEEK_API_KEY"
	}
	if c.API.Key != "" && !c.providersExplicit && p.APIKey == "" {
		p.APIKey = c.API.Key
	}
	if p.FirstTokenTimeoutMs == 0 {
		p.FirstTokenTimeoutMs = c.API.FirstTokenTimeoutMs
	}
	if p.ChunkStallTimeoutMs == 0 {
		p.ChunkStallTimeoutMs = c.API.ChunkStallTimeoutMs
	}
	c.Providers["deepseek"] = p
}
