package repair

import (
	"encoding/json"
	"strings"
)

const maxArgsSize = 1 << 20 // 1 MiB

// ArgsRepair describes the result of a JSON truncation repair attempt.
type ArgsRepair struct {
	Raw      string   // original input
	Repaired string   // repaired output (or original if unrecoverable)
	Changed  bool     // true if any modification was made
	Valid    bool     // true if result is valid JSON
	Fallback bool     // true if input was unrecoverable or oversized
	Notes    []string // diagnostic notes
}

// RepairJSONArgs attempts to repair malformed JSON arguments.
// It only applies deterministic corrections: removing trailing commas,
// stripping control characters in strings, closing unterminated strings,
// and appending missing brackets/braces.
func RepairJSONArgs(raw string) ArgsRepair {
	result := ArgsRepair{Raw: raw, Repaired: raw}

	// Step 1: Fast-path valid JSON
	if json.Valid([]byte(raw)) {
		result.Valid = true
		result.Repaired = raw
		return result
	}

	// Step 2: Reject oversized input
	if len(raw) > maxArgsSize {
		result.Valid = false
		result.Fallback = true
		result.Notes = append(result.Notes, "input exceeds 1 MiB limit")
		return result
	}

	// Attempt repair
	repaired := repairJSON(raw)
	result.Repaired = repaired
	result.Changed = repaired != raw

	// Step 7: Check if repaired JSON is valid
	if json.Valid([]byte(repaired)) {
		result.Valid = true
		return result
	}

	// Step 8: Still invalid - return original
	result.Repaired = raw
	result.Valid = false
	result.Fallback = true
	result.Notes = append(result.Notes, "unrecoverable JSON")
	return result
}

// repairJSON applies deterministic repairs to malformed JSON.
func repairJSON(s string) string {
	// Step 3: Strip control characters inside JSON strings
	s = stripControlChars(s)

	// Step 4: Remove trailing commas before } or ]
	s = removeTrailingCommas(s)

	// Step 5 & 6: Close unterminated strings and add missing brackets
	s = closeUnclosed(s)

	return s
}

// stripControlChars removes literal control characters (newlines, tabs, etc.)
// that appear inside JSON strings.
func stripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
			b.WriteByte(ch)
			continue
		}

		if inString {
			// Inside a string: remove or escape control characters
			if ch < 0x20 {
				// Control character - remove it (could also escape, but removal is deterministic)
				continue
			}
		}
		b.WriteByte(ch)
	}

	return b.String()
}

// removeTrailingCommas removes commas that appear immediately before } or ]
func removeTrailingCommas(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}

		if !inString && ch == ',' {
			// Look ahead for } or ]
			remaining := s[i+1:]
			trimmed := strings.TrimLeft(remaining, " \t\n\r")
			if len(trimmed) > 0 && (trimmed[0] == '}' || trimmed[0] == ']') {
				// Skip this comma
				continue
			}
		}
		b.WriteByte(ch)
	}

	return b.String()
}

// closeUnclosed closes unterminated strings and adds missing brackets/braces.
func closeUnclosed(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 10) // extra space for closers

	inString := false
	stack := make([]byte, 0, 16) // stack of expected closers: } or ]

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
			b.WriteByte(ch)
			continue
		}

		if inString {
			b.WriteByte(ch)
			continue
		}

		// Outside string: track brackets
		switch ch {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == ch {
				stack = stack[:len(stack)-1]
			}
		}
		b.WriteByte(ch)
	}

	// Step 5: Close unterminated string
	if inString {
		b.WriteByte('"')
	}

	// Step 6: Append missing closers (in reverse order of stack)
	for i := len(stack) - 1; i >= 0; i-- {
		b.WriteByte(stack[i])
	}

	return b.String()
}
