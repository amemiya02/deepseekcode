package gateway_test

import (
	"encoding/json"
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

// newGitServer creates a gateway rooted at a git-initialized temp dir with at
// least one commit. Returns the server and the repo root.
func newGitServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	sm := acp.NewSessionManager(cpStubFactory)
	h := gateway.NewHandler(sm, "", gateway.WithWorkspaceRoot(dir))
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, dir
}

// newNoRootServer creates a gateway with no workspace root.
func newNoRootServer(t *testing.T) *httptest.Server {
	t.Helper()
	sm := acp.NewSessionManager(cpStubFactory)
	h := gateway.NewHandler(sm, "")
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func TestHandleBranches(t *testing.T) {
	ts, _ := newGitServer(t)

	resp, err := http.Get(ts.URL + "/v1/git/branches")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out struct {
		Branches []struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
		} `json:"branches"`
		Current string `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Branches) == 0 {
		t.Fatal("expected at least one branch")
	}
	if out.Current == "" {
		t.Fatal("expected current branch to be set")
	}
	found := false
	for _, b := range out.Branches {
		if b.Current && b.Name == out.Current {
			found = true
		}
	}
	if !found {
		t.Fatalf("current branch %q not marked current in list: %+v", out.Current, out.Branches)
	}
}

func TestHandleBranches_NoRoot(t *testing.T) {
	ts := newNoRootServer(t)

	resp, err := http.Get(ts.URL + "/v1/git/branches")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Branches []struct{} `json:"branches"`
		Current  string     `json:"current"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Branches) != 0 {
		t.Fatalf("expected empty branches, got %d", len(out.Branches))
	}
}

func TestHandleCheckout(t *testing.T) {
	ts, dir := newGitServer(t)

	// Create a second branch.
	if out, err := exec.Command("git", "-C", dir, "branch", "feature/test").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v: %s", err, out)
	}

	resp, err := http.Post(ts.URL+"/v1/git/checkout", "application/json",
		strings.NewReader(`{"branch":"feature/test"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Branch  string `json:"branch"`
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("expected success, got error: %s", out.Error)
	}
	if out.Branch != "feature/test" {
		t.Fatalf("expected branch feature/test, got %s", out.Branch)
	}

	// Verify the branch actually switched.
	curOut, err := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(curOut)); got != "feature/test" {
		t.Fatalf("expected git on feature/test, got %s", got)
	}
}

func TestHandleCheckout_BadBranch(t *testing.T) {
	ts, _ := newGitServer(t)

	resp, err := http.Post(ts.URL+"/v1/git/checkout", "application/json",
		strings.NewReader(`{"branch":"nonexistent"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Success {
		t.Fatal("expected failure for nonexistent branch")
	}
	if out.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestHandleCheckout_NoRoot(t *testing.T) {
	ts := newNoRootServer(t)

	resp, err := http.Post(ts.URL+"/v1/git/checkout", "application/json",
		strings.NewReader(`{"branch":"main"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Success {
		t.Fatal("expected failure with no workspace root")
	}
}
