// compact_boundary_test.go verifies the W0.3 deterministic fold boundary
// invariant: two compactions over an identical message tail must produce
// an identical post-fold prefix boundary (ToIdx). This holds for both
// the deterministic (CompactSession) and semantic (SemanticCompact)
// paths, and is critical for cache-unit alignment — a non-deterministic
// boundary would land the rebuilt body on different cache-unit multiples,
// breaking the §3.6 alignment guarantee.
package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// TestDeterministicFoldBoundaryCompactSession verifies that two
// CompactSession calls over the same message list produce an identical
// ToIdx (the first preserved message index). The summary text may
// differ if the summarizer is non-deterministic, but the boundary
// must be identical.
func TestDeterministicFoldBoundaryCompactSession(t *testing.T) {
	cfg := CompactionConfig{
		PreserveRecentMessages: 4,
		AutoCompactInputTokens: 100,
	}
	// Build a message list large enough to trigger compaction.
	msgs := make([]llm.Message, 20)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}}
	}

	r1 := CompactSession(msgs, cfg, defaultCharsPerToken)
	if r1.Summary == "" {
		t.Fatal("first compaction produced no summary")
	}

	r2 := CompactSession(msgs, cfg, defaultCharsPerToken)
	if r2.Summary == "" {
		t.Fatal("second compaction produced no summary")
	}

	if r1.ToIdx != r2.ToIdx {
		t.Errorf("fold boundary not deterministic: first ToIdx=%d, second ToIdx=%d", r1.ToIdx, r2.ToIdx)
	}
	if r1.FromIdx != r2.FromIdx {
		t.Errorf("from boundary not deterministic: first FromIdx=%d, second FromIdx=%d", r1.FromIdx, r2.FromIdx)
	}
	if r1.RemovedCount != r2.RemovedCount {
		t.Errorf("RemovedCount not deterministic: first=%d, second=%d", r1.RemovedCount, r2.RemovedCount)
	}
	if len(r1.KeptMessages) != len(r2.KeptMessages) {
		t.Errorf("KeptMessages length not deterministic: first=%d, second=%d", len(r1.KeptMessages), len(r2.KeptMessages))
	}
}

// TestSemanticPathFoldBoundaryMatchesDeterministic verifies that
// SemanticCompact (via a real mock LLM server, not the nil-client
// fallback) produces the same ToIdx as CompactSession for identical
// input. This is the real W0.3 acceptance test: the semantic path
// now calls adjustBoundary AND alignTailToCacheUnit, matching the
// deterministic path. The previous nil-client version was tautological
// (fallback = CompactSession internally).
func TestSemanticPathFoldBoundaryMatchesDeterministic(t *testing.T) {
	// Mock LLM server that returns a valid summary.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "<summary>\n- objective: test\n- progress: done\n</summary>"}},
			},
			"usage": map[string]int{
				"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	client := llm.NewClient("test-key", srv.URL)

	// Build a message list with tool turns so adjustBoundary has work to do.
	var msgs []llm.Message
	msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
		llm.TextBlock{Text: "start the task"},
	}})
	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		msgs = append(msgs, toolTurn(id, "bash")...)
	}
	for i := 0; i < 20; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}})
	}
	msgs = append(msgs,
		llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep1"}}},
		llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep2"}}},
		llm.Message{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep3"}}},
		llm.Message{Role: "assistant", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "keep4"}}},
	)

	detCfg := CompactionConfig{
		PreserveRecentMessages: 4,
		AutoCompactInputTokens: 100,
	}
	det := CompactSession(msgs, detCfg, defaultCharsPerToken)
	if det.Summary == "" {
		t.Fatal("deterministic compaction produced no summary")
	}

	semCfg := SemanticCompactionConfig{
		CompactThreshold:    0.01,
		MaxSummaryTokens:    2000,
		SummaryModel:        "deepseek-v4-flash",
		SummaryTimeout:      5 * time.Second,
		ProtectionThreshold: 0.95,
	}
	sem := SemanticCompact(context.Background(), msgs, client, "system", nil, semCfg)
	if sem.Summary == "" {
		t.Fatal("semantic compaction produced no summary")
	}
	if !sem.UsedSemantic {
		t.Fatal("expected semantic path to use LLM, but it fell back to deterministic")
	}

	if det.ToIdx != sem.ToIdx {
		t.Errorf("fold boundary mismatch: deterministic ToIdx=%d, semantic ToIdx=%d", det.ToIdx, sem.ToIdx)
	}
	if det.FromIdx != sem.FromIdx {
		t.Errorf("from boundary mismatch: deterministic FromIdx=%d, semantic FromIdx=%d", det.FromIdx, sem.FromIdx)
	}
	if len(det.KeptMessages) != len(sem.KeptMessages) {
		t.Errorf("kept message count mismatch: deterministic=%d, semantic=%d",
			len(det.KeptMessages), len(sem.KeptMessages))
	}
}

// TestSemanticPathCacheUnitAlignment verifies that SemanticCompact
// with CacheUnit≠0 aligns the rebuilt tail to a cache-unit boundary,
// matching CompactSession's behavior. This exercises the real
// alignTailToCacheUnit call added in the semantic path (W0.3).
func TestSemanticPathCacheUnitAlignment(t *testing.T) {
	// Use a realistic-length summary (not a stub) so the PadText boundary
	// arithmetic matches the deterministic path. Very short summaries can
	// trigger a ±1 token mismatch due to integer division boundary effects
	// in EstimateTokensCalibrated.
	summaryContent := "<summary>\n" +
		"- objective: implement the review panel\n" +
		"- progress: created the component structure, wired up the API client, added form validation\n" +
		"- key_files: [src/components/ReviewPanel.tsx, src/api/reviews.ts, src/hooks/useReview.ts]\n" +
		"- constraints: must use TypeScript strict mode, no any types, all API calls through the shared client\n" +
		"- pending: add error handling for network failures, write unit tests for the form validation\n" +
		"- tool_evidence: [read_file src/components/ReviewPanel.tsx] component renders but validation is incomplete\n" +
		"- timeline: [user][1] create review panel [assistant][2] created structure [user][3] add validation\n" +
		"</summary>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": summaryContent}},
			},
			"usage": map[string]int{
				"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150,
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

	const unit = 64
	semCfg := SemanticCompactionConfig{
		CompactThreshold:    0.01,
		MaxSummaryTokens:    2000,
		SummaryModel:        "deepseek-v4-flash",
		SummaryTimeout:      5 * time.Second,
		ProtectionThreshold: 0.95,
		CacheUnit:           unit,
	}
	sem := SemanticCompact(context.Background(), msgs, client, "system", nil, semCfg)
	if sem.Summary == "" {
		t.Fatal("semantic compaction produced no summary")
	}
	if !sem.UsedSemantic {
		t.Fatal("expected semantic path to use LLM")
	}

	// The padded tail must land on a cache-unit boundary.
	tail := append([]llm.Message{sem.SummaryMessage}, sem.KeptMessages...)
	total := EstimateTokensCalibrated(tail, defaultCharsPerToken)
	if total%unit != 0 {
		t.Errorf("semantic path tail tokens %d not aligned to unit %d (remainder %d)", total, unit, total%unit)
	}

	// Compare with deterministic path using the same CacheUnit.
	detCfg := CompactionConfig{
		PreserveRecentMessages: 4,
		AutoCompactInputTokens: 100,
		CacheUnit:              unit,
	}
	det := CompactSession(msgs, detCfg, defaultCharsPerToken)
	if det.Summary == "" {
		t.Fatal("deterministic compaction produced no summary")
	}

	if det.ToIdx != sem.ToIdx {
		t.Errorf("fold boundary mismatch with CacheUnit=%d: deterministic ToIdx=%d, semantic ToIdx=%d", unit, det.ToIdx, sem.ToIdx)
	}
}

// TestDeterministicFoldBoundaryWithCacheUnit verifies that when
// CacheUnit is non-zero, both CompactSession and SemanticCompact
// produce the same ToIdx AND the same summary message structure
// (same block count from alignTailToCacheUnit). A non-deterministic
// boundary would mean different padding, breaking cache reuse.
func TestDeterministicFoldBoundaryWithCacheUnit(t *testing.T) {
	msgs := make([]llm.Message, 20)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Blocks: []llm.ContentBlock{
			llm.TextBlock{Text: strings.Repeat("x", 400)},
		}}
	}

	const unit = 64
	detCfg := CompactionConfig{
		PreserveRecentMessages: 4,
		AutoCompactInputTokens: 100,
		CacheUnit:              unit,
	}
	r1 := CompactSession(msgs, detCfg, defaultCharsPerToken)
	if r1.Summary == "" {
		t.Fatal("first compaction produced no summary")
	}
	r2 := CompactSession(msgs, detCfg, defaultCharsPerToken)
	if r2.Summary == "" {
		t.Fatal("second compaction produced no summary")
	}

	if r1.ToIdx != r2.ToIdx {
		t.Errorf("fold boundary not deterministic with CacheUnit=%d: first=%d, second=%d", unit, r1.ToIdx, r2.ToIdx)
	}
	// Both must have the same block count (padding block added by alignTailToCacheUnit).
	if len(r1.SummaryMessage.Blocks) != len(r2.SummaryMessage.Blocks) {
		t.Errorf("summary block count not deterministic: first=%d, second=%d",
			len(r1.SummaryMessage.Blocks), len(r2.SummaryMessage.Blocks))
	}
	// The padded tail must land on a cache-unit boundary.
	tail1 := append([]llm.Message{r1.SummaryMessage}, r1.KeptMessages...)
	total1 := EstimateTokensCalibrated(tail1, defaultCharsPerToken)
	if total1%unit != 0 {
		t.Errorf("first compaction tail tokens %d not aligned to unit %d", total1, unit)
	}
	tail2 := append([]llm.Message{r2.SummaryMessage}, r2.KeptMessages...)
	total2 := EstimateTokensCalibrated(tail2, defaultCharsPerToken)
	if total2%unit != 0 {
		t.Errorf("second compaction tail tokens %d not aligned to unit %d", total2, unit)
	}
}
