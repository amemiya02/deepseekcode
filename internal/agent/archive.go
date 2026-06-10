// archive.go persists compaction-dropped messages to disk before the
// live transcript replaces them (spec §2.3) — recoverable, best-effort,
// never blocks a turn.
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

type archivedBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     string `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type archivedMessage struct {
	Role   string          `json:"role"`
	Blocks []archivedBlock `json:"blocks"`
}

// archiveCompactedMessages writes one meta line + one JSONL line per
// dropped message into dir/<label>/compaction-<timestamp>.jsonl and
// returns the path. Best-effort: callers log, never fail the turn.
func archiveCompactedMessages(dir, label string, msgs []llm.Message) (string, error) {
	sub := filepath.Join(dir, label)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(sub, "compaction-"+time.Now().UTC().Format("2006-01-02T15-04-05.000000000Z")+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(map[string]any{"removed": len(msgs), "archived_at": time.Now().UTC().Format(time.RFC3339)}); err != nil {
		return "", err
	}
	for _, m := range msgs {
		am := archivedMessage{Role: m.Role}
		for _, b := range m.Blocks {
			switch v := b.(type) {
			case llm.TextBlock:
				am.Blocks = append(am.Blocks, archivedBlock{Type: "text", Text: v.Text})
			case llm.ThinkingBlock:
				am.Blocks = append(am.Blocks, archivedBlock{Type: "thinking", Text: v.Text})
			case llm.ToolUseBlock:
				am.Blocks = append(am.Blocks, archivedBlock{Type: "tool_use", ID: v.ID, Name: v.Name, Input: string(v.Input)})
			case llm.ToolResultBlock:
				am.Blocks = append(am.Blocks, archivedBlock{Type: "tool_result", ToolUseID: v.ToolUseID, Content: v.Content})
			}
		}
		if err := enc.Encode(am); err != nil {
			return "", err
		}
	}
	return path, nil
}
