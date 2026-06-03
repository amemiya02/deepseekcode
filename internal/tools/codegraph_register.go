package tools

import "github.com/amemiya02/deepseekcode/internal/codegraph"

// RegisterCodegraphTools registers all four codegraph tools into reg.
// Call this after creating the Index and before starting the agent loop.
func RegisterCodegraphTools(reg *Registry, idx *codegraph.Index) {
	reg.Register(NewCodegraphSearchTool(idx))
	reg.Register(NewCodegraphCallersTool(idx))
	reg.Register(NewCodegraphCalleesTool(idx))
	reg.Register(NewCodegraphImpactTool(idx))
}
