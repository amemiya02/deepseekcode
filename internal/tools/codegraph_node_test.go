package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestCodegraphNodeTool_Name and TestCodegraphNodeTool_IsReadOnly test
// constant-returning methods; no index walk needed.
func TestCodegraphNodeTool_Name(t *testing.T) {
	tool := tools.NewCodegraphNodeTool(nil)
	if tool.Name() != "codegraph_node" {
		t.Errorf("Name() = %q, want codegraph_node", tool.Name())
	}
}

func TestCodegraphNodeTool_IsReadOnly(t *testing.T) {
	tool := tools.NewCodegraphNodeTool(nil)
	if !tool.IsReadOnly() {
		t.Error("codegraph_node must be read-only")
	}
}

func TestCodegraphNodeTool_Execute(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphNodeTool(idx)

	// The fixture pkgPath is "github.com/amemiya02/deepseekcode/testdata/fixtures/simple"
	// and Add is a top-level func, so its NodeID is pkgPath + ".Add".
	id := "github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add"
	params, _ := json.Marshal(map[string]string{"id": id})
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
	// Assert kind and file:line are present so broken output formats are detected.
	if !strings.Contains(result.Content, "func") {
		t.Errorf("result content missing kind 'func'; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, ".go:") {
		t.Errorf("result content missing file:line token; got: %s", result.Content)
	}
}

func TestCodegraphNodeTool_ExecuteMissingID(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphNodeTool(idx)

	params, _ := json.Marshal(map[string]string{})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("Execute with missing id param should return IsError=true")
	}
	if result.Content == "" {
		t.Error("error result Content must not be empty")
	}
}

// TestCodegraphNodeTool_ExecuteNotFound verifies that a nonexistent ID returns
// a non-error result with informative content (consistent with codegraph_search).
func TestCodegraphNodeTool_ExecuteNotFound(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphNodeTool(idx)

	params, _ := json.Marshal(map[string]string{"id": "github.com/nonexistent/pkg.ZzzzGhost"})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("not-found result should not be IsError=true; got: %s", result.Content)
	}
	if result.Content == "" {
		t.Error("not-found result Content must not be empty")
	}
}
