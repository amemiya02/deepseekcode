package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func toolFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "testdata", "fixtures", "simple")
}

func buildToolIndex(t *testing.T) *codegraph.Index {
	t.Helper()
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	if err := idx.Rebuild(toolFixtureDir(t)); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return idx
}

// TestCodegraphSearchTool_Name and TestCodegraphSearchTool_IsReadOnly test
// constant-returning methods; no index walk needed.
func TestCodegraphSearchTool_Name(t *testing.T) {
	tool := tools.NewCodegraphSearchTool(nil)
	if tool.Name() != "codegraph_search" {
		t.Errorf("Name() = %q, want codegraph_search", tool.Name())
	}
}

func TestCodegraphSearchTool_IsReadOnly(t *testing.T) {
	tool := tools.NewCodegraphSearchTool(nil)
	if !tool.IsReadOnly() {
		t.Error("codegraph_search must be read-only")
	}
}

func TestCodegraphSearchTool_Execute(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphSearchTool(idx)

	params, _ := json.Marshal(map[string]string{"name": "Add"})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Add") {
		t.Errorf("result content does not mention Add; got: %s", result.Content)
	}
	// Assert that kind ("func") and a file:line token are present so that a
	// broken output format does not silently pass.
	if !strings.Contains(result.Content, "func") {
		t.Errorf("result content missing kind 'func'; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, ".go:") {
		t.Errorf("result content missing file:line token (e.g. 'util.go:3'); got: %s", result.Content)
	}
}

func TestCodegraphSearchTool_ExecuteMissingParam(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphSearchTool(idx)

	params, _ := json.Marshal(map[string]string{})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("Execute with missing name param should return IsError=true")
	}
}

// TestCodegraphSearchTool_ExecuteNotFound verifies the no-results path returns
// a non-error result (not IsError=true) with informative content.
func TestCodegraphSearchTool_ExecuteNotFound(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphSearchTool(idx)

	params, _ := json.Marshal(map[string]string{"name": "ZzzzDoesNotExistXxx"})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("no-match result should not be IsError=true; got: %s", result.Content)
	}
	if result.Content == "" {
		t.Error("no-match result Content must not be empty")
	}
}
