package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
)

const codegraphNodeMaxBytes = 32 * 1024 // 32 KiB

// CodegraphNodeTool implements Tool for codegraph_node.
// It looks up a single node by its fully-qualified NodeID and returns its
// metadata (kind, file, line) together with the declaration snippet.
type CodegraphNodeTool struct {
	idx *codegraph.Index
}

// NewCodegraphNodeTool creates a CodegraphNodeTool backed by idx.
func NewCodegraphNodeTool(idx *codegraph.Index) *CodegraphNodeTool {
	return &CodegraphNodeTool{idx: idx}
}

func (t *CodegraphNodeTool) Name() string { return "codegraph_node" }

func (t *CodegraphNodeTool) Description() string {
	return "Look up a single symbol by its fully-qualified node ID. Returns kind, file, line, and source snippet."
}

func (t *CodegraphNodeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Fully-qualified node ID, e.g. \"github.com/org/repo/pkg.FuncName\""}
		},
		"required": ["id"]
	}`)
}

func (t *CodegraphNodeTool) IsReadOnly() bool { return true }

func (t *CodegraphNodeTool) Execute(ctx context.Context, params json.RawMessage) (Result, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return Errf("invalid params: %v", err), nil
	}
	if p.ID == "" {
		return Errf("id parameter is required"), nil
	}
	n := t.idx.Store().Node(codegraph.NodeID(p.ID))
	if n == nil {
		return Result{Content: fmt.Sprintf("no node found with id %q", p.ID)}, nil
	}
	content := fmt.Sprintf("# %s (%s)\n\nFile: %s:%d\n\n```go\n%s\n```\n",
		n.Name, n.Kind.String(), n.File, n.Line, n.Snippet)
	return Result{Content: content}.Truncate(codegraphNodeMaxBytes), nil
}
