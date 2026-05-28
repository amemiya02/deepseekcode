package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/structsearch"
)

type StructSearchTool struct {
	root string
}

func NewStructSearchTool(root string) *StructSearchTool {
	return &StructSearchTool{root: root}
}

func (*StructSearchTool) Name() string { return "struct_search" }

func (*StructSearchTool) Description() string {
	return "Search source code structurally by symbol kind and name. Use this before text grep when looking for Go functions."
}

func (*StructSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","enum":["function"]},"name":{"type":"string"}},"required":["kind"]}`)
}

func (t *StructSearchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("struct_search: invalid args: %v", err), nil
	}
	results, err := structsearch.Search(t.root, structsearch.Query{Language: "go", Kind: p.Kind, Name: p.Name})
	if err != nil {
		return Result{}, err
	}
	if len(results) == 0 {
		return Result{Content: "no structural matches"}, nil
	}
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "%s:%d %s\n", filepath.Base(r.Path), r.Line, r.Name)
	}
	return Result{Content: b.String()}, nil
}
