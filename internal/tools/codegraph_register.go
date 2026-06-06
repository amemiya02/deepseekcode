package tools

import "github.com/amemiya02/deepseekcode/internal/codegraph"

// RegisterCodegraphTools registers all five codegraph tools into reg as
// TierLazy, matching the pattern used by other index-backed on-demand tools
// (e.g. struct_search). Call this after creating the Index and before
// starting the agent loop.
func RegisterCodegraphTools(reg *Registry, idx *codegraph.Index) {
	reg.RegisterWithTier(NewCodegraphSearchTool(idx), TierLazy)
	reg.RegisterWithTier(NewCodegraphCallersTool(idx), TierLazy)
	reg.RegisterWithTier(NewCodegraphCalleesTool(idx), TierLazy)
	reg.RegisterWithTier(NewCodegraphImpactTool(idx), TierLazy)
	reg.RegisterWithTier(NewCodegraphNodeTool(idx), TierLazy)
}
