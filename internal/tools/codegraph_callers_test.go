package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestCodegraphCallersTool_Name(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCallersTool(idx)
	if tool.Name() != "codegraph_callers" {
		t.Errorf("Name() = %q, want codegraph_callers", tool.Name())
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
	if !strings.Contains(result.Content, "Run") {
		t.Errorf("expected Run in callers of Add; got: %s", result.Content)
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
