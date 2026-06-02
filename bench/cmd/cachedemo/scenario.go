// bench/cmd/cachedemo/scenario.go
package main

import (
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// baseSystemPrompt is a realistic, sizeable system prompt — large enough that
// system+tools clears DeepSeek's ~1,024-token cache floor in a real run.
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

// stablePrefix is the byte-identical static prefix used on every turn of the
// cache-stable arm.
func stablePrefix() llm.StaticPrefix {
	return llm.StaticPrefix{System: baseSystemPrompt, Tools: fixedTools()}
}

// driftPrefix models Reasonix's append-only tool growth: the prefix is
// byte-identical to stablePrefix() while turn < driftAt; from driftAt onward a
// "notify" tool is appended, changing the serialized tool bytes and busting the
// DeepSeek prompt cache for every subsequent turn.
func driftPrefix(turn, driftAt int) llm.StaticPrefix {
	if turn < driftAt {
		return stablePrefix()
	}
	tools := append(fixedTools(), llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "notify",
			Description: "Send a notification",
			Parameters:  []byte(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
		},
	})
	return llm.StaticPrefix{System: baseSystemPrompt, Tools: tools}
}

// naivePrefix mutates the system prompt per turn (a volatile counter), so the
// prefix bytes — and the DeepSeek cache key — change every turn. This is the
// failure mode most generic agents fall into (timestamps, turn counters,
// reordered tools).
func naivePrefix(turn int) llm.StaticPrefix {
	return llm.StaticPrefix{
		System: fmt.Sprintf("Session turn %d.\n%s", turn, baseSystemPrompt),
		Tools:  fixedTools(),
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
