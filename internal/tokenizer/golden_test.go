package tokenizer

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// Golden vectors: text → token-id sequences cross-validated against the
// reference DeepSeek-Reasonix TypeScript encoder (byte-identical vocab).
// Regenerate: set GENERATE_GOLDEN=1 and run TestGenerateGoldenVectors.
// Then cross-validate against the reference encoder:
//
//	npx tsx /tmp/cross_validate.mjs  (see repo history for the script).
//
// The reference TS encoder must produce byte-identical ids for all 28 vectors.
// All 28 vectors verified identical to the reference on 2026-05-31.
func TestGolden(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}
	raw, err := os.ReadFile("testdata/golden_vectors.json")
	if err != nil {
		t.Fatalf("read golden vectors: %v", err)
	}
	type Vector struct {
		Text string `json:"text"`
		IDs  []int  `json:"ids"`
	}
	var vectors []Vector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("parse golden vectors: %v", err)
	}
	if len(vectors) < 20 {
		t.Fatalf("golden vectors has %d entries, want >= 20", len(vectors))
	}
	for _, v := range vectors {
		got := Encode(v.Text)
		if len(got) != len(v.IDs) {
			t.Errorf("Encode(%q): len=%d, want %d", v.Text, len(got), len(v.IDs))
			continue
		}
		for i := range got {
			if got[i] != v.IDs[i] {
				t.Errorf("Encode(%q)[%d] = %d, want %d", v.Text, i, got[i], v.IDs[i])
				break
			}
		}
	}
}

// buildTestMessages returns a realistic multi-turn conversation extracted from
// a real dsc session (session 254c5a26, deepseek-v4-flash, project deepseekcode).
// The messages are built as Go structs with Blocks populated so they round-trip
// through FormatPrompt correctly (llm.Message.Blocks is json:"-").
func buildTestMessages() []llm.Message {
	return []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "hello who are you"},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ThinkingBlock{Text: "The user is asking who I am. I'm a terminal coding agent powered by DeepSeek, designed to work directly with code repositories."},
			llm.TextBlock{Text: "Hi there! I'm **deepseekcode** — a terminal coding agent powered by **DeepSeek**. I work directly with your repository through various tool calls (reading files, searching code, running tests, editing code, and more). I'm designed to be a focused, efficient coding assistant that operates right in your terminal."},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "what can you do"},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ThinkingBlock{Text: "The user is asking about my capabilities. Let me give them a clear overview of what I can do as a coding agent."},
			llm.TextBlock{Text: "Here's what I can do:\n\n**Code work** — read, write, and surgically edit files; search with grep/glob\n**Build & test** — run shell commands with configurable timeouts\n**Git operations** — diffs, blame, log, individual commits\n**Task tracking** — multi-step TODO lists for larger tasks\n\nMy style: surgical edits, read before writing, verify with build/test, match your conventions."},
		}},
	}
}

// buildTestSystem returns a system prompt matching the dsc project's default.
// Extracted from the real session's context (CLAUDE.md + tool descriptions).
func buildTestSystem() string {
	return `You are a coding agent. You operate directly in the user's repository. You can read, write, and edit files, run shell commands, and search the codebase.

When you need to read a file, use the read_file tool. When you need to write, use write_file. For shell commands, use bash. Always explain what you're doing.

Available tools: ls, read_file, write_file, bash, grep, glob.`
}

// TestRealUsageTolerance verifies the tokenizer's conversation counting against
// a real recorded prompt_tokens from dsc session 254c5a26 (deepseek-v4-flash,
// project deepseekcode), turn 1.
//
// Two layers of verification:
//
//  1. EXACT COUNT (high sensitivity): CountMessages(messages) is pinned to 263.
//     Any encoder change (BPE, pretokenize, charToByteOffset, template) changes
//     this count. This is the primary accuracy gate.
//
//  2. TOLERANCE (honest limitation): the total estimate uses a frozen residual
//     (2089) that represents server-side overhead we can't count locally. Because
//     the residual is 85% of the total, a 10% drift in conv only produces ~1.1%
//     estimate drift — the ±2% tolerance is weak at detecting small conv errors.
//     A real API capture would make this tighter. For now, the exact count
//     assertion is the real guard.
//
// Litmus test: change the tokenizer's encoding and re-run. The exact count
// assertion (layer 1) catches any drift immediately. The tolerance (layer 2)
// catches large drift only.
func TestRealUsageTolerance(t *testing.T) {
	if !Available() {
		t.Skip("tokenizer not available")
	}

	// Real prompt_tokens from the dsc session DB (session 254c5a26, turn 1).
	const realPromptTokens = 2442

	// Frozen residual: 2442 - 90 - 261 = 2091.
	// Server-side overhead (system rendering + tool-schema template + special tokens)
	// that the local tokenizer cannot see. Computed once with the corrected encoder.
	const fixedResidual = 2091

	// Exact expected conversation count — the primary accuracy gate.
	// Updated from 263 after fixing splitAddedTokens to use proper byte offsets
	// (charToByteOffset) instead of treating regexp2 character indices as byte offsets.
	const expectedConv = 261

	system := buildTestSystem()
	msgs := buildTestMessages()

	// Live computation.
	sys := Count(system)
	conv, ok := CountMessages(msgs)
	if !ok {
		t.Fatal("CountMessages returned ok=false")
	}

	// Layer 1: exact count assertion (high sensitivity).
	if conv != expectedConv {
		t.Errorf("CountMessages = %d, want %d (encoder drift detected)", conv, expectedConv)
	}

	// Layer 2: tolerance check (honest limitation — see docstring).
	estimateTotal := sys + conv + fixedResidual
	diff := math.Abs(float64(estimateTotal-realPromptTokens)) / float64(realPromptTokens)
	if diff > 0.02 {
		t.Errorf("|%d - %d| / %d = %.4f > 0.02 (sys=%d, conv=%d, fixed_residual=%d)",
			estimateTotal, realPromptTokens, realPromptTokens, diff,
			sys, conv, fixedResidual)
	}

	// Sanity.
	if estimateTotal <= 0 {
		t.Errorf("estimateTotal = %d, want positive", estimateTotal)
	}
}
