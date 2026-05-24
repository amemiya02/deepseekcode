package mcp

import (
	"context"
	"encoding/json"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// mcpAdaptedTool wraps an MCP tool in the tools.Tool interface so the
// agent can call it alongside built-in tools.
type mcpAdaptedTool struct {
	fullName string
	desc     string
	schema   json.RawMessage
	reg      *Registry
}

func (t *mcpAdaptedTool) Name() string                { return t.fullName }
func (t *mcpAdaptedTool) Description() string         { return t.desc }
func (t *mcpAdaptedTool) Parameters() json.RawMessage { return t.schema }

func (t *mcpAdaptedTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	content, isError, err := t.reg.CallTool(ctx, t.fullName, args)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Content: content, IsError: isError}, nil
}

// BridgeAll returns one tools.Tool per registered MCP tool from
// connected servers. Results are wrapped as mcpAdaptedTool so
// Execute dispatches through the registry.
func BridgeAll(reg *Registry) []tools.Tool {
	metas := reg.Tools()
	out := make([]tools.Tool, len(metas))
	for i, m := range metas {
		out[i] = &mcpAdaptedTool{
			fullName: m.Name,
			desc:     m.Description,
			schema:   m.InputSchema,
			reg:      reg,
		}
	}
	return out
}
