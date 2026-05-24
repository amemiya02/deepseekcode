package llm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// TestMarshalCacheStableByteCompat is load-bearing: any drift between
// legacy-field and Blocks-form serialization invalidates DeepSeek's
// prompt cache (50–120× cost multiplier). The two representations
// must produce byte-identical MarshalCacheStable output.
func TestMarshalCacheStableByteCompat(t *testing.T) {
	mkLegacy := func() Request {
		return Request{
			Model: "deepseek-v4-flash",
			Messages: []Message{
				{Role: "user", Content: "hi"},
				{
					Role:             "assistant",
					Content:          "hello",
					ReasoningContent: "ponder",
					ToolCalls: []ToolCall{{
						ID:       "x",
						Type:     "function",
						Function: ToolCallFunc{Name: "bash", Arguments: `{"command":"ls"}`},
					}},
				},
				{Role: "tool", ToolCallID: "x", Content: "file1\nfile2\n"},
			},
		}
	}
	mkBlocks := func() Request {
		return Request{
			Model: "deepseek-v4-flash",
			Messages: []Message{
				{Role: "user", Blocks: []ContentBlock{TextBlock{Text: "hi"}}},
				{Role: "assistant", Blocks: []ContentBlock{
					ThinkingBlock{Text: "ponder"},
					TextBlock{Text: "hello"},
					ToolUseBlock{ID: "x", Name: "bash", Input: json.RawMessage(`{"command":"ls"}`)},
				}},
				{Role: "tool", Blocks: []ContentBlock{
					ToolResultBlock{ToolUseID: "x", Content: "file1\nfile2\n"},
				}},
			},
		}
	}

	a, err := mkLegacy().MarshalCacheStable()
	if err != nil {
		t.Fatalf("legacy marshal: %v", err)
	}
	b, err := mkBlocks().MarshalCacheStable()
	if err != nil {
		t.Fatalf("blocks marshal: %v", err)
	}

	if !bytes.Equal(a, b) {
		t.Errorf("legacy vs blocks bytes differ:\nlegacy: %s\nblocks: %s", a, b)
	}

	ha := sha256.Sum256(a)
	hb := sha256.Sum256(b)
	if ha != hb {
		t.Errorf("SHA-256 mismatch:\nlegacy: %s\nblocks: %s", hex.EncodeToString(ha[:]), hex.EncodeToString(hb[:]))
	}
}
