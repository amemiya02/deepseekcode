package gateway

import (
	"reflect"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
)

func TestBuildOneMCPOmitsEmpties(t *testing.T) {
	got := buildOneMCP(config.MCPServerConfig{Command: "codegraph", Args: []string{"mcp"}})
	want := map[string]any{"command": "codegraph", "args": []string{"mcp"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestBuildOneMCPDisabledAndSSE(t *testing.T) {
	got := buildOneMCP(config.MCPServerConfig{Transport: "sse", URL: "http://x", Disabled: true})
	if got["transport"] != "sse" || got["url"] != "http://x" || got["disabled"] != true {
		t.Fatalf("got %#v", got)
	}
	if _, ok := got["command"]; ok {
		t.Error("empty command must be omitted")
	}
}

func TestMutateMCPInMemoryUpsertToggleDelete(t *testing.T) {
	raw := map[string]any{}
	orig := mcpRawIO
	mcpRawIO = rawIO{
		read:  func() (map[string]any, error) { return raw, nil },
		write: func(m map[string]any) error { raw = m; return nil },
	}
	defer func() { mcpRawIO = orig }()

	if err := mutateMCP("cg", func(s map[string]any) { s["cg"] = buildOneMCP(config.MCPServerConfig{Command: "codegraph"}) }); err != nil {
		t.Fatal(err)
	}
	if raw["mcp_servers"].(map[string]any)["cg"].(map[string]any)["command"] != "codegraph" {
		t.Fatalf("upsert failed: %#v", raw)
	}
	if err := mutateMCP("cg", func(s map[string]any) { s["cg"].(map[string]any)["disabled"] = true }); err != nil {
		t.Fatal(err)
	}
	if raw["mcp_servers"].(map[string]any)["cg"].(map[string]any)["disabled"] != true {
		t.Fatal("toggle failed")
	}
	if err := mutateMCP("cg", func(s map[string]any) { delete(s, "cg") }); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["mcp_servers"].(map[string]any)["cg"]; ok {
		t.Fatal("delete failed")
	}
}
