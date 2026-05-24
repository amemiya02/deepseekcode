package prompt

import (
	"strings"
	"testing"
)

// TestBuildStableStaticPrefix pins the cache invariant: two
// builders that agree on StaticBase + Instructions but differ on
// Project must produce identical bytes up to the boundary marker.
func TestBuildStableStaticPrefix(t *testing.T) {
	a := SystemPromptBuilder{
		StaticBase:   "BASE",
		Instructions: []InstructionFile{{Path: "DEEPSEEK.md", Content: "do X"}},
		Project: &ProjectContext{
			CWD: "/a", CurrentDate: "2026-01-01",
			OSName: "darwin", OSVersion: "24.0", Shell: "zsh",
			GitStatus: " M file.go\n",
		},
	}
	b := a
	b.Project = &ProjectContext{
		CWD: "/b", CurrentDate: "2099-12-31",
		OSName: "linux", OSVersion: "6.0", Shell: "bash",
		GitStatus: "untracked.go\n", GitDiff: "diff --stat\n",
	}

	pa := a.Build()
	pb := b.Build()

	ia := strings.Index(pa, DynamicContextBoundary)
	ib := strings.Index(pb, DynamicContextBoundary)
	if ia < 0 || ib < 0 {
		t.Fatalf("boundary missing: pa=%d pb=%d", ia, ib)
	}
	if pa[:ia] != pb[:ib] {
		t.Errorf("static prefix drift:\nA: %q\nB: %q", pa[:ia], pb[:ib])
	}
	if pa[ia:] == pb[ib:] {
		t.Errorf("dynamic suffix should differ but matches")
	}
}

func TestBuildOmitsProjectWhenNil(t *testing.T) {
	out := (&SystemPromptBuilder{StaticBase: "X"}).Build()
	if !strings.HasSuffix(out, DynamicContextBoundary) {
		t.Errorf("nil Project should produce no suffix; got tail %q", lastN(out, 80))
	}
}

func TestBuildOmitsInstructionsWhenEmpty(t *testing.T) {
	out := (&SystemPromptBuilder{StaticBase: "X"}).Build()
	if strings.Contains(out, "Instruction Files") {
		t.Errorf("empty instructions should not render the section")
	}
}

func TestBuildShowsNoDiffSentinel(t *testing.T) {
	out := (&SystemPromptBuilder{
		StaticBase: "X",
		Project:    &ProjectContext{GitStatus: "M a.go\n"},
	}).Build()
	if !strings.Contains(out, "(no diff)") {
		t.Errorf("missing (no diff) sentinel; got: %s", out)
	}
}

func TestBuildShowsCleanStatusSentinel(t *testing.T) {
	out := (&SystemPromptBuilder{
		StaticBase: "X",
		Project:    &ProjectContext{GitDiff: "diff --stat\n"},
	}).Build()
	if !strings.Contains(out, "(clean)") {
		t.Errorf("missing (clean) sentinel; got: %s", out)
	}
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
