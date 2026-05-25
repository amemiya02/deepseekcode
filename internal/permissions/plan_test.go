package permissions

import (
	"testing"
)

func TestModePlanDecide(t *testing.T) {
	pol := New(ModePlan, "/repo", nil, nil, nil)

	t.Run("read_only_tool_allowed", func(t *testing.T) {
		dec, reason := pol.Decide(Check{Tool: &fakeTool{name: "read_file", readOnly: true}})
		if dec != Allow {
			t.Errorf("read_file: got %v, want Allow; reason: %s", dec, reason)
		}
	})

	t.Run("write_file_denied", func(t *testing.T) {
		dec, _ := pol.Decide(Check{Tool: &fakeTool{name: "write_file", readOnly: false}})
		if dec != Deny {
			t.Errorf("write_file: got %v, want Deny", dec)
		}
	})

	t.Run("bash_denied", func(t *testing.T) {
		dec, _ := pol.Decide(Check{Tool: &fakeTool{name: "bash", readOnly: false}})
		if dec != Deny {
			t.Errorf("bash: got %v, want Deny", dec)
		}
	})

	t.Run("edit_file_denied", func(t *testing.T) {
		dec, _ := pol.Decide(Check{Tool: &fakeTool{name: "edit_file", readOnly: false}})
		if dec != Deny {
			t.Errorf("edit_file: got %v, want Deny", dec)
		}
	})

	t.Run("apply_patch_denied", func(t *testing.T) {
		dec, _ := pol.Decide(Check{Tool: &fakeTool{name: "apply_patch", readOnly: false}})
		if dec != Deny {
			t.Errorf("apply_patch: got %v, want Deny", dec)
		}
	})

	t.Run("grep_allowed", func(t *testing.T) {
		dec, reason := pol.Decide(Check{Tool: &fakeTool{name: "grep", readOnly: true}})
		if dec != Allow {
			t.Errorf("grep: got %v, want Allow; reason: %s", dec, reason)
		}
	})

	t.Run("question_allowed", func(t *testing.T) {
		dec, reason := pol.Decide(Check{Tool: &fakeTool{name: "question", readOnly: true}})
		if dec != Allow {
			t.Errorf("question: got %v, want Allow; reason: %s", dec, reason)
		}
	})

	t.Run("plan_exit_allowed", func(t *testing.T) {
		dec, reason := pol.Decide(Check{Tool: &fakeTool{name: "plan_exit", readOnly: true}})
		if dec != Allow {
			t.Errorf("plan_exit: got %v, want Allow; reason: %s", dec, reason)
		}
	})

	t.Run("unknown_tool_denied", func(t *testing.T) {
		dec, _ := pol.Decide(Check{Tool: &fakeTool{name: "mystery", readOnly: false}})
		if dec != Deny {
			t.Errorf("mystery: got %v, want Deny", dec)
		}
	})
}

func TestModePlanDangerLevel(t *testing.T) {
	// ModePlan should be at least as safe as ModeReadOnly.
	if danger(ModePlan) > danger(ModeReadOnly) {
		t.Errorf("ModePlan danger (%d) should be <= ModeReadOnly danger (%d)",
			danger(ModePlan), danger(ModeReadOnly))
	}
}

func TestDeriveChildModePlanNotEscalated(t *testing.T) {
	// Parent is ReadOnly; requesting ModePlan should not escalate.
	child := New(ModeReadOnly, "/repo", nil, nil, nil).DeriveChild(ModePlan)
	if child.Mode != ModeReadOnly && child.Mode != ModePlan {
		t.Errorf("child.Mode = %v, should stay at ReadOnly or Plan (both danger=0)", child.Mode)
	}
	if child.Mode == ModeYolo || child.Mode == ModeDefault {
		t.Errorf("child.Mode = %v, should not be escalated from ReadOnly", child.Mode)
	}
}
