package tools

import "testing"

func TestRegisterBuiltinsContainsApplyPatch(t *testing.T) {
	r := New()
	RegisterBuiltins(r, 1024, 1024, t.TempDir())
	tool, ok := r.Get("apply_patch")
	if !ok {
		t.Fatal("apply_patch not registered")
	}
	if tool.Name() != "apply_patch" {
		t.Errorf("Name = %q, want %q", tool.Name(), "apply_patch")
	}
}
