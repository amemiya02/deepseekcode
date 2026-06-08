package cache

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// helper to build a minimal StaticPrefix.
func pfx(sys string, toolNames ...string) llm.StaticPrefix {
	tools := make([]llm.Tool, len(toolNames))
	for i, n := range toolNames {
		tools[i] = llm.Tool{Type: "function", Function: llm.ToolFunction{Name: n}}
	}
	return llm.StaticPrefix{System: sys, Tools: tools}
}

func TestAttribute_DecisionTree(t *testing.T) {
	const model = "deepseek-v4-flash"

	tests := []struct {
		name     string
		input    Input
		wantCause Cause
	}{
		{
			name: "cold first: no prior prefix",
			input: Input{
				Turn:  1,
				Model: model,
				Prev:  nil, // cold
				Cur:   pfx("sys"),
				Usage: llm.Usage{PromptCacheHitTokens: 0, PromptCacheMissTokens: 5000, CompletionTokens: 200},
			},
			wantCause: CauseColdFirst,
		},
		{
			name: "compaction reset: epoch bumped",
			input: Input{
				Turn:      5,
				Model:     model,
				Prev:      pfxPtr(pfx("sys")),
				Cur:       pfx("sys"),
				PrevEpoch: Epoch(1),
				CurEpoch:  Epoch(2), // bumped
				Usage:     llm.Usage{PromptCacheHitTokens: 100, PromptCacheMissTokens: 4000, CompletionTokens: 150},
			},
			wantCause: CauseCompactReset,
		},
		{
			name: "prefix mutation: system changed",
			input: Input{
				Turn:      3,
				Model:     model,
				Prev:      pfxPtr(pfx("sys-old")),
				Cur:       pfx("sys-new"),
				PrevEpoch: Epoch(1),
				CurEpoch:  Epoch(1),
				Usage:     llm.Usage{PromptCacheHitTokens: 0, PromptCacheMissTokens: 3000, CompletionTokens: 100},
			},
			wantCause: CausePrefixMut,
		},
		{
			name: "steady: stable prefix, same epoch",
			input: Input{
				Turn:      4,
				Model:     model,
				Prev:      pfxPtr(pfx("sys")),
				Cur:       pfx("sys"),
				PrevEpoch: Epoch(1),
				CurEpoch:  Epoch(1),
				Usage:     llm.Usage{PromptCacheHitTokens: 8000, PromptCacheMissTokens: 200, CompletionTokens: 100},
			},
			wantCause: CauseSteady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Attribute(tt.input)
			if got.Dominant != tt.wantCause {
				t.Errorf("Dominant = %q, want %q", got.Dominant, tt.wantCause)
			}
			if got.CostCNY <= 0 {
				t.Errorf("CostCNY = %v, want > 0 (flash pricing should produce a positive cost)", got.CostCNY)
			}
		})
	}
}

func TestAttribute_ResidualEstimate(t *testing.T) {
	// With Unit=128 and TailTokens=300, the incomplete tail block is 300%128=44.
	in := Input{
		Turn:       10,
		Model:      "deepseek-v4-flash",
		Prev:       pfxPtr(pfx("sys")),
		Cur:        pfx("sys"),
		PrevEpoch:  Epoch(1),
		CurEpoch:   Epoch(1),
		Unit:       128,
		TailTokens: 300,
		Usage:      llm.Usage{PromptCacheHitTokens: 5000, PromptCacheMissTokens: 300, CompletionTokens: 100},
	}

	got := Attribute(in)
	if got.ResidualEst != 300%128 {
		t.Errorf("ResidualEst = %d, want %d", got.ResidualEst, 300%128)
	}
	if got.Dominant != CauseSteady {
		t.Errorf("Dominant = %q, want %q", got.Dominant, CauseSteady)
	}
}

func TestAttribute_ResidualDisabledWhenUnitUnknown(t *testing.T) {
	// Unit=0 should disable the residual estimate, yielding 0.
	in := Input{
		Turn:       10,
		Model:      "deepseek-v4-flash",
		Prev:       pfxPtr(pfx("sys")),
		Cur:        pfx("sys"),
		PrevEpoch:  Epoch(1),
		CurEpoch:   Epoch(1),
		Unit:       0, // disabled
		TailTokens: 300,
		Usage:      llm.Usage{PromptCacheHitTokens: 5000, PromptCacheMissTokens: 300, CompletionTokens: 100},
	}

	got := Attribute(in)
	if got.ResidualEst != 0 {
		t.Errorf("ResidualEst = %d, want 0 when Unit=0", got.ResidualEst)
	}
	if got.Dominant != CauseSteady {
		t.Errorf("Dominant = %q, want %q", got.Dominant, CauseSteady)
	}
}

// pfxPtr returns a pointer to the given StaticPrefix.
func pfxPtr(p llm.StaticPrefix) *llm.StaticPrefix {
	return &p
}
