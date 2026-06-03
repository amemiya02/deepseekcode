package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

const (
	codegraphCalleesMaxNodes = 50
	codegraphCalleesMaxBytes = 32 * 1024 // 32 KiB
)

// CodegraphCalleesTool implements Tool for codegraph_callees.
type CodegraphCalleesTool struct {
	idx *codegraph.Index
}

func NewCodegraphCalleesTool(idx *codegraph.Index) *CodegraphCalleesTool {
	return &CodegraphCalleesTool{idx: idx}
}

func (t *CodegraphCalleesTool) Name() string { return "codegraph_callees" }

func (t *CodegraphCalleesTool) Description() string {
	return "Return all symbols that the given symbol directly calls (by fully-qualified ID)."
}

func (t *CodegraphCalleesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"symbol_id": {"type": "string", "description": "Fully-qualified symbol ID, e.g. github.com/org/repo/pkg.FuncName"}
		},
		"required": ["symbol_id"]
	}`)
}

func (t *CodegraphCalleesTool) IsReadOnly() bool { return true }

func (t *CodegraphCalleesTool) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p struct {
		SymbolID string `json:"symbol_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Errf("invalid params: %v", err), nil
	}
	if p.SymbolID == "" {
		return Errf("symbol_id parameter is required"), nil
	}
	nodes := t.idx.Callees(codegraph.NodeID(p.SymbolID))
	if len(nodes) == 0 {
		return Result{Content: fmt.Sprintf("no callees found for %q", p.SymbolID)}, nil
	}
	// Cap to avoid unbounded output for symbols with many callees.
	capped := nodes
	if len(capped) > codegraphCalleesMaxNodes {
		capped = capped[:codegraphCalleesMaxNodes]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("`%s` calls %d symbol(s):\n\n", p.SymbolID, len(nodes)))
	for _, n := range capped {
		sb.WriteString(fmt.Sprintf("- **%s** (%s) — %s:%d\n", n.Name, n.Kind.String(), n.File, n.Line))
	}
	if len(nodes) > codegraphCalleesMaxNodes {
		sb.WriteString(fmt.Sprintf("\n[showing first %d of %d callees]\n", codegraphCalleesMaxNodes, len(nodes)))
	}
	return Result{Content: sb.String()}.Truncate(codegraphCalleesMaxBytes), nil
}
