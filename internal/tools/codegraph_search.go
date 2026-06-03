package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

// CodegraphSearchTool implements Tool for codegraph_search.
type CodegraphSearchTool struct {
	idx *codegraph.Index
}

// NewCodegraphSearchTool creates a CodegraphSearchTool backed by idx.
func NewCodegraphSearchTool(idx *codegraph.Index) *CodegraphSearchTool {
	return &CodegraphSearchTool{idx: idx}
}

func (t *CodegraphSearchTool) Name() string { return "codegraph_search" }

func (t *CodegraphSearchTool) Description() string {
	return "Search the code knowledge graph for symbols by name. Returns matching nodes with kind, file, and line."
}

func (t *CodegraphSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Symbol name to search for (exact or prefix match)"}
		},
		"required": ["name"]
	}`)
}

func (t *CodegraphSearchTool) IsReadOnly() bool { return true }

func (t *CodegraphSearchTool) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Result{IsError: true, Content: "invalid params: " + err.Error()}, nil
	}
	if p.Name == "" {
		return Result{IsError: true, Content: "name parameter is required"}, nil
	}
	nodes := t.idx.Search(p.Name)
	if len(nodes) == 0 {
		return Result{Content: fmt.Sprintf("no symbols found matching %q", p.Name)}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d symbol(s) matching %q:\n\n", len(nodes), p.Name))
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) — %s:%d\n", n.Name, n.Kind.String(), n.File, n.Line))
	}
	return Result{Content: sb.String()}, nil
}
