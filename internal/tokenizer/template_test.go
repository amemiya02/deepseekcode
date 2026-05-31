package tokenizer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestFormatPrompt_BasicTurn(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}},
	}
	prompt := FormatPrompt(msgs, false)
	if !strings.Contains(prompt, tokBOS) {
		t.Fatal("missing BOS")
	}
	if !strings.Contains(prompt, tokUser) {
		t.Fatal("missing User token")
	}
	if !strings.Contains(prompt, "hi") {
		t.Fatal("missing user content")
	}
	if !strings.Contains(prompt, tokAssistant) {
		t.Fatal("missing Assistant token")
	}
	if !strings.Contains(prompt, tokThinkEnd) {
		t.Fatal("missing ThinkEnd")
	}
}

func TestFormatPrompt_ToolUseDSML(t *testing.T) {
	input := json.RawMessage(`{"path":"a.go"}`)
	msgs := []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{Name: "read_file", Input: input},
		}},
	}
	prompt := FormatPrompt(msgs, false)
	if !strings.Contains(prompt, `<｜DSML｜invoke name="read_file">`) {
		t.Fatalf("missing DSML invoke, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `string="true"`) {
		t.Fatalf("missing string=true for string arg, got:\n%s", prompt)
	}
}

func TestFormatPrompt_ToolResultMerged(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "read it"}}},
		{Role: "tool", Blocks: []llm.ContentBlock{
			llm.ToolResultBlock{Content: "file contents here"},
		}},
	}
	prompt := FormatPrompt(msgs, false)
	if !strings.Contains(prompt, "<tool_result>file contents here</tool_result>") {
		t.Fatalf("missing tool_result, got:\n%s", prompt)
	}
	// Tool result should be merged into the user message (no separate role marker for tool).
	if !strings.Contains(prompt, "read it") {
		t.Fatalf("user content lost, got:\n%s", prompt)
	}
}

func TestFormatPrompt_Empty(t *testing.T) {
	prompt := FormatPrompt(nil, false)
	want := tokAssistant + tokThinkEnd
	if prompt != want {
		t.Fatalf("empty prompt = %q, want %q", prompt, want)
	}
}

func TestEncodeArgsToDsml_ObjectAndArray(t *testing.T) {
	// Object arg → each key becomes a separate DSML parameter with JSON-encoded value.
	// Values must be JSON (not Go fmt.Sprintf("%v") map syntax).
	input := json.RawMessage(`{"deep":true,"n":3}`)
	msgs := []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{Name: "test_tool", Input: input},
		}},
	}
	prompt := FormatPrompt(msgs, false)
	// Go map syntax would produce "map[deep:true n:3]" — must not appear.
	if strings.Contains(prompt, "map[") {
		t.Fatalf("non-string args rendered as Go map syntax, got:\n%s", prompt)
	}
	// Each key must appear as a DSML parameter name.
	if !strings.Contains(prompt, `name="deep"`) {
		t.Fatalf("missing parameter 'deep', got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `name="n"`) {
		t.Fatalf("missing parameter 'n', got:\n%s", prompt)
	}
	// Boolean value must be JSON `true`, not Go `true` (same in this case, but
	// the code path is json.Marshal, not fmt.Sprintf).
	if !strings.Contains(prompt, ">true</") {
		t.Fatalf("missing boolean value 'true', got:\n%s", prompt)
	}

	// Array arg → JSON array string, not Go [x y] syntax.
	inputArr := json.RawMessage(`{"items":["x","y"]}`)
	msgsArr := []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{Name: "test_tool", Input: inputArr},
		}},
	}
	promptArr := FormatPrompt(msgsArr, false)
	// Go slice syntax would produce "[x y]" — must not appear.
	if strings.Contains(promptArr, "[x y]") {
		t.Fatalf("array rendered as Go slice syntax, got:\n%s", promptArr)
	}
	// Must contain the JSON array (string="false" for non-string value).
	if !strings.Contains(promptArr, `["x","y"]`) && !strings.Contains(promptArr, `["x", "y"]`) {
		t.Fatalf("missing JSON array value, got:\n%s", promptArr)
	}
}

func TestEncodeArgsToDsml_Deterministic(t *testing.T) {
	input := json.RawMessage(`{"z":1,"a":2,"m":3}`)
	// Run 10 times — output must be identical every time (sorted keys).
	var first string
	for i := 0; i < 10; i++ {
		msgs := []llm.Message{
			{Role: "assistant", Blocks: []llm.ContentBlock{
				llm.ToolUseBlock{Name: "t", Input: input},
			}},
		}
		prompt := FormatPrompt(msgs, false)
		if i == 0 {
			first = prompt
		} else if prompt != first {
			t.Fatalf("iteration %d produced different output than iteration 0", i)
		}
	}
	// Keys must appear in sorted order.
	idxA := strings.Index(first, `"a"`)
	idxM := strings.Index(first, `"m"`)
	idxZ := strings.Index(first, `"z"`)
	if !(idxA < idxM && idxM < idxZ) {
		t.Fatalf("keys not sorted: a@%d m@%d z@%d", idxA, idxM, idxZ)
	}
}

func TestCountMessages_Available(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "explain how Go goroutines work in detail"}}},
	}
	n, ok := CountMessages(msgs)
	if !ok {
		t.Fatal("CountMessages returned ok=false")
	}
	if n == 0 {
		t.Fatal("CountMessages returned 0")
	}
}
