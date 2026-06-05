package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestDiffReturnsPatch(t *testing.T) {
	root := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "main.go")
	runGit("commit", "-m", "init")
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)

	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandlerWithRoot(sm, "", root)
	ts := httptest.NewServer(h)
	defer ts.Close()

	r, _ := http.Get(ts.URL + "/v1/diff?path=main.go")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("diff: got %d", r.StatusCode)
	}
	var out struct {
		Path  string `json:"path"`
		Patch string `json:"patch"`
	}
	json.NewDecoder(r.Body).Decode(&out)
	r.Body.Close()
	if !strings.Contains(out.Patch, "func main") || !strings.Contains(out.Patch, "@@") {
		t.Fatalf("patch missing change: %q", out.Patch)
	}
}

func TestDiffRejectsEscape(t *testing.T) {
	ts, _ := newWorkspaceServer(t)
	r, _ := http.Get(ts.URL + "/v1/diff?path=" + url.QueryEscape("../../../etc/passwd"))
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("escape: got %d, want 400", r.StatusCode)
	}
	r.Body.Close()
}

func TestDiffNonGitRootIsEmpty(t *testing.T) {
	ts, _ := newWorkspaceServer(t) // temp root, NOT a git repo
	r, _ := http.Get(ts.URL + "/v1/diff?path=main.go")
	if r.StatusCode != http.StatusOK {
		t.Fatalf("non-git diff: got %d, want 200", r.StatusCode)
	}
	var out struct {
		Patch string `json:"patch"`
	}
	json.NewDecoder(r.Body).Decode(&out)
	r.Body.Close()
	if out.Patch != "" {
		t.Fatalf("non-git patch = %q, want empty", out.Patch)
	}
}
