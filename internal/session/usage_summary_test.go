package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func tempStore(t *testing.T) (*Store, func()) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	return store, func() { store.Close() }
}

func TestUsageSummary(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/tmp/test", "deepseek-v4-flash", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Insert two assistant rows with usage.
	for _, msg := range []Message{
		{
			Role:  "assistant",
			Model: "deepseek-v4-flash",
			Usage: llm.Usage{PromptCacheHitTokens: 100, PromptCacheMissTokens: 50, CompletionTokens: 10},
		},
		{
			Role:  "assistant",
			Model: "deepseek-v4-flash",
			Usage: llm.Usage{PromptCacheHitTokens: 200, PromptCacheMissTokens: 30, CompletionTokens: 20},
		},
	} {
		if _, err := store.AppendMessage(ctx, sess.ID, msg); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}

	sum, err := store.UsageSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if sum.Turns != 2 {
		t.Errorf("Turns = %d, want 2", sum.Turns)
	}
	if sum.CacheHitTokens != 300 {
		t.Errorf("CacheHitTokens = %d, want 300", sum.CacheHitTokens)
	}
	if sum.CacheMissTokens != 80 {
		t.Errorf("CacheMissTokens = %d, want 80", sum.CacheMissTokens)
	}
	if sum.OutputTokens != 30 {
		t.Errorf("OutputTokens = %d, want 30", sum.OutputTokens)
	}
	// Cost is computed by llm.Cost on insert; verify it's non-zero.
	if sum.CostYuan <= 0 {
		t.Errorf("CostYuan = %f, want > 0", sum.CostYuan)
	}
	// SavedYuan should be non-zero (cache hits save money).
	if sum.SavedYuan <= 0 {
		t.Errorf("SavedYuan = %f, want > 0", sum.SavedYuan)
	}
}

func TestUsageSummaryEmptySession(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/tmp/test", "deepseek-v4-flash", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	sum, err := store.UsageSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if sum.Turns != 0 || sum.CostYuan != 0 || sum.SavedYuan != 0 {
		t.Errorf("expected zero summary for empty session, got %+v", sum)
	}
}

func TestUsageSummaryUnknownModel(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/tmp/test", "unknown-model", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	msg := Message{
		Role:  "assistant",
		Model: "unknown-model",
		Usage: llm.Usage{PromptCacheHitTokens: 100, PromptCacheMissTokens: 0, CompletionTokens: 0},
	}
	if _, err := store.AppendMessage(ctx, sess.ID, msg); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	sum, err := store.UsageSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	// Unknown model: stored cost should still count, but SavedYuan is 0.
	if sum.CostYuan != 0 {
		t.Errorf("CostYuan = %f for unknown model, want 0 (no pricing entry on insert)", sum.CostYuan)
	}
	if sum.SavedYuan != 0 {
		t.Errorf("SavedYuan = %f for unknown model, want 0", sum.SavedYuan)
	}
	if sum.Turns != 1 {
		t.Errorf("Turns = %d, want 1", sum.Turns)
	}
}

func TestUsageSummaryUserMessagesIgnored(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	sess, err := store.NewSession(ctx, "/tmp/test", "deepseek-v4-flash", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// A user message with usage columns should not count.
	msg := Message{
		Role:  "user",
		Model: "deepseek-v4-flash",
		Usage: llm.Usage{PromptCacheHitTokens: 100, PromptCacheMissTokens: 50, CompletionTokens: 10},
	}
	if _, err := store.AppendMessage(ctx, sess.ID, msg); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	sum, err := store.UsageSummary(ctx, sess.ID)
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if sum.Turns != 0 {
		t.Errorf("Turns = %d, want 0 (user messages ignored)", sum.Turns)
	}
}
