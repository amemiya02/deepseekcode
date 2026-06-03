package memory

import "regexp"

// secretPatterns lists regexes whose matches are redacted.
// Each pattern should only redact the sensitive value portion,
// not surrounding context, so anchors are used carefully.
var secretPatterns = []*regexp.Regexp{
	// OpenAI / Anthropic / DeepSeek style API keys
	regexp.MustCompile(`sk-[A-Za-z0-9\-_]{20,}`),
	// DeepSeek dsk- keys
	regexp.MustCompile(`dsk-[A-Za-z0-9\-_]{20,}`),
	// Generic long hex secrets (32+ chars) after = or :
	regexp.MustCompile(`(?i)(secret[_\-]?(?:key|token|access[_\-]?key)|api[_\-]?key|token|password)\s*[=:]\s*\S{20,}`),
	// Bearer tokens (JWT and other long base64url strings)
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-_\.]{20,}`),
	// AWS-style keys
	regexp.MustCompile(`(?i)(AKIA|ASIA|AROA|AIDA)[A-Z0-9]{16}`),
	// Generic env var assignments with long values (SECRET/KEY/TOKEN/PASSWORD in name)
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
