package llm

import (
	"bytes"
	"os"
	"testing"
)

// TestDeepSeekMarshalGoldenUnchanged loads the committed golden bytes produced
// by Request.MarshalCacheStable() and asserts the current implementation
// produces byte-identical output for the same fixed input.
//
// If this test fails the frozen wire has drifted — stop and investigate
// before touching any other provider.
func TestDeepSeekMarshalGoldenUnchanged(t *testing.T) {
	t.Setenv("DEEPSEEKCODE_REASONING_DROP", "")
	t.Setenv("DEEPSEEKCODE_REASONING_RETAIN", "")

	golden, err := os.ReadFile("testdata/deepseek_marshal_golden.json")
	if err != nil {
		t.Fatalf("golden fixture missing: %v — run Plan 2 golden-lock step first", err)
	}

	req := fixedDeepSeekRequest() // shared helper defined in this file
	got, err := req.MarshalCacheStable()
	if err != nil {
		t.Fatalf("MarshalCacheStable: %v", err)
	}

	if !bytes.Equal(got, golden) {
		t.Fatalf("DeepSeek wire bytes changed!\ngot  %d bytes\nwant %d bytes\nfirst diff at byte %d",
			len(got), len(golden), firstDiff(got, golden))
	}
}

// fixedDeepSeekRequest returns a deterministic Request used by all golden
// tests.  Fields must never change between commits — add new fields to
// provider-specific structs, not here.
//
// NOTE: Message.Content does not exist in this codebase; the canonical
// representation is Message.Blocks with ContentBlock values.
func fixedDeepSeekRequest() Request {
	return Request{
		Model: "deepseek-v4-flash",
		Messages: []Message{
			{Role: "system", Blocks: []ContentBlock{TextBlock{Text: "You are a helpful assistant."}}},
			{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "What is 2+2?"}}},
		},
		MaxTokens:   256,
		Temperature: func() *float64 { v := 0.0; return &v }(),
		Stream:      true,
	}
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
