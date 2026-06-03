package llm

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// fixedAnthropicRequest returns the canonical fixed request used by all
// Anthropic golden tests. Adapted from the plan spec to use Message.Blocks
// (the real codebase representation) and *float64 for Temperature.
func fixedAnthropicRequest() Request {
	temp := 0.0
	return Request{
		Model: "claude-sonnet-4-5",
		Messages: []Message{
			{Role: "system", Blocks: []ContentBlock{TextBlock{Text: "You are a helpful assistant."}}},
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "What is 2+2?"}}},
			{Role: "assistant", Blocks: []ContentBlock{TextBlock{Text: "The answer is 4."}}},
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "Explain why."}}},
		},
		MaxTokens:   256,
		Temperature: &temp,
		Stream:      true,
	}
}

func TestAnthropicMarshalShape(t *testing.T) {
	req := fixedAnthropicRequest()
	b, err := anthropicMarshal(req)
	if err != nil {
		t.Fatalf("anthropicMarshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// system must be a top-level array, not inside messages
	sys, ok := wire["system"].([]any)
	if !ok || len(sys) == 0 {
		t.Fatalf("expected top-level system array, got %T %v", wire["system"], wire["system"])
	}

	// messages must not contain a system turn
	msgs, ok := wire["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages array")
	}
	for i, m := range msgs {
		mm := m.(map[string]any)
		if mm["role"] == "system" {
			t.Errorf("messages[%d] has role=system; system must be top-level", i)
		}
	}

	// cache_control must appear on the last system block
	lastSys := sys[len(sys)-1].(map[string]any)
	if lastSys["cache_control"] == nil {
		t.Errorf("last system block missing cache_control")
	}

	// cache_control must appear on the last human turn's last content block
	lastMsg := msgs[len(msgs)-1].(map[string]any)
	switch c := lastMsg["content"].(type) {
	case []any:
		if len(c) == 0 {
			t.Errorf("last user message content slice is empty")
			break
		}
		lastBlock := c[len(c)-1].(map[string]any)
		if lastBlock["cache_control"] == nil {
			t.Errorf("last user message content block missing cache_control")
		}
	case string:
		t.Errorf("expected content block array for last user turn, got string")
	}

	// must not contain stream field (Anthropic passes it via header)
	if _, has := wire["stream"]; has {
		t.Errorf("wire must not contain 'stream' key for Anthropic")
	}

	// max_tokens must be present and correct
	mt, ok := wire["max_tokens"].(float64)
	if !ok || int(mt) != req.MaxTokens {
		t.Errorf("max_tokens = %v, want %d", wire["max_tokens"], req.MaxTokens)
	}
}

func TestAnthropicMarshalGolden(t *testing.T) {
	golden, err := os.ReadFile("testdata/anthropic_marshal_golden.json")
	if err != nil {
		t.Fatalf("golden fixture missing: %v — run GEN_GOLDEN=1 go test -run TestAnthropicGenGolden to regenerate", err)
	}

	req := fixedAnthropicRequest()
	got, err := anthropicMarshal(req)
	if err != nil {
		t.Fatalf("anthropicMarshal: %v", err)
	}

	if !bytes.Equal(got, golden) {
		t.Fatalf("Anthropic wire bytes changed!\ngot  %d bytes\nwant %d bytes\nfirst diff at byte %d",
			len(got), len(golden), firstDiff(got, golden))
	}
}

func TestAnthropicGenGolden(t *testing.T) {
	if os.Getenv("GEN_GOLDEN") == "" {
		t.Skip("set GEN_GOLDEN=1 to regenerate")
	}
	req := fixedAnthropicRequest()
	b, err := anthropicMarshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/anthropic_marshal_golden.json", b, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes to testdata/anthropic_marshal_golden.json", len(b))
}
