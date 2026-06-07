package gateway_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
)

// postUpload builds a multipart body with the given name→content file parts
// and POSTs it to /v1/upload.
func postUpload(t *testing.T, ts *httptest.Server, files map[string]string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := http.Post(ts.URL+"/v1/upload", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type uploadResponse struct {
	Files []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"files"`
}

func TestUploadSavesIntoWorkspace(t *testing.T) {
	ts, root := newWorkspaceServer(t)
	r := postUpload(t, ts, map[string]string{"hello.txt": "hi"})
	if r.StatusCode != http.StatusOK {
		t.Fatalf("upload: got %d", r.StatusCode)
	}
	var out uploadResponse
	json.NewDecoder(r.Body).Decode(&out)
	r.Body.Close()
	if len(out.Files) != 1 || out.Files[0].Name != "hello.txt" || out.Files[0].Path != ".deepseek/uploads/hello.txt" {
		t.Fatalf("upload payload = %+v", out)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".deepseek", "uploads", "hello.txt"))
	if err != nil || string(raw) != "hi" {
		t.Fatalf("saved file = %q, %v", raw, err)
	}
}

func TestUploadDedupesCollidingNames(t *testing.T) {
	ts, root := newWorkspaceServer(t)
	for _, r := range []*http.Response{
		postUpload(t, ts, map[string]string{"a.pdf": "one"}),
		postUpload(t, ts, map[string]string{"a.pdf": "two"}),
	} {
		if r.StatusCode != http.StatusOK {
			t.Fatalf("upload: got %d", r.StatusCode)
		}
		r.Body.Close()
	}
	if raw, _ := os.ReadFile(filepath.Join(root, ".deepseek", "uploads", "a.pdf")); string(raw) != "one" {
		t.Fatalf("a.pdf = %q, want untouched first upload", raw)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, ".deepseek", "uploads", "a-1.pdf")); string(raw) != "two" {
		t.Fatalf("a-1.pdf = %q, want second upload under deduped name", raw)
	}
}

func TestUploadSanitizesTraversalNames(t *testing.T) {
	ts, root := newWorkspaceServer(t)
	r := postUpload(t, ts, map[string]string{"../../evil.txt": "x"})
	if r.StatusCode != http.StatusOK {
		t.Fatalf("upload: got %d", r.StatusCode)
	}
	var out uploadResponse
	json.NewDecoder(r.Body).Decode(&out)
	r.Body.Close()
	if len(out.Files) != 1 || out.Files[0].Path != ".deepseek/uploads/evil.txt" {
		t.Fatalf("upload payload = %+v", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".deepseek", "uploads", "evil.txt")); err != nil {
		t.Fatalf("sanitized file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil.txt")); err == nil {
		t.Fatal("traversal escaped the workspace root")
	}
}

func TestUploadRejectsGetAndEmpty(t *testing.T) {
	ts, _ := newWorkspaceServer(t)
	r, _ := http.Get(ts.URL + "/v1/upload")
	if r.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET upload: got %d, want 405", r.StatusCode)
	}
	r.Body.Close()
	// A multipart body with no "file" parts is a 400.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("note", "no files here")
	mw.Close()
	r2, _ := http.Post(ts.URL+"/v1/upload", mw.FormDataContentType(), &buf)
	if r2.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty upload: got %d, want 400", r2.StatusCode)
	}
	r2.Body.Close()
}

// TestChangedHidesUploadsInGitRepo verifies the full loop in a real git repo:
// an upload lands under .deepseek/uploads, gets excluded via .git/info/exclude,
// and /v1/changed never lists it in the review pane.
func TestChangedHidesUploadsInGitRepo(t *testing.T) {
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

	sm := acp.NewSessionManager(stubAgentFactory)
	h := gateway.NewHandlerWithRoot(sm, "", root)
	ts := httptest.NewServer(h)
	defer ts.Close()

	r := postUpload(t, ts, map[string]string{"notes.md": "n"})
	if r.StatusCode != http.StatusOK {
		t.Fatalf("upload: got %d", r.StatusCode)
	}
	r.Body.Close()

	// .git/info/exclude now carries the .deepseek/ line.
	raw, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(raw), ".deepseek/") {
		t.Fatalf("info/exclude = %q, %v", raw, err)
	}

	cr, _ := http.Get(ts.URL + "/v1/changed")
	var out struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	json.NewDecoder(cr.Body).Decode(&out)
	cr.Body.Close()
	for _, e := range out.Entries {
		if strings.HasPrefix(e.Path, ".deepseek") {
			t.Fatalf("changed lists attachment scratch: %+v", out.Entries)
		}
	}
}
