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
	out.WriteString(DynamicContextBoundary)
	if b.Project != nil {
		out.WriteString(renderProject(*b.Project))
	}
	return out.String()
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
