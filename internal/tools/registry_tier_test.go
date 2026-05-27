package tools

import (
	"testing"
)

func TestRegisterAutoTier(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.Register(&stubTool{name: "bash"})
	reg.Register(&stubTool{name: "git_diff"})
	reg.Register(&stubTool{name: "grep"})

	tier, ok := reg.TierOf("read_file")
	if !ok || tier != TierCore {
		t.Errorf("read_file: got (%q, %v), want (core, true)", tier, ok)
	}
	tier, ok = reg.TierOf("bash")
	if !ok || tier != TierCore {
		t.Errorf("bash: got (%q, %v), want (core, true)", tier, ok)
	}
	tier, ok = reg.TierOf("git_diff")
	if !ok || tier != TierProfile {
		t.Errorf("git_diff: got (%q, %v), want (profile, true)", tier, ok)
	}
}

func TestRegisterWithTier(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.RegisterWithTier(&stubTool{name: "custom_tool"}, TierLazy)

	tier, ok := reg.TierOf("custom_tool")
	if !ok || tier != TierLazy {
		t.Errorf("custom_tool: got (%q, %v), want (lazy, true)", tier, ok)
	}

	tier, ok = reg.TierOf("read_file")
	if !ok || tier != TierCore {
		t.Errorf("read_file should remain core: got (%q, %v)", tier, ok)
	}
}

func TestRegisterOverwritePreservesExplicitTier(t *testing.T) {
	reg := New()
	reg.RegisterWithTier(&stubTool{name: "tool_a"}, TierLazy)
	reg.Register(&stubTool{name: "tool_a"})

	tier, ok := reg.TierOf("tool_a")
	if !ok || tier != TierLazy {
		t.Errorf("explicit tier should be preserved on re-register: got (%q, %v)", tier, ok)
	}
}

func TestByTier(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.Register(&stubTool{name: "bash"})
	reg.Register(&stubTool{name: "git_diff"})
	reg.Register(&stubTool{name: "git_show"})
	reg.RegisterWithTier(&stubTool{name: "lazy_tool"}, TierLazy)

	coreTools := reg.ByTier(TierCore)
	if len(coreTools) != 2 {
		t.Fatalf("expected 2 core tools, got %d", len(coreTools))
	}
	if coreTools[0].Name() != "bash" || coreTools[1].Name() != "read_file" {
		t.Errorf("core tools not sorted: %v, %v", coreTools[0].Name(), coreTools[1].Name())
	}

	profileTools := reg.ByTier(TierProfile)
	if len(profileTools) != 2 {
		t.Fatalf("expected 2 profile tools, got %d", len(profileTools))
	}

	lazyTools := reg.ByTier(TierLazy)
	if len(lazyTools) != 1 {
		t.Fatalf("expected 1 lazy tool, got %d", len(lazyTools))
	}
	if lazyTools[0].Name() != "lazy_tool" {
		t.Errorf("expected lazy_tool, got %q", lazyTools[0].Name())
	}
}

func TestByTierEmpty(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})

	lazyTools := reg.ByTier(TierLazy)
	if len(lazyTools) != 0 {
		t.Errorf("expected 0 lazy tools, got %d", len(lazyTools))
	}
}

func TestTierTools(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.Register(&stubTool{name: "bash"})
	reg.Register(&stubTool{name: "git_diff"})
	reg.RegisterWithTier(&stubTool{name: "lazy_tool"}, TierLazy)

	t.Run("core only", func(t *testing.T) {
		sub := reg.TierTools(TierCore)
		all := sub.All()
		if len(all) != 2 {
			t.Fatalf("expected 2 core tools, got %d", len(all))
		}
	})

	t.Run("core + profile", func(t *testing.T) {
		sub := reg.TierTools(TierCore, TierProfile)
		all := sub.All()
		if len(all) != 3 {
			t.Fatalf("expected 3 tools (core+profile), got %d", len(all))
		}
	})

	t.Run("all tiers", func(t *testing.T) {
		sub := reg.TierTools(TierCore, TierProfile, TierLazy)
		all := sub.All()
		if len(all) != 4 {
			t.Fatalf("expected 4 tools (all tiers), got %d", len(all))
		}
	})

	t.Run("unknown tier", func(t *testing.T) {
		sub := reg.TierTools("nonexistent")
		all := sub.All()
		if len(all) != 0 {
			t.Errorf("expected 0 tools for unknown tier, got %d", len(all))
		}
	})
}

func TestTierToolsPreservesTiers(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.RegisterWithTier(&stubTool{name: "custom"}, TierLazy)

	sub := reg.TierTools(TierCore, TierLazy)
	tier, ok := sub.TierOf("custom")
	if !ok || tier != TierLazy {
		t.Errorf("subset should preserve tier: got (%q, %v)", tier, ok)
	}
}

func TestDefaultProfilesOnlyCore(t *testing.T) {
	reg := New()
	reg.Register(&stubTool{name: "read_file"})
	reg.Register(&stubTool{name: "write_file"})
	reg.Register(&stubTool{name: "edit_file"})
	reg.Register(&stubTool{name: "bash"})
	reg.Register(&stubTool{name: "bash_pty"})
	reg.Register(&stubTool{name: "glob"})
	reg.Register(&stubTool{name: "grep"})
	reg.Register(&stubTool{name: "ls"})
	reg.Register(&stubTool{name: "todo_write"})
	reg.Register(&stubTool{name: "apply_patch"})
	reg.Register(&stubTool{name: "git_diff"})
	reg.Register(&stubTool{name: "git_show"})

	coreReg := reg.TierTools(TierCore)
	all := coreReg.All()
	for _, tool := range all {
		if tool.Name() == "git_diff" || tool.Name() == "git_show" {
			t.Errorf("profile tool %q should not be in core-only registry", tool.Name())
		}
	}
}
