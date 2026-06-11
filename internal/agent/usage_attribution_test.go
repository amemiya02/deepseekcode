package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/config"
	"github.com/amemiya02/deepseekcode/internal/llmtest"
)

// TestEventTurnUsageEmittedPerRequest verifies that EventTurnUsage is published
// for every LLM request, including escalation re-issues. A scripted two-request
// turn (flash + escalation to pro) must emit two events with the correct model
// IDs and cache-hit values.
func TestEventTurnUsageEmittedPerRequest(t *testing.T) {
	srv := llmtest.NewServer(
		// Turn 1: flash emits NEEDS_PRO to trigger escalation.
		llmtest.Turn{
			Text: "<<<NEEDS_PRO>>>",
			Usage: &llmtest.Usage{
				Prompt:     500,
				Completion: 10,
				CacheHit:   400,
				CacheMiss:  100,
			},
		},
		// Turn 2: escalated pro turn.
		llmtest.Turn{
			Text: "done",
			Usage: &llmtest.Usage{
				Prompt:     500,
				Completion: 20,
				CacheHit:   480,
				CacheMiss:  20,
			},
		},
	)
	defer srv.Close()

	a := newMockLoopAgent(t, srv)
	a.EscalationModel = config.ModelPro

	// Capture EventTurnUsage events from the bus.
	sub := a.Bus().Subscribe(64)
	var (
		mu     sync.Mutex
		events []EventTurnUsage
	)
	go func() {
		for env := range sub.C {
			if e, ok := env.Event.(EventTurnUsage); ok {
				mu.Lock()
				events = append(events, e)
				mu.Unlock()
			}
		}
	}()

	reason, err := a.Run(context.Background(), "solve this")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != StopModelDone {
		t.Fatalf("reason = %v, want StopModelDone", reason)
	}

	// Wait for bus to drain.
	a.bus.Close()

	mu.Lock()
	got := events
	mu.Unlock()

	if len(got) != 2 {
		t.Fatalf("EventTurnUsage count = %d, want 2 (flash + pro)", len(got))
	}

	// First event: flash turn.
	if got[0].Model != config.ModelFlash {
		t.Errorf("event[0].Model = %q, want %q", got[0].Model, config.ModelFlash)
	}
	if got[0].CacheHitTokens != 400 {
		t.Errorf("event[0].CacheHitTokens = %d, want 400", got[0].CacheHitTokens)
	}
	if got[0].CacheMissTokens != 100 {
		t.Errorf("event[0].CacheMissTokens = %d, want 100", got[0].CacheMissTokens)
	}
	if got[0].PromptTokens != 500 {
		t.Errorf("event[0].PromptTokens = %d, want 500", got[0].PromptTokens)
	}
	if got[0].CompletionTokens != 10 {
		t.Errorf("event[0].CompletionTokens = %d, want 10", got[0].CompletionTokens)
	}

	// Second event: escalated pro turn.
	if got[1].Model != config.ModelPro {
		t.Errorf("event[1].Model = %q, want %q", got[1].Model, config.ModelPro)
	}
	if got[1].CacheHitTokens != 480 {
		t.Errorf("event[1].CacheHitTokens = %d, want 480", got[1].CacheHitTokens)
	}
	if got[1].CacheMissTokens != 20 {
		t.Errorf("event[1].CacheMissTokens = %d, want 20", got[1].CacheMissTokens)
	}
	if got[1].PromptTokens != 500 {
		t.Errorf("event[1].PromptTokens = %d, want 500", got[1].PromptTokens)
	}
	if got[1].CompletionTokens != 20 {
		t.Errorf("event[1].CompletionTokens = %d, want 20", got[1].CompletionTokens)
	}
}
