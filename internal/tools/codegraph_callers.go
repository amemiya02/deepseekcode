package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

// CodegraphCallersTool implements Tool for codegraph_callers.
type CodegraphCallersTool struct {
	idx *codegraph.Index
}

func NewCodegraphCallersTool(idx *codegraph.Index) *CodegraphCallersTool {
	return &CodegraphCallersTool{idx: idx}
}

func (t *CodegraphCallersTool) Name() string { return "codegraph_callers" }

func (t *CodegraphCallersTool) Description() string {
	return "Return all symbols that directly call the given symbol (by fully-qualified ID)."
}

func (t *CodegraphCallersTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"symbol_id": {"type": "string", "description": "Fully-qualified symbol ID, e.g. github.com/org/repo/pkg.FuncName"}
		},
		"required": ["symbol_id"]
	}`)
}

func (t *CodegraphCallersTool) IsReadOnly() bool { return true }

func (t *CodegraphCallersTool) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p struct {
		SymbolID string `json:"symbol_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Errf("invalid params: %v", err), nil
	}
	if p.SymbolID == "" {
		return Errf("symbol_id parameter is required"), nil
	}
	nodes := t.idx.Callers(codegraph.NodeID(p.SymbolID))
	if len(nodes) == 0 {
		return Result{Content: fmt.Sprintf("no callers found for %q", p.SymbolID)}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d caller(s) of `%s`:\n\n", len(nodes), p.SymbolID))
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) — %s:%d\n", n.Name, n.Kind.String(), n.File, n.Line))
	}
	return Result{Content: sb.String()}, nil
}
