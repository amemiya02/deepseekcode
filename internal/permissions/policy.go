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
	"os"
	"path/filepath"
	"runtime"
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
	ModePlan                 // read-only + question/plan_exit whitelist, deny rest
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
	case ModePlan:
		if isReadOnly(c.Tool) || c.Tool.Name() == "question" || c.Tool.Name() == "plan_exit" {
			return Allow, "plan mode: read-only/whitelisted"
		}
		return Deny, "blocked by plan mode"
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
	//
	// Symlinks are resolved before classification so the gate agrees with the
	// tool layer's tools.ResolveAndCheck: a symlink inside cwd that points
	// outside (or into .git / a secret path) must NOT pass as a "safe write
	// inside cwd". Both the cwd anchor and the target are symlink-resolved; a
	// non-existent leaf resolves via its deepest existing parent.
	if pa, ok := c.Tool.(tools.PathAware); ok {
		paths := pa.AffectedPaths(c.Args)
		for _, raw := range paths {
			cwdReal, real, err := p.resolveAffected(raw)
			if err != nil {
				return Ask, "unresolvable path" // can't tell, be safe
			}
			if p.matchesSecret(real) {
				return Ask, "path matches secret pattern"
			}
			if !withinCwd(cwdReal, real) {
				return Ask, "path outside cwd"
			}
		}
		// Write/edit tools with all paths inside cwd and non-secret → auto-allow.
		if c.Tool.Name() == "write_file" || c.Tool.Name() == "edit_file" {
			return Allow, "safe write inside cwd"
		}
	}

	// Bash/bash_pty: classify intent, then apply tiered policy.
	if c.Tool.Name() == "bash" || c.Tool.Name() == "bash_pty" {
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

// DeriveChild returns a derived policy for sub-agent use: copies
// cwd/secret/bashAllowlist, shares read-only Rules pointer. Mode is
// clamped to the safer of parent and requested — sub-agents cannot
// escalate privilege.
func (p *Policy) DeriveChild(requested Mode) *Policy {
	return New(clampMode(p.Mode, requested), p.Cwd, p.SecretPathPatterns, p.BashAllowlist(), p.Rules)
}

// DeriveChildWithCwd is like DeriveChild but overrides the child's Cwd.
// Used when a sub-agent runs in a worktree with a different working directory.
func (p *Policy) DeriveChildWithCwd(requested Mode, cwd string) *Policy {
	child := p.DeriveChild(requested)
	child.Cwd = cwd
	return child
}

// ModeFromRuleset maps a `permission_ruleset` agent-def frontmatter value to a
// Mode. Recognized names: "default", "yolo", "read-only"/"readonly",
// "ask-all"/"askall", "plan". Returns ok=false for an unrecognized name so the
// caller can fall back and warn.
//
// Because spawn feeds the result through DeriveChild → clampMode, a ruleset can
// only ever RESTRICT a child relative to its parent, never escalate it: a child
// of a default-mode parent that requests "yolo" is clamped back to default.
func ModeFromRuleset(name string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "default":
		return ModeDefault, true
	case "yolo":
		return ModeYolo, true
	case "read-only", "readonly":
		return ModeReadOnly, true
	case "ask-all", "askall":
		return ModeAskAll, true
	case "plan":
		return ModePlan, true
	}
	return ModeDefault, false
}

// ModeMoreDangerousThan reports whether mode a permits strictly more than b on
// the same danger ordering clampMode uses. An admin floor (requirements.toml
// max_mode) uses it to refuse a launch mode that exceeds the configured ceiling.
func ModeMoreDangerousThan(a, b Mode) bool {
	return danger(a) > danger(b)
}

// clampMode returns the safer (lower danger) of a and b.
func clampMode(a, b Mode) Mode {
	if danger(a) <= danger(b) {
		return a
	}
	return b
}

// danger returns a numeric danger level for mode comparison.
// Lower = safer.
func danger(m Mode) int {
	switch m {
	case ModePlan:
		return 0
	case ModeReadOnly:
		return 0
	case ModeAskAll:
		return 1
	case ModeDefault:
		return 2
	case ModeYolo:
		return 3
	default:
		return 0
	}
}

// resolveAffected canonicalizes an affected path the way the tool layer's
// tools.ResolveAndCheck does, so the permission gate classifies the same bytes
// the tool will actually touch. It returns the symlink-resolved cwd anchor and
// the symlink-resolved target.
//
// Resolution mirrors ResolveAndCheck/evalExistingPath: relative paths join
// against the lexical cwd; symlinks are resolved on both the cwd anchor and the
// target; a non-existent leaf resolves via its deepest existing parent. When
// EvalSymlinks cannot run (e.g. the configured cwd does not exist on disk), the
// helper falls back to the lexical absolute path, preserving the gate's prior
// behavior for such configurations rather than failing closed everywhere.
func (p *Policy) resolveAffected(raw string) (cwdReal, real string, err error) {
	cwdAbs, err := filepath.Abs(p.Cwd)
	if err != nil {
		return "", "", err
	}
	cwdReal = cwdAbs
	if resolved, e := filepath.EvalSymlinks(cwdAbs); e == nil {
		cwdReal = resolved
	}

	// Relative targets resolve against the lexical cwd (matching ResolveAndCheck,
	// which joins against cwdAbs, not the symlink-resolved cwd).
	target := raw
	if !filepath.IsAbs(raw) {
		target = filepath.Join(cwdAbs, raw)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}
	real = evalExistingPath(abs)
	return cwdReal, real, nil
}

// evalExistingPath resolves symlinks on the deepest existing ancestor of abs
// and appends the non-existing remainder. This handles paths that do not exist
// yet (e.g. write_file creating a new file). It mirrors the unexported helper
// of the same name in internal/tools (which cannot be imported here), but never
// errors: an unresolvable path falls back to its lexical form.
func evalExistingPath(abs string) string {
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	existing := abs
	for {
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs
		}
		existing = parent
		if resolved, e := filepath.EvalSymlinks(existing); e == nil {
			rel, _ := filepath.Rel(existing, abs)
			return filepath.Join(resolved, rel)
		}
	}
}

// withinCwd reports whether path is equal to or resides inside root, after both
// have been canonicalized by the caller. It mirrors tools.isWithin: case-
// insensitive on Windows, and rejects parent-escapes and absolute rels.
func withinCwd(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
		path = strings.ToLower(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." && !filepath.IsAbs(rel))
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
