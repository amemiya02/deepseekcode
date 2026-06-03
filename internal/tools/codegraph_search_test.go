package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

func TestCodegraphSearchTool_Name(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphSearchTool(idx)
	if tool.Name() != "codegraph_search" {
		t.Errorf("Name() = %q, want codegraph_search", tool.Name())
	}
}

func TestCodegraphSearchTool_IsReadOnly(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphSearchTool(idx)
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
	if !containsString(result.Content, "Add") {
		t.Errorf("result content does not mention Add; got: %s", result.Content)
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

func containsString(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || len(s) >= len(sub) && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = os.DevNull // suppress unused import
