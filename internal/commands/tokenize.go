package commands

import "regexp"

var tokenizeRe = regexp.MustCompile(`"[^"]*"|'[^']*'|[^\s"']+`)

// Tokenize splits argline by whitespace, respecting single and double
// quotes. Matched quote pairs are stripped. Returns nil if no tokens.
func Tokenize(argline string) []string {
	matches := tokenizeRe.FindAllString(argline, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		n := len(m)
		if n >= 2 {
			if (m[0] == '"' && m[n-1] == '"') || (m[0] == '\'' && m[n-1] == '\'') {
				m = m[1 : n-1]
			}
		}
		out = append(out, m)
	}
	return out
}
