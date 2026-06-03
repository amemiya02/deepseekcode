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

	// SkillDirectory is the canonical stable skill directory text, exactly
	// as produced by skills.Store.PromptIndex() / IndexText() — one line per
	// skill in the form
	//
	//	name | short_description | run_mode | version_hash | allowed_tools
	//
	// It is rendered into the static prefix (before the dynamic boundary) so
	// the model-visible bytes carry the body-derived version_hash. Because
	// this is the same text that feeds the epoch's skill-dir hash, a skill
	// body edit moves the prefix and the epoch hash together. Empty → no
	// ## Skills section. Never contains local absolute paths or skill bodies.
	SkillDirectory string

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
	out.WriteString(renderSkillDirectory(b.SkillDirectory))
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

// InjectRecalled appends recalled memory facts into the dynamic section
// of the system prompt. It ensures the recalled text always appears after
// DynamicContextBoundary, preventing cache-prefix contamination.
// If the boundary is not yet present in systemPrompt, it is inserted first.
func InjectRecalled(systemPrompt, recalled string) string {
	if !strings.Contains(systemPrompt, DynamicContextBoundary) {
		// No boundary yet — append boundary then recalled.
		return systemPrompt + DynamicContextBoundary + recalled
	}
	return systemPrompt + recalled
}

// renderSkillDirectory wraps the canonical stable skill directory (from
// skills.Store.PromptIndex()) with the "## Skills" header. Empty input
// returns "" so no section is emitted.
//
// The directory text is rendered verbatim — it is the exact same bytes that
// feed the epoch's skill-dir hash and skills.Store.VersionHash(), so the
// model-visible prefix and the cache-epoch hash move together. Each line is
//
//	name | short_description | run_mode | version_hash | allowed_tools
//
// carrying the body-derived version_hash but never a skill body or a local
// absolute path. The model loads a skill body on demand with skill_read.
func renderSkillDirectory(dir string) string {
	dir = strings.TrimRight(dir, "\n")
	if dir == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Skills\n")
	b.WriteString("(Load a skill's full instructions on demand with skill_read using the skill name. " +
		"Each line is: name | description | run_mode | version_hash | allowed_tools.)\n")
	b.WriteString(dir)
	b.WriteByte('\n')
	return b.String()
}
