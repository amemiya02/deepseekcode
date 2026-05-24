package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// canonicalize ToolUseBlock.Input bytes before reflect.DeepEqual so
// equivalent JSON with different whitespace doesn't false-negative.
func canonicalize(blocks []ContentBlock) []ContentBlock {
	out := make([]ContentBlock, len(blocks))
	for i, b := range blocks {
		if tu, ok := b.(ToolUseBlock); ok {
			var buf bytes.Buffer
			if err := json.Compact(&buf, tu.Input); err == nil {
				tu.Input = json.RawMessage(buf.Bytes())
			}
			out[i] = tu
			continue
		}
		out[i] = b
	}
	return out
}

func TestMarshalBlocks(t *testing.T) {
	cases := []struct {
		name   string
		blocks []ContentBlock
	}{
		{
			"text_only",
			[]ContentBlock{TextBlock{Text: "hi"}},
		},
		{
			"thinking_only",
			[]ContentBlock{ThinkingBlock{Text: "let me think"}},
		},
		{
			"tool_use_only",
			[]ContentBlock{ToolUseBlock{ID: "t1", Name: "bash", Input: json.RawMessage(`{"command":"ls"}`)}},
		},
		{
			"tool_result_only",
			[]ContentBlock{ToolResultBlock{ToolUseID: "t1", Content: "ok", IsError: false}},
		},
		{
			"tool_result_error",
			[]ContentBlock{ToolResultBlock{ToolUseID: "t2", Content: "boom", IsError: true}},
		},
		{
			"mixed",
			[]ContentBlock{
				ThinkingBlock{Text: "plan"},
				TextBlock{Text: "ack"},
				ToolUseBlock{ID: "a", Name: "read", Input: json.RawMessage(`{"path":"/x"}`)},
				ToolResultBlock{ToolUseID: "a", Content: "data"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := MarshalBlocks(tc.blocks)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got, err := UnmarshalBlocks(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(canonicalize(tc.blocks), canonicalize(got)) {
				t.Fatalf("roundtrip mismatch:\nwant %#v\ngot  %#v", tc.blocks, got)
			}
		})
	}
}

func TestUnmarshalBlocksUnknownKind(t *testing.T) {
	_, err := UnmarshalBlocks([]byte(`[{"kind":"banana","text":"x"}]`))
	if err == nil {
		t.Fatal("expected error on unknown kind, got nil")
	}
	if !strings.Contains(err.Error(), "unknown block kind") {
		t.Errorf("error should mention %q; got %v", "unknown block kind", err)
	}
}

func TestUnmarshalBlocksEmpty(t *testing.T) {
	cases := []string{"null", "[]", ""}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got, err := UnmarshalBlocks([]byte(in))
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if got != nil {
				t.Errorf("want nil, got %#v", got)
			}
		})
	}
}

func TestUnmarshalBlocksMissingKind(t *testing.T) {
	_, err := UnmarshalBlocks([]byte(`[{"text":"x"}]`))
	if err == nil {
		t.Fatal("expected error on missing kind")
	}
}
