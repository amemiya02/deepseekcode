package agent

import (
	"math"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// T4.1 — calibrated token estimation. These tests pin that the cold-start path
// is byte-identical to char/4, that the calibrated ratio scales the estimate
// (and thus the compaction trigger and budget projection), and that
// calibration from provider usage is clamped/EMA-smoothed and never poisoned by
// a missing usage frame.

// msgsWithChars returns one user message whose single TextBlock has n 'x' chars.
func msgsWithChars(n int) []llm.Message {
	return []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: strings.Repeat("x", n)}}}}
}

func TestEstimateTokensCalibrated(t *testing.T) {
	msgs := msgsWithChars(400) // 400 chars, 1 message

	// (a) ratio 4.0 reproduces the cold-start EstimateTokens exactly.
	if cal, base := EstimateTokensCalibrated(msgs, 4.0), EstimateTokens(msgs); cal != base {
		t.Errorf("ratio 4.0 = %d, want EstimateTokens = %d", cal, base)
	}

	// (b) ratio 2.0 roughly doubles the char-derived component (400/2=200 vs
	// 400/4=100), plus the constant per-message overhead.
	wantHalfRatio := perMessageOverhead + 400/2
	if got := EstimateTokensCalibrated(msgs, 2.0); got != wantHalfRatio {
		t.Errorf("ratio 2.0 = %d, want %d (overhead + 400/2)", got, wantHalfRatio)
	}

	// (c) non-positive ratio falls back to the cold-start prior.
	if zero, base := EstimateTokensCalibrated(msgs, 0), EstimateTokensCalibrated(msgs, defaultCharsPerToken); zero != base {
		t.Errorf("ratio 0 = %d, want cold-start = %d", zero, base)
	}

	// (d) per-message overhead is added regardless of ratio (empty text → just overhead).
	empty := []llm.Message{{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: ""}}}}
	if got := EstimateTokensCalibrated(empty, 2.0); got != perMessageOverhead {
		t.Errorf("empty message = %d, want perMessageOverhead = %d", got, perMessageOverhead)
	}
}

func TestContextPressureScalesWithRatio(t *testing.T) {
	msgs := msgsWithChars(4000)
	p4 := ContextPressure(msgs, 1_000_000, 4.0)
	p2 := ContextPressure(msgs, 1_000_000, 2.0)
	if !(p2 > p4) {
		t.Fatalf("a smaller chars-per-token ratio must raise pressure (CJK fires earlier): p2=%v p4=%v", p2, p4)
	}
	// The char-derived component doubles; with large text it dominates the fixed
	// overhead, so p2 ≈ 2*p4.
	if ratio := p2 / p4; ratio < 1.8 || ratio > 2.05 {
		t.Errorf("pressure ratio p2/p4 = %v, want ~2.0", ratio)
	}
}

func TestProjectedTurnCostCNYUsesCalibratedRatio(t *testing.T) {
	req := llm.Request{Messages: msgsWithChars(40_000), MaxTokens: 1000}
	c4 := ProjectedTurnCostCNY("deepseek-v4-flash", req, 4.0, 0)
	c2 := ProjectedTurnCostCNY("deepseek-v4-flash", req, 2.0, 0)
	if !(c4 > 0 && c2 > c4) {
		t.Errorf("smaller ratio must raise projected cost (more input tokens): c4=%v c2=%v", c4, c2)
	}
}

func TestCalibrateCharsPerTokenColdStart(t *testing.T) {
	a := &Agent{}
	a.calibrateCharsPerToken(msgsWithChars(400), llm.Usage{PromptTokens: 100})
	if a.charsPerToken != 4.0 {
		t.Errorf("cold-start ratio = %v, want 4.0 (400 chars / 100 tokens, seeded directly)", a.charsPerToken)
	}
}

func TestCalibrateCharsPerTokenEMA(t *testing.T) {
	a := &Agent{charsPerToken: 4.0} // already seeded
	// Second frame observes 200/100 = 2.0; EMA = 0.3*2.0 + 0.7*4.0 = 3.4.
	a.calibrateCharsPerToken(msgsWithChars(200), llm.Usage{PromptTokens: 100})
	want := emaAlpha*2.0 + (1-emaAlpha)*4.0
	if math.Abs(a.charsPerToken-want) > 1e-9 {
		t.Errorf("EMA ratio = %v, want %v", a.charsPerToken, want)
	}
}

func TestCalibrateCharsPerTokenZeroUsageIsNoop(t *testing.T) {
	a := &Agent{charsPerToken: 5.0}
	a.calibrateCharsPerToken(msgsWithChars(400), llm.Usage{PromptTokens: 0}) // no usage frame
	if a.charsPerToken != 5.0 {
		t.Errorf("zero PromptTokens must leave the prior ratio intact; got %v want 5.0", a.charsPerToken)
	}
}

func TestCalibrateCharsPerTokenClamps(t *testing.T) {
	// Absurdly LOW observed ratio (100 chars / 10000 tokens = 0.01) clamps to 1.0.
	low := &Agent{}
	low.calibrateCharsPerToken(msgsWithChars(100), llm.Usage{PromptTokens: 10_000})
	if low.charsPerToken != 1.0 {
		t.Errorf("low observed ratio should clamp to 1.0; got %v", low.charsPerToken)
	}
	// Absurdly HIGH observed ratio (1e6 chars / 1 token) clamps to 12.0.
	high := &Agent{}
	high.calibrateCharsPerToken(msgsWithChars(1_000_000), llm.Usage{PromptTokens: 1})
	if high.charsPerToken != 12.0 {
		t.Errorf("high observed ratio should clamp to 12.0; got %v", high.charsPerToken)
	}
}

// TestCalibrateCharsPerTokenStableUnderAlternatingPayloads is the
// anti-oscillation regression the design review asked for: alternating
// CJK-like (low ratio) and code-like (high ratio) frames must keep the EMA
// bounded inside the clamp band, never running away or going non-finite.
func TestCalibrateCharsPerTokenStableUnderAlternatingPayloads(t *testing.T) {
	a := &Agent{}
	// Observed ratios alternate ~1.5 (CJK) and ~3.5 (code) across many turns.
	for i := 0; i < 40; i++ {
		chars, tokens := 150, 100 // ratio 1.5
		if i%2 == 1 {
			chars, tokens = 350, 100 // ratio 3.5
		}
		a.calibrateCharsPerToken(msgsWithChars(chars), llm.Usage{PromptTokens: tokens})
		if math.IsNaN(a.charsPerToken) || math.IsInf(a.charsPerToken, 0) {
			t.Fatalf("ratio went non-finite at turn %d", i)
		}
		if a.charsPerToken < 1.0 || a.charsPerToken > 12.0 {
			t.Fatalf("ratio escaped the clamp band at turn %d: %v", i, a.charsPerToken)
		}
	}
	// The EMA of values in [1.5, 3.5] must settle inside that interval.
	if a.charsPerToken < 1.5 || a.charsPerToken > 3.5 {
		t.Errorf("EMA settled outside the observed band [1.5,3.5]: %v", a.charsPerToken)
	}
}
