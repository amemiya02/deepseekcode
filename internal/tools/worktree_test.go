package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/worktree"
)

// fakeWorktreeManager implements WorktreeManager for testing.
type fakeWorktreeManager struct {
	createCalled bool
	createBranch string
	createRev    string
	createResult worktree.Worktree
	createErr    error

	removeCalled bool
	removeBranch string
	removeForce  bool
	removeErr    error

	listResult []worktree.Worktree
	listErr    error

	pruneCalled bool
	pruneErr    error

	statusCalled bool
	statusBranch string
	statusResult worktree.Worktree
	statusErr    error
}

func (f *fakeWorktreeManager) Create(_ context.Context, branch, baseRev string) (worktree.Worktree, error) {
	f.createCalled = true
	f.createBranch = branch
	f.createRev = baseRev
	return f.createResult, f.createErr
}

func (f *fakeWorktreeManager) Remove(_ context.Context, branch string, force bool) error {
	f.removeCalled = true
	f.removeBranch = branch
	f.removeForce = force
	return f.removeErr
}

func (f *fakeWorktreeManager) List(_ context.Context) ([]worktree.Worktree, error) {
	return f.listResult, f.listErr
}

func (f *fakeWorktreeManager) Prune(_ context.Context) error {
	f.pruneCalled = true
	return f.pruneErr
}

func (f *fakeWorktreeManager) Status(_ context.Context, branch string) (worktree.Worktree, error) {
	f.statusCalled = true
	f.statusBranch = branch
	return f.statusResult, f.statusErr
}

func TestWorktreeToolNameAndReadOnly(t *testing.T) {
	tool := NewWorktreeTool(nil)
	if tool.Name() != "worktree" {
		t.Errorf("Name() = %q, want 'worktree'", tool.Name())
	}
	if tool.IsReadOnly() {
		t.Error("IsReadOnly() should be false")
	}
}

func TestWorktreeToolParameters(t *testing.T) {
	tool := NewWorktreeTool(nil)
	params := tool.Parameters()
	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}
}

func TestWorktreeToolActionRequired(t *testing.T) {
	tool := NewWorktreeTool(&fakeWorktreeManager{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing action")
	}
}

func TestWorktreeToolUnknownAction(t *testing.T) {
	tool := NewWorktreeTool(&fakeWorktreeManager{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"unknown"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for unknown action")
	}
}

func TestWorktreeToolNilManager(t *testing.T) {
	tool := NewWorktreeTool(nil)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for nil manager")
	}
}

func TestWorktreeToolCreate(t *testing.T) {
	fm := &fakeWorktreeManager{
		createResult: worktree.Worktree{
			Branch: "f/x",
			Path:   "/repo/.deepseek/worktrees/f/x",
			Head:   "abc1234",
		},
	}
	tool := NewWorktreeTool(fm)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"create","branch":"f/x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !fm.createCalled {
		t.Fatal("Create not called")
	}
	if fm.createBranch != "f/x" {
		t.Errorf("branch = %q, want f/x", fm.createBranch)
	}
	if fm.createRev != "HEAD" {
		t.Errorf("baseRev = %q, want HEAD", fm.createRev)
	}
	if !strings.Contains(result.Content, "created") {
		t.Errorf("expected 'created' in content, got %q", result.Content)
	}
}

func TestWorktreeToolCreateWithBaseRev(t *testing.T) {
	fm := &fakeWorktreeManager{
		createResult: worktree.Worktree{Branch: "f/y", Path: "/p", Head: "def"},
	}
	tool := NewWorktreeTool(fm)

	result, err := tool.Execute(context.Background(),
		json.RawMessage(`{"action":"create","branch":"f/y","base_rev":"main"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if fm.createRev != "main" {
		t.Errorf("baseRev = %q, want main", fm.createRev)
	}
}

func TestWorktreeToolCreateMissingBranch(t *testing.T) {
	tool := NewWorktreeTool(&fakeWorktreeManager{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"create"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing branch")
	}
}

func TestWorktreeToolList(t *testing.T) {
	fm := &fakeWorktreeManager{
		listResult: []worktree.Worktree{
			{Branch: "main", Path: "/repo", Head: "abc1234", Locked: false},
			{Branch: "feat/x", Path: "/repo/.deepseek/worktrees/feat/x", Head: "def5678", Locked: true},
		},
	}
	tool := NewWorktreeTool(fm)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	// Should have a header row.
	if !strings.Contains(result.Content, "branch") {
		t.Errorf("expected header 'branch' in list output, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "feat/x") {
		t.Errorf("expected 'feat/x' in list output, got %q", result.Content)
	}
}

func TestWorktreeToolRemove(t *testing.T) {
	fm := &fakeWorktreeManager{}
	tool := NewWorktreeTool(fm)

	result, err := tool.Execute(context.Background(),
		json.RawMessage(`{"action":"remove","branch":"feat/x","force":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.removeCalled {
		t.Error("Remove not called")
	}
	if !fm.removeForce {
		t.Error("expected force=true")
	}
	if !strings.Contains(result.Content, "removed") {
		t.Errorf("expected 'removed' in content, got %q", result.Content)
	}
}

func TestWorktreeToolPrune(t *testing.T) {
	fm := &fakeWorktreeManager{}
	tool := NewWorktreeTool(fm)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"prune"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.pruneCalled {
		t.Error("Prune not called")
	}
	if !strings.Contains(result.Content, "pruned") {
		t.Errorf("expected 'pruned' in content, got %q", result.Content)
	}
}

func TestWorktreeToolStatus(t *testing.T) {
	fm := &fakeWorktreeManager{
		statusResult: worktree.Worktree{
			Branch: "feat/x",
			Head:   "abc123",
			Locked: false,
		},
	}
	tool := NewWorktreeTool(fm)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"status","branch":"feat/x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fm.statusCalled {
		t.Error("Status not called")
	}
	if fm.statusBranch != "feat/x" {
		t.Errorf("branch = %q, want feat/x", fm.statusBranch)
	}
	if !strings.Contains(result.Content, "feat/x") {
		t.Errorf("expected 'feat/x' in content, got %q", result.Content)
	}
}

func TestWorktreeToolInvalidJSON(t *testing.T) {
	tool := NewWorktreeTool(&fakeWorktreeManager{})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for invalid JSON")
	}
}
