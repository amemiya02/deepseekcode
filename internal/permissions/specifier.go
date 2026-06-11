package permissions

import (
	"fmt"
	"regexp"
	"strings"
)

// toolFamily maps friendly tool names used in specifier rules to the
// internal tool names that appear in PermissionRule.ToolPattern.
// For example, "Bash" expands to both "bash" and "bash_pty".
var toolFamily = map[string][]string{
	"Bash":      {"bash", "bash_pty"},
	"Read":      {"read_file"},
	"Edit":      {"edit_file"},
	"Write":     {"write_file"},
	"WebFetch":  {"web_fetch"},
	"WebSearch": {"web_search"},
}

// ParseSpecifierRule parses a raw specifier string like "Bash(go test:*)"
// or "Read(src/**)" into one or more PermissionRule entries. The decision
// parameter must be "allow", "deny", or "ask".
func ParseSpecifierRule(raw, decision string) ([]PermissionRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty rule")
	}
	name, spec, hasSpec, err := splitToolSpec(raw)
	if err != nil {
		return nil, err
	}
	tools := toolFamily[name]
	if tools == nil {
		tools = []string{name}
	}
	var s *Specifier
	if hasSpec {
		s, err = compileSpecifier(name, spec)
		if err != nil {
			return nil, err
		}
	}
	out := make([]PermissionRule, 0, len(tools))
	for _, tn := range tools {
		out = append(out, PermissionRule{ToolPattern: tn, Specifier: s, Decision: decision})
	}
	return out, nil
}

func splitToolSpec(raw string) (name, spec string, hasSpec bool, err error) {
	open := strings.IndexByte(raw, '(')
	if open < 0 {
		return raw, "", false, nil
	}
	if !strings.HasSuffix(raw, ")") {
		return "", "", false, fmt.Errorf("unbalanced parens in %q", raw)
	}
	name = strings.TrimSpace(raw[:open])
	spec = raw[open+1 : len(raw)-1]
	if name == "" {
		return "", "", false, fmt.Errorf("missing tool name in %q", raw)
	}
	return name, spec, true, nil
}

func compileSpecifier(name, spec string) (*Specifier, error) {
	switch name {
	case "Bash":
		spec = strings.TrimSpace(spec)
		if spec == "*" {
			return &Specifier{Field: "command", Prefix: ""}, nil
		}
		if strings.HasSuffix(spec, ":*") {
			return &Specifier{Field: "command", Prefix: strings.TrimSpace(strings.TrimSuffix(spec, ":*"))}, nil
		}
		return &Specifier{Field: "command", Prefix: spec}, nil
	default:
		return &Specifier{Field: "path", Glob: spec}, nil
	}
}

// globMatch matches path against a glob pattern supporting ** (any subtree)
// and * (single segment, no slash). Returns true if path matches.
func globMatch(glob, path string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); {
		switch {
		case strings.HasPrefix(glob[i:], "**"):
			if i+2 >= len(glob) {
				// trailing ** matches everything
				b.WriteString(".*")
			} else {
				// mid-pattern ** matches zero or more path segments;
				// skip the slash that follows since (.*/)? already has one
				b.WriteString("(.*/)?")
				if i+2 < len(glob) && glob[i+2] == '/' {
					i++ // skip the trailing / of **/
				}
			}
			i += 2
		case glob[i] == '*':
			b.WriteString("[^/]*")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
			i++
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(path)
}
