package tools

import "testing"

func TestCloneForCWD(t *testing.T) {
	reg := New()
	RegisterBuiltins(reg, 1<<20, 1<<20, "/parent/cwd")

	// Verify write_file is cwd-bound.
	t.Run("write_file", func(t *testing.T) {
		tool, ok := reg.Get("write_file")
		if !ok {
			t.Fatal("write_file not found")
		}
		cloned := CloneForCWD(tool, "/child/cwd")
		wf, ok := cloned.(*WriteFile)
		if !ok {
			t.Fatalf("cloned type: %T", cloned)
		}
		if wf.CWD != "/child/cwd" {
			t.Errorf("CWD = %q, want /child/cwd", wf.CWD)
		}
		// Original must not be affected.
		orig := tool.(*WriteFile)
		if orig.CWD != "/parent/cwd" {
			t.Errorf("original CWD mutated: %q", orig.CWD)
		}
	})

	t.Run("read_file", func(t *testing.T) {
		tool, _ := reg.Get("read_file")
		cloned := CloneForCWD(tool, "/child/cwd")
		rf := cloned.(*ReadFile)
		if rf.CWD != "/child/cwd" {
			t.Errorf("CWD = %q, want /child/cwd", rf.CWD)
		}
	})

	t.Run("non_cwd_bound", func(t *testing.T) {
		tool, _ := reg.Get("bash")
		cloned := CloneForCWD(tool, "/child/cwd")
		// Should be the SAME pointer (not cloned).
		if cloned != tool {
			t.Error("non-cwd tools should be shared")
		}
	})

	t.Run("registry_clone", func(t *testing.T) {
		sub := reg.Subset([]string{"write_file", "bash"})
		cloned := sub.CloneForCWD("/worktree/cwd")

		wf, _ := cloned.Get("write_file")
		if wf.(*WriteFile).CWD != "/worktree/cwd" {
			t.Errorf("registry clone write_file CWD = %q", wf.(*WriteFile).CWD)
		}

		bash, _ := cloned.Get("bash")
		origBash, _ := sub.Get("bash")
		if bash != origBash {
			t.Error("bash should be shared between registries")
		}
	})
}
