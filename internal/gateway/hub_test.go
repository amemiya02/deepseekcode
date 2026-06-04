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
		{acp.AgentEvent{Kind: acp.EventKindInfo, Text: "step"}, "step"},
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

func TestMapToolStartPayloadIsJSON(t *testing.T) {
	ev := mapAgentEvent(acp.AgentEvent{
		Kind: acp.EventKindToolStart, ToolCallID: "c1",
		ToolName: "write_file", ToolArgs: `{"path":"a.go"}`,
	})
	var p struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Args string `json:"args"`
	}
	if err := json.Unmarshal([]byte(ev.data), &p); err != nil {
		t.Fatalf("tool_start data not JSON: %v (%q)", err, ev.data)
	}
	if p.Name != "write_file" || p.ID != "c1" {
		t.Errorf("tool_start payload = %+v", p)
	}
}
