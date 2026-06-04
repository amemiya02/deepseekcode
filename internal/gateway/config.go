package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/amemiya02/deepseekcode/internal/config"
)

// ConfigDTO is the SPA-facing settings shape. It is a curated, flat projection
// of config.Config — only the fields the Settings UI edits — so the wire
// contract stays stable even as config.Config grows. Field names are the JSON
// keys the SPA's system.ts depends on.
type ConfigDTO struct {
	Theme               string `json:"theme"`
	Accent              string `json:"accent"`
	Density             string `json:"density"`
	Language            string `json:"language"`
	TranscriptVerbosity string `json:"transcriptVerbosity"`
	Model               string `json:"model"`
	ReasoningEffort     string `json:"reasoningEffort"`
	BaseURL             string `json:"baseUrl"`
	AutoRoute           bool   `json:"autoRoute"`
	EscalationModel     string `json:"escalationModel"`
	DuetEnabled         bool   `json:"duetEnabled"`
	SandboxEnabled      bool   `json:"sandboxEnabled"`
	SandboxNetwork      bool   `json:"sandboxNetwork"`
	AutoReasoning       bool   `json:"autoReasoning"`
	AutoClarify         bool   `json:"autoClarify"`

	// Network / proxy
	ProxyMode   string `json:"proxyMode"`
	ProxyScheme string `json:"proxyScheme"`
	ProxyURL    string `json:"proxyUrl"`
	NoProxy     string `json:"noProxy"`

	// Keybindings
	VimKeybindings bool `json:"vimKeybindings"`

	// Budget / tools
	MaxReadBytes  int64 `json:"maxReadBytes"`
	MaxWriteBytes int64 `json:"maxWriteBytes"`

	// Permissions
	PermissionDefault string `json:"permissionDefault"`

	// Sessions & storage
	SessionTTLDays       int `json:"sessionTTLDays"`
	SessionSnapshotKeep  int `json:"sessionSnapshotKeep"`
	SessionAutoResumeAge int `json:"sessionAutoResumeAge"`

	// Editor / appearance
	TransparentBackground bool `json:"transparentBackground"`
}

// Config seam: tests swap config load/save so they never touch the real
// ~/.deepseek tree. Production wires config.Load and a TOML writer.
var (
	cfgMu   sync.RWMutex
	cfgLoad = config.Load
	cfgSave = saveConfigDefault
)

// SetConfigSeam overrides the load/save funcs for tests. A nil save leaves the
// default in place.
func SetConfigSeam(load func() (config.Config, error), save func(config.Config) error) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if load != nil {
		cfgLoad = load
	}
	if save != nil {
		cfgSave = save
	}
}

// ResetConfigSeam restores production load/save.
func ResetConfigSeam() {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	cfgLoad = config.Load
	cfgSave = saveConfigDefault
}

func loadConfig() (config.Config, error) {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfgLoad()
}

func storeConfig(c config.Config) error {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfgSave(c)
}

// configToDTO projects the full config tree onto the SPA-facing DTO. Accent and
// Density are surfaced so the Appearance section can round-trip them.
func configToDTO(c config.Config) ConfigDTO {
	return ConfigDTO{
		Theme:               c.Defaults.Theme,
		Accent:              c.UI.Accent,
		Density:             c.UI.Density,
		Language:            c.UI.Language,
		Model:               c.Defaults.Model,
		ReasoningEffort:     c.Defaults.ReasoningEffort,
		BaseURL:             c.API.BaseURL,
		AutoRoute:           c.Routing.AutoRoute,
		EscalationModel:     c.Routing.EscalationModel,
		DuetEnabled:         c.Duet.Enabled,
		SandboxEnabled:      c.Sandbox.Enabled,
		SandboxNetwork:      c.Sandbox.AllowNetwork,
		AutoReasoning:       c.Defaults.AutoReasoning,
		AutoClarify:         c.Clarify.AutoClarify,
		TranscriptVerbosity: "normal", // UI-only preference; persisted client-side, echoed for shape stability.

		// Network / proxy
		ProxyMode:   c.Network.ProxyMode,
		ProxyScheme: c.Network.ProxyScheme,
		ProxyURL:    c.Network.ProxyURL,
		NoProxy:     c.Network.NoProxy,

		// Keybindings
		VimKeybindings: c.Defaults.VimKeybindings,

		// Budget / tools
		MaxReadBytes:  c.Tools.MaxReadBytes,
		MaxWriteBytes: c.Tools.MaxWriteBytes,

		// Permissions
		PermissionDefault: c.Permissions.Default,

		// Sessions & storage
		SessionTTLDays:       c.Sessions.TTLDays,
		SessionSnapshotKeep:  c.Sessions.SnapshotKeep,
		SessionAutoResumeAge: c.Sessions.AutoResumeAge,

		// Editor / appearance
		TransparentBackground: c.UI.TransparentBackground,
	}
}

// applyDTO overlays the editable DTO fields back onto a loaded config so unset
// concerns (mcp servers, hooks, secrets) are preserved untouched. Accent and
// Density are applied so the Appearance section persists them.
func applyDTO(c config.Config, d ConfigDTO) config.Config {
	if d.Theme != "" {
		c.Defaults.Theme = d.Theme
	}
	if d.Accent != "" {
		c.UI.Accent = d.Accent
	}
	if d.Density != "" {
		c.UI.Density = d.Density
	}
	if d.Language != "" {
		c.UI.Language = d.Language
	}
	if d.Model != "" {
		c.Defaults.Model = d.Model
	}
	if d.ReasoningEffort != "" {
		c.Defaults.ReasoningEffort = d.ReasoningEffort
	}
	if d.BaseURL != "" {
		c.API.BaseURL = d.BaseURL
	}
	if d.EscalationModel != "" {
		c.Routing.EscalationModel = d.EscalationModel
	}
	c.Routing.AutoRoute = d.AutoRoute
	c.Duet.Enabled = d.DuetEnabled
	c.Sandbox.Enabled = d.SandboxEnabled
	c.Sandbox.AllowNetwork = d.SandboxNetwork
	c.Defaults.AutoReasoning = d.AutoReasoning
	c.Clarify.AutoClarify = d.AutoClarify

	// Network / proxy
	if d.ProxyMode != "" {
		c.Network.ProxyMode = d.ProxyMode
	}
	if d.ProxyScheme != "" {
		c.Network.ProxyScheme = d.ProxyScheme
	}
	if d.ProxyURL != "" {
		c.Network.ProxyURL = d.ProxyURL
	}
	c.Network.NoProxy = d.NoProxy

	// Keybindings
	c.Defaults.VimKeybindings = d.VimKeybindings

	// Budget / tools
	if d.MaxReadBytes > 0 {
		c.Tools.MaxReadBytes = d.MaxReadBytes
	}
	if d.MaxWriteBytes > 0 {
		c.Tools.MaxWriteBytes = d.MaxWriteBytes
	}

	// Permissions
	if d.PermissionDefault != "" {
		c.Permissions.Default = d.PermissionDefault
	}

	// Sessions & storage
	if d.SessionTTLDays > 0 {
		c.Sessions.TTLDays = d.SessionTTLDays
	}
	if d.SessionSnapshotKeep > 0 {
		c.Sessions.SnapshotKeep = d.SessionSnapshotKeep
	}
	if d.SessionAutoResumeAge > 0 {
		c.Sessions.AutoResumeAge = d.SessionAutoResumeAge
	}

	// Editor / appearance
	c.UI.TransparentBackground = d.TransparentBackground

	return c
}

// handleConfig implements GET/PUT /v1/config.
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		c, err := loadConfig()
		if err != nil {
			http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, configToDTO(c))
	case http.MethodPut:
		var d ConfigDTO
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		c, err := loadConfig()
		if err != nil {
			http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		c = applyDTO(c, d)
		if err := c.Validate(); err != nil {
			http.Error(w, "invalid config: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := storeConfig(c); err != nil {
			http.Error(w, "save config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, configToDTO(c))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// saveConfigDefault writes the editable surface of c to the user config file
// (~/.deepseek/config.toml), preserving unrelated keys already present. It only
// writes the fields the Settings UI owns so secrets and MCP tables are never
// clobbered.
func saveConfigDefault(c config.Config) error {
	dir := config.UserConfigDir()
	if dir == "" {
		return errNoHome{}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.toml")

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &existing); err != nil {
			return err
		}
	}
	setSub(existing, "defaults", map[string]any{
		"theme":            c.Defaults.Theme,
		"model":            c.Defaults.Model,
		"reasoning_effort": c.Defaults.ReasoningEffort,
		"auto_reasoning":   c.Defaults.AutoReasoning,
		"vim_keybindings":  c.Defaults.VimKeybindings,
	})
	setSub(existing, "api", map[string]any{"base_url": c.API.BaseURL})
	setSub(existing, "ui", map[string]any{
		"language":               c.UI.Language,
		"accent":                 c.UI.Accent,
		"density":                c.UI.Density,
		"transparent_background": c.UI.TransparentBackground,
	})
	setSub(existing, "routing", map[string]any{
		"auto_route":       c.Routing.AutoRoute,
		"escalation_model": c.Routing.EscalationModel,
	})
	setSub(existing, "duet", map[string]any{"enabled": c.Duet.Enabled})
	setSub(existing, "sandbox", map[string]any{
		"enabled":       c.Sandbox.Enabled,
		"allow_network": c.Sandbox.AllowNetwork,
	})
	setSub(existing, "clarify", map[string]any{"auto_clarify": c.Clarify.AutoClarify})
	setSub(existing, "network", map[string]any{
		"proxy_mode":   c.Network.ProxyMode,
		"proxy_scheme": c.Network.ProxyScheme,
		"proxy_url":    c.Network.ProxyURL,
		"no_proxy":     c.Network.NoProxy,
	})
	setSub(existing, "tools", map[string]any{
		"max_read_bytes":  c.Tools.MaxReadBytes,
		"max_write_bytes": c.Tools.MaxWriteBytes,
	})
	setSub(existing, "permissions", map[string]any{
		"default": c.Permissions.Default,
	})
	setSub(existing, "sessions", map[string]any{
		"ttl_days":             c.Sessions.TTLDays,
		"snapshot_keep":        c.Sessions.SnapshotKeep,
		"auto_resume_age_hours": c.Sessions.AutoResumeAge,
	})

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(existing)
}

// setSub merges kv into the named sub-table, creating it if absent and
// preserving any keys it does not overwrite.
func setSub(root map[string]any, name string, kv map[string]any) {
	sub, _ := root[name].(map[string]any)
	if sub == nil {
		sub = map[string]any{}
	}
	for k, v := range kv {
		sub[k] = v
	}
	root[name] = sub
}

type errNoHome struct{}

func (errNoHome) Error() string { return "no home directory for config" }
