package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestCheckBudget_DefaultPolicyAllows(t *testing.T) {
	// Zero policy (default) should always allow
	policy := BudgetPolicy{}
	state := BudgetState{SpentCNY: 100.0}

	allow, warn := CheckBudget(policy, state, 1.0)
	if !allow {
		t.Error("default zero policy should allow turns")
	}
	if warn {
		t.Error("default zero policy should not warn")
	}
}

func TestCheckBudget_HardThresholdBlocks(t *testing.T) {
	policy := BudgetPolicy{HardCNY: 2.0}
	state := BudgetState{SpentCNY: 1.9}

	// Projected 0.2 brings total to 2.1 >= HardCNY 2.0
	allow, _ := CheckBudget(policy, state, 0.2)
	if allow {
		t.Error("hard threshold should block when total >= HardCNY")
	}
}

func TestCheckBudget_WarnThresholdEmitsOnce(t *testing.T) {
	policy := BudgetPolicy{WarnCNY: 1.0, HardCNY: 10.0}
	state := BudgetState{SpentCNY: 0.9, Warned: false}

	// First check: crossing warn threshold
	allow, warn := CheckBudget(policy, state, 0.2)
	if !allow {
		t.Error("warn threshold should not block")
	}
	if !warn {
		t.Error("should warn on first crossing of WarnCNY")
	}

	// Second check: already warned
	state.Warned = true
	allow, warn = CheckBudget(policy, state, 0.1)
	if !allow {
		t.Error("should still allow after warning")
	}
	if warn {
		t.Error("should not warn again after Warned=true")
	}
}

func TestCheckBudget_ExactEquality(t *testing.T) {
	// Exact equality with thresholds should trigger
	t.Run("warn exact", func(t *testing.T) {
		policy := BudgetPolicy{WarnCNY: 1.0}
		state := BudgetState{SpentCNY: 0.5, Warned: false}

		_, warn := CheckBudget(policy, state, 0.5)
		if !warn {
			t.Error("exact equality with WarnCNY should warn")
		}
	})

	t.Run("hard exact", func(t *testing.T) {
		policy := BudgetPolicy{HardCNY: 1.0}
		state := BudgetState{SpentCNY: 0.5}

		allow, _ := CheckBudget(policy, state, 0.5)
		if allow {
			t.Error("exact equality with HardCNY should block")
		}
	})
}

func TestCheckBudget_NegativeProjected(t *testing.T) {
	// Negative projected cost treated as zero
	policy := BudgetPolicy{WarnCNY: 1.0, HardCNY: 2.0}
	state := BudgetState{SpentCNY: 0.5, Warned: false}

	allow, warn := CheckBudget(policy, state, -0.5)
	if !allow {
		t.Error("negative projected should be treated as zero, allowing turn")
	}
	if warn {
		t.Error("negative projected should not trigger warning")
	}
}

func TestCheckBudget_WarnCNYOnly(t *testing.T) {
	// Warn-only policy (no hard limit)
	policy := BudgetPolicy{WarnCNY: 1.0}
	state := BudgetState{SpentCNY: 0.5, Warned: false}

	allow, warn := CheckBudget(policy, state, 1.0)
	if !allow {
		t.Error("warn-only policy should never block")
	}
	if !warn {
		t.Error("should warn when crossing WarnCNY")
	}

	// Even well beyond warn threshold, should still allow
	state.Warned = true
	allow, warn = CheckBudget(policy, state, 10.0)
	if !allow {
		t.Error("warn-only policy should allow even far beyond threshold")
	}
	if warn {
		t.Error("should not warn again after Warned=true")
	}
}

func TestCheckBudget_HardCNYOnly(t *testing.T) {
	// Hard-only policy (no warn threshold)
	policy := BudgetPolicy{HardCNY: 2.0}
	state := BudgetState{SpentCNY: 1.0}

	allow, warn := CheckBudget(policy, state, 0.5)
	if !allow {
		t.Error("below HardCNY should allow")
	}
	if warn {
		t.Error("hard-only policy should never warn below threshold")
	}

	allow, warn = CheckBudget(policy, state, 1.0)
	if allow {
		t.Error("at HardCNY should block")
	}
	if warn {
		t.Error("hard-only policy should never warn, even at hard threshold")
	}
}

func TestCheckBudget_BothThresholds(t *testing.T) {
	// Policy with both warn and hard thresholds
	policy := BudgetPolicy{WarnCNY: 1.0, HardCNY: 2.0}

	tests := []struct {
		name      string
		spent     float64
		projected float64
		warned    bool
		wantAllow bool
		wantWarn  bool
	}{
		{"below both", 0.5, 0.2, false, true, false},
		{"at warn", 0.8, 0.2, false, true, true},
		{"above warn not at hard", 1.2, 0.2, true, true, false},
		{"at hard", 1.8, 0.2, false, false, true},
		{"at hard already warned", 1.8, 0.2, true, false, false},
		{"above hard", 2.5, 0.1, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := BudgetState{SpentCNY: tt.spent, Warned: tt.warned}
			allow, warn := CheckBudget(policy, state, tt.projected)
			if allow != tt.wantAllow {
				t.Errorf("allow = %v, want %v", allow, tt.wantAllow)
			}
			if warn != tt.wantWarn {
				t.Errorf("warn = %v, want %v", warn, tt.wantWarn)
			}
		})
	}
}

// Integration tests for budget gate in Agent.runStep

func TestBudgetHardBlockPreventsStream(t *testing.T) {
	var streamCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamCalls++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "test-model")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(1)}

	// Set hard budget that blocks immediately (spent >= hard)
	a.BudgetPolicy = BudgetPolicy{HardCNY: 1.0}
	a.BudgetState = BudgetState{SpentCNY: 1.0}

	_, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if streamCalls != 0 {
		t.Errorf("Client.Stream should not be called when hard budget blocks, got %d calls", streamCalls)
	}
}

func TestBudgetWarnEmitsOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
		)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "test-model")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(3)}

	// Set warn budget that triggers on first step (spent + projected >= warn)
	// Since projectedCNY is 0.0 in runStep, spent must be >= warn threshold
	a.BudgetPolicy = BudgetPolicy{WarnCNY: 0.5, HardCNY: 100.0}
	a.BudgetState = BudgetState{SpentCNY: 0.5}

	// Collect events synchronously with bounded drain after Run
	sub := a.Bus().Subscribe(256)

	_, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Drain events synchronously with deadline
	var infoEvents []string
	deadline := time.After(time.Second)
done:
	for {
		select {
		case env := <-sub.C:
			if info, ok := env.Event.(EventInfo); ok {
				infoEvents = append(infoEvents, info.Text)
			}
		case <-deadline:
			break done
		default:
			break done
		}
	}

	// Count budget warning events
	warnCount := 0
	for _, msg := range infoEvents {
		if msg == "budget warning: projected session cost reached warning threshold" {
			warnCount++
		}
	}

	if warnCount != 1 {
		t.Errorf("expected exactly 1 budget warning event, got %d", warnCount)
	}

	// Verify Warned was set
	if !a.BudgetState.Warned {
		t.Error("BudgetState.Warned should be true after warning")
	}
}

func TestBudgetAccumulatesObservedSpend(t *testing.T) {
	// Use deepseek-v4-flash which has pricing defined
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Usage: prompt_tokens=1000, completion_tokens=500, cache_hit=0
		// Cost for deepseek-v4-flash: (0*0.02 + 1000*1.0 + 500*2.0) / 1_000_000 = 0.002
		emitSSE(w,
			`{"choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":1000,"total_tokens":1500}}`,
		)
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "deepseek-v4-flash")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(1)}

	// Start with zero spend
	a.BudgetPolicy = BudgetPolicy{WarnCNY: 100.0, HardCNY: 200.0}
	a.BudgetState = BudgetState{SpentCNY: 0}

	_, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Expected cost: (0*0.02 + 1000*1.0 + 500*2.0) / 1_000_000 = 0.002
	expectedCost := llm.Cost("deepseek-v4-flash", llm.Usage{
		PromptCacheHitTokens:  0,
		PromptCacheMissTokens: 1000,
		CompletionTokens:      500,
	})

	if a.BudgetState.SpentCNY != expectedCost {
		t.Errorf("BudgetState.SpentCNY = %v, want %v", a.BudgetState.SpentCNY, expectedCost)
	}

	if a.BudgetState.SpentCNY <= 0 {
		t.Error("BudgetState.SpentCNY should have increased from observed usage")
	}
}

func TestBudgetWarnFromAccumulatedSpend(t *testing.T) {
	// Step 1: return tool call (accumulates 0.2 CNY, SpentCNY becomes 0.2)
	// Step 2: budget check sees SpentCNY=0.2 >= WarnCNY=0.15, emits warning, then returns stop
	var stepCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stepCount++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if stepCount == 1 {
			// Return a tool call so the loop continues to step 2
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":100000,"completion_tokens":50000,"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":100000,"total_tokens":150000}}`,
			)
		} else {
			// Step 2: return stop
			emitSSE(w,
				`{"choices":[{"index":0,"delta":{"content":"done"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":50000,"completion_tokens":25000,"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":50000,"total_tokens":75000}}`,
			)
		}
	}))
	defer srv.Close()

	client := llm.NewClient("k", srv.URL)
	reg := tools.New()
	reg.Register(echoTool{})
	pol := permissions.New(permissions.ModeYolo, "", nil, nil, nil)
	a := New(client, reg, pol, "deepseek-v4-flash")
	a.System = "sys"
	a.StopWhen = []StopCondition{MaxSteps(5)}

	// Warn at 0.15 CNY — step 1 accumulates 0.2 CNY, so step 2 budget check should warn
	a.BudgetPolicy = BudgetPolicy{WarnCNY: 0.15, HardCNY: 10.0}
	a.BudgetState = BudgetState{SpentCNY: 0}

	// Collect events synchronously
	sub := a.Bus().Subscribe(256)

	_, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Drain events
	var infoEvents []string
	deadline := time.After(time.Second)
done:
	for {
		select {
		case env := <-sub.C:
			if info, ok := env.Event.(EventInfo); ok {
				infoEvents = append(infoEvents, info.Text)
			}
		case <-deadline:
			break done
		default:
			break done
		}
	}

	// Count budget warning events
	warnCount := 0
	for _, msg := range infoEvents {
		if msg == "budget warning: projected session cost reached warning threshold" {
			warnCount++
		}
	}

	if warnCount != 1 {
		t.Errorf("expected exactly 1 budget warning from accumulated spend, got %d", warnCount)
	}

	// Verify Warned was set
	if !a.BudgetState.Warned {
		t.Error("BudgetState.Warned should be true after warning from accumulated spend")
	}

	// Verify spend actually accumulated from step 1 (0.2 CNY) + step 2 (0.15 CNY) = 0.35 CNY
	if a.BudgetState.SpentCNY <= 0 {
		t.Error("BudgetState.SpentCNY should have accumulated from observed usage")
	}

	// Verify we ran 2 steps (tool call + stop)
	if stepCount != 2 {
		t.Errorf("expected 2 steps, got %d", stepCount)
	}
}
