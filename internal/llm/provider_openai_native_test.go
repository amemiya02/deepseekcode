package llm

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func fixedOpenAINativeRequest() Request {
	temp := 0.0
	return Request{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "system", Blocks: []ContentBlock{TextBlock{Text: "You are a helpful assistant."}}},
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "What is 2+2?"}}},
		},
		MaxTokens:   256,
		Temperature: &temp,
		Stream:      true,
	}
}

func TestOpenAINativeMarshalShape(t *testing.T) {
	req := fixedOpenAINativeRequest()
	b, err := openaiNativeMarshal(req)
	if err != nil {
		t.Fatalf("openaiNativeMarshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// must contain stream:true
	if s, _ := wire["stream"].(bool); !s {
		t.Errorf("expected stream:true, got %v", wire["stream"])
	}

	// must NOT contain reasoning_effort, cache_control, prefix
	for _, forbidden := range []string{"reasoning_effort", "cache_control", "prefix"} {
		if _, has := wire[forbidden]; has {
			t.Errorf("wire must not contain %q", forbidden)
		}
	}

	// must contain messages array
	msgs, ok := wire["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("expected non-empty messages array")
	}

	// system message stays inside messages (OpenAI style)
	first := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("expected first message role=system, got %v", first["role"])
	}

	// max_tokens present
	if wire["max_tokens"] == nil {
		t.Errorf("expected max_tokens")
	}
}

func TestOpenAINativeMarshalGolden(t *testing.T) {
	golden, err := os.ReadFile("testdata/openai_native_marshal_golden.json")
	if err != nil {
		t.Skipf("golden not yet committed: %v", err)
	}

	req := fixedOpenAINativeRequest()
	got, err := openaiNativeMarshal(req)
	if err != nil {
		t.Fatalf("openaiNativeMarshal: %v", err)
	}

	if !bytes.Equal(got, golden) {
		t.Fatalf("OpenAI Native wire bytes changed!\ngot  %d bytes\nwant %d bytes\nfirst diff at byte %d",
			len(got), len(golden), firstDiff(got, golden))
	}
}

func TestOpenAINativeGenGolden(t *testing.T) {
	if os.Getenv("GEN_GOLDEN") == "" {
		t.Skip("set GEN_GOLDEN=1 to regenerate")
	}
	req := fixedOpenAINativeRequest()
	b, err := openaiNativeMarshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/openai_native_marshal_golden.json", b, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d bytes", len(b))
}
