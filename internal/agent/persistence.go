package agent

import (
	"context"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Persister abstracts the session/snapshot bookkeeping the agent needs.
// internal/session.Store and internal/snapshots.Manager satisfy this
// through the SessionPersister type in cmd/dsc/main.go (or a test
// double). Keeping the interface here lets the agent stay decoupled.
type Persister interface {
	// SessionID is the session this agent is associated with.
	SessionID() string

	// AppendUserMessage records a user turn.
	AppendUserMessage(ctx context.Context, content string) (int, error)

	// AppendAssistant records an assistant turn (text + reasoning + tool_calls).
	AppendAssistant(ctx context.Context, content, reasoning string, toolCalls []llm.ToolCall, model string, usage llm.Usage) (int, error)

	// AppendToolResult records a tool-result turn.
	AppendToolResult(ctx context.Context, toolCallID, content string) (int, error)

	// TakeSnapshot snapshots the given paths before a mutating tool runs.
	// stepIdx is the message index of the assistant turn that contained
	// the tool call; the snapshot manager uses it to namespace files on
	// disk so /undo can revert a specific step.
	TakeSnapshot(stepIdx int, paths []string) (int, error)

	// SetActiveModel persists the result of a /models switch.
	SetActiveModel(ctx context.Context, model string) error
}

// AffectedPathsFor returns the static affected paths for a tool call
// (used by the snapshot manager). Bash returns nil since its effects
// are unknown statically; the destructive-bash check + permission
// prompt are the safety net there.
func AffectedPathsFor(reg *tools.Registry, call llm.ToolCall) []string {
	tool, ok := reg.Get(call.Function.Name)
	if !ok {
		return nil
	}
	if pa, ok := tool.(tools.PathAware); ok {
		return pa.AffectedPaths([]byte(call.Function.Arguments))
	}
	return nil
}
