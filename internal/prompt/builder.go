// Package prompt assembles the cache-stable system prompt for the
// agent.
//
// Cache-stability invariants enforced here:
//
//  1. The static prefix (StaticBase + InstructionFiles) is never
//     allowed to drift mid-session. Updating the binary's
//     BasePromptV1 constant counts as a release-time bump that
//     intentionally invalidates the prompt cache.
//  2. Per-turn dynamic context (git status, current date, etc.) is
//     placed AFTER the DynamicContextBoundary marker so it never
//     pollutes the cached prefix.
//  3. The boundary marker string itself is a load-bearing constant.
//     Changing it is a release-time bump.
//
// The Build method composes Static prefix → boundary → Dynamic
// suffix in that exact order. Callers that need only the cached
// prefix can split on DynamicContextBoundary.
package prompt

import (
	"fmt"
	"strings"
)

// DynamicContextBoundary marks the cut between cache-stable static
// prompt content and per-turn dynamic context. Everything after this
// string is regenerated on every turn; everything before it must
// stay byte-stable across the session.
const DynamicContextBoundary = "\n=== DYNAMIC CONTEXT (per-turn, do not cache) ===\n"

// SystemPromptBuilder composes the system prompt from a static base,
// session-once instruction files, and per-turn project context. Hold
// one per agent.
type SystemPromptBuilder struct {
	// StaticBase is the binary-versioned base prompt. Empty falls
	// back to BasePromptV1.
	StaticBase string

	// Instructions is the loaded set of whitelist instruction files
	// (DEEPSEEK.md, AGENTS.md, .deepseek/instructions.md). Session-once.
	Instructions []InstructionFile

	// Skills is the session-once list of discovered SKILL.md files.
	// Rendered into the static prefix (before the dynamic boundary)
	// so it stays cache-stable. nil or empty → no ## Skills section.
	Skills []Skill

	// Project carries the dynamic per-turn context (git status, date,
	// os). nil disables the dynamic suffix entirely.
	Project *ProjectContext
}

// InstructionFile is a loaded whitelist instruction file. Content is
// already truncated by the loader.
type InstructionFile struct {
	Path    string
	Content string
}

// ProjectContext is the dynamic per-turn context appended after the
// DynamicContextBoundary marker. Git fields are refreshed by the
// agent on each step; the static fields (CWD, OSName, OSVersion,
// Shell, CurrentDate) are filled once at session start.
type ProjectContext struct {
	CWD          string
	CurrentDate  string
	GitStatus    string
	GitDiff      string
	ActiveBranch string
	OSName       string
	OSVersion    string
	Shell        string
}

// Build returns the assembled system prompt:
//
//	<StaticBase>
//	<rendered instructions> (omitted if no instructions)
//	<DynamicContextBoundary>
//	<rendered project context>  (omitted if Project is nil)
//
// The bytes before DynamicContextBoundary must stay identical
// across turns within one session — cache invariant.
func (b *SystemPromptBuilder) Build() string {
	base := b.StaticBase
	if base == "" {
		base = BasePromptV1
	}
	var out strings.Builder
	out.WriteString(base)
	if len(b.Instructions) > 0 {
		out.WriteString("\n\n## Instruction Files (session-once)\n")
		for _, f := range b.Instructions {
			fmt.Fprintf(&out, "\n[%s]\n%s\n", f.Path, f.Content)
		}
	}
	out.WriteString(RenderSkillsBlock(b.Skills))
	out.WriteString(DynamicContextBoundary)
	if b.Project != nil {
		out.WriteString(renderProject(*b.Project))
	}
	return out.String()
}

// RenderProjectContext renders the dynamic project context (cwd, git
// status, git diff) as a string. Exported so the agent can append it
// after a frozen static prefix without re-building the full prompt.
func RenderProjectContext(p ProjectContext) string {
	return renderProject(p)
}

func renderProject(p ProjectContext) string {
	var b strings.Builder
	b.WriteString("\n## Project\n")
	if p.CWD != "" {
		fmt.Fprintf(&b, "- cwd: %s\n", p.CWD)
	}
	if p.CurrentDate != "" {
		fmt.Fprintf(&b, "- date: %s\n", p.CurrentDate)
	}
	if p.OSName != "" {
		fmt.Fprintf(&b, "- os: %s %s\n", p.OSName, p.OSVersion)
	}
	if p.Shell != "" {
		fmt.Fprintf(&b, "- shell: %s\n", p.Shell)
	}
	if p.ActiveBranch != "" {
		fmt.Fprintf(&b, "- branch: %s\n", p.ActiveBranch)
	}
	b.WriteString("\n## Git status\n")
	if strings.TrimSpace(p.GitStatus) == "" {
		b.WriteString("(clean)\n")
	} else {
		b.WriteString(p.GitStatus)
		if !strings.HasSuffix(p.GitStatus, "\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n## Git diff\n")
	if strings.TrimSpace(p.GitDiff) == "" {
		b.WriteString("(no diff)\n")
	} else {
		b.WriteString(p.GitDiff)
		if !strings.HasSuffix(p.GitDiff, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

const maxSkillsBlockChars = 4000

// RenderSkillsBlock generates the "## Skills" section for the system prompt.
// Empty slice returns "". Each skill is one line:
//
//   - {name}: {description}
//
// Description is truncated to 200 chars. Total block capped at ~4000 chars;
// overflow appends "... (N more skills omitted)".
func RenderSkillsBlock(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Skills\n")
	b.WriteString("(Use the skill name to reference skills; load full instructions via read_file when needed)\n")
	total := 0
	omitted := 0
	for _, s := range skills {
		desc := s.Description
		if r := []rune(desc); len(r) > 200 {
			desc = string(r[:200]) + "..."
		}
		var line string
		if desc != "" {
			line = fmt.Sprintf("- %s: %s\n", s.Name, desc)
		} else {
			line = fmt.Sprintf("- %s\n", s.Name)
		}
		if total+len(line) > maxSkillsBlockChars {
			omitted++
			continue
		}
		b.WriteString(line)
		total += len(line)
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "... (%d more skills omitted)\n", omitted)
	}
	return b.String()
}
