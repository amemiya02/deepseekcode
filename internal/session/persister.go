package session

import (
	"context"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/snapshots"
)

// Persister is the concrete agent.Persister wrapping a Store + a
// snapshots.Manager. Live state for one session.
//
// Construct with NewPersister. Reuse for the lifetime of the session;
// it's safe to call from the agent's goroutine but not from multiple
// goroutines concurrently (database/sql handles its own pooling, but
// the ID/step counters live in this struct).
type Persister struct {
	store  *Store
	snaps  *snapshots.Manager
	sessID string
}

// NewPersister returns a Persister wrapping store + snaps for sessionID.
// snaps may be nil to disable snapshotting.
func NewPersister(store *Store, snaps *snapshots.Manager, sessionID string) *Persister {
	return &Persister{store: store, snaps: snaps, sessID: sessionID}
}

// SessionID returns the session this persister is bound to.
func (p *Persister) SessionID() string { return p.sessID }

// AppendUserMessage records a user turn from a typed block slice.
func (p *Persister) AppendUserMessage(ctx context.Context, blocks []llm.ContentBlock) (int, error) {
	return p.store.AppendMessage(ctx, p.sessID, Message{
		Role:      "user",
		Blocks:    blocks,
		Timestamp: time.Now().UTC(),
	})
}

// AppendAssistant records an assistant turn from a typed block slice
// (Thinking → Text → ToolUse in emission order).
func (p *Persister) AppendAssistant(ctx context.Context, blocks []llm.ContentBlock, model string, usage llm.Usage) (int, error) {
	return p.store.AppendMessage(ctx, p.sessID, Message{
		Role:      "assistant",
		Blocks:    blocks,
		Model:     model,
		Usage:     usage,
		Timestamp: time.Now().UTC(),
	})
}

// AppendToolResult records the result of one tool_use as a single
// ToolResultBlock-bearing tool message.
func (p *Persister) AppendToolResult(ctx context.Context, toolUseID string, content string, isError bool) (int, error) {
	return p.store.AppendMessage(ctx, p.sessID, Message{
		Role: "tool",
		Blocks: []llm.ContentBlock{llm.ToolResultBlock{
			ToolUseID: toolUseID,
			Content:   content,
			IsError:   isError,
		}},
		Timestamp: time.Now().UTC(),
	})
}

// TakeSnapshot snapshots the given paths before a mutating tool runs.
func (p *Persister) TakeSnapshot(stepIdx int, paths []string) (int, error) {
	if p.snaps == nil {
		return 0, nil
	}
	return p.snaps.Take(p.sessID, stepIdx, paths)
}

// SetActiveModel persists a /models switch.
func (p *Persister) SetActiveModel(ctx context.Context, model string) error {
	return p.store.UpdateModel(ctx, p.sessID, model)
}

// Undo restores the last n step snapshots and returns the number of
// files restored. Returns 0 if no snapshots manager is configured.
func (p *Persister) Undo(n int) (int, error) {
	if p.snaps == nil {
		return 0, nil
	}
	return p.snaps.Undo(p.sessID, n)
}

// ReplaceWithCompaction forwards to Store.ReplaceWithCompaction
// scoped to this persister's session.
func (p *Persister) ReplaceWithCompaction(ctx context.Context, fromIdx, toIdx int, summary string) (int, error) {
	return p.store.ReplaceWithCompaction(ctx, p.sessID, fromIdx, toIdx, summary)
}
