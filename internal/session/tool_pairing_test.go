package session

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestRepairDanglingToolCalls(t *testing.T) {
	user := Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}}
	asst := func(ids ...string) Message {
		blocks := []llm.ContentBlock{llm.ThinkingBlock{Text: "t"}}
		for _, id := range ids {
			blocks = append(blocks, llm.ToolUseBlock{ID: id, Name: "echo", Input: json.RawMessage("{}")})
		}
		return Message{Role: "assistant", Blocks: blocks}
	}
	toolRes := func(id string) Message {
		return Message{Role: "tool", ToolCallID: id,
			Blocks: []llm.ContentBlock{llm.ToolResultBlock{ToolUseID: id, Content: "ok"}}}
	}

	t.Run("fully paired is returned unchanged", func(t *testing.T) {
		in := []Message{user, asst("c1"), toolRes("c1")}
		out := repairDanglingToolCalls(in)
		if len(out) != len(in) {
			t.Fatalf("paired history changed length: %d -> %d", len(in), len(out))
		}
	})

	t.Run("dangling call at end gets a synthetic error result", func(t *testing.T) {
		in := []Message{user, asst("c1")}
		out := repairDanglingToolCalls(in)
		if len(out) != 3 {
			t.Fatalf("want 3 messages after repair, got %d", len(out))
		}
		last := out[2]
		if last.Role != "tool" || last.ToolCallID != "c1" {
			t.Fatalf("synthetic result missing/wrong: %+v", last)
		}
		tr, ok := last.Blocks[0].(llm.ToolResultBlock)
		if !ok || tr.ToolUseID != "c1" || !tr.IsError {
			t.Fatalf("synthetic ToolResultBlock wrong: %+v", last.Blocks)
		}
	})

	t.Run("partial pairing fills only the missing call", func(t *testing.T) {
		in := []Message{user, asst("c1", "c2"), toolRes("c1")}
		out := repairDanglingToolCalls(in)
		if len(out) != 4 {
			t.Fatalf("want 4 messages (synthesize only c2), got %d", len(out))
		}
		var c2Synth bool
		for _, m := range out {
			if m.Role == "tool" && m.ToolCallID == "c2" {
				c2Synth = true
			}
		}
		if !c2Synth {
			t.Fatal("missing call c2 was not repaired")
		}
	})

	t.Run("no tool calls is untouched", func(t *testing.T) {
		in := []Message{user, {Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}}}
		if out := repairDanglingToolCalls(in); len(out) != len(in) {
			t.Fatalf("plain text history changed: %d -> %d", len(in), len(out))
		}
	})
}

// TestReplayRepairsInterruptedToolCall pins the crash-recovery path end to
// end: an assistant tool_call turn persisted without its result (process
// interrupted) replays with a synthesized result so the next DeepSeek request
// is well-formed.
func TestReplayRepairsInterruptedToolCall(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	sess, err := store.NewSession(ctx, "/proj", "deepseek-v4-flash", false)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	p := NewPersister(store, nil, sess.ID)
	if _, err := p.AppendUserMessage(ctx, []llm.ContentBlock{llm.TextBlock{Text: "do it"}}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if _, err := p.AppendAssistant(ctx, []llm.ContentBlock{
		llm.ThinkingBlock{Text: "plan"},
		llm.ToolUseBlock{ID: "c1", Name: "echo", Input: json.RawMessage("{}")},
	}, "deepseek-v4-flash", llm.Usage{}); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	// Interrupted here — the tool result is never persisted.

	msgs, err := store.Replay(ctx, sess.ID)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("no messages replayed")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "tool" || last.ToolCallID != "c1" {
		t.Fatalf("Replay did not repair the dangling tool_call; last message = %+v", last)
	}
}
