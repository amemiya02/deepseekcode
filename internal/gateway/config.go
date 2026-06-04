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
	})
	setSub(existing, "api", map[string]any{"base_url": c.API.BaseURL})
	setSub(existing, "ui", map[string]any{
		"language": c.UI.Language,
		"accent":   c.UI.Accent,
		"density":  c.UI.Density,
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
