package memory

import "regexp"

// secretPatterns lists regexes whose matches are redacted.
// Patterns redact the sensitive value portion only; the variable name or
// surrounding context is preserved where possible.
// NOTE: sk-/dsk-/bearer patterns must precede the generic env-var catch-all
// (last entry) so that e.g. "token=sk-proj-…" is matched by the sk- rule
// (which replaces only the key value) before the env-var rule could consume
// the whole assignment.
var secretPatterns = []*regexp.Regexp{
	// OpenAI / Anthropic / DeepSeek style API keys — replace value only
	regexp.MustCompile(`sk-[A-Za-z0-9\-_]{20,}`),
	// DeepSeek dsk- keys — replace value only
	regexp.MustCompile(`dsk-[A-Za-z0-9\-_]{20,}`),
	// Generic long hex secrets (32+ chars) after = or :
	regexp.MustCompile(`(?i)(secret[_\-]?(?:key|token|access[_\-]?key)|api[_\-]?key|token|password)\s*[=:]\s*\S{20,}`),
	// Bearer tokens (JWT and other long base64url strings)
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-_\.]{20,}`),
	// AWS-style keys
	regexp.MustCompile(`(?i)(AKIA|ASIA|AROA|AIDA)[A-Z0-9]{16}`),
	// Generic env var assignments with long values (SECRET/KEY/TOKEN/PASSWORD in name).
	// This intentionally replaces the whole assignment (NAME=value) because a
	// look-behind is not available in Go's RE2 dialect. Must remain last so the
	// more-specific sk-/dsk-/bearer patterns above take priority.
	regexp.MustCompile(`(?i)(?:SECRET|KEY|TOKEN|PASSWORD)[A-Za-z0-9_]*\s*=\s*\S{16,}`),
}

// privateTagPattern matches <private>...</private> blocks (including multiline).
var privateTagPattern = regexp.MustCompile(`(?s)<private>.*?</private>`)

const redacted = "[REDACTED]"

// StripSecrets removes known secret patterns and <private>…</private> blocks
// from text before it is persisted to the memory store.
// It is idempotent: running it twice yields the same result.
func StripSecrets(text string) string {
	// Strip private tags first (multiline, dotall).
	text = privateTagPattern.ReplaceAllString(text, redacted)
	// Strip each secret pattern.
	for _, re := range secretPatterns {
		text = re.ReplaceAllString(text, redacted)
	}
	return text
}
