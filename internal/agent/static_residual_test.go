package agent

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
)

func TestLearnStaticResidual_Clamps(t *testing.T) {
	if !tokenizer.Available() {
		t.Skip("tokenizer not available")
	}
	a := &Agent{System: "test system prompt"}
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
	}
	// usage.PromptTokens smaller than sys+conv → residual should clamp to 0.
	a.learnStaticResidual(1, msgs)
	if a.staticPrefixResidual != 0 {
		t.Fatalf("residual = %d, want 0 (clamped)", a.staticPrefixResidual)
	}
}

func TestLearnStaticResidual_OneShot(t *testing.T) {
	if !tokenizer.Available() {
		t.Skip("tokenizer not available")
	}
	a := &Agent{System: "test system prompt"}
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
	}
	// First call sets the residual.
	a.learnStaticResidual(999999, msgs)
	first := a.staticPrefixResidual
	// Second call should not change it.
	a.learnStaticResidual(0, msgs)
	if a.staticPrefixResidual != first {
		t.Fatalf("residual changed on second call: %d → %d", first, a.staticPrefixResidual)
	}
	if !a.staticResidualLearned {
		t.Fatal("staticResidualLearned should be true after first call")
	}
}

func TestLearnStaticResidual_Positive(t *testing.T) {
	if !tokenizer.Available() {
		t.Skip("tokenizer not available")
	}
	a := &Agent{System: "test system prompt"}
	msgs := []llm.Message{
		{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hello"}}},
	}
	sys := tokenizer.Count(a.System)
	conv, _ := tokenizer.CountMessages(msgs)
	target := sys + conv + 1234
	a.learnStaticResidual(target, msgs)
	if a.staticPrefixResidual != 1234 {
		t.Fatalf("residual = %d, want 1234", a.staticPrefixResidual)
	}
}
