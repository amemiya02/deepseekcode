package permissions

import (
	"testing"
)

func TestDeriveChild(t *testing.T) {
	t.Run("default parent read-only request", func(t *testing.T) {
		parent := New(ModeDefault, "/repo", []string{".env"}, []string{"git status *"}, nil)
		child := parent.DeriveChild(ModeReadOnly)
		if child.Mode != ModeReadOnly {
			t.Errorf("child.Mode = %v, want ModeReadOnly", child.Mode)
		}
	})

	t.Run("read-only parent yolo request clamps to read-only", func(t *testing.T) {
		parent := New(ModeReadOnly, "/repo", nil, nil, nil)
		child := parent.DeriveChild(ModeYolo)
		if child.Mode != ModeReadOnly {
			t.Errorf("child.Mode = %v, want ModeReadOnly (no escalation)", child.Mode)
		}
	})

	t.Run("yolo parent yolo request stays yolo", func(t *testing.T) {
		parent := New(ModeYolo, "/repo", nil, nil, nil)
		child := parent.DeriveChild(ModeYolo)
		if child.Mode != ModeYolo {
			t.Errorf("child.Mode = %v, want ModeYolo", child.Mode)
		}
	})

	t.Run("child cwd equals parent cwd", func(t *testing.T) {
		parent := New(ModeDefault, "/repo", nil, nil, nil)
		child := parent.DeriveChild(ModeReadOnly)
		if child.Cwd != parent.Cwd {
			t.Errorf("child.Cwd = %q, want %q", child.Cwd, parent.Cwd)
		}
	})

	t.Run("child secret patterns equal parent", func(t *testing.T) {
		parent := New(ModeDefault, "/repo", []string{".env", "*.key"}, nil, nil)
		child := parent.DeriveChild(ModeReadOnly)
		if len(child.SecretPathPatterns) != 2 {
			t.Fatalf("child.SecretPathPatterns = %v, want 2 items", child.SecretPathPatterns)
		}
		if child.SecretPathPatterns[0] != ".env" || child.SecretPathPatterns[1] != "*.key" {
			t.Errorf("child.SecretPathPatterns = %v, want [.env *.key]", child.SecretPathPatterns)
		}
	})

	t.Run("child bash allowlist is independent copy", func(t *testing.T) {
		parent := New(ModeDefault, "/repo", nil, []string{"git status *"}, nil)
		child := parent.DeriveChild(ModeReadOnly)

		// Verify child inherited the allowlist.
		childList := child.BashAllowlist()
		if len(childList) != 1 || childList[0] != "git status *" {
			t.Fatalf("child.BashAllowlist() = %v, want [git status *]", childList)
		}

		// Mutate child — parent must not change.
		child.AllowBashPattern("foo *")
		parentList := parent.BashAllowlist()
		for _, p := range parentList {
			if p == "foo *" {
				t.Fatal("child mutation leaked to parent allowlist")
			}
		}
	})

	t.Run("default parent ask-all request", func(t *testing.T) {
		parent := New(ModeDefault, "/repo", nil, nil, nil)
		child := parent.DeriveChild(ModeAskAll)
		if child.Mode != ModeAskAll {
			t.Errorf("child.Mode = %v, want ModeAskAll", child.Mode)
		}
	})

	t.Run("ask-all parent default request clamps to ask-all", func(t *testing.T) {
		parent := New(ModeAskAll, "/repo", nil, nil, nil)
		child := parent.DeriveChild(ModeDefault)
		// danger(AskAll)=1 < danger(Default)=2, so clamp picks AskAll
		if child.Mode != ModeAskAll {
			t.Errorf("child.Mode = %v, want ModeAskAll (safer)", child.Mode)
		}
	})
}

func TestClampMode(t *testing.T) {
	tests := []struct {
		a, b Mode
		want Mode
	}{
		{ModeDefault, ModeReadOnly, ModeReadOnly},
		{ModeReadOnly, ModeYolo, ModeReadOnly},
		{ModeYolo, ModeYolo, ModeYolo},
		{ModeReadOnly, ModeReadOnly, ModeReadOnly},
		{ModeDefault, ModeDefault, ModeDefault},
		{ModeAskAll, ModeDefault, ModeAskAll},
		{ModeYolo, ModeDefault, ModeDefault},
	}
	for _, tt := range tests {
		got := clampMode(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("clampMode(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
