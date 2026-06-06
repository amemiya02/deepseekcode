package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/mcp"
)

// rawIO is the read/write seam over ~/.deepseek/config.toml as a raw map, so the
// MCP writer edits only the targeted [mcp_servers.<name>] table and never
// re-marshals the env-expanded in-memory Config (which would bake $HOME-style
// placeholders in other servers). Tests swap this for an in-memory pair.
type rawIO struct {
	read  func() (map[string]any, error)
	write func(map[string]any) error
}

var mcpRawIO = rawIO{read: readUserConfigRaw, write: writeUserConfigRaw}

func userConfigPath() (string, error) {
	dir := config.UserConfigDir()
	if dir == "" {
		return "", errNoHome{}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func readUserConfigRaw() (map[string]any, error) {
	path, err := userConfigPath()
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func writeUserConfigRaw(m map[string]any) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(m)
}

// mutateMCP reads the raw config map, applies fn to the (created-if-absent)
// mcp_servers sub-table, and writes the whole map back. Other tables are
// preserved verbatim.
func mutateMCP(name string, fn func(servers map[string]any)) error {
	existing, err := mcpRawIO.read()
	if err != nil {
		return err
	}
	servers, _ := existing["mcp_servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	fn(servers)
	existing["mcp_servers"] = servers
	return mcpRawIO.write(existing)
}

// buildOneMCP renders one server config as a TOML-encodable map, omitting empty
// fields so the written table stays minimal.
func buildOneMCP(srv config.MCPServerConfig) map[string]any {
	m := map[string]any{}
	if srv.Transport != "" {
		m["transport"] = srv.Transport
	}
	if srv.Command != "" {
		m["command"] = srv.Command
	}
	if srv.URL != "" {
		m["url"] = srv.URL
	}
	if len(srv.Args) > 0 {
		m["args"] = srv.Args
	}
	if len(srv.Env) > 0 {
		m["env"] = srv.Env
	}
	if srv.TimeoutSeconds > 0 {
		m["timeout_seconds"] = srv.TimeoutSeconds
	}
	if len(srv.EnabledTools) > 0 {
		m["enabled_tools"] = srv.EnabledTools
	}
	if len(srv.DisabledTools) > 0 {
		m["disabled_tools"] = srv.DisabledTools
	}
	if srv.Disabled {
		m["disabled"] = true
	}
	return m
}

type mcpItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Transport string `json:"transport"`
	Command   string `json:"command"`
	Status    string `json:"status,omitempty"`
	ToolCount int    `json:"toolCount,omitempty"`
}

type mcpListResponse struct {
	Items []mcpItem `json:"items"`
}

type mcpUpsert struct {
	Name           string            `json:"name"`
	Transport      string            `json:"transport"`
	Command        string            `json:"command"`
	URL            string            `json:"url"`
	Args           []string          `json:"args"`
	Env            map[string]string `json:"env"`
	Disabled       bool              `json:"disabled"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

func transportOrStdio(tr string) string {
	if tr == "" {
		return "stdio"
	}
	return tr
}

func (h *Handler) writeMCPList(w http.ResponseWriter) {
	cfg, err := loadConfig()
	if err != nil {
		writeJSON(w, mcpListResponse{Items: []mcpItem{}})
		return
	}
	names := make([]string, 0, len(cfg.MCPServers))
	for n := range cfg.MCPServers {
		names = append(names, n)
	}
	sort.Strings(names)

	type st struct {
		ok    bool
		tools int
	}
	status := map[string]st{}
	if h.mcpReg != nil {
		for _, s := range h.mcpReg.Snapshots() {
			status[s.Name] = st{s.State == mcp.StateConnected, s.ToolCount}
		}
	}

	items := make([]mcpItem, 0, len(names))
	for _, n := range names {
		srv := cfg.MCPServers[n]
		it := mcpItem{ID: n, Name: n, Enabled: !srv.Disabled, Transport: transportOrStdio(srv.Transport), Command: srv.Command}
		if s, ok := status[n]; ok {
			if s.ok {
				it.Status = "connected"
			} else {
				it.Status = "failed"
			}
			it.ToolCount = s.tools
		}
		items = append(items, it)
	}
	writeJSON(w, mcpListResponse{Items: items})
}

// handleMCP implements GET (list) and POST (add/update) on /v1/mcp.
func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.writeMCPList(w)
	case http.MethodPost:
		var u mcpUpsert
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil || strings.TrimSpace(u.Name) == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		cfg, err := loadConfig()
		if err != nil {
			http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		srv := config.MCPServerConfig{
			Transport: u.Transport, Command: u.Command, URL: u.URL, Args: u.Args,
			Env: u.Env, TimeoutSeconds: u.TimeoutSeconds, Disabled: u.Disabled,
		}
		tr := transportOrStdio(u.Transport)
		if tr == "stdio" && strings.TrimSpace(u.Command) == "" {
			http.Error(w, "command is required for stdio transport", http.StatusBadRequest)
			return
		}
		if tr == "sse" && strings.TrimSpace(u.URL) == "" {
			http.Error(w, "url is required for sse transport", http.StatusBadRequest)
			return
		}
		if cfg.MCPServers == nil {
			cfg.MCPServers = map[string]config.MCPServerConfig{}
		}
		cfg.MCPServers[u.Name] = srv
		if err := mutateMCP(u.Name, func(s map[string]any) { s[u.Name] = buildOneMCP(srv) }); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		h.writeMCPList(w)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleMCPByName implements DELETE (remove) and POST (toggle) on /v1/mcp/{name}.
// Toggle patches only the `disabled` key so other raw fields survive.
func (h *Handler) handleMCPByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/mcp/")
	if name == "" || strings.Contains(name, "/") {
		http.Error(w, "bad name", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := mutateMCP(name, func(s map[string]any) { delete(s, name) }); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		h.writeMCPList(w)
	case http.MethodPost:
		var body struct {
			Disabled bool `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := mutateMCP(name, func(s map[string]any) {
			m, _ := s[name].(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			if body.Disabled {
				m["disabled"] = true
			} else {
				delete(m, "disabled")
			}
			s[name] = m
		}); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		h.writeMCPList(w)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
