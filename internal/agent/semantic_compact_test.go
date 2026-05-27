// semantic_compact_test.go tests for semantic compaction behavior.
package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// --- Test 1: 75% context pressure emits warning ---
func TestShouldSemanticCompact_WarnAt75Percent(t *testing.T) {
	cfg := SemanticCompactionConfig{
		WarnThreshold:       0.75,
		CompactThreshold:    0.80,
		ProtectionThreshold: 0.90,
	}
	action := ShouldSemanticCompact(0.75, cfg)
	if action != "warn" {
		t.Errorf("expected 'warn' at 75%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(0.76, cfg)
	if action != "warn" {
		t.Errorf("expected 'warn' at 76%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(0.79, cfg)
	if action != "warn" {
		t.Errorf("expected 'warn' at 79%% pressure; got %q", action)
	}
}

// --- Test 2: 80% context pressure attempts semantic compaction ---
func TestShouldSemanticCompact_CompactAt80Percent(t *testing.T) {
	cfg := SemanticCompactionConfig{
		WarnThreshold:       0.75,
		CompactThreshold:    0.80,
		ProtectionThreshold: 0.90,
	}
	action := ShouldSemanticCompact(0.80, cfg)
	if action != "compact" {
		t.Errorf("expected 'compact' at 80%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(0.85, cfg)
	if action != "compact" {
		t.Errorf("expected 'compact' at 85%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(0.89, cfg)
	if action != "compact" {
		t.Errorf("expected 'compact' at 89%% pressure; got %q", action)
	}
}

// --- Test 3: 90% context pressure enters protection mode ---
func TestShouldSemanticCompact_ProtectAt90Percent(t *testing.T) {
	cfg := SemanticCompactionConfig{
		WarnThreshold:       0.75,
		CompactThreshold:    0.80,
		ProtectionThreshold: 0.90,
	}
	action := ShouldSemanticCompact(0.90, cfg)
	if action != "protect" {
		t.Errorf("expected 'protect' at 90%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(0.95, cfg)
	if action != "protect" {
		t.Errorf("expected 'protect' at 95%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(1.0, cfg)
	if action != "protect" {
		t.Errorf("expected 'protect' at 100%% pressure; got %q", action)
	}
}

// --- Test: Below thresholds returns none ---
func TestShouldSemanticCompact_NoneBelowThreshold(t *testing.T) {
	cfg := SemanticCompactionConfig{
		WarnThreshold:       0.75,
		CompactThreshold:    0.80,
		ProtectionThreshold: 0.90,
	}
	action := ShouldSemanticCompact(0.50, cfg)
	if action != "none" {
		t.Errorf("expected 'none' at 50%% pressure; got %q", action)
	}
	action = ShouldSemanticCompact(0.74, cfg)
	if action != "none" {
		t.Errorf("expected 'none' at 74%% pressure; got %q", action)
	}
}

// --- Test: Default thresholds when zero ---
func TestShouldSemanticCompact_DefaultThresholds(t *testing.T) {
	cfg := SemanticCompactionConfig{} // all zero → use defaults
	action := ShouldSemanticCompact(0.75, cfg)
	if action != "warn" {
		t.Errorf("expected 'warn' with default thresholds at 0.75; got %q", action)
	}
	action = ShouldSemanticCompact(0.80, cfg)
	if action != "compact" {
		t.Errorf("expected 'compact' with default thresholds at 0.80; got %q", action)
	}
	action = ShouldSemanticCompact(0.90, cfg)
	if action != "protect" {
		t.Errorf("expected 'protect' with default thresholds at 0.90; got %q", action)
	}
}

// --- ContextPressure tests ---
func TestContextPressure(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)}, // ~100 tokens
		}},
	}
	pressure := ContextPressure(msgs, 1000)
	if pressure < 0.10 || pressure > 0.12 {
		t.Errorf("expected ~0.10 pressure; got %f", pressure)
	}
}

func TestContextPressure_ZeroMax(t *testing.T) {
	msgs := []llm.Message{{Role: "user"}}
	if p := ContextPressure(msgs, 0); p != 0 {
		t.Errorf("expected 0 for zero max; got %f", p)
	}
}

// --- Test 4, 5, 6, 7: Semantic summary uses Flash, disables thinking,
// has 15s timeout, records cost ---
func TestSemanticCompact_UsesFlashDisablesThinkingTimeoutCost(t *testing.T) {
	var receivedModel string
	var receivedThinking *llm.ThinkingOptions
	var receivedMaxTokens int

	// Fake LLM server that returns a structured summary.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the request body to verify model, thinking, max_tokens.
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Model     string              `json:"model"`
			Thinking  *llm.ThinkingOptions `json:"thinking,omitempty"`
			MaxTokens int                 `json:"max_tokens"`
		}
		_ = json.Unmarshal(body, &req)
		receivedModel = req.Model
		receivedThinking = req.Thinking
		receivedMaxTokens = req.MaxTokens

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "<summary>\n- objective: test\n- progress: done\n- key_files: [a.go]\n- constraints: none\n- pending: none\n- tool_evidence: none\n- timeline: [user][1] test\n</summary>"}},
			},
			"usage": map[string]int{
				"prompt_tokens":            100,
				"completion_tokens":        50,
				"total_tokens":             150,
				"prompt_cache_hit_tokens":  0,
				"prompt_cache_miss_tokens": 100,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := llm.NewClient("test-key", srv.URL)

	// Build messages large enough to trigger compaction.
	msgs := make([]llm.Message, 20)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}}
	}

	cfg := SemanticCompactionConfig{
		SummaryModel:     "deepseek-v4-flash",
		SummaryTimeout:   15 * time.Second,
		MaxSummaryTokens: 2000,
	}

	t0 := time.Now()
	res := SemanticCompact(context.Background(), msgs, client, "system prompt", nil, cfg)
	elapsed := time.Since(t0)

	// Test 4: Uses Flash model
	if receivedModel != "deepseek-v4-flash" {
		t.Errorf("expected model deepseek-v4-flash; got %q", receivedModel)
	}
	if !res.UsedSemantic {
		t.Error("expected UsedSemantic=true")
	}

	// Test 5: Thinking disabled (nil = disabled in DeepSeek V4)
	if receivedThinking != nil {
		t.Errorf("expected thinking=nil (disabled); got %+v", receivedThinking)
	}

	// Test 6: Timeout enforced (should complete well under 15s)
	if elapsed > 10*time.Second {
		t.Errorf("summary call took too long: %v (expected < 10s)", elapsed)
	}

	// Test 7: Cost recorded
	if res.SummaryCost <= 0 {
		t.Errorf("expected SummaryCost > 0; got %f", res.SummaryCost)
	}

	// MaxTokens should be set
	if receivedMaxTokens != 2000 {
		t.Errorf("expected max_tokens=2000; got %d", receivedMaxTokens)
	}

	// Summary should contain the content
	if !strings.Contains(res.Summary, "objective") {
		t.Errorf("summary missing 'objective': %s", res.Summary)
	}
}

// --- Test 8: Summary preserves pinned skill facts and negative constraints ---
func TestSemanticCompact_PreservesConstraints(t *testing.T) {
	var capturedPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		// The prompt is in the user message
		for _, m := range req.Messages {
			if m.Role == "user" {
				capturedPrompt = m.Content
			}
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "<summary>\n- objective: refactor\n- constraints: never delete comments, preserve API\n</summary>"}},
			},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := llm.NewClient("test-key", srv.URL)

	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "Refactor parser.go. Never delete comments. Preserve API compatibility."},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "I will refactor without deleting comments."},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("y", 400)},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("z", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("w", 400)},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("a", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("b", 400)},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("c", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("d", 400)},
		}},
	}

	cfg := SemanticCompactionConfig{
		SummaryModel:     "deepseek-v4-flash",
		SummaryTimeout:   15 * time.Second,
		MaxSummaryTokens: 2000,
	}

	res := SemanticCompact(context.Background(), msgs, client, "system prompt", nil, cfg)
	if !res.UsedSemantic {
		t.Fatal("expected semantic compaction")
	}

	// Verify the prompt includes instructions about constraints
	if !strings.Contains(capturedPrompt, "Pinned skill facts") {
		t.Error("prompt missing 'Pinned skill facts' instruction")
	}
	if !strings.Contains(capturedPrompt, "Negative constraints") {
		t.Error("prompt missing 'Negative constraints' instruction")
	}
	if !strings.Contains(capturedPrompt, "CRITICAL CONSTRAINTS") {
		t.Error("prompt missing 'CRITICAL CONSTRAINTS' section")
	}
}

// --- Test 9: Summary preserves current task and relevant file paths ---
func TestSemanticCompact_PreservesTaskAndFiles(t *testing.T) {
	var capturedPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		for _, m := range req.Messages {
			if m.Role == "user" {
				capturedPrompt = m.Content
			}
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "<summary>\n- objective: add tests\n- key_files: [parser.go, parser_test.go]\n</summary>"}},
			},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := llm.NewClient("test-key", srv.URL)

	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "Add tests for parser.go. Create parser_test.go."},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "Working on parser_test.go"},
			llm.ToolUseBlock{ID: "t1", Name: "edit_file", Input: json.RawMessage(`{"path":"parser.go"}`)},
		}},
		{Role: "tool", Blocks: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "t1", Content: "wrote 10 lines"},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("y", 400)},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("z", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("w", 400)},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("a", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("b", 400)},
		}},
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("c", 400)},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("d", 400)},
		}},
	}

	cfg := SemanticCompactionConfig{
		SummaryModel:     "deepseek-v4-flash",
		SummaryTimeout:   15 * time.Second,
		MaxSummaryTokens: 2000,
	}

	res := SemanticCompact(context.Background(), msgs, client, "system prompt", nil, cfg)
	if !res.UsedSemantic {
		t.Fatal("expected semantic compaction")
	}

	// Prompt should include instructions about file paths and objective
	if !strings.Contains(capturedPrompt, "Changed file paths") {
		t.Error("prompt missing 'Changed file paths' instruction")
	}
	if !strings.Contains(capturedPrompt, "Current objective") {
		t.Error("prompt missing 'Current objective' instruction")
	}
	// The prompt should contain actual message content including file paths
	if !strings.Contains(capturedPrompt, "parser.go") {
		t.Error("prompt should include 'parser.go' from messages")
	}
}

// --- Test 10: Failure falls back to deterministic compaction ---
func TestSemanticCompact_FallbackToDeterministic(t *testing.T) {
	t.Run("server_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		}))
		defer srv.Close()

		client := llm.NewClient("test-key", srv.URL)
		msgs := make([]llm.Message, 20)
		for i := range msgs {
			msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: strings.Repeat("x", 400)},
			}}
		}

		cfg := SemanticCompactionConfig{
			SummaryModel:     "deepseek-v4-flash",
			SummaryTimeout:   15 * time.Second,
			MaxSummaryTokens: 2000,
		}

		res := SemanticCompact(context.Background(), msgs, client, "system prompt", nil, cfg)
		if res.UsedSemantic {
			t.Error("expected fallback (UsedSemantic=false)")
		}
		if res.FallbackReason == "" {
			t.Error("expected FallbackReason to be set")
		}
		if res.Summary == "" {
			t.Error("expected deterministic summary on fallback")
		}
	})

	t.Run("nil_client", func(t *testing.T) {
		msgs := make([]llm.Message, 20)
		for i := range msgs {
			msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: strings.Repeat("x", 400)},
			}}
		}

		cfg := SemanticCompactionConfig{
			SummaryModel:     "deepseek-v4-flash",
			SummaryTimeout:   15 * time.Second,
			MaxSummaryTokens: 2000,
		}

		res := SemanticCompact(context.Background(), msgs, nil, "system prompt", nil, cfg)
		if res.UsedSemantic {
			t.Error("expected fallback with nil client")
		}
		if res.FallbackReason == "" {
			t.Error("expected FallbackReason to be set")
		}
		if res.Summary == "" {
			t.Error("expected deterministic summary on fallback")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": "summary"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		client := llm.NewClient("test-key", srv.URL)
		msgs := make([]llm.Message, 20)
		for i := range msgs {
			msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
				llm.TextBlock{Text: strings.Repeat("x", 400)},
			}}
		}

		// Very short timeout to force fallback
		cfg := SemanticCompactionConfig{
			SummaryModel:     "deepseek-v4-flash",
			SummaryTimeout:   1 * time.Millisecond,
			MaxSummaryTokens: 2000,
		}

		res := SemanticCompact(context.Background(), msgs, client, "system prompt", nil, cfg)
		if res.UsedSemantic {
			t.Error("expected fallback on timeout")
		}
		if !strings.Contains(res.FallbackReason, "LLM call failed") {
			t.Errorf("expected fallback reason about LLM failure; got %q", res.FallbackReason)
		}
	})
}

// --- Test 11: Static prefix hash is unchanged after compaction ---
func TestSemanticCompact_StaticPrefixHashUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "<summary>\n- objective: test\n- progress: done\n</summary>"}},
			},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := llm.NewClient("test-key", srv.URL)

	systemPrompt := "You are a coding agent."
	tools := []llm.Tool{
		{Type: "function", Function: llm.ToolFunction{Name: "bash", Description: "run command"}},
	}

	// Compute prefix hash before compaction
	before := llm.ComputeFingerprint(llm.PrefixInput{
		SystemPrompt: systemPrompt,
		Tools:        tools,
	})

	msgs := make([]llm.Message, 20)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}}
	}

	cfg := SemanticCompactionConfig{
		SummaryModel:     "deepseek-v4-flash",
		SummaryTimeout:   15 * time.Second,
		MaxSummaryTokens: 2000,
	}

	res := SemanticCompact(context.Background(), msgs, client, systemPrompt, tools, cfg)
	if !res.UsedSemantic {
		t.Fatal("expected semantic compaction")
	}

	// Compute prefix hash after compaction — should be identical
	after := llm.ComputeFingerprint(llm.PrefixInput{
		SystemPrompt: systemPrompt,
		Tools:        tools,
	})

	if before.CombinedSHA256 != after.CombinedSHA256 {
		t.Errorf("static prefix hash changed: before=%s after=%s", before.CombinedSHA256, after.CombinedSHA256)
	}
	if before.SystemSHA256 != after.SystemSHA256 {
		t.Errorf("system prefix hash changed: before=%s after=%s", before.SystemSHA256, after.SystemSHA256)
	}
	if before.ToolsSHA256 != after.ToolsSHA256 {
		t.Errorf("tools prefix hash changed: before=%s after=%s", before.ToolsSHA256, after.ToolsSHA256)
	}
}

// --- Test: buildSemanticSummaryPrompt contains expected sections ---
func TestBuildSemanticSummaryPrompt(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: "Hello, please help with parser.go"},
		}},
		{Role: "assistant", Blocks: []llm.ContentBlock{
			llm.ThinkingBlock{Text: "I should read the file first"},
			llm.TextBlock{Text: "Reading parser.go now."},
			llm.ToolUseBlock{ID: "t1", Name: "read_file", Input: json.RawMessage(`{}`)},
		}},
		{Role: "tool", Blocks: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "t1", Content: "package main\nfunc main() {}"},
		}},
	}

	prompt := buildSemanticSummaryPrompt(msgs)

	for _, want := range []string{
		"CRITICAL CONSTRAINTS",
		"Pinned skill facts",
		"Current objective",
		"Negative constraints",
		"Changed file paths",
		"Recent tool evidence",
		"<summary>",
		"</summary>",
		"objective:",
		"progress:",
		"key_files:",
		"constraints:",
		"pending:",
		"tool_evidence:",
		"timeline:",
		"[1] user:",
		"[2] assistant:",
		"[3] tool:",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// --- Test: SemanticCompact returns empty when no compaction needed ---
func TestSemanticCompact_EmptyWhenTooFewMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}},
		{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
	}
	cfg := SemanticCompactionConfig{
		SummaryModel:     "deepseek-v4-flash",
		SummaryTimeout:   15 * time.Second,
		MaxSummaryTokens: 2000,
	}
	res := SemanticCompact(context.Background(), msgs, nil, "system", nil, cfg)
	if res.Summary != "" {
		t.Errorf("expected empty result for short message list; got summary=%q", res.Summary)
	}
}

// --- Test: DefaultSemanticCompactionConfig ---
func TestDefaultSemanticCompactionConfig(t *testing.T) {
	cfg := defaultSemanticCompactionConfig()
	if cfg.WarnThreshold != 0.75 {
		t.Errorf("WarnThreshold: got %f want 0.75", cfg.WarnThreshold)
	}
	if cfg.CompactThreshold != 0.80 {
		t.Errorf("CompactThreshold: got %f want 0.80", cfg.CompactThreshold)
	}
	if cfg.ProtectionThreshold != 0.90 {
		t.Errorf("ProtectionThreshold: got %f want 0.90", cfg.ProtectionThreshold)
	}
	if cfg.SummaryModel != "deepseek-v4-flash" {
		t.Errorf("SummaryModel: got %q want deepseek-v4-flash", cfg.SummaryModel)
	}
	if cfg.SummaryTimeout != 15*time.Second {
		t.Errorf("SummaryTimeout: got %v want 15s", cfg.SummaryTimeout)
	}
	if cfg.MaxSummaryTokens != 2000 {
		t.Errorf("MaxSummaryTokens: got %d want 2000", cfg.MaxSummaryTokens)
	}
}
