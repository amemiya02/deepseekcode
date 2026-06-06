package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestCodegraphCalleesTool_Name(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCalleesTool(idx)
	if tool.Name() != "codegraph_callees" {
		t.Errorf("Name() = %q, want codegraph_callees", tool.Name())
	}
}

func TestCodegraphCalleesTool_Execute(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCalleesTool(idx)

	symID := "github.com/amemiya02/deepseekcode/testdata/fixtures/simple.Run"
	params, _ := json.Marshal(map[string]string{"symbol_id": symID})
	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Add") {
		t.Errorf("expected Add in callees of Run; got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Multiply") {
		t.Errorf("expected Multiply in callees of Run; got: %s", result.Content)
	}
}

func TestCodegraphCalleesTool_MissingParam(t *testing.T) {
	idx := buildToolIndex(t)
	tool := tools.NewCodegraphCalleesTool(idx)
	params, _ := json.Marshal(map[string]string{})
	result, _ := tool.Execute(context.Background(), params)
	if !result.IsError {
		t.Error("missing symbol_id should return IsError=true")
	}
}
