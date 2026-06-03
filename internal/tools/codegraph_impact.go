package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

// CodegraphImpactTool implements Tool for codegraph_impact.
type CodegraphImpactTool struct {
	idx *codegraph.Index
}

func NewCodegraphImpactTool(idx *codegraph.Index) *CodegraphImpactTool {
	return &CodegraphImpactTool{idx: idx}
}

func (t *CodegraphImpactTool) Name() string { return "codegraph_impact" }

func (t *CodegraphImpactTool) Description() string {
	return "Return all symbols that would be transitively affected if the given symbol changed (reverse call graph BFS)."
}

func (t *CodegraphImpactTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"symbol_id": {"type": "string", "description": "Fully-qualified symbol ID of the symbol being changed"}
		},
		"required": ["symbol_id"]
	}`)
}

func (t *CodegraphImpactTool) IsReadOnly() bool { return true }

func (t *CodegraphImpactTool) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p struct {
		SymbolID string `json:"symbol_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Result{IsError: true, Content: "invalid params: " + err.Error()}, nil
	}
	if p.SymbolID == "" {
		return Result{IsError: true, Content: "symbol_id parameter is required"}, nil
	}
	nodes := t.idx.Impact(codegraph.NodeID(p.SymbolID))
	if len(nodes) == 0 {
		return Result{Content: fmt.Sprintf("no symbols transitively impacted by changes to %q", p.SymbolID)}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Changing `%s` transitively impacts %d symbol(s):\n\n", p.SymbolID, len(nodes)))
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) — %s:%d\n", n.Name, n.Kind.String(), n.File, n.Line))
	}
	return Result{Content: sb.String()}, nil
}
