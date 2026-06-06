package acp

import "testing"

func TestNewEventKindsDistinct(t *testing.T) {
	kinds := []EventKind{
		EventKindTextDelta, EventKindInfo, EventKindDone,
		EventKindPermission, EventKindAsk, EventKindToolStart, EventKindToolEnd,
	}
	seen := map[EventKind]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Fatalf("duplicate EventKind value %d", k)
		}
		seen[k] = true
	}
}

func TestAgentEventCarriesInteractiveFields(t *testing.T) {
	var got PermissionDecision
	ev := AgentEvent{
		Kind:        EventKindPermission,
		PermID:      "perm-1",
		ToolName:    "write_file",
		ToolArgs:    `{"path":"a.go"}`,
		Respond:     func(d PermissionDecision) { got = d },
	}
	if ev.PermID != "perm-1" || ev.ToolName != "write_file" {
		t.Fatalf("permission fields not set: %+v", ev)
	}
	ev.Respond(PermissionAllowOnce)
	if got != PermissionAllowOnce {
		t.Fatalf("Respond did not deliver decision, got %v", got)
	}
}
