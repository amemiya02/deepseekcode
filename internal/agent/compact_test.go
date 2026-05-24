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

func TestAdjustBoundaryToolPair(t *testing.T) {
	// helper constructors for readable test fixtures.
	use := func(id string) llm.Message {
		return llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{ID: id, Name: "t", Input: json.RawMessage("{}")},
		}}
	}
	result := func(id string) llm.Message {
		return llm.Message{Role: "tool", Blocks: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: id, Content: "out"},
		}}
	}
	text := func(role string) llm.Message {
		return llm.Message{Role: role, Blocks: []llm.ContentBlock{llm.TextBlock{Text: role}}}
	}

	cases := []struct {
		name     string
		messages []llm.Message
		toIdx    int
		want     int
	}{
		{
			"no_tools",
			[]llm.Message{text("user"), text("assistant"), text("user")},
			2,
			2,
		},
		{
			"use_and_result_both_in_window",
			[]llm.Message{text("user"), use("a"), result("a"), text("user"), text("assistant")},
			3,
			3,
		},
		{
			"use_in_window_result_outside",
			[]llm.Message{text("user"), use("a"), text("user"), result("a"), text("assistant")},
			3, // would split — must advance past result at idx 3
			4,
		},
		{
			"nested_multi_turn_pushes_past_both",
			[]llm.Message{
				use("a"),       // 0
				text("user"),   // 1
				use("b"),       // 2
				result("a"),    // 3
				text("user"),   // 4
				result("b"),    // 5
				text("assistant"), // 6
			},
			3, // wants [0,3); but use(b) at idx 2 has result at 5
			6,
		},
		{
			"orphan_use_pulled_back",
			[]llm.Message{
				text("user"), // 0
				use("a"),     // 1 — orphan: no result anywhere
				text("user"), // 2
				text("assistant"), // 3
			},
			3,
			1, // exclude idx 1 from compaction so the orphan stays in tail
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := adjustBoundary(tc.messages, tc.toIdx)
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestCompactSessionFullFlow(t *testing.T) {
	mk := func(n int) []llm.Message {
		out := make([]llm.Message, n)
		for i := range out {
			out[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: strings.Repeat("x", 400)},
			}}
		}
		return out
	}

	t.Run("no_compact_when_below_threshold", func(t *testing.T) {
		got := CompactSession(mk(20), CompactionConfig{
			PreserveRecentMessages: 4, AutoCompactInputTokens: 10_000,
		})
		if got.Summary != "" {
			t.Errorf("expected empty result; got %+v", got)
		}
	})

	t.Run("compacts_above_threshold", func(t *testing.T) {
		msgs := mk(20)
		got := CompactSession(msgs, CompactionConfig{
			PreserveRecentMessages: 4, AutoCompactInputTokens: 100,
		})
		if got.Summary == "" {
			t.Fatal("expected compaction; got empty")
		}
		if got.FromIdx != 0 || got.ToIdx != 16 {
			t.Errorf("window: got [%d,%d), want [0,16)", got.FromIdx, got.ToIdx)
		}
		if got.RemovedCount != 16 {
			t.Errorf("RemovedCount: got %d want 16", got.RemovedCount)
		}
		if len(got.KeptMessages) != 4 {
			t.Errorf("KeptMessages len: got %d want 4", len(got.KeptMessages))
		}
		if got.SummaryMessage.Role != "system" {
			t.Errorf("SummaryMessage role: got %q want system", got.SummaryMessage.Role)
		}
	})
}

func TestSummarizeMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "Please refactor parser.go and add tests."},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ThinkingBlock{Text: "I should also check helpers.go"},
			llm.TextBlock{Text: "Done editing parser.go. TODO: update README.md next."},
			llm.ToolUseBlock{ID: "a", Name: "edit_file", Input: json.RawMessage(`{"path":"parser.go"}`)},
		}},
		{Role: "tool", Blocks: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "a", Content: "wrote 12 lines"},
		}},
	}
	got := summarizeMessages(msgs)
	for _, want := range []string{
		"<summary>",
		"</summary>",
		"- messages: 3 total",
		"- tools_used: [edit_file]",
		"- recent_requests:",
		"- pending_work:",
		"TODO",
		"- key_files: [",
		"parser.go",
		"README.md",
		"helpers.go",
		"- current_work:",
		"- timeline:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestSummarizeMessagesEmpty(t *testing.T) {
	if got := summarizeMessages(nil); got != "" {
		t.Errorf("nil input should produce empty; got %q", got)
	}
}

func TestSummarizeMessagesDeterministic(t *testing.T) {
	msgs := []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ToolUseBlock{ID: "z", Name: "b_tool", Input: json.RawMessage("{}")},
			llm.ToolUseBlock{ID: "y", Name: "a_tool", Input: json.RawMessage("{}")},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "modify a.go and b.go"},
		}},
	}
	first := summarizeMessages(msgs)
	second := summarizeMessages(msgs)
	if first != second {
		t.Errorf("summary not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
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
