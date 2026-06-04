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
