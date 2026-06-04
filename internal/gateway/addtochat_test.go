package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

// newAddToChatServer roots a gateway at a temp dir seeded with two files so the
// add-to-chat contents path has something to read.
func newAddToChatServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "util.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sm := acp.NewSessionManager(cpStubFactory) // defined in checkpoint_test.go
	h := gateway.NewHandler(sm, "", gateway.WithWorkspaceRoot(root))
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, root
}

func TestAddToChatFileRef(t *testing.T) {
	ts, _ := newAddToChatServer(t)
	resp, err := http.Post(ts.URL+"/v1/add-to-chat", "application/json",
		strings.NewReader(`{"path":"pkg/util.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Label   string `json:"label"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Label != "pkg/util.go" {
		t.Errorf("label = %q, want pkg/util.go", out.Label)
	}
	if !strings.HasPrefix(out.Content, "@pkg/util.go") {
		t.Errorf("content = %q, want @pkg/util.go prefix", out.Content)
	}
}

func TestAddToChatContents(t *testing.T) {
	ts, _ := newAddToChatServer(t)
	resp, err := http.Post(ts.URL+"/v1/add-to-chat", "application/json",
		strings.NewReader(`{"path":"main.go","include_contents":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Content string `json:"content"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !strings.Contains(out.Content, "package main") {
		t.Errorf("content missing file body: %q", out.Content)
	}
}

func TestAddToChatFolderRef(t *testing.T) {
	ts, _ := newAddToChatServer(t)
	resp, err := http.Post(ts.URL+"/v1/add-to-chat", "application/json",
		strings.NewReader(`{"path":"pkg","is_dir":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Label   string `json:"label"`
		Content string `json:"content"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Label != "pkg/" || !strings.Contains(out.Content, "(folder)") {
		t.Errorf("got label=%q content=%q", out.Label, out.Content)
	}
}

func TestAddToChatSelection(t *testing.T) {
	ts, _ := newAddToChatServer(t)
	resp, err := http.Post(ts.URL+"/v1/add-to-chat", "application/json",
		strings.NewReader(`{"text":"some snippet"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Label   string `json:"label"`
		Content string `json:"content"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Label != "selection" || !strings.Contains(out.Content, "some snippet") {
		t.Errorf("got label=%q content=%q", out.Label, out.Content)
	}
}

func TestAddToChatRejectsEscape(t *testing.T) {
	ts, _ := newAddToChatServer(t)
	resp, err := http.Post(ts.URL+"/v1/add-to-chat", "application/json",
		strings.NewReader(`{"path":"../secret","include_contents":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on path escape, got %d", resp.StatusCode)
	}
}
