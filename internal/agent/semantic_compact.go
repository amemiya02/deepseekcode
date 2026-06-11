// semantic_compact.go adds LLM-powered semantic compaction on top of
// the deterministic local summarizer. When context pressure hits the
// semantic threshold, the agent calls deepseek-v4-flash (thinking
// disabled) to produce a richer summary. Falls back to deterministic
// compaction on failure.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
)

// SemanticCompactionConfig controls semantic compaction behavior.
type SemanticCompactionConfig struct {
	// WarnThreshold is the context ratio (0-1) at which to emit
	// a warning and prepare pinned facts. Default: 0.50
	WarnThreshold float64

	// CompactThreshold is the context ratio at which to attempt
	// semantic compaction. Default: 0.90
	CompactThreshold float64

	// ProtectionThreshold is the context ratio at which to enter
	// protection mode (preserve task continuity over full history).
	// Default: 0.95
	ProtectionThreshold float64

	// UsableWindowTokens is the model's effective context window in tokens.
	// Used for documentation and future window-aware heuristics. ContextPressure
	// already takes maxContextTokens as a parameter, so this field does not
	// change the pressure calculation — it records the window size the thresholds
	// were tuned against. Default: 1_000_000 (DeepSeek V4).
	UsableWindowTokens int

	// SummaryModel is the model to use for semantic summaries.
	// Default: "deepseek-v4-flash"
	SummaryModel string

	// SummaryTimeout is the timeout for the summary request.
	// Default: 15 seconds
	SummaryTimeout time.Duration

	// MaxSummaryTokens caps the LLM's max_tokens parameter — the model
	// is told to produce at most this many output tokens. Default: 2000.
	// This is a wire-level cap sent to the provider; the model may still
	// produce fewer tokens. See also SummaryTokenBudget (the post-hoc
	// hard-clamp backstop).
	MaxSummaryTokens int

	// SummaryTokenBudget caps the token count of the semantic summary.
	// The summary is re-sent every turn after a fold; a verbose summary
	// is permanent body weight. After the LLM produces a summary, it is
	// hard-clamped to this budget as a backstop (model may ignore the
	// prompt instruction). Default: 2048.
	SummaryTokenBudget int

	// CacheUnit is the measured DeepSeek cache-unit boundary (tokens)
	// used to align the rebuilt post-compaction tail. Carried here so
	// SemanticCompact can pass it to alignTailToCacheUnit without
	// requiring a separate CompactionConfig argument. 0 (default) =
	// disabled (no alignment). See CompactionConfig.CacheUnit.
	CacheUnit int
}

// SemanticCompactionResult is what SemanticCompact produces.
type SemanticCompactionResult struct {
	Summary        string
	FromIdx, ToIdx int
	RemovedCount   int
	SummaryMessage llm.Message
	KeptMessages   []llm.Message
	UsedSemantic   bool    // true if LLM summary was used
	SummaryCost    float64 // cost of the LLM call, if any
	FallbackReason string  // why deterministic fallback was used
}

// ContextPressure returns the current context usage ratio (0-1).
func ContextPressure(messages []llm.Message, maxContextTokens int, charsPerToken float64) float64 {
	if maxContextTokens <= 0 {
		return 0
	}
	return float64(EstimateTokensCalibrated(messages, charsPerToken)) / float64(maxContextTokens)
}

// ShouldSemanticCompact decides whether semantic compaction should fire.
// Returns the action: "none", "warn", "compact", or "protect".
func ShouldSemanticCompact(pressure float64, cfg SemanticCompactionConfig) string {
	if cfg.WarnThreshold <= 0 {
		cfg.WarnThreshold = 0.50
	}
	if cfg.CompactThreshold <= 0 {
		cfg.CompactThreshold = 0.90
	}
	if cfg.ProtectionThreshold <= 0 {
		cfg.ProtectionThreshold = 0.95
	}

	switch {
	case pressure >= cfg.ProtectionThreshold:
		return "protect"
	case pressure >= cfg.CompactThreshold:
		return "compact"
	case pressure >= cfg.WarnThreshold:
		return "warn"
	default:
		return "none"
	}
}

// defaultSemanticCompactionConfig returns a SemanticCompactionConfig with
// sensible defaults.
func defaultSemanticCompactionConfig() SemanticCompactionConfig {
	return SemanticCompactionConfig{
		WarnThreshold:       0.50,
		CompactThreshold:    0.90,
		ProtectionThreshold: 0.95,
		UsableWindowTokens:  1_000_000,
		SummaryModel:        "deepseek-v4-flash",
		SummaryTimeout:      15 * time.Second,
		MaxSummaryTokens:    2000,
		SummaryTokenBudget:  2048,
	}
}

// SemanticCompact attempts LLM-powered compaction, falling back to
// deterministic on failure. The LLM call:
//   - Uses deepseek-v4-flash
//   - Disables thinking
//   - Has a 15 second timeout
//   - Reuses the same static system prefix
//   - Preserves pinned skills and constraints
//   - Preserves current objective
//   - Preserves negative constraints
//   - Preserves changed file paths
//   - Preserves recent tool evidence
//   - Records its own usage and cost
func SemanticCompact(
	ctx context.Context,
	messages []llm.Message,
	client *llm.Client,
	systemPrompt string,
	tools []llm.Tool,
	cfg SemanticCompactionConfig,
) SemanticCompactionResult {
	if cfg.SummaryModel == "" {
		cfg.SummaryModel = "deepseek-v4-flash"
	}
	if cfg.SummaryTimeout <= 0 {
		cfg.SummaryTimeout = 15 * time.Second
	}
	if cfg.MaxSummaryTokens <= 0 {
		cfg.MaxSummaryTokens = 2000
	}
	if cfg.SummaryTokenBudget <= 0 {
		cfg.SummaryTokenBudget = 2048
	}

	// Determine compaction window via existing ShouldCompact logic.
	// We need to compact the same way as the deterministic path.
	compCfg := CompactionConfig{
		PreserveRecentMessages: 4,
		MaxEstimatedTokens:     cfg.MaxSummaryTokens,
		AutoCompactInputTokens: 1, // always trigger for semantic path
	}
	// compCfg.AutoCompactInputTokens==1 always trips, so the calibrated ratio
	// is irrelevant here; pass the cold-start prior.
	ok, fromIdx, toIdx := ShouldCompact(messages, compCfg, defaultCharsPerToken)
	if !ok {
		return SemanticCompactionResult{}
	}
	toIdx = adjustBoundary(messages, toIdx)
	if toIdx <= fromIdx {
		return SemanticCompactionResult{}
	}

	kept := make([]llm.Message, len(messages)-toIdx)
	copy(kept, messages[toIdx:])

	// Attempt LLM-powered summary.
	summary, usage, err := callSummaryModel(ctx, client, systemPrompt, tools, messages[fromIdx:toIdx], cfg)
	if err != nil {
		return fallbackToDeterministic(messages, compCfg, fmt.Sprintf("LLM call failed: %v", err))
	}
	if strings.TrimSpace(summary) == "" {
		return fallbackToDeterministic(messages, compCfg, "LLM returned empty summary")
	}

	// Hard-clamp the summary to the token budget. The model may ignore the
	// prompt instruction, so we enforce it here as a backstop.
	summary = clampToTokenBudget(summary, cfg.SummaryTokenBudget)

	cost := llm.Cost(cfg.SummaryModel, usage)
	// Assistant-role body message, consistent with the deterministic path
	// (T4.3) — never a second system message in the wire.
	summaryMsg := llm.Message{
		Role:   "assistant",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: summary}},
	}
	// §3.6: align the rebuilt tail ([summary || kept]) to a cache-unit boundary,
	// matching CompactSession's behavior. Without this, the semantic path would
	// produce a different post-fold prefix boundary than the deterministic path
	// for the same tail content, violating the W0.3 determinism invariant.
	summaryMsg = alignTailToCacheUnit(summaryMsg, kept, cfg.CacheUnit)

	return SemanticCompactionResult{
		Summary:        summary,
		FromIdx:        fromIdx,
		ToIdx:          toIdx,
		RemovedCount:   toIdx - fromIdx,
		SummaryMessage: summaryMsg,
		KeptMessages:   kept,
		UsedSemantic:   true,
		SummaryCost:    cost,
	}
}

// callSummaryModel makes a non-streaming LLM call to the Flash model
// for summarization. Returns the summary text and usage, or an error.
func callSummaryModel(
	ctx context.Context,
	client *llm.Client,
	systemPrompt string,
	tools []llm.Tool,
	messages []llm.Message,
	cfg SemanticCompactionConfig,
) (string, llm.Usage, error) {
	if client == nil {
		return "", llm.Usage{}, fmt.Errorf("nil LLM client")
	}

	prompt := buildSemanticSummaryPrompt(messages, cfg.SummaryTokenBudget)

	// Build a minimal request: system prompt + summary prompt.
	// Thinking disabled for cost savings.
	req := llm.Request{
		Model: cfg.SummaryModel,
		Messages: []llm.Message{
			{Role: "system", Blocks: []llm.ContentBlock{llm.TextBlock{Text: systemPrompt}}},
			{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		Stream:    false,
		Thinking:  nil, // disabled — saves cost
		MaxTokens: cfg.MaxSummaryTokens,
		Tools:     tools,
	}

	body, err := req.MarshalCacheStable()
	if err != nil {
		return "", llm.Usage{}, fmt.Errorf("marshal summary request: %w", err)
	}

	// Use a bounded context for the summary call.
	callCtx, cancel := context.WithTimeout(ctx, cfg.SummaryTimeout)
	defer cancel()

	url := client.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", llm.Usage{}, fmt.Errorf("create summary request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+client.APIKey)

	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		return "", llm.Usage{}, fmt.Errorf("summary request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", llm.Usage{}, fmt.Errorf("summary API error %d: %s", resp.StatusCode, string(buf))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", llm.Usage{}, fmt.Errorf("read summary response: %w", err)
	}

	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *llm.Usage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", llm.Usage{}, fmt.Errorf("parse summary response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", llm.Usage{}, fmt.Errorf("summary returned empty choices")
	}

	var usage llm.Usage
	if cr.Usage != nil {
		usage = *cr.Usage
	}

	return strings.TrimSpace(cr.Choices[0].Message.Content), usage, nil
}

// buildSemanticSummaryPrompt constructs the prompt for the Flash model.
// It includes:
//   - The messages to summarize
//   - Instructions to preserve pinned facts, current task, constraints
//   - Instructions to produce a structured summary
func buildSemanticSummaryPrompt(messages []llm.Message, tokenBudget int) string {
	var b strings.Builder
	b.WriteString("You are a conversation summarizer. Produce a concise, structured summary of the following messages.\n\n")
	if tokenBudget > 0 {
		fmt.Fprintf(&b, "Your summary MUST be under %d tokens. Be concise.\n\n", tokenBudget)
	}
	b.WriteString("CRITICAL CONSTRAINTS — you MUST preserve these in the summary:\n")
	b.WriteString("1. **Pinned skill facts**: any specific facts, rules, or constraints mentioned\n")
	b.WriteString("2. **Current objective/task**: what the user is trying to accomplish RIGHT NOW\n")
	b.WriteString("3. **Negative constraints**: things explicitly forbidden or to avoid\n")
	b.WriteString("4. **Changed file paths**: all file paths that were read, modified, or discussed\n")
	b.WriteString("5. **Recent tool evidence**: key results from the last few tool calls\n")
	b.WriteString("6. **Prior summary carry-forward**: if any message below is itself a <summary>...</summary> block from an earlier compaction, you MUST preserve ALL of its facts (every file path, tool, and constraint) in your summary — those facts are from the start of the session and exist ONLY in that block. Never drop them.\n\n")
	b.WriteString("Format your response as:\n")
	b.WriteString("<summary>\n")
	b.WriteString("- objective: (current task/goal)\n")
	b.WriteString("- progress: (what has been done so far)\n")
	b.WriteString("- key_files: (file paths involved)\n")
	b.WriteString("- constraints: (pinned facts, negative constraints)\n")
	b.WriteString("- pending: (what needs to happen next)\n")
	b.WriteString("- tool_evidence: (key results from recent tools)\n")
	b.WriteString("- timeline: (brief chronological notes)\n")
	b.WriteString("</summary>\n\n")
	b.WriteString("Messages to summarize:\n")
	for i, m := range messages {
		fmt.Fprintf(&b, "[%d] %s: ", i+1, m.Role)
		// A prior compaction summary holds the only copy of early-session facts;
		// include it in full (not truncated to 500 chars) so constraint 6 has the
		// facts to carry forward.
		if prior := priorSummaryText(m); prior != "" {
			b.WriteString(prior)
			b.WriteString("\n")
			continue
		}
		for _, blk := range m.Blocks {
			switch v := blk.(type) {
			case llm.TextBlock:
				b.WriteString(truncateRunes(singleLine(v.Text), 500))
			case llm.ThinkingBlock:
				b.WriteString("(thinking) ")
				b.WriteString(truncateRunes(singleLine(v.Text), 200))
			case llm.ToolUseBlock:
				fmt.Fprintf(&b, "(tool_use %s)", v.Name)
			case llm.ToolResultBlock:
				b.WriteString(truncateRunes(singleLine(v.Content), 300))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// clampToTokenBudget truncates text to fit within the given token budget.
// Uses the exact tokenizer when available, falling back to a len/4 heuristic.
// Truncation is done at the last newline boundary that fits, so the summary
// remains structurally valid.
func clampToTokenBudget(text string, budget int) string {
	if budget <= 0 {
		return text
	}
	tok := countTextTokens(text)
	if tok <= budget {
		return text
	}
	// Binary search for the longest prefix that fits within the budget.
	lo, hi := 0, len(text)
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		if countTextTokens(text[:mid]) <= budget {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	truncated := text[:lo]
	// Try to cut at a newline for structural validity.
	if idx := strings.LastIndex(truncated, "\n"); idx > len(truncated)/2 {
		truncated = truncated[:idx+1]
	}
	return strings.TrimSpace(truncated)
}

// countTextTokens returns the token count of a plain text string.
// Uses the exact tokenizer when available, else the len/4 heuristic.
func countTextTokens(text string) int {
	if text == "" {
		return 0
	}
	if n := tokenizer.Count(text); n > 0 {
		return n
	}
	// Fallback: approximate 4 chars per token.
	return max(1, len(text)/4)
}

// fallbackToDeterministic wraps the existing CompactSession for fallback.
func fallbackToDeterministic(messages []llm.Message, cfg CompactionConfig, reason string) SemanticCompactionResult {
	// Reached with cfg.AutoCompactInputTokens==1 (always trips), so the ratio
	// is irrelevant; the cold-start prior keeps this fallback deterministic.
	res := CompactSession(messages, cfg, defaultCharsPerToken)
	if res.Summary == "" {
		return SemanticCompactionResult{FallbackReason: reason}
	}
	return SemanticCompactionResult{
		Summary:        res.Summary,
		FromIdx:        res.FromIdx,
		ToIdx:          res.ToIdx,
		RemovedCount:   res.RemovedCount,
		SummaryMessage: res.SummaryMessage,
		KeptMessages:   res.KeptMessages,
		UsedSemantic:   false,
		FallbackReason: reason,
	}
}
