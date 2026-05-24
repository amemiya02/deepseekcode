package agent

import (
	"context"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Persister abstracts the session/snapshot bookkeeping the agent needs.
// internal/session.Persister and internal/snapshots.Manager satisfy
// this. Keeping the interface here lets the agent stay decoupled.
type Persister interface {
	// SessionID is the session this agent is associated with.
	SessionID() string

	// AppendUserMessage records a user turn as a typed block slice
	// (typically one TextBlock; multimodal extensions later).
	AppendUserMessage(ctx context.Context, blocks []llm.ContentBlock) (int, error)

	// AppendAssistant records an assistant turn as a typed block slice
	// (Thinking, Text, ToolUse — order matches model emission).
	AppendAssistant(ctx context.Context, blocks []llm.ContentBlock, model string, usage llm.Usage) (int, error)

	// AppendToolResult records the result of one tool_use, identified
	// by toolUseID. isError true marks an infrastructure failure so
	// renderers can color it.
	AppendToolResult(ctx context.Context, toolUseID string, content string, isError bool) (int, error)

	// TakeSnapshot snapshots the given paths before a mutating tool runs.
	// stepIdx is the message index of the assistant turn that contained
	// the tool call; the snapshot manager uses it to namespace files on
	// disk so /undo can revert a specific step.
	TakeSnapshot(stepIdx int, paths []string) (int, error)

	// SetActiveModel persists the result of a /models switch.
	SetActiveModel(ctx context.Context, model string) error

	// ReplaceWithCompaction atomically deletes messages in [fromIdx,
	// toIdx) and inserts a synthetic summary message at fromIdx,
	// renumbering subsequent messages to keep idx contiguous. Returns
	// the idx of the inserted summary. The full transactional
	// implementation lands in Phase 2 (T-208); for now this method
	// exists so the rest of the system can compile against the final
	// interface shape.
	ReplaceWithCompaction(ctx context.Context, fromIdx, toIdx int, summary string) (int, error)
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
