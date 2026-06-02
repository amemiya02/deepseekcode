// internal/routing/clarify.go
package routing

import "strings"

// vaguePhrases are whole-prompt patterns that carry no concrete target.
var vaguePhrases = []string{"fix it", "make it better", "improve this",
	"do the thing", "make it work", "clean it up", "optimize it"}

// NeedsClarification reports whether a user prompt is too under-specified to act
// on safely — DeepSeek-V4's own survey names "misinterpretation of vague
// prompts" as a top failure mode, and clarifying first is far cheaper than a
// wrong Pro/Max turn. It returns up to two clarifying questions.
func NeedsClarification(userText string) (bool, []string) {
	t := strings.TrimSpace(strings.ToLower(userText))
	if t == "" {
		return true, []string{"What would you like me to do?"}
	}
	for _, p := range vaguePhrases {
		if t == p {
			return true, []string{
				"Which file or component should I change?",
				"What's the concrete outcome you want?",
			}
		}
	}
	// Very short with no path-like token is vague.
	if len(strings.Fields(t)) <= 3 && !strings.ContainsAny(t, "/.") {
		return true, []string{"Which file or area does this refer to?"}
	}
	return false, nil
}
