package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		name string
		in   []llm.Message
		want int // approximate; we allow ±5 in the strict-value subcase
	}{
		{
			"nil",
			nil,
			0,
		},
		{
			"empty_message",
			[]llm.Message{{Role: "user"}},
			perMessageOverhead,
		},
		{
			"single_text_100_chars",
			[]llm.Message{{Role: "user", Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: strings.Repeat("x", 100)},
			}}},
			perMessageOverhead + 25,
		},
		{
			"tool_use_counts_name_and_input",
			[]llm.Message{{Role: "assistant", Blocks: []llm.ContentBlock{
				llm.ToolUseBlock{ID: "a", Name: strings.Repeat("n", 8), Input: json.RawMessage(strings.Repeat("i", 16))},
			}}},
			perMessageOverhead + 6, // (8+16)/4 = 6
		},
		{
			"multi_message_sums",
			[]llm.Message{
				{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("a", 40)}}},
				{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("b", 40)}}},
			},
			2*perMessageOverhead + 10 + 10,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateTokens(tc.in)
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestEstimateTokensApproxFor100Chars(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: strings.Repeat("x", 100)},
	}}}
	got := EstimateTokens(msgs)
	// Want ≈ 25 ± 5 tolerance (with overhead the budget is ~30).
	if got < 20 || got > 40 {
		t.Errorf("100-char text estimated %d tokens; expected ~25 ± tolerance", got)
	}
}
