package permissions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestPolicyRulesOverrideTiered(t *testing.T) {
	engine := &RuleEngine{
		Deny: []PermissionRule{
			{ToolPattern: "bash", ArgsPattern: "", Decision: "deny"},
		},
		Allow: []PermissionRule{
			{ToolPattern: "read_file", ArgsPattern: "", Decision: "allow"},
		},
	}

	pol := New(ModeDefault, "/tmp", nil, nil, engine)

	// bash should be denied by rule even though tiered would ask.
	dec, reason := pol.Decide(Check{
		Tool: &fakeTool{name: "bash", readOnly: false},
		Args: json.RawMessage(`{"command":"git status"}`),
	})
	if dec != Deny {
		t.Fatalf("rule deny should override tiered: got %v", dec)
	}
	if reason == "" {
		t.Error("reason should not be empty")
	}

	// read_file should be allowed by rule.
	dec, _ = pol.Decide(Check{
		Tool: &fakeTool{name: "read_file", readOnly: true},
		Args: json.RawMessage(`{"path":"x"}`),
	})
	if dec != Allow {
		t.Fatalf("rule allow should work: got %v", dec)
	}

	// Unknown tool falls through to tiered (ask).
	dec, _ = pol.Decide(Check{
		Tool: &fakeTool{name: "unknown_tool", readOnly: false},
		Args: json.RawMessage(`{}`),
	})
	if dec != Ask {
		t.Fatalf("unknown tool should fallback to tiered ask: got %v", dec)
	}
}

func TestPolicyDecideReturnsReason(t *testing.T) {
	pol := New(ModeYolo, "/tmp", nil, nil, nil)

	_, reason := pol.Decide(Check{
		Tool: &fakeTool{name: "bash", readOnly: false},
		Args: json.RawMessage(`{}`),
	})
	if reason == "" {
		t.Error("Decide should return a reason")
	}
}

func TestPolicyRulesNilEngine(t *testing.T) {
	pol := New(ModeReadOnly, "/tmp", nil, nil, nil)

	dec, _ := pol.Decide(Check{
		Tool: &fakeTool{name: "read_file", readOnly: true},
		Args: json.RawMessage(`{}`),
	})
	if dec != Allow {
		t.Fatalf("read-only tool should be allowed: got %v", dec)
	}
}

func TestMinModeFor(t *testing.T) {
	tests := []struct {
		tool string
		want Mode
	}{
		{"bash", ModeDefault},
		{"write_file", ModeDefault},
		{"edit_file", ModeDefault},
		{"unknown_tool", ModeDefault},
	}
	for _, tc := range tests {
		got := MinModeFor(tc.tool)
		if got != tc.want {
			t.Errorf("MinModeFor(%q) = %v, want %v", tc.tool, got, tc.want)
		}
	}
}

type fakeTool struct {
	name     string
	readOnly bool
}

func (f *fakeTool) Name() string                { return f.name }
func (f *fakeTool) Description() string         { return "fake" }
func (f *fakeTool) Parameters() json.RawMessage { return nil }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}
func (f *fakeTool) IsReadOnly() bool { return f.readOnly }

func TestNonRuleDenyReasonPrefix(t *testing.T) {
	// Read-only mode deny must NOT claim "denied by rule".
	pol := New(ModeReadOnly, "/tmp", nil, nil, nil)
	_, reason := pol.Decide(Check{
		Tool: &fakeTool{name: "bash", readOnly: false},
		Args: json.RawMessage(`{}`),
	})
	if strings.HasPrefix(reason, "matched deny rule:") {
		t.Errorf("non-rule deny should not start with 'matched deny rule:': got %q", reason)
	}
	if strings.Contains(reason, "denied by rule") {
		t.Errorf("non-rule deny should not contain 'denied by rule': got %q", reason)
	}
}

func TestPolicyModeOverridesRules(t *testing.T) {
	engine := &RuleEngine{
		Deny: []PermissionRule{
			{ToolPattern: "bash", ArgsPattern: "", Decision: "deny"},
		},
	}

	// Yolo mode must override deny rules.
	pol := New(ModeYolo, "/tmp", nil, nil, engine)
	dec, _ := pol.Decide(Check{
		Tool: &fakeTool{name: "bash", readOnly: false},
		Args: json.RawMessage(`{"command":"rm -rf /"}`),
	})
	if dec != Allow {
		t.Fatalf("yolo must override deny rule: got %v", dec)
	}

	// Read-only mode deny must take priority over allow rules.
	allowEngine := &RuleEngine{
		Allow: []PermissionRule{
			{ToolPattern: "write_file", ArgsPattern: "", Decision: "allow"},
		},
	}
	pol2 := New(ModeReadOnly, "/tmp", nil, nil, allowEngine)
	dec2, _ := pol2.Decide(Check{
		Tool: &fakeTool{name: "write_file", readOnly: false},
		Args: json.RawMessage(`{}`),
	})
	if dec2 != Deny {
		t.Fatalf("read-only mode must override allow rule: got %v", dec2)
	}
}

func TestPolicyMinModeForGating(t *testing.T) {
	// All current tools return ModeDefault, so MinModeFor should never deny.
	pol := New(ModeDefault, "/tmp", nil, nil, nil)
	dec, _ := pol.Decide(Check{
		Tool: &fakeTool{name: "bash", readOnly: false},
		Args: json.RawMessage(`{}`),
	})
	if dec == Deny {
		t.Fatalf("ModeDefault should satisfy all current tools: got %v", dec)
	}
}
