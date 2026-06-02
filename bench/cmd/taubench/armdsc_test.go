package taubench

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llmtest"
)

// mustMarshalRunResult marshals rr to JSON, failing the test on error.
func mustMarshalRunResult(t *testing.T, rr RunResult) string {
	t.Helper()
	b, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("marshal RunResult: %v", err)
	}
	return string(b)
}

// contains reports whether sub appears in s.
func contains(s, sub string) bool { return strings.Contains(s, sub) }

// TestRunResultJSONShape pins the camelCase wire field names so the dsc arm
// and the reasonix arm serialize byte-compatibly with the Reasonix
// types.ts RunResult (the report aggregator depends on this).
func TestRunResultJSONShape(t *testing.T) {
	rr := RunResult{
		TaskID:              "t01",
		Mode:                "dsc",
		Pass:                true,
		Turns:               3,
		ToolCalls:           2,
		CacheHitRatio:       0.9,
		CostUsd:             0.0001,
		ClaudeEquivalentUsd: 0.0005,
		PromptTokens:        1000,
		CompletionTokens:    50,
		Truncated:           false,
		FinalAgentMessage:   "done",
	}
	b := mustMarshalRunResult(t, rr)
	for _, key := range []string{
		`"taskId"`, `"mode"`, `"pass"`, `"turns"`, `"toolCalls"`,
		`"cacheHitRatio"`, `"costUsd"`, `"claudeEquivalentUsd"`,
		`"promptTokens"`, `"completionTokens"`, `"truncated"`,
		`"finalAgentMessage"`,
	} {
		if !contains(b, key) {
			t.Errorf("RunResult JSON missing key %s; got %s", key, b)
		}
	}
}

// TestRunDSCArmMockedSingleTurn proves the whole dsc arm wiring compiles and
// runs end-to-end against the offline mock DeepSeek server with no network:
// the user-sim opens, the agent calls a retail tool and answers, the user-sim
// STOPs, the world mutation lands, the Check passes, and a RunResult comes back
// with usage summed from the agent's EventStepFinish stream.
func TestRunDSCArmMockedSingleTurn(t *testing.T) {
	// Agent server: turn 1 calls update_address on o_1002 (a processing order,
	// so the mutation succeeds); turn 2 confirms. Two model calls = two
	// EventStepFinish frames whose usage we sum.
	agentSrv := llmtest.NewServer(
		llmtest.Turn{
			ToolCalls: []llmtest.ToolCall{{
				ID:   "call_1",
				Name: "update_address",
				Args: `{"orderId":"o_1002","address":"5 Birch Rd, NYC, NY 10001"}`,
			}},
			Finish: "tool_calls",
			Usage:  &llmtest.Usage{Prompt: 1000, Completion: 20, CacheHit: 900, CacheMiss: 100},
		},
		llmtest.Turn{
			Text:  "Your address on o_1002 is updated to 5 Birch Rd, NYC, NY 10001.",
			Usage: &llmtest.Usage{Prompt: 1100, Completion: 30, CacheHit: 1000, CacheMiss: 100},
		},
	)
	defer agentSrv.Close()

	// User-sim is driven by a deterministic scripted client: opening message,
	// then ##STOP##. No network.
	sim := &scriptedSimClient{replies: []string{
		"Hi, I'd like to change the address on my order o_1002.",
		"##STOP##",
	}}

	task := Tasks[0] // t01_address_happy → check passes when o_1002 address changes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rr, err := runDSCArm(ctx, dscArmConfig{
		agentClient: agentSrv.Client(),
		simClient:   sim,
		task:        task,
		cwd:         t.TempDir(),
	})
	if err != nil {
		t.Fatalf("runDSCArm: %v", err)
	}

	if rr.TaskID != "t01_address_happy" {
		t.Errorf("TaskID = %q, want t01_address_happy", rr.TaskID)
	}
	if rr.Mode != "dsc" {
		t.Errorf("Mode = %q, want dsc", rr.Mode)
	}
	if !rr.Pass {
		t.Errorf("Pass = false, want true — the address mutation should have landed")
	}
	if rr.Turns != 1 {
		t.Errorf("Turns = %d, want 1 (one user→agent exchange before STOP)", rr.Turns)
	}
	if rr.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", rr.ToolCalls)
	}
	if rr.Truncated {
		t.Errorf("Truncated = true, want false — STOP arrived before maxTurns")
	}
	// Usage is summed across both EventStepFinish frames: prompt 1000+1100,
	// completion 20+30.
	if rr.PromptTokens != 2100 {
		t.Errorf("PromptTokens = %d, want 2100", rr.PromptTokens)
	}
	if rr.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", rr.CompletionTokens)
	}
	// cacheHitRatio = hits/(hits+misses) over the summed usage = 1900/2100.
	if got := rr.CacheHitRatio; got < 0.90 || got > 0.91 {
		t.Errorf("CacheHitRatio = %v, want ~0.905 (1900/2100)", got)
	}
	// costUsd is computed via llm.Cost(flash, summed usage) → ¥, converted to
	// USD; it must be > 0 and finite.
	if rr.CostUsd <= 0 {
		t.Errorf("CostUsd = %v, want > 0", rr.CostUsd)
	}
	if rr.FinalAgentMessage == "" {
		t.Errorf("FinalAgentMessage is empty, want the agent's closing text")
	}
}

// scriptedSimClient is a deterministic chatClient for the user-sim side: it
// returns its scripted replies in order, ignoring the request. No network.
type scriptedSimClient struct {
	replies []string
	idx     int
}

func (s *scriptedSimClient) Chat(_ context.Context, _ simChatRequest) (string, error) {
	if s.idx >= len(s.replies) {
		return "##STOP##", nil
	}
	r := s.replies[s.idx]
	s.idx++
	return r, nil
}
