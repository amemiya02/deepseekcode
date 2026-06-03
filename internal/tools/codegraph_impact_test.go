package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestCodegraphImpactTool_Name(t *testing.T) {
	// Name() never touches the index; use nil for speed.
	tool := tools.NewCodegraphImpactTool(nil)
	if tool.Name() != "codegraph_impact" {
		t.Errorf("Name() = %q, want codegraph_impact", tool.Name())
	}
}

func TestCodegraphImpactTool_Execute(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphImpactTool(idx)

	symID := "github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Add"
	params, _ := json.Marshal(map[string]string{"symbol_id": symID})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error: %s", result.Content)
	}
	// Structural assertions: symbol name, kind, and file:line token must all appear.
	if !strings.Contains(result.Content, "Run") {
		t.Errorf("expected Run in impact set of Add; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "func") {
		t.Errorf("result content missing kind 'func'; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, ".go:") {
		t.Errorf("result content missing file:line token (e.g. 'simple.go:7'); got: %s", result.Content)
	}
}

func TestCodegraphImpactTool_MissingParam(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphImpactTool(idx)
	params, _ := json.Marshal(map[string]string{})
	result, _ := tool.Execute(context.Background(), params)
	if !result.IsError {
		t.Error("missing symbol_id should return IsError=true")
	}
}

func TestCodegraphImpactTool_IsReadOnly(t *testing.T) {
	tool := tools.NewCodegraphImpactTool(nil)
	if !tool.IsReadOnly() {
		t.Error("codegraph_impact must be read-only")
	}
}
