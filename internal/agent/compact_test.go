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

func TestShouldCompact(t *testing.T) {
	// Helper: build n messages, each roughly tokensPer tokens worth.
	mk := func(n, tokensPer int) []llm.Message {
		out := make([]llm.Message, n)
		for i := range out {
			out[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: strings.Repeat("x", tokensPer*4)},
			}}
		}
		return out
	}

	t.Run("below_threshold", func(t *testing.T) {
		cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 10_000}
		ok, _, _ := ShouldCompact(mk(20, 100), cfg)
		if ok {
			t.Error("expected no compaction below threshold")
		}
	})

	t.Run("above_threshold_picks_window", func(t *testing.T) {
		cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 100}
		ok, from, to := ShouldCompact(mk(20, 100), cfg)
		if !ok {
			t.Fatal("expected compaction above threshold")
		}
		if from != 0 {
			t.Errorf("fromIdx: got %d want 0", from)
		}
		if to != 16 {
			t.Errorf("toIdx: got %d want 16 (len=20, preserve=4)", to)
		}
	})

	t.Run("too_short", func(t *testing.T) {
		cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 1}
		ok, _, _ := ShouldCompact(mk(6, 1000), cfg)
		if ok {
			t.Error("expected no compaction when len ≤ preserve*2")
		}
	})

	t.Run("zero_preserve_defaults_to_4", func(t *testing.T) {
		cfg := CompactionConfig{AutoCompactInputTokens: 100}
		ok, _, to := ShouldCompact(mk(20, 100), cfg)
		if !ok {
			t.Fatal("expected compaction")
		}
		if to != 16 {
			t.Errorf("expected default preserve=4 → toIdx=16; got %d", to)
		}
	})

	t.Run("zero_threshold_disables", func(t *testing.T) {
		cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 0}
		ok, _, _ := ShouldCompact(mk(20, 1000), cfg)
		if ok {
			t.Error("zero threshold should disable compaction")
		}
	})
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
