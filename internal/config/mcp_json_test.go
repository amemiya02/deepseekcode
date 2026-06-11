package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDotMCPJSON(t *testing.T) {
	path := filepath.Join("testdata", "dot-mcp.json")
	servers, err := LoadDotMCPJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	cg, ok := servers["codegraph"]
	if !ok {
		t.Fatal("missing codegraph server")
	}
	if cg.Command != "codegraph-mcp" {
		t.Errorf("codegraph command = %q", cg.Command)
	}
	if len(cg.Args) != 1 || cg.Args[0] != "serve" {
		t.Errorf("codegraph args = %v", cg.Args)
	}
	if cg.Env["PORT"] != "9000" {
		t.Errorf("codegraph env PORT = %q", cg.Env["PORT"])
	}

	rem, ok := servers["remote"]
	if !ok {
		t.Fatal("missing remote server")
	}
	if rem.URL != "https://example.test/mcp" {
		t.Errorf("remote url = %q", rem.URL)
	}
	if rem.Transport != "sse" {
		t.Errorf("remote transport = %q", rem.Transport)
	}
}

func TestLoadDotMCPJSONAbsentIsEmpty(t *testing.T) {
	servers, err := LoadDotMCPJSON(filepath.Join("testdata", "does-not-exist.json"))
	if err != nil {
		t.Fatalf("absent file should not error: %v", err)
	}
	if servers != nil {
		t.Errorf("expected nil, got %v", servers)
	}
}
