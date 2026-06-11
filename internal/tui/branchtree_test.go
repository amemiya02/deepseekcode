package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/session"
)

func TestRenderBranchTree(t *testing.T) {
	sessions := []session.Session{
		{ID: "root", Summary: "main"},
		{ID: "child-a", ParentID: "root", BranchPoint: 5, Summary: "experiment A"},
		{ID: "child-b", ParentID: "root", BranchPoint: 3, Summary: "experiment B"},
		{ID: "grandchild", ParentID: "child-a", BranchPoint: 2, Summary: "refinement"},
	}

	out := renderBranchTree(sessions, "child-a")

	// All nodes must appear.
	for _, want := range []string{"main", "experiment A", "experiment B", "refinement"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// Active session marked with "->".
	if !strings.Contains(out, "-> experiment A") {
		t.Errorf("active session not marked:\n%s", out)
	}
	// Inactive sessions should not have "->" before their title.
	if strings.Contains(out, "-> main") {
		t.Errorf("root should not be marked active:\n%s", out)
	}

	// Branch point annotations present.
	if !strings.Contains(out, "(fork @ msg 5)") {
		t.Errorf("missing branch point 5 annotation:\n%s", out)
	}
	if !strings.Contains(out, "(fork @ msg 3)") {
		t.Errorf("missing branch point 3 annotation:\n%s", out)
	}
	if !strings.Contains(out, "(fork @ msg 2)") {
		t.Errorf("missing branch point 2 annotation:\n%s", out)
	}
}

func TestRenderBranchTreeDeterminism(t *testing.T) {
	sessions := []session.Session{
		{ID: "root", Summary: "r"},
		{ID: "b", ParentID: "root", BranchPoint: 1},
		{ID: "a", ParentID: "root", BranchPoint: 1},
	}

	// Render twice; order must be identical (sorted by ID).
	one := renderBranchTree(sessions, "")
	two := renderBranchTree(sessions, "")
	if one != two {
		t.Errorf("non-deterministic output:\n---\n%s\n---\n%s", one, two)
	}
	// "a" (lexicographically first) should come before "b".
	aIdx := strings.Index(one, "a\n")
	bIdx := strings.Index(one, "b\n")
	if aIdx > bIdx {
		t.Errorf("expected 'a' before 'b', got:\n%s", one)
	}
}

func TestRenderBranchTreeEmpty(t *testing.T) {
	out := renderBranchTree(nil, "")
	if out != "(no sessions)" {
		t.Errorf("expected (no sessions), got %q", out)
	}
}
