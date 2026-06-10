package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestArchiveCompactedMessagesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{ID: "t1", Name: "read_file", Input: json.RawMessage(`{"path":"a.go"}`)},
		}},
		{Role: "tool", Blocks: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "t1", Content: "package a"},
		}},
	}
	path, err := archiveCompactedMessages(dir, "sess-1", msgs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1+len(msgs) { // meta line + one line per message
		t.Fatalf("want %d lines, got %d", 1+len(msgs), len(lines))
	}
	if !strings.Contains(lines[0], `"removed":3`) || !strings.Contains(lines[2], "read_file") {
		t.Fatalf("archive content wrong:\n%s", raw)
	}
}

func TestArchiveCompactedMessagesCreatesDir(t *testing.T) {
	dir := t.TempDir()
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "test"}}},
	}
	path, err := archiveCompactedMessages(dir, "new-session", msgs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("archived file should exist: %v", err)
	}
}

func TestArchiveCompactedMessagesEmpty(t *testing.T) {
	dir := t.TempDir()
	msgs := []llm.Message{}
	path, err := archiveCompactedMessages(dir, "sess-empty", msgs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 { // only the meta line
		t.Fatalf("want 1 line for empty archive, got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"removed":0`) {
		t.Fatalf("meta line should say removed:0, got %s", lines[0])
	}
}

func TestArchiveCompactedMessagesAllBlockTypes(t *testing.T) {
	dir := t.TempDir()
	msgs := []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ThinkingBlock{Text: "reasoning about this"},
			llm.TextBlock{Text: "here is the answer"},
		}},
	}
	path, err := archiveCompactedMessages(dir, "sess-blocks", msgs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	content := string(raw)
	if !strings.Contains(content, "thinking") || !strings.Contains(content, "reasoning about this") {
		t.Fatalf("thinking block not archived correctly:\n%s", content)
	}
	if !strings.Contains(content, "here is the answer") {
		t.Fatalf("text block not archived correctly:\n%s", content)
	}
}
