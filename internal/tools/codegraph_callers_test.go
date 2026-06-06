package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestCodegraphCallersTool_Name(t *testing.T) {
	// Name() never touches the index; use nil for speed.
	tool := tools.NewCodegraphCallersTool(nil)
	if tool.Name() != "codegraph_callers" {
		t.Errorf("Name() = %q, want codegraph_callers", tool.Name())
	}
}

func TestCodegraphCallersTool_IsReadOnly(t *testing.T) {
	tool := tools.NewCodegraphCallersTool(nil)
	if !tool.IsReadOnly() {
		t.Error("codegraph_callers must be read-only")
	}
}

func TestCodegraphCallersTool_Execute(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCallersTool(idx)

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
		t.Errorf("expected Run in callers of Add; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "func") {
		t.Errorf("result content missing kind 'func'; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, ".go:") {
		t.Errorf("result content missing file:line token (e.g. 'simple.go:7'); got: %s", result.Content)
	}
}

func TestCodegraphCallersTool_NotFound(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCallersTool(idx)

	params, _ := json.Marshal(map[string]string{"symbol_id": "github.com/does/not/exist.ZzzzSymbol"})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("no-callers result should not be IsError=true; got: %s", result.Content)
	}
	if result.Content == "" {
		t.Error("no-callers result Content must not be empty")
	}
}

func TestCodegraphCallersTool_MissingParam(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCallersTool(idx)
	params, _ := json.Marshal(map[string]string{})
	result, _ := tool.Execute(context.Background(), params)
	if !result.IsError {
		t.Error("missing symbol_id should return IsError=true")
	}
}
