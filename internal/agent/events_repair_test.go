package agent

import (
	"testing"
)

func TestEventRepairIsAgentEvent(t *testing.T) {
	// Verify EventRepair implements the Event interface.
	var _ Event = EventRepair{}

	ev := EventRepair{
		Kind:       "args_completed",
		Tool:       "read_file",
		CallID:     "c1",
		Message:    "args completed",
		BeforeHash: "abc123",
		AfterHash:  "def456",
	}

	// The isAgentEvent() method must exist and the type must satisfy Event.
	ev.isAgentEvent()

	if ev.Kind != "args_completed" {
		t.Errorf("expected Kind args_completed, got %q", ev.Kind)
	}
	if ev.Tool != "read_file" {
		t.Errorf("expected Tool read_file, got %q", ev.Tool)
	}
	if ev.Message != "args completed" {
		t.Errorf("expected Message 'args completed', got %q", ev.Message)
	}
}

func TestEventRepairFields(t *testing.T) {
	tests := []struct {
		name       string
		ev         EventRepair
		wantKind   string
		wantTool   string
		wantMsg    string
		wantCallID string
	}{
		{
			name:       "args_completed",
			ev:         EventRepair{Kind: "args_completed", Tool: "read_file", CallID: "c1", Message: "args completed"},
			wantKind:   "args_completed",
			wantTool:   "read_file",
			wantMsg:    "args completed",
			wantCallID: "c1",
		},
		{
			name:       "args_invalid",
			ev:         EventRepair{Kind: "args_invalid", Tool: "bash", CallID: "c2", Message: "invalid JSON"},
			wantKind:   "args_invalid",
			wantTool:   "bash",
			wantMsg:    "invalid JSON",
			wantCallID: "c2",
		},
		{
			name:       "recovered",
			ev:         EventRepair{Kind: "recovered", Tool: "grep", CallID: "recovered_1", Message: "recovered from reasoning_content"},
			wantKind:   "recovered",
			wantTool:   "grep",
			wantMsg:    "recovered from reasoning_content",
			wantCallID: "recovered_1",
		},
		{
			name:       "suppressed",
			ev:         EventRepair{Kind: "suppressed", Tool: "read_file", CallID: "c3", Message: "storm breaker suppressed"},
			wantKind:   "suppressed",
			wantTool:   "read_file",
			wantMsg:    "storm breaker suppressed",
			wantCallID: "c3",
		},
		{
			name:       "schema_complex",
			ev:         EventRepair{Kind: "schema_complex", Tool: "question", CallID: "c4", Message: "schema depth 4 exceeds threshold"},
			wantKind:   "schema_complex",
			wantTool:   "question",
			wantMsg:    "schema depth 4 exceeds threshold",
			wantCallID: "c4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ev.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", tt.ev.Kind, tt.wantKind)
			}
			if tt.ev.Tool != tt.wantTool {
				t.Errorf("Tool = %q, want %q", tt.ev.Tool, tt.wantTool)
			}
			if tt.ev.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", tt.ev.Message, tt.wantMsg)
			}
			if tt.ev.CallID != tt.wantCallID {
				t.Errorf("CallID = %q, want %q", tt.ev.CallID, tt.wantCallID)
			}
		})
	}
}
