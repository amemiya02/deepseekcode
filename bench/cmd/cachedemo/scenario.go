// bench/cmd/cachedemo/scenario.go
package main

import (
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// productionScale, when true, swaps the small synthetic prefix below for
// productionPrefix() — dsc's real default system prompt + real registered tool
// set (≈ the live ~8K-token static prefix). It is OFF by default so the fast
// unit tests keep using the small, deterministic baseSystemPrompt/fixedTools
// pair; main.go flips it on via a flag for a production-scale run. Every
// prefix builder (stable/drift/naive) reads the active base through
// activeSystemPrompt()/activeBaseTools(), so the existing drift/naive logic is
// reused verbatim regardless of scale.
var productionScale = false

// baseSystemPrompt is a realistic, sizeable system prompt — large enough that
// system+tools clears DeepSeek's ~1,024-token cache floor in a real run, but
// still far below dsc's real ~8K-token prefix (see productionPrefix).
var baseSystemPrompt = strings.Repeat(
	"You are deepseekcode, a terminal coding agent for DeepSeek V4. "+
		"Use the provided tools to read and edit files and run commands. "+
		"Prefer minimal, correct changes and explain briefly. ", 24)

// fixedTools is a small, stable tool set standing in for a real agent's schemas.
func fixedTools() []llm.Tool {
	return []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{
			Name: "read_file", Description: "Read a file",
			Parameters: []byte(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "edit_file", Description: "Edit a file",
			Parameters: []byte(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"}},"required":["path","old","new"]}`)}},
		{Type: "function", Function: llm.ToolFunction{
			Name: "bash", Description: "Run a shell command",
			Parameters: []byte(`{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"]}`)}},
	}
}

// realToolSet returns dsc's real registered tool surface, serialized exactly as
// the agent would send it (name-sorted, same descriptions and JSON-Schema). It
// is built from the live tools.RegisterBuiltins registry (plus the lazy
// struct-search tool) rather than copy-pasting bytes, so the demo's production
// prefix tracks dsc's real tool JSON if the tool set changes. The CWD/byte-limit
// args do not affect the serialized prefix bytes, so zero/empty values are fine.
func realToolSet() []llm.Tool {
	r := tools.New()
	tools.RegisterBuiltins(r, 0, 0, "")
	r.RegisterWithTier(tools.NewStructSearchTool(""), tools.TierLazy)
	return r.AsLLMTools()
}

// productionSystemPrompt is dsc's real binary-versioned static base prompt
// (prompt.BasePromptV1) — the exact cache-stable head the SystemPromptBuilder
// freezes by default (builder.go: empty StaticBase falls back to BasePromptV1).
// It is the real prefix base, not the small behavioral snippet, so the demo's
// large arm exercises the same bytes a live session would freeze. Session-once
// instruction files (DEEPSEEK.md/AGENTS.md) and the skill directory push a real
// session well past this; they are session-specific, so we intentionally do not
// fabricate them here.
var productionSystemPrompt = prompt.BasePromptV1

// productionPrefix is the production-scale static prefix: dsc's real base prompt
// (productionSystemPrompt) plus the real registered tool set (realToolSet),
// totalling roughly 3.3K tokens (~1.2K base + ~2.2K tool JSON) — about 3x the
// synthetic ~1.1K prefix. The synthetic prefix is small enough that the
// mid-session-drift cache penalty hides under output-token variance; this one
// is large enough for the drift miss to register against the cache floor.
// Gated behind productionScale so the fast unit tests keep the small prefix.
func productionPrefix() llm.StaticPrefix {
	return llm.StaticPrefix{System: productionSystemPrompt, Tools: realToolSet()}
}

// activeSystemPrompt is the system text the current scale uses as the prefix
// base. Every prefix builder routes through it so drift/naive logic is
// scale-agnostic.
func activeSystemPrompt() string {
	if productionScale {
		return productionSystemPrompt
	}
	return baseSystemPrompt
}

// activeBaseTools is the base tool set the current scale uses as the prefix
// base. driftPrefix appends its "notify" tool on top of this slice.
func activeBaseTools() []llm.Tool {
	if productionScale {
		return realToolSet()
	}
	return fixedTools()
}

// stablePrefix is the byte-identical static prefix used on every turn of the
// cache-stable arm.
func stablePrefix() llm.StaticPrefix {
	return llm.StaticPrefix{System: activeSystemPrompt(), Tools: activeBaseTools()}
}

// driftPrefix models Reasonix's append-only tool growth: the prefix is
// byte-identical to stablePrefix() while turn < driftAt; from driftAt onward a
// "notify" tool is appended, changing the serialized tool bytes and busting the
// DeepSeek prompt cache for every subsequent turn. The nonce is embedded in the
// appended tool's description so each run produces a never-before-seen post-drift
// prefix (genuine cache miss), while pre-drift turns remain nonce-free and
// byte-identical to stablePrefix().
func driftPrefix(turn, driftAt int, nonce string) llm.StaticPrefix {
	if turn < driftAt {
		return stablePrefix()
	}
	base := activeBaseTools()
	drifted := append(append([]llm.Tool{}, base...), llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "notify",
			Description: fmt.Sprintf("Send a notification [run:%s]", nonce),
			Parameters:  []byte(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
		},
	})
	return llm.StaticPrefix{System: activeSystemPrompt(), Tools: drifted}
}

// naivePrefix mutates the system prompt per turn and per run (a volatile
// counter + per-run nonce), so the prefix bytes — and the DeepSeek cache key —
// change every turn AND every run. This is the failure mode most generic agents
// fall into (timestamps, turn counters, reordered tools). The nonce ensures
// that even across re-runs the naive arm always produces genuine cache misses,
// keeping the naive-vs-stable contrast intact.
func naivePrefix(turn int, nonce string) llm.StaticPrefix {
	return llm.StaticPrefix{
		System: fmt.Sprintf("Session %s turn %d.\n%s", nonce, turn, activeSystemPrompt()),
		Tools:  activeBaseTools(),
	}
}

// buildRequest assembles one streaming chat request: the static prefix as the
// system message + this turn's user message. IncludeUsage is always set so the
// final SSE frame reports cache hit/miss tokens.
func buildRequest(model string, prefix llm.StaticPrefix, userText string) llm.Request {
	return llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Blocks: []llm.ContentBlock{llm.TextBlock{Text: prefix.System}}},
			{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: userText}}},
		},
		Stream:        true,
		StreamOptions: &llm.StreamOptions{IncludeUsage: true},
		Tools:         prefix.Tools,
		Thinking:      llm.ThinkingEnabled(false),
	}
}

// demoTurns is the fixed, deterministic user-turn script both arms replay.
func demoTurns(n int) []string {
	base := []string{
		"List the Go files in internal/llm.",
		"Read internal/llm/cache_metrics.go and summarize Cost().",
		"What does CacheSavings compute?",
		"Find where prompt_cache_hit_tokens is read.",
		"Explain the prefix fingerprint in one sentence.",
		"Which models are in the Prices table?",
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, base[i%len(base)])
	}
	return out
}
