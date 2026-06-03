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

func TestBuildSkillsBeforeBoundary(t *testing.T) {
	out := (&SystemPromptBuilder{
		StaticBase:     "X",
		SkillDirectory: "pdf | edit PDFs | direct | a1b2c3d4e5f6 | read_file\n",
		Project:        &ProjectContext{CWD: "/a"},
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
	// The version_hash carried by the directory must land in the static
	// prefix — that is the whole point of rendering IndexText() here.
	if !strings.Contains(out[:boundaryIdx], "a1b2c3d4e5f6") {
		t.Error("version_hash from the skill directory missing from the static prefix")
	}
}

func TestBuildOmitsSkillsWhenEmpty(t *testing.T) {
	out := (&SystemPromptBuilder{StaticBase: "X"}).Build()
	if strings.Contains(out, "## Skills") {
		t.Error("empty skills should not render ## Skills section")
	}
}

func TestRenderSkillDirectoryEmpty(t *testing.T) {
	if got := renderSkillDirectory(""); got != "" {
		t.Errorf("empty directory: got %q, want empty", got)
	}
	if got := renderSkillDirectory("\n\n"); got != "" {
		t.Errorf("whitespace-only directory: got %q, want empty", got)
	}
}

func TestInjectRecalled(t *testing.T) {
	boundary := DynamicContextBoundary

	cases := []struct {
		name          string
		systemPrompt  string
		recalled      string
		wantPrefix    string // text that must appear before boundary in result
		wantInDynamic string // text that must appear after boundary in result
		// wantPreserved is pre-existing trailing dynamic content that must SURVIVE
		// injection. A regression to a naive append (systemPrompt[:after]+recalled,
		// dropping systemPrompt[after:]) would silently delete it.
		wantPreserved string
	}{
		{
			name:          "boundary present, recalled inserted after boundary",
			systemPrompt:  "Static base." + boundary + "Existing dynamic.",
			recalled:      "recalled fact",
			wantPrefix:    "Static base.",
			wantInDynamic: "recalled fact",
			wantPreserved: "Existing dynamic.",
		},
		{
			name:          "boundary absent, boundary appended then recalled",
			systemPrompt:  "Static base only.",
			recalled:      "recalled fact",
			wantPrefix:    "Static base only.",
			wantInDynamic: "recalled fact",
		},
		{
			name:          "empty recalled string leaves prompt structurally intact",
			systemPrompt:  "Static base." + boundary + "Existing dynamic.",
			recalled:      "",
			wantPrefix:    "Static base.",
			wantInDynamic: "Existing dynamic.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := InjectRecalled(tc.systemPrompt, tc.recalled)

			idx := strings.Index(result, boundary)
			if idx < 0 {
				t.Fatalf("boundary not found in result: %q", result)
			}
			frozenPart := result[:idx]
			dynamicPart := result[idx:]

			if tc.wantPrefix != "" && !strings.Contains(frozenPart, tc.wantPrefix) {
				t.Errorf("frozen prefix %q does not contain %q", frozenPart, tc.wantPrefix)
			}
			if tc.recalled != "" && strings.Contains(frozenPart, tc.recalled) {
				t.Errorf("recalled text %q leaked into frozen prefix %q", tc.recalled, frozenPart)
			}
			if tc.wantInDynamic != "" && !strings.Contains(dynamicPart, tc.wantInDynamic) {
				t.Errorf("dynamic section %q does not contain %q", dynamicPart, tc.wantInDynamic)
			}
			// Pre-existing trailing content after the boundary must be preserved.
			// This catches a regression to a naive append that drops
			// systemPrompt[after:].
			if tc.wantPreserved != "" && !strings.Contains(dynamicPart, tc.wantPreserved) {
				t.Errorf("pre-existing trailing content %q was dropped from result %q",
					tc.wantPreserved, result)
			}
		})
	}
}

func TestRenderSkillDirectoryVerbatim(t *testing.T) {
	dir := "pdf | edit PDFs | direct | abc123abc123 | read_file\ngit | git ops | subagent | def456def456 | bash"
	got := renderSkillDirectory(dir)
	if !strings.Contains(got, "## Skills") {
		t.Error("missing ## Skills heading")
	}
	// The directory text is rendered verbatim, including version hashes.
	if !strings.Contains(got, dir) {
		t.Errorf("directory not rendered verbatim; got: %s", got)
	}
	// A stable directory never leaks a local path or a body.
	if strings.Contains(got, "/") && strings.Contains(got, "SKILL.md") {
		t.Errorf("skills block leaked a local path; got: %s", got)
	}
}
