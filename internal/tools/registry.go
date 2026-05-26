// Package tools implements deepseekcode's built-in tool surface.
//
// The 12 v0.1 tools live here. MCP-provided tools are bridged into the
// same Tool interface by internal/mcp so callers see one homogeneous
// registry.
//
// Schema serialization is deterministic (sorted JSON-Schema keys), which
// matters because every byte of tool JSON ends up in the prompt prefix
// and stable bytes are what makes the API-side prompt cache fire.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// DefaultMaxResultBytes is the upper bound on tool-result Content length
// fed back to the model. Results exceeding this are truncated by the
// agent loop so that a single tool call cannot flood the context window.
const DefaultMaxResultBytes = 50000

// Tool is the abstract built-in or MCP tool. Implementations are
// expected to be safe to call from arbitrary goroutines.
type Tool interface {
	// Name is the function-call identifier the model uses.
	Name() string

	// Description is a one- to four-sentence English summary. Goes into
	// the prompt; keep it specific.
	Description() string

	// Parameters returns the JSON-Schema describing the tool's args.
	// The bytes do not need to be canonical — the registry canonicalizes
	// during AsLLMTools().
	Parameters() json.RawMessage

	// Execute runs the tool. args is the model's JSON arguments. The
	// returned Result is shown to the model; any non-nil error indicates
	// infrastructure failure the agent should surface (and may retry).
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

// ReadOnlyHint is an optional interface tools may implement to declare
// they never mutate state. The permissions package uses this to
// short-circuit approval prompts; tools that don't implement it default
// to "not read-only" (safe default).
type ReadOnlyHint interface {
	IsReadOnly() bool
}

// PathAware is an optional interface tools may implement to declare
// which filesystem paths their invocation will read or write. The
// snapshot manager and permissions checker both consult this. Returning
// nil means "no paths known statically" — bash falls into this category.
type PathAware interface {
	AffectedPaths(args json.RawMessage) []string
}

// Registry is the set of tools available to the agent for a session.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register installs a tool. Later registrations of the same name
// overwrite earlier ones; intentional, so MCP can shadow a built-in
// if a user wants.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the named tool or (nil, false).
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools in name-sorted order. Sorting is
// load-bearing: the LLM request layer serializes tools in this order,
// and stable order maximizes prompt-cache hits across turns.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, len(names))
	for i, n := range names {
		out[i] = r.tools[n]
	}
	return out
}

// AsLLMTools converts the registry contents into the llm.Tool slice
// the client sends to DeepSeek. Schemas are *not* canonicalized here;
// request.go's MarshalCacheStable handles that uniformly.
func (r *Registry) AsLLMTools() []llm.Tool {
	all := r.All()
	out := make([]llm.Tool, len(all))
	for i, t := range all {
		out[i] = llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		}
	}
	return out
}

// Subset returns a new Registry containing only the tools whose names
// appear in names. Unknown names are silently ignored.
// names being nil or empty returns an empty Registry (not the full set).
func (r *Registry) Subset(names []string) *Registry {
	sub := New()
	for _, n := range names {
		if t, ok := r.Get(n); ok {
			sub.Register(t)
		}
	}
	return sub
}

// CloneForCWD returns a new Registry where every cwd-aware tool has been
// cloned with its working directory rebound to cwd. Tools without a cwd
// dependency are shared with the original registry.
func (r *Registry) CloneForCWD(cwd string) *Registry {
	cloned := New()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, t := range r.tools {
		cloned.tools[name] = CloneForCWD(t, cwd)
	}
	return cloned
}

// MustParams marshals v to JSON and panics on error. Tools call this
// from their Parameters() method on a Go struct literal, which can't
// fail in practice. Keeps each tool definition readable.
func MustParams(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("tools: marshalling parameters: %v", err))
	}
	return b
}
