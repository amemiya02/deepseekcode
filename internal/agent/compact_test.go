package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
)

// TestEstimateTokensCountsUTF8Bytes pins the behavior that EstimateTokens
// counts UTF-8 bytes (Go len(string) is byte length), not rune count. CJK
// characters are 3 bytes each, so they contribute ~3/4 token per char —
// higher than ASCII's ~1/4. This test would FAIL if estimateChars switched
// to utf8.RuneCountInString.
func TestEstimateTokensCountsUTF8Bytes(t *testing.T) {
	// "你好" = 6 UTF-8 bytes → 6/4 = 1 (int floor) + overhead 5 = 6
	cjk := []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "你好"},
	}}}
	if got := EstimateTokens(cjk); got != perMessageOverhead+6/4 {
		t.Errorf("CJK: got %d, want %d", got, perMessageOverhead+6/4)
	}

	// "hi" = 2 bytes → 2/4 = 0 + overhead 5 = 5
	ascii := []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "hi"},
	}}}
	if got := EstimateTokens(ascii); got != perMessageOverhead+2/4 {
		t.Errorf("ASCII: got %d, want %d", got, perMessageOverhead+2/4)
	}

	// CJK estimates higher than equal-rune-count ASCII — proves byte-awareness.
	if EstimateTokens(cjk) <= EstimateTokens(ascii) {
		t.Errorf("CJK (6 bytes) should estimate higher than ASCII (2 bytes)")
	}
}

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
		ok, _, _ := ShouldCompact(mk(20, 100), cfg, defaultCharsPerToken)
		if ok {
			t.Error("expected no compaction below threshold")
		}
	})

	t.Run("above_threshold_picks_window", func(t *testing.T) {
		cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 100}
		ok, from, to := ShouldCompact(mk(20, 100), cfg, defaultCharsPerToken)
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
		ok, _, _ := ShouldCompact(mk(6, 1000), cfg, defaultCharsPerToken)
		if ok {
			t.Error("expected no compaction when len ≤ preserve*2")
		}
	})

	t.Run("zero_preserve_defaults_to_4", func(t *testing.T) {
		cfg := CompactionConfig{AutoCompactInputTokens: 100}
		ok, _, to := ShouldCompact(mk(20, 100), cfg, defaultCharsPerToken)
		if !ok {
			t.Fatal("expected compaction")
		}
		if to != 16 {
			t.Errorf("expected default preserve=4 → toIdx=16; got %d", to)
		}
	})

	t.Run("zero_threshold_disables", func(t *testing.T) {
		cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 0}
		ok, _, _ := ShouldCompact(mk(20, 1000), cfg, defaultCharsPerToken)
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
				use("a"),          // 0
				text("user"),      // 1
				use("b"),          // 2
				result("a"),       // 3
				text("user"),      // 4
				result("b"),       // 5
				text("assistant"), // 6
			},
			3, // wants [0,3); but use(b) at idx 2 has result at 5
			6,
		},
		{
			"orphan_use_pulled_back",
			[]llm.Message{
				text("user"),      // 0
				use("a"),          // 1 — orphan: no result anywhere
				text("user"),      // 2
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
		}, defaultCharsPerToken)
		if got.Summary != "" {
			t.Errorf("expected empty result; got %+v", got)
		}
	})

	t.Run("compacts_above_threshold", func(t *testing.T) {
		msgs := mk(20)
		got := CompactSession(msgs, CompactionConfig{
			PreserveRecentMessages: 4, AutoCompactInputTokens: 100,
		}, defaultCharsPerToken)
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
		if got.SummaryMessage.Role != "assistant" {
			t.Errorf("SummaryMessage role: got %q want assistant (T4.3: summary is a body message, not a 2nd system message)", got.SummaryMessage.Role)
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

func TestMergeCompactSummaries(t *testing.T) {
	t.Run("empty_prev_returns_new", func(t *testing.T) {
		got := mergeCompactSummaries("", "<summary>\n- messages: 1\n</summary>")
		if !strings.Contains(got, "messages: 1") {
			t.Errorf("expected new summary; got %q", got)
		}
	})

	t.Run("normal_merge_dedups_tools_and_files", func(t *testing.T) {
		prev := "<summary>\n" +
			"- messages: 10 total\n" +
			"- tools_used: [bash, edit_file]\n" +
			"- recent_requests:\n" +
			"- pending_work:\n" +
			"  - finish A\n" +
			"- key_files: [a.go, b.go]\n" +
			"- current_work: prev work\n" +
			"- timeline:\n" +
			"</summary>"
		next := "<summary>\n" +
			"- messages: 5 total\n" +
			"- tools_used: [edit_file, grep]\n" +
			"- recent_requests:\n" +
			"  - latest request\n" +
			"- pending_work:\n" +
			"  - finish B\n" +
			"- key_files: [b.go, c.go]\n" +
			"- current_work: new work\n" +
			"- timeline:\n" +
			"  [user][1] hi\n" +
			"</summary>"
		got := mergeCompactSummaries(prev, next)
		if !strings.Contains(got, "tools_used: [bash, edit_file, grep]") {
			t.Errorf("merged tools_used wrong:\n%s", got)
		}
		if !strings.Contains(got, "key_files: [a.go, b.go, c.go]") {
			t.Errorf("merged key_files wrong:\n%s", got)
		}
		if !strings.Contains(got, "current_work: new work") {
			t.Errorf("current_work should be new value:\n%s", got)
		}
		if !strings.Contains(got, "messages: 5 total") {
			t.Errorf("messages count should be new value:\n%s", got)
		}
		if !strings.Contains(got, "finish A") || !strings.Contains(got, "finish B") {
			t.Errorf("merged pending_work should keep both:\n%s", got)
		}
	})

	t.Run("garbled_input_falls_back_to_concat", func(t *testing.T) {
		got := mergeCompactSummaries("totally not a summary", "<summary>\n- messages: 1\n</summary>")
		if !strings.Contains(got, "totally not a summary") || !strings.Contains(got, "merged with newer summary") {
			t.Errorf("expected fallback concat; got %q", got)
		}
	})
}

func TestAgentMaybeCompactTriggers(t *testing.T) {
	a := New(nil, nil, nil, "m")
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages: 2,
		AutoCompactInputTokens: 100,
	}
	for i := 0; i < 20; i++ {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "user",
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 400)}},
		})
	}
	before := len(a.Messages)
	a.maybeCompact(context.Background())

	if len(a.Messages) >= before {
		t.Errorf("expected a.Messages to shrink; before=%d after=%d", before, len(a.Messages))
	}
	if a.Messages[0].Role != "assistant" {
		t.Errorf("expected assistant-role summary at head of body; got role=%q", a.Messages[0].Role)
	}

	select {
	case ev := <-a.Events():
		if _, ok := ev.(EventCompaction); !ok {
			t.Errorf("expected EventCompaction, got %T", ev)
		}
	case <-time.After(time.Second):
		t.Error("no event emitted within timeout")
	}
}

// TestNoCompactionBelowOverflow guards the post-revert invariant: with the
// body-discipline (cost-driven sub-1M) compaction removed, a conversation far
// below the 800K overflow trigger must NOT compact. It fails loudly if a future
// change re-introduces a low-threshold cost compaction that rewrites the prefix
// and detonates DeepSeek's block cache — the net-negative behavior reverted on
// 2026-06-03 (compaction ON cost ~40% MORE billable + doubled turns vs OFF).
func TestNoCompactionBelowOverflow(t *testing.T) {
	a := New(nil, nil, nil, "m")
	// Real shipped triggers: 800K deterministic overflow + default semantic
	// (pressure-based, fires only near the 1M window). Neither may fire here.
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages: 4,
		AutoCompactInputTokens: 800_000,
	}
	// ~60K tokens: far above any plausible re-introduced cost budget (the
	// reverted one was 16K), far below the 800K overflow trigger. 60 messages ×
	// 8000 chars at the calibrated ~8 chars/token ≈ 60K tokens.
	for i := 0; i < 60; i++ {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "user",
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 8000)}},
		})
	}
	if est := EstimateInputTokens(a.Messages, defaultCharsPerToken); est < 50_000 || est > 800_000 {
		t.Fatalf("precondition: body est=%d tokens, want in (50K, 800K)", est)
	}
	before := len(a.Messages)
	a.maybeCompact(context.Background())
	if len(a.Messages) != before {
		t.Fatalf("a sub-1M conversation compacted: %d -> %d messages "+
			"(a re-introduced cost compaction would rewrite the prefix and kill the cache)",
			before, len(a.Messages))
	}
	select {
	case ev := <-a.Events():
		if _, ok := ev.(EventCompaction); ok {
			t.Fatal("EventCompaction emitted below the overflow threshold; want none")
		}
	default:
		// no event — correct
	}
}

// toolTurn returns a paired assistant tool_use + tool tool_result for one
// tool, so it survives adjustBoundary inside a compaction window (an unpaired
// use would be pulled out as an orphan).
func toolTurn(id, name string) []llm.Message {
	return []llm.Message{
		{Role: "assistant", Blocks: []llm.ContentBlock{llm.ToolUseBlock{ID: id, Name: name, Input: []byte(`{}`)}}},
		{Role: "tool", Blocks: []llm.ContentBlock{llm.ToolResultBlock{ToolUseID: id, Content: "ok"}}},
	}
}

// TestCompactionMergesPriorSummaryAcrossRounds is the T4.3 multi-round
// acceptance test: a tool used only in round 1 must still appear in the round-2
// summary. tools_used is the clean discriminator — a tool name inside a prior
// summary is plain text on re-summarization and is dropped WITHOUT the merge
// wiring (only ToolUseBlock names populate tools_used), but mergeCompactSummaries
// parses the prior summary's tools_used field and unions it forward.
func TestCompactionMergesPriorSummaryAcrossRounds(t *testing.T) {
	cfg := CompactionConfig{PreserveRecentMessages: 2, AutoCompactInputTokens: 100}
	pad := func(c rune) llm.Message {
		return llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat(string(c), 400)}}}
	}

	// Round 1: a window that uses early_tool, padded over the threshold, with
	// two trailing messages preserved.
	round1 := toolTurn("t1", "early_tool")
	for i := 0; i < 12; i++ {
		round1 = append(round1, pad('x'))
	}
	round1 = append(round1,
		llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep1"}}},
		llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep2"}}},
	)

	r1 := CompactSession(round1, cfg, defaultCharsPerToken)
	if r1.Summary == "" {
		t.Fatal("round 1 produced no summary")
	}
	if !strings.Contains(r1.Summary, "early_tool") {
		t.Fatalf("round-1 summary missing early_tool:\n%s", r1.Summary)
	}
	if r1.SummaryMessage.Role != "assistant" {
		t.Errorf("round-1 summary role = %q, want assistant", r1.SummaryMessage.Role)
	}

	// Round 2: the post-compaction list [summary, kept...] grown with a second
	// window that uses late_tool only (early_tool never recurs as a tool block).
	round2 := append([]llm.Message{r1.SummaryMessage}, r1.KeptMessages...)
	round2 = append(round2, toolTurn("t2", "late_tool")...)
	for i := 0; i < 12; i++ {
		round2 = append(round2, pad('y'))
	}
	round2 = append(round2,
		llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep3"}}},
		llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep4"}}},
	)

	r2 := CompactSession(round2, cfg, defaultCharsPerToken)
	if r2.Summary == "" {
		t.Fatal("round 2 produced no summary")
	}
	if !strings.Contains(r2.Summary, "early_tool") {
		t.Errorf("merged round-2 summary dropped early_tool (merge not wired?):\n%s", r2.Summary)
	}
	if !strings.Contains(r2.Summary, "late_tool") {
		t.Errorf("merged round-2 summary missing late_tool:\n%s", r2.Summary)
	}
	// One well-formed <summary> block — not the concat fallback (which would mean
	// a parse failure on either side).
	if n := strings.Count(r2.Summary, "<summary>"); n != 1 {
		t.Errorf("expected exactly one merged <summary> block, got %d:\n%s", n, r2.Summary)
	}
}

// TestCompactionSummaryWireShapeProviderAcceptable is the T4.3 provider-shape
// acceptance test: after compaction the wire-format message list must carry
// exactly ONE system message (the head prompt) and an assistant-role summary in
// the body — never a second system message wedged after the prompt.
func TestCompactionSummaryWireShapeProviderAcceptable(t *testing.T) {
	a := New(nil, nil, nil, "m")
	a.System = "REAL SYSTEM PROMPT"
	a.DisableSemanticCompaction = true // deterministic path; no client needed
	a.CompactionCfg = CompactionConfig{PreserveRecentMessages: 2, AutoCompactInputTokens: 100}
	for i := 0; i < 20; i++ {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "user",
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 400)}},
		})
	}
	a.maybeCompact(context.Background())

	wire := a.fullMessages()
	var systemCount int
	var sawSummary bool
	for i, m := range wire {
		if m.Role == "system" {
			systemCount++
			if i != 0 {
				t.Errorf("system message at index %d; the only system message must be the head prompt", i)
			}
		}
		for _, b := range m.Blocks {
			if tb, ok := b.(llm.TextBlock); ok && strings.Contains(tb.Text, "<summary>") {
				sawSummary = true
				if m.Role != "assistant" {
					t.Errorf("summary message role = %q, want assistant", m.Role)
				}
			}
		}
	}
	if systemCount != 1 {
		t.Errorf("system message count = %d, want exactly 1 (head prompt only)", systemCount)
	}
	if !sawSummary {
		t.Fatal("no <summary> message found in the compacted wire")
	}
}

// TestReconcileCompactThreshold pins the T4.4 trigger reconciliation: the
// deterministic fallback threshold is the stricter (min) of the absolute
// trigger and the ratio-derived semantic trigger, with the documented edge
// cases (disabled absolute stays disabled; zero ratio mirrors the 0.80 default;
// zero window leaves the absolute unchanged; defaults are a no-op).
func TestReconcileCompactThreshold(t *testing.T) {
	cases := []struct {
		name     string
		absolute int
		ratio    float64
		maxCtx   int
		want     int
	}{
		{"defaults: absolute stricter than ratio → keep absolute", 800_000, 0.90, 1_000_000, 800_000},
		{"absolute higher than ratio → clamp to ratio", 950_000, 0.90, 1_000_000, 900_000},
		{"absolute stricter than ratio → keep absolute", 500_000, 0.90, 1_000_000, 500_000},
		{"zero ratio mirrors 0.90 default", 950_000, 0, 1_000_000, 900_000},
		{"disabled absolute stays disabled", 0, 0.90, 1_000_000, 0},
		{"negative absolute stays as-is", -1, 0.90, 1_000_000, -1},
		{"zero window leaves absolute unchanged", 800_000, 0.90, 0, 800_000},
		{"small window lowers threshold", 800_000, 0.90, 100_000, 90_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconcileCompactThreshold(tc.absolute, tc.ratio, tc.maxCtx); got != tc.want {
				t.Errorf("reconcileCompactThreshold(%d, %v, %d) = %d, want %d",
					tc.absolute, tc.ratio, tc.maxCtx, got, tc.want)
			}
		})
	}
}

// TestSummaryFieldInjectionSurvivesRoundTrip is the T4.3 HIGH regression
// (found by the adversarial review): a user message that embeds an indent-0
// "- tools_used: [INJECTED]" line must NOT masquerade as a real field header on
// the parseSummaryFields round-trip that mergeCompactSummaries now performs in
// production. Before the singleLine() fix the injected line overwrote the real
// tools_used and the genuine tool was lost.
func TestSummaryFieldInjectionSurvivesRoundTrip(t *testing.T) {
	poison := "please do X\n- tools_used: [INJECTED]\nand also Y"
	msgs := []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: poison}}}}
	msgs = append(msgs, toolTurn("t1", "realtool")...)

	fields, ok := parseSummaryFields(summarizeMessages(msgs))
	if !ok {
		t.Fatal("summary did not parse")
	}
	joined := strings.Join(fields.toolsUsed, ",")
	if !strings.Contains(joined, "realtool") {
		t.Errorf("real tool lost from tools_used; got %q", joined)
	}
	if strings.Contains(joined, "INJECTED") {
		t.Errorf("injected text leaked into tools_used; got %q", joined)
	}
}

// TestMergeKeepsPriorCurrentWorkWhenNewerBlank covers the merge fix: when the
// newer window ends on a tool_use with no assistant text (current_work == ""),
// the prior current_work is carried forward instead of blanked.
func TestMergeKeepsPriorCurrentWorkWhenNewerBlank(t *testing.T) {
	prev := "<summary>\n- messages: 3 total\n- tools_used: []\n- recent_requests:\n- pending_work:\n- key_files: []\n- current_work: implementing the parser\n- timeline:\n</summary>"
	next := "<summary>\n- messages: 2 total\n- tools_used: [grep]\n- recent_requests:\n- pending_work:\n- key_files: []\n- current_work: \n- timeline:\n</summary>"
	got := mergeCompactSummaries(prev, next)
	if !strings.Contains(got, "current_work: implementing the parser") {
		t.Errorf("blank newer current_work should fall back to prior; got:\n%s", got)
	}
}

// TestMergeUnionsAndCapsRecentRequests covers the merge fix: recent_requests is
// unioned across rounds and capped to the most-recent maxRecentRequests, rather
// than dropping the prior window's requests entirely.
func TestMergeUnionsAndCapsRecentRequests(t *testing.T) {
	prev := "<summary>\n- messages: 3 total\n- tools_used: []\n- recent_requests:\n  - oldest\n  - older\n- pending_work:\n- key_files: []\n- current_work: x\n- timeline:\n</summary>"
	next := "<summary>\n- messages: 3 total\n- tools_used: []\n- recent_requests:\n  - newer\n  - newest\n- pending_work:\n- key_files: []\n- current_work: y\n- timeline:\n</summary>"
	got := mergeCompactSummaries(prev, next)
	// Combined [oldest, older, newer, newest] capped to last 3 → drop "oldest".
	if strings.Contains(got, "oldest") {
		t.Errorf("expected oldest request dropped by the cap; got:\n%s", got)
	}
	for _, want := range []string{"older", "newer", "newest"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q retained in merged recent_requests; got:\n%s", want, got)
		}
	}
}

// TestEstimateInputTokensExactPath verifies that EstimateInputTokens returns
// the exact tokenizer count (not the heuristic) when the tokenizer is available.
// This is the D-2 acceptance test: with the exact CountMessages wired in,
// EstimateInputTokens(msgs, 4.0) must equal tokenizer.Count(tokenizer.FormatPrompt(msgs, false)).
func TestEstimateInputTokensExactPath(t *testing.T) {
	if !tokenizer.Available() {
		t.Skip("tokenizer not available")
	}

	// Build a ≥10KB conversation.
	msgs := make([]llm.Message, 0, 41)
	msgs = append(msgs, llm.Message{Role: "system", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "You are a coding agent. You operate directly in the user's repository."},
	}})
	for i := 0; i < 20; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 200)},
		}})
		msgs = append(msgs, llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("y", 300)},
		}})
	}

	prompt := tokenizer.FormatPrompt(msgs, false)
	if len(prompt) < 10_000 {
		t.Fatalf("prompt is only %d bytes, want ≥10KB", len(prompt))
	}

	got := EstimateInputTokens(msgs, 4.0)
	want := tokenizer.Count(prompt)
	if got != want {
		t.Errorf("EstimateInputTokens=%d, tokenizer.Count(FormatPrompt)=%d — exact path should match", got, want)
	}
}

// TestPriorSummaryDetectionRejectsProse covers the false-positive guard: a model
// message that merely mentions <summary>...</summary> in prose is NOT treated as
// a prior compaction summary; a whole-message summary IS.
func TestPriorSummaryDetectionRejectsProse(t *testing.T) {
	prose := llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{
		Text: "I'll wrap the output in a <summary>...</summary> block as you asked.",
	}}}
	if got := priorSummaryText(prose); got != "" {
		t.Errorf("prose mentioning the tags must not be detected as a summary; got %q", got)
	}

	real := llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{
		Text: "<summary>\n- messages: 1 total\n</summary>",
	}}}
	if got := priorSummaryText(real); got == "" {
		t.Error("a whole-message summary must be detected")
	}
}

func TestAgentMaybeCompactNoOpBelowThreshold(t *testing.T) {
	a := New(nil, nil, nil, "m")
	a.CompactionCfg = CompactionConfig{
		PreserveRecentMessages: 2,
		AutoCompactInputTokens: 1_000_000, // unreachable
	}
	a.Messages = []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}},
		{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "ok"}}},
	}
	a.maybeCompact(context.Background())
	if len(a.Messages) != 2 {
		t.Errorf("expected no-op; got %d messages", len(a.Messages))
	}
	select {
	case ev := <-a.Events():
		t.Errorf("no event expected; got %T", ev)
	default:
	}
}

// TestShouldCompactRespectsTailTokensFloor verifies the token-based verbatim
// tail floor: when TailTokensFloor is set, the kept tail must contain at least
// that many tokens, even if PreserveRecentMessages would keep fewer.
func TestShouldCompactRespectsTailTokensFloor(t *testing.T) {
	// 60 messages x 4000 chars each; low absolute threshold so token pressure
	// triggers. With the tokenizer ~500 tokens/msg, total ≈ 30k tokens — well
	// above the 16k floor. With TailTokensFloor=16384 the window must end
	// early enough to keep >=16k tokens verbatim.
	msgs := make([]llm.Message, 60)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", 4000)}}}
	}
	cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 1000, TailTokensFloor: 16384}
	ok, from, to := ShouldCompact(msgs, cfg, 4.0)
	if !ok {
		t.Fatal("expected compaction to trigger")
	}
	if from != 0 {
		t.Fatalf("from=%d", from)
	}
	tail := EstimateInputTokens(msgs[to:], 4.0)
	if tail < 16384 {
		t.Fatalf("tail floor violated: kept only %d tokens (to=%d)", tail, to)
	}
	if to <= 0 {
		t.Fatal("floor consumed the whole window")
	}
}

// TestShouldCompactSkipsUneconomicFold verifies the fold-economics gate: when the
// foldable region [0, toIdx) is smaller than MinFoldTokens, compaction is skipped
// because the body rewrite wouldn't save enough tokens to justify itself.
func TestShouldCompactSkipsUneconomicFold(t *testing.T) {
	// 12 msgs × ~205 tokens ≈ 2460 total. A high tail floor leaves a
	// foldable sliver below MinFoldTokens → must skip; a low floor
	// leaves a large fold → must compact.
	msgs := make([]llm.Message, 12)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("y", 800)}}}
	}
	cfg := CompactionConfig{PreserveRecentMessages: 4, AutoCompactInputTokens: 100,
		TailTokensFloor: 2300, MinFoldTokens: 400}
	if ok, _, _ := ShouldCompact(msgs, cfg, 4.0); ok {
		t.Fatal("fold below MinFoldTokens must not compact")
	}
	cfg.TailTokensFloor = 800
	if ok, _, _ := ShouldCompact(msgs, cfg, 4.0); !ok {
		t.Fatal("economic fold must compact")
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
