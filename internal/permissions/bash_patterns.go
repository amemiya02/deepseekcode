package permissions

import (
	"strings"
)

// bashPattern reduces a full command to a coarse pattern for allowlist
// matching. See docs/design.md §8.2.
//
// Heuristic:
//   1. Take the first word as the verb.
//   2. If the verb has a subcommand (git status / npm install / kubectl
//      delete / cargo build / docker push), include the subcommand.
//   3. Replace remaining args with a single "*".
//   4. Collapse trailing whitespace.
//
// Examples:
//
//   git status -sb           → "git status *"
//   git status               → "git status *"
//   npm install foo bar      → "npm install *"
//   rm -rf node_modules      → "rm -rf *"
//   ls                       → "ls"
//   ./build.sh --verbose     → "./build.sh *"
//
// We deliberately keep this loose; users who want surgical patterns can
// hand-edit ~/.deepseek/config.toml's allow_bash list.
func bashPattern(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	tokens := tokenize(cmd)
	if len(tokens) == 0 {
		return ""
	}
	verb := tokens[0]

	// Verbs with significant subcommands.
	subverbs := map[string]bool{
		"git":        true,
		"npm":        true,
		"pnpm":       true,
		"yarn":       true,
		"kubectl":    true,
		"cargo":      true,
		"docker":     true,
		"go":         true,
		"make":       true,
		"terraform":  true,
		"helm":       true,
		"poetry":     true,
		"pip":        true,
		"uv":         true,
		"brew":       true,
		"systemctl":  true,
		"apt":        true,
		"apt-get":    true,
	}

	// Verbs where the first flag should stay in the pattern.
	stickyFirstFlag := map[string]bool{
		"rm": true,
	}

	pattern := verb
	if subverbs[verb] && len(tokens) > 1 && !strings.HasPrefix(tokens[1], "-") {
		pattern += " " + tokens[1]
	} else if stickyFirstFlag[verb] && len(tokens) > 1 && strings.HasPrefix(tokens[1], "-") {
		pattern += " " + tokens[1]
	}

	if len(tokens) == 1 {
		return pattern
	}
	return pattern + " *"
}

// tokenize is a tiny shell-like splitter: respects single + double
// quotes, splits on whitespace otherwise. Good enough for pattern
// extraction; we don't need full POSIX semantics.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == ' ' || c == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
