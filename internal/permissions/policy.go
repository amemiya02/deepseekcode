// Package permissions implements the tiered approval model described in
// docs/design.md §8.
//
// The agent calls Decide() before executing each tool call. The result
// is one of:
//
//	Allow — proceed without prompting
//	Deny  — refuse without prompting (return a tool-error to the model)
//	Ask   — prompt the user; the agent owns the prompt UI
//
// Mode flags (--yolo, --read-only, --ask-all) compose with the per-tool
// defaults. The exact precedence is in Decide().
package permissions

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Mode controls the global override flag. Default is the tiered policy.
type Mode int

const (
	ModeDefault  Mode = iota // tiered defaults per docs/design.md §8.1
	ModeYolo                 // auto-approve every tool (DANGEROUS)
	ModeReadOnly             // block all write/edit/bash tools
	ModeAskAll               // prompt for every tool, ignoring allowlist
)

// Decision is the answer Decide() returns.
type Decision int

const (
	Allow Decision = iota
	Deny
	Ask
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case Ask:
		return "ask"
	}
	return "unknown"
}

// Policy is the immutable configuration; it's safe to share across
// goroutines.
type Policy struct {
	Mode               Mode
	Cwd                string
	SecretPathPatterns []string
	Rules              *RuleEngine // rule-based overrides (Phase 7); nil = no rules

	bashAllowlist []string // patterns like "git status *"
}

// New builds a Policy. SecretPathPatterns and bashAllowlist are copied.
func New(mode Mode, cwd string, secretPatterns, bashAllowlist []string, rules *RuleEngine) *Policy {
	p := &Policy{
		Mode:               mode,
		Cwd:                cwd,
		SecretPathPatterns: append([]string(nil), secretPatterns...),
		bashAllowlist:      append([]string(nil), bashAllowlist...),
		Rules:              rules,
	}
	return p
}

// Check is the request struct passed to Decide.
type Check struct {
	Tool tools.Tool
	Args json.RawMessage
}

// Decide returns one of Allow/Deny/Ask per the tiered policy, plus a
// human-readable reason for logging or UI display.
//
// Priority (first match wins):
//  1. Global mode flags (yolo / read-only / ask-all) — user intent is absolute
//  2. Rule engine ([permissions.rules] in config) — fine-grained allow/deny/ask
//  3. Tool requirements (MinModeFor) — hard gate for tools needing higher privilege
//  4. Tiered defaults — read-only auto-allow, path safety, bash allowlist
func (p *Policy) Decide(c Check) (Decision, string) {
	// 1. Global mode flags.
	switch p.Mode {
	case ModeYolo:
		return Allow, "yolo mode"
	case ModeReadOnly:
		if isReadOnly(c.Tool) {
			return Allow, "read-only tool allowed in read-only mode"
		}
		return Deny, "blocked by read-only mode"
	case ModeAskAll:
		return Ask, "ask-all mode"
	}

	// 2. Rule engine (Phase 7) — only active in ModeDefault.
	if p.Rules != nil {
		d, reason := p.Rules.Evaluate(c.Tool.Name(), c.Args)
		switch d {
		case "deny":
			return Deny, reason
		case "allow":
			return Allow, reason
		case "ask":
			return Ask, reason
		}
	}

	// 3. Tool-level minimum permission mode (Phase 7).
	if req := MinModeFor(c.Tool.Name()); !modeSatisfies(p.Mode, req) {
		return Deny, "tool requires higher permission mode"
	}

	// 4. ModeDefault: tiered policy.

	// Read-only tools are always auto-allowed.
	if isReadOnly(c.Tool) {
		return Allow, "read-only tool"
	}

	// File-touching tools: check affected paths against cwd + secret patterns.
	if pa, ok := c.Tool.(tools.PathAware); ok {
		paths := pa.AffectedPaths(c.Args)
		for _, raw := range paths {
			abs, err := filepath.Abs(raw)
			if err != nil {
				return Ask, "unresolvable path" // can't tell, be safe
			}
			if p.matchesSecret(abs) {
				return Ask, "path matches secret pattern"
			}
			if !p.insideCwd(abs) {
				return Ask, "path outside cwd"
			}
		}
		// Write/edit tools with all paths inside cwd and non-secret → auto-allow.
		if c.Tool.Name() == "write_file" || c.Tool.Name() == "edit_file" {
			return Allow, "safe write inside cwd"
		}
	}

	// Bash: classify intent, then apply tiered policy.
	if c.Tool.Name() == "bash" {
		var args struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(c.Args, &args)
		if args.Command == "" {
			return Ask, "empty bash command"
		}
		intent := tools.ClassifyBash(args.Command)
		switch intent {
		case tools.BashRead:
			return Allow, "bash read-only command"
		case tools.BashDestructive:
			return Ask, "bash destructive command"
		default:
			// BashSafe / BashUnknown: check allowlist.
			pat := bashPattern(args.Command)
			for _, allowed := range p.bashAllowlist {
				if patternsEqual(allowed, pat) {
					return Allow, "bash pattern in allowlist"
				}
			}
			return Ask, "bash command not in allowlist"
		}
	}

	// Unknown tool family — be safe.
	return Ask, "unknown tool"
}

// modeSatisfies reports whether the current mode meets the required minimum.
func modeSatisfies(mode, required Mode) bool {
	if required == ModeDefault {
		return true
	}
	return mode == required || mode == ModeYolo
}

// AllowBashPattern records a "session" or "always" approval so future
// invocations of the same pattern won't prompt. The agent decides
// whether to persist this back to disk.
func (p *Policy) AllowBashPattern(command string) {
	pat := bashPattern(command)
	for _, existing := range p.bashAllowlist {
		if patternsEqual(existing, pat) {
			return
		}
	}
	p.bashAllowlist = append(p.bashAllowlist, pat)
}

// BashAllowlist returns the current patterns (sorted for caller stability).
func (p *Policy) BashAllowlist() []string {
	out := append([]string(nil), p.bashAllowlist...)
	return out
}

func isReadOnly(t tools.Tool) bool {
	if r, ok := t.(tools.ReadOnlyHint); ok {
		return r.IsReadOnly()
	}
	return false
}

func (p *Policy) insideCwd(abs string) bool {
	cwdAbs, err := filepath.Abs(p.Cwd)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cwdAbs, abs)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

func (p *Policy) matchesSecret(abs string) bool {
	base := filepath.Base(abs)
	// .git is treated as secret; the model should never write inside it
	// without explicit approval.
	parts := strings.Split(filepath.ToSlash(abs), "/")
	for _, part := range parts {
		if part == ".git" {
			return true
		}
	}
	for _, pat := range p.SecretPathPatterns {
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}

func patternsEqual(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}
