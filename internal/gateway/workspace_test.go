package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

// newWorkspaceServer builds a handler rooted at a temp dir with one file and
// one subdir, so the workspace-read tests are hermetic.
func newWorkspaceServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandlerWithRoot(sm, "", root) // see Step 3 note on the constructor seam
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, root
}

func TestFilesListsRoot(t *testing.T) {
	ts, _ := newWorkspaceServer(t)
	r, _ := http.Get(ts.URL + "/v1/files")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("files: got %d", r.StatusCode)
	}
	var out struct {
		Entries []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
		} `json:"entries"`
	}
	json.NewDecoder(r.Body).Decode(&out)
	r.Body.Close()
	var sawFile, sawDir bool
	for _, e := range out.Entries {
		if e.Name == "main.go" && !e.IsDir {
			sawFile = true
		}
		if e.Name == "sub" && e.IsDir {
			sawDir = true
		}
	}
	if !sawFile || !sawDir {
		t.Fatalf("entries = %+v, want main.go (file) + sub (dir)", out.Entries)
	}
}

func TestFileReadsContent(t *testing.T) {
	ts, _ := newWorkspaceServer(t)
	r, _ := http.Get(ts.URL + "/v1/file?path=main.go")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("file: got %d", r.StatusCode)
	}
	var out struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Binary    bool   `json:"binary"`
		Truncated bool   `json:"truncated"`
	}
	json.NewDecoder(r.Body).Decode(&out)
	r.Body.Close()
	if out.Content != "package main\n" || out.Binary {
		t.Fatalf("file payload = %+v", out)
	}
}

func TestFileRejectsEscape(t *testing.T) {
	ts, _ := newWorkspaceServer(t)
	// A .. escape must be rejected with 400 — the path-confinement guard.
	r, _ := http.Get(ts.URL + "/v1/file?path=" + url.QueryEscape("../../../etc/passwd"))
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("escape: got %d, want 400", r.StatusCode)
	}
	r.Body.Close()
}

func TestChangedReturnsEntries(t *testing.T) {
	ts, _ := newWorkspaceServer(t)
	r, _ := http.Get(ts.URL + "/v1/changed")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("changed: got %d", r.StatusCode)
	}
	var out struct {
		Entries []struct {
			Path    string `json:"path"`
			Status  string `json:"status"`
			Deleted bool   `json:"deleted"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode changed: %v", err)
	}
	// A pristine (or non-git) temp root yields an empty list, not an error.
	r.Body.Close()
}
