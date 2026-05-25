package prompt

import (
	"fmt"
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

func TestBuildSkillsBeforeBoundary(t *testing.T) {
	out := (&SystemPromptBuilder{
		StaticBase: "X",
		Skills:     []Skill{{Name: "pdf", Description: "edit PDFs", Path: "/p/pdf/SKILL.md"}},
		Project:    &ProjectContext{CWD: "/a"},
	}).Build()
	skillsIdx := strings.Index(out, "## Skills")
	boundaryIdx := strings.Index(out, DynamicContextBoundary)
	if skillsIdx < 0 {
		t.Fatal("## Skills section missing")
	}
	if boundaryIdx < 0 {
		t.Fatal("DynamicContextBoundary missing")
	}
	if skillsIdx >= boundaryIdx {
		t.Errorf("## Skills at %d is not before boundary at %d", skillsIdx, boundaryIdx)
	}
}

func TestBuildOmitsSkillsWhenEmpty(t *testing.T) {
	out := (&SystemPromptBuilder{StaticBase: "X"}).Build()
	if strings.Contains(out, "## Skills") {
		t.Error("empty skills should not render ## Skills section")
	}
}

func TestRenderSkillsBlockEmpty(t *testing.T) {
	if got := RenderSkillsBlock(nil); got != "" {
		t.Errorf("nil skills: got %q, want empty", got)
	}
	if got := RenderSkillsBlock([]Skill{}); got != "" {
		t.Errorf("empty skills: got %q, want empty", got)
	}
}

func TestRenderSkillsBlockBasic(t *testing.T) {
	got := RenderSkillsBlock([]Skill{
		{Name: "pdf", Description: "edit PDFs", Path: "/p/.deepseek/skills/pdf/SKILL.md"},
	})
	if !strings.Contains(got, "## Skills") {
		t.Error("missing ## Skills heading")
	}
	if !strings.Contains(got, "pdf: edit PDFs") {
		t.Error("missing skill line with description")
	}
	if !strings.Contains(got, "file: /p/.deepseek/skills/pdf/SKILL.md") {
		t.Error("missing file path")
	}
}

func TestRenderSkillsBlockNoDescription(t *testing.T) {
	got := RenderSkillsBlock([]Skill{
		{Name: "bare", Path: "/x/SKILL.md"},
	})
	if !strings.Contains(got, "- bare (file:") {
		t.Errorf("missing bare skill line; got: %s", got)
	}
	if strings.Contains(got, ":  (file:") {
		t.Error("empty description should not produce ': ' separator")
	}
}

func TestRenderSkillsBlockTruncation(t *testing.T) {
	skills := make([]Skill, 100)
	for i := range skills {
		skills[i] = Skill{
			Name:        fmt.Sprintf("skill-%03d", i),
			Description: "desc",
			Path:        fmt.Sprintf("/p/skill-%03d/SKILL.md", i),
		}
	}
	got := RenderSkillsBlock(skills)
	if !strings.Contains(got, "more skills omitted") {
		t.Error("expected truncation with 100 skills")
	}
	if len(got) > 4200 {
		t.Errorf("block too long: %d chars", len(got))
	}
}
