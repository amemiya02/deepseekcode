package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

// extResp mirrors the SPA's ExtensionsSection expectation: {"items":[{id,name,enabled}]}.
type extResp struct {
	Items []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	} `json:"items"`
}

func getExt(t *testing.T, url string) extResp {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200", url, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("%s: content-type = %q, want application/json", url, ct)
	}
	var out extResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s: decode: %v", url, err)
	}
	return out
}

// newExtServer builds a handler rooted at a hermetic temp dir so the skills /
// memory discovery is reproducible, and wires a config seam carrying MCP
// servers + hooks for the config-backed endpoints.
func newExtServer(t *testing.T, root string, cfg config.Config) *httptest.Server {
	t.Helper()
	gateway.SetConfigSeam(func() (config.Config, error) { return cfg, nil }, nil)
	t.Cleanup(func() { gateway.ResetConfigSeam() })
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandlerWithRoot(sm, "", root)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func TestExtensionsMCPListsConfiguredServers(t *testing.T) {
	cfg := config.Default()
	cfg.MCPServers = map[string]config.MCPServerConfig{
		"github":  {Transport: "stdio", Command: "gh-mcp"},
		"weather": {Transport: "sse", URL: "https://example.test/sse"},
	}
	ts := newExtServer(t, t.TempDir(), cfg)

	out := getExt(t, ts.URL+"/v1/mcp")
	if len(out.Items) != 2 {
		t.Fatalf("mcp items = %d, want 2 (%+v)", len(out.Items), out.Items)
	}
	// Deterministic sort by name.
	if out.Items[0].Name != "github" || out.Items[1].Name != "weather" {
		t.Errorf("mcp names = [%q %q], want [github weather]", out.Items[0].Name, out.Items[1].Name)
	}
	for _, it := range out.Items {
		if it.ID == "" || !it.Enabled {
			t.Errorf("mcp item %+v: want non-empty id and enabled", it)
		}
	}
}

func TestExtensionsHooksListsConfiguredHooks(t *testing.T) {
	cfg := config.Default()
	cfg.Hooks = []config.HookItemConfig{
		{Event: "PreToolUse", Type: "subprocess", Command: "/usr/bin/guard"},
		{Event: "PostToolUse", Type: "builtin", Name: "duet"},
	}
	ts := newExtServer(t, t.TempDir(), cfg)

	out := getExt(t, ts.URL+"/v1/hooks")
	if len(out.Items) != 2 {
		t.Fatalf("hooks items = %d, want 2 (%+v)", len(out.Items), out.Items)
	}
	if out.Items[0].Name != "PreToolUse: /usr/bin/guard" {
		t.Errorf("hook[0].name = %q", out.Items[0].Name)
	}
	if out.Items[1].Name != "PostToolUse: duet" {
		t.Errorf("hook[1].name = %q", out.Items[1].Name)
	}
}

func TestExtensionsSkillsListsOnDiskSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // hermetic: ignore the developer's ~/.../skills
	root := t.TempDir()
	skillDir := filepath.Join(root, ".deepseek", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: my-skill\ndescription: A test skill\n---\n# My Skill\nDo a thing.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := newExtServer(t, root, config.Default())

	out := getExt(t, ts.URL+"/v1/skills")
	found := false
	for _, it := range out.Items {
		if it.Name == "my-skill" && it.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("skills items = %+v, want one named my-skill", out.Items)
	}
}

func TestExtensionsMemoryListsStoredRecords(t *testing.T) {
	root := t.TempDir()
	memDir := filepath.Join(root, ".deepseek")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two live records + one deleted; the deleted ID must not appear.
	lines := `{"id":"a","content":"remember the api base url","created_at":"2024-01-01T00:00:00Z"}
{"id":"b","content":"the build uses make build-web","created_at":"2024-01-02T00:00:00Z"}
{"id":"a","content":"remember the api base url","deleted":true,"created_at":"2024-01-03T00:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(memDir, "memory.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := newExtServer(t, root, config.Default())

	out := getExt(t, ts.URL+"/v1/memory")
	if len(out.Items) != 1 {
		t.Fatalf("memory items = %d, want 1 live (%+v)", len(out.Items), out.Items)
	}
	if out.Items[0].ID != "b" || out.Items[0].Name == "" {
		t.Errorf("memory item = %+v, want id=b with a snippet name", out.Items[0])
	}
}

// TestExtensionsNeverNotFound is the regression guard for the Settings 404 bug:
// every Extensions endpoint must return 200 with a non-null items array even
// when no subsystem is configured and no workspace files exist. HOME is
// redirected to an empty temp dir so skill discovery (which also scans the
// user home) is hermetic and yields an empty list.
func TestExtensionsNeverNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ts := newExtServer(t, t.TempDir(), config.Default())
	for _, path := range []string{"/v1/mcp", "/v1/hooks", "/v1/skills", "/v1/memory"} {
		out := getExt(t, ts.URL+path)
		if out.Items == nil {
			t.Errorf("%s: items is null, want [] (non-nil empty slice)", path)
		}
		if len(out.Items) != 0 {
			t.Errorf("%s: items = %d, want 0 for empty config/workspace", path, len(out.Items))
		}
	}
}

// TestExtensionsRejectsNonGET confirms the read-only endpoints reject mutations.
func TestExtensionsRejectsNonGET(t *testing.T) {
	ts := newExtServer(t, t.TempDir(), config.Default())
	for _, path := range []string{"/v1/mcp", "/v1/hooks", "/v1/skills", "/v1/memory"} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: status = %d, want 405", path, resp.StatusCode)
		}
	}
}
