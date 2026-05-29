package permissions

import "testing"

func TestModeFromRuleset(t *testing.T) {
	cases := []struct {
		name string
		want Mode
		ok   bool
	}{
		{"default", ModeDefault, true},
		{"yolo", ModeYolo, true},
		{"read-only", ModeReadOnly, true},
		{"readonly", ModeReadOnly, true},
		{"ask-all", ModeAskAll, true},
		{"askall", ModeAskAll, true},
		{"plan", ModePlan, true},
		{"  YOLO  ", ModeYolo, true}, // trim + case-insensitive
		{"bogus", ModeDefault, false},
		{"", ModeDefault, false},
	}
	for _, c := range cases {
		got, ok := ModeFromRuleset(c.name)
		if got != c.want || ok != c.ok {
			t.Errorf("ModeFromRuleset(%q) = (%v,%v), want (%v,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestRulesetCannotEscalate confirms the restrict-only guarantee: a ruleset
// name fed through DeriveChild can never raise a child above its parent.
func TestRulesetCannotEscalate(t *testing.T) {
	parent := New(ModeDefault, "", nil, nil, nil)
	m, ok := ModeFromRuleset("yolo")
	if !ok {
		t.Fatal("yolo should map to a Mode")
	}
	if child := parent.DeriveChild(m); child.Mode == ModeYolo {
		t.Errorf("child escalated to ModeYolo above a ModeDefault parent; clampMode must prevent this (got %v)", child.Mode)
	}
}
