package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
)

// withFakeMCPIO installs an in-memory raw store + a load seam, returning cleanup.
func withFakeMCPIO(t *testing.T, seed map[string]config.MCPServerConfig) (*map[string]any, func()) {
	t.Helper()
	raw := map[string]any{}
	servers := map[string]any{}
	for n, s := range seed {
		servers[n] = buildOneMCP(s)
	}
	if len(servers) > 0 {
		raw["mcp_servers"] = servers
	}
	origIO := mcpRawIO
	mcpRawIO = rawIO{
		read:  func() (map[string]any, error) { return raw, nil },
		write: func(m map[string]any) error { raw = m; return nil },
	}
	SetConfigSeam(func() (config.Config, error) {
		c := config.Default()
		c.MCPServers = map[string]config.MCPServerConfig{}
		if s, ok := raw["mcp_servers"].(map[string]any); ok {
			for n, v := range s {
				m, _ := v.(map[string]any)
				srv := config.MCPServerConfig{}
				if cmd, ok := m["command"].(string); ok {
					srv.Command = cmd
				}
				if d, ok := m["disabled"].(bool); ok {
					srv.Disabled = d
				}
				if tr, ok := m["transport"].(string); ok {
					srv.Transport = tr
				}
				c.MCPServers[n] = srv
			}
		}
		return c, nil
	}, nil)
	return &raw, func() { mcpRawIO = origIO; ResetConfigSeam() }
}

func TestMCPGetListsConfiguredServers(t *testing.T) {
	_, cleanup := withFakeMCPIO(t, map[string]config.MCPServerConfig{
		"codegraph": {Command: "codegraph", Args: []string{"mcp"}},
	})
	defer cleanup()
	req := httptest.NewRequest(http.MethodGet, "/v1/mcp", nil)
	rec := httptest.NewRecorder()
	NewHandler(nil, "").ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct{ Items []mcpItem `json:"items"` }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Items) != 1 || body.Items[0].Name != "codegraph" || !body.Items[0].Enabled {
		t.Fatalf("items = %#v", body.Items)
	}
}

func TestMCPPostAddsServer(t *testing.T) {
	raw, cleanup := withFakeMCPIO(t, nil)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp", strings.NewReader(`{"name":"cg","transport":"stdio","command":"codegraph","args":["mcp"]}`))
	rec := httptest.NewRecorder()
	NewHandler(nil, "").ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	servers := (*raw)["mcp_servers"].(map[string]any)
	if servers["cg"].(map[string]any)["command"] != "codegraph" {
		t.Fatalf("not persisted: %#v", *raw)
	}
}

func TestMCPPostRejectsEmptyStdioCommand(t *testing.T) {
	_, cleanup := withFakeMCPIO(t, nil)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp", strings.NewReader(`{"name":"bad","transport":"stdio"}`))
	rec := httptest.NewRecorder()
	NewHandler(nil, "").ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestMCPDeleteRemovesServer(t *testing.T) {
	raw, cleanup := withFakeMCPIO(t, map[string]config.MCPServerConfig{"cg": {Command: "codegraph"}})
	defer cleanup()
	req := httptest.NewRequest(http.MethodDelete, "/v1/mcp/cg", nil)
	rec := httptest.NewRecorder()
	NewHandler(nil, "").ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	servers, _ := (*raw)["mcp_servers"].(map[string]any)
	if _, ok := servers["cg"]; ok {
		t.Fatal("server not deleted")
	}
}

func TestMCPToggleDisables(t *testing.T) {
	raw, cleanup := withFakeMCPIO(t, map[string]config.MCPServerConfig{"cg": {Command: "codegraph"}})
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/cg", strings.NewReader(`{"disabled":true}`))
	rec := httptest.NewRecorder()
	NewHandler(nil, "").ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	servers := (*raw)["mcp_servers"].(map[string]any)
	if servers["cg"].(map[string]any)["disabled"] != true {
		t.Fatalf("not toggled: %#v", *raw)
	}
}
