package gateway

import (
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

func TestMapAgentEventNames(t *testing.T) {
	cases := []struct {
		in   acp.AgentEvent
		name string
	}{
		{acp.AgentEvent{Kind: acp.EventKindTextDelta, Text: "hi"}, "message_delta"},
		// EventKindInfo has no dedicated Contract-2 event; mapped to message_delta.
		{acp.AgentEvent{Kind: acp.EventKindInfo, Text: "step"}, "message_delta"},
		{acp.AgentEvent{Kind: acp.EventKindToolStart, ToolName: "write_file"}, "tool_start"},
		{acp.AgentEvent{Kind: acp.EventKindToolEnd, ToolCallID: "c1"}, "tool_end"},
		{acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"}, "turn_done"},
	}
	for _, c := range cases {
		got := mapAgentEvent(c.in)
		if got.name != c.name {
			t.Errorf("kind %d -> name %q, want %q", c.in.Kind, got.name, c.name)
		}
	}
}

func TestMapMessageDeltaPayload(t *testing.T) {
	ev := mapAgentEvent(acp.AgentEvent{Kind: acp.EventKindTextDelta, Text: "hello"})
	var p struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatalf("message_delta data not JSON: %v (%q)", err, ev.data)
	}
	if p.Text != "hello" {
		t.Errorf("message_delta text = %q, want %q", p.Text, "hello")
	}
}

func TestMapTurnDonePayload(t *testing.T) {
	ev := mapAgentEvent(acp.AgentEvent{Kind: acp.EventKindDone, StopReason: "end_turn"})
	var p struct {
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatalf("turn_done data not JSON: %v (%q)", err, ev.data)
	}
	if p.StopReason != "end_turn" {
		t.Errorf("turn_done stop_reason = %q, want %q", p.StopReason, "end_turn")
	}
}

func TestMapLiveSignalNames(t *testing.T) {
	cases := []struct {
		in   acp.AgentEvent
		name string
	}{
		{acp.AgentEvent{Kind: acp.EventKindCache, TurnPct: 0.94, AvgPct: 0.9, Prefixes: 1}, "cache_update"},
		{acp.AgentEvent{Kind: acp.EventKindCost, TurnCNY: 0.1, SessionCNY: 0.3, OutputTokens: 50}, "cost_update"},
		{acp.AgentEvent{Kind: acp.EventKindRouting, From: "deepseek-v4-flash", To: "deepseek-v4-pro", Reason: "marker"}, "routing"},
		{acp.AgentEvent{Kind: acp.EventKindJob, Running: 2}, "job_update"},
		{acp.AgentEvent{Kind: acp.EventKindRetry, Attempt: 1, Max: 1}, "retry"},
		{acp.AgentEvent{Kind: acp.EventKindThinking, Text: "hmm"}, "thinking_delta"},
		{acp.AgentEvent{Kind: acp.EventKindToolDelta, ToolCallID: "c1", ToolDelta: "out"}, "tool_delta"},
		{acp.AgentEvent{Kind: acp.EventKindPlan, Plan: []acp.PlanItem{{Text: "do", Status: "in_progress"}}}, "plan_update"},
	}
	for _, c := range cases {
		if got := mapAgentEvent(c.in); got.name != c.name {
			t.Errorf("kind %d -> %q, want %q", c.in.Kind, got.name, c.name)
		}
	}
}

func TestCacheUpdatePayloadIsRatio(t *testing.T) {
	ev := mapAgentEvent(acp.AgentEvent{Kind: acp.EventKindCache, TurnPct: 0.94, AvgPct: 0.9, Prefixes: 2, Eviction: true})
	var p struct {
		TurnPct  float64 `json:"turn_pct"`
		AvgPct   float64 `json:"avg_pct"`
		Prefixes int     `json:"prefixes"`
		Eviction bool    `json:"eviction"`
	}
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatalf("cache_update not JSON: %v (%q)", err, ev.data)
	}
	// C-2: ratios, not percents. 0.94 must survive verbatim (NOT 94).
	if p.TurnPct != 0.94 || p.AvgPct != 0.9 || p.Prefixes != 2 || !p.Eviction {
		t.Errorf("cache_update payload = %+v", p)
	}
}

func TestPlanUpdateMapsCompletedToDone(t *testing.T) {
	ev := mapAgentEvent(acp.AgentEvent{Kind: acp.EventKindPlan, Plan: []acp.PlanItem{{Text: "x", Status: "done"}}})
	var p struct {
		Items []struct {
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatalf("plan_update not JSON: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].Status != "done" {
		t.Errorf("plan_update payload = %+v", p)
	}
}

func TestMapToolStartPayloadIsJSON(t *testing.T) {
	ev := mapAgentEvent(acp.AgentEvent{
		Kind: acp.EventKindToolStart, ToolCallID: "c1",
		ToolName: "write_file", ToolArgs: `{"path":"a.go"}`,
		ToolReadOnly: true,
	})
	// Args must be an embedded object, not a string; read_only must be present.
	var p struct {
		ID       string         `json:"id"`
		Name     string         `json:"name"`
		Args     map[string]any `json:"args"`
		ReadOnly bool           `json:"read_only"`
	}
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatalf("tool_start data not JSON: %v (%q)", err, ev.data)
	}
	if p.Name != "write_file" || p.ID != "c1" {
		t.Errorf("tool_start id/name = %q/%q, want c1/write_file", p.ID, p.Name)
	}
	if p.Args == nil {
		t.Error("tool_start args is nil, want embedded object")
	}
	if v, ok := p.Args["path"]; !ok || v != "a.go" {
		t.Errorf("tool_start args[path] = %v, want a.go", v)
	}
	if !p.ReadOnly {
		t.Error("tool_start read_only = false, want true")
	}
}
