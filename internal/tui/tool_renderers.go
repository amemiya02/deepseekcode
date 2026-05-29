package tui

import (
	"encoding/json"
	"fmt"
)

// RenderToolSummary renders a one-line tool summary with width-aware
// truncation. Different tools get different summary formats.
func RenderToolSummary(tool, args, result string, isError bool, width int) string {
	if width < 20 {
		width = 20
	}

	var summary string

	switch tool {
	case "read_file":
		summary = renderReadFileSummary(args, result, isError)
	case "bash":
		summary = renderBashSummary(args, result, isError)
	case "grep":
		summary = renderGrepSummary(args, result, isError)
	default:
		summary = renderDefaultSummary(tool, args, result, isError)
	}

	// UTF-8 safe truncation using rune-based slicing
	runes := []rune(summary)
	if len(runes) > width {
		summary = string(runes[:width-1]) + "…"
	}
	return summary
}

// renderReadFileSummary renders a summary for read_file tool.
func renderReadFileSummary(args, result string, isError bool) string {
	path := extractJSONString(args, "path")
	if path == "" {
		path = "unknown"
	}

	if isError {
		return fmt.Sprintf("✗ read %s", path)
	}

	// Count lines in result. lineCount counts the final line even without a
	// trailing newline (strings.Count undercounts it); show the count only for
	// genuinely multi-line output.
	lines := lineCount(result)
	if lines > 1 {
		return fmt.Sprintf("read %s (%d lines)", path, lines)
	}
	return fmt.Sprintf("read %s", path)
}

// renderBashSummary renders a summary for bash tool.
func renderBashSummary(args, result string, isError bool) string {
	cmd := extractJSONString(args, "command")
	if cmd == "" {
		cmd = "unknown"
	}

	// Truncate long commands (UTF-8 safe)
	cmdRunes := []rune(cmd)
	if len(cmdRunes) > 50 {
		cmd = string(cmdRunes[:47]) + "..."
	}

	if isError {
		return fmt.Sprintf("✗ bash: %s", cmd)
	}

	// Count lines in result (see renderReadFileSummary on lineCount vs strings.Count).
	lines := lineCount(result)
	if lines > 1 {
		return fmt.Sprintf("bash: %s (%d lines)", cmd, lines)
	}
	return fmt.Sprintf("bash: %s", cmd)
}

// renderGrepSummary renders a summary for grep tool.
func renderGrepSummary(args, result string, isError bool) string {
	pattern := extractJSONString(args, "pattern")
	if pattern == "" {
		pattern = "unknown"
	}

	// Truncate long patterns (UTF-8 safe)
	patternRunes := []rune(pattern)
	if len(patternRunes) > 30 {
		pattern = string(patternRunes[:27]) + "..."
	}

	if isError {
		return fmt.Sprintf("✗ grep: %s", pattern)
	}

	// Count matches (one per result line; lineCount counts the last line too).
	matches := lineCount(result)
	if matches > 1 {
		return fmt.Sprintf("grep: %s (%d matches)", pattern, matches)
	}
	return fmt.Sprintf("grep: %s", pattern)
}

// renderDefaultSummary renders a summary for other tools.
func renderDefaultSummary(tool, args, result string, isError bool) string {
	if isError {
		return fmt.Sprintf("✗ %s", tool)
	}

	// Count lines in result (see renderReadFileSummary on lineCount vs strings.Count).
	lines := lineCount(result)
	if lines > 1 {
		return fmt.Sprintf("%s (%d lines)", tool, lines)
	}
	return tool
}

// extractJSONString returns the string value of key in a JSON object, or "" if
// args isn't a well-formed object or key isn't a string. It parses with
// encoding/json (the same approach as compactArgs) rather than substring
// scanning, so escaped quotes and nested braces don't confuse it.
func extractJSONString(args, key string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
