package tui

import (
	"fmt"
	"strings"
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

	// Count lines in result
	lines := strings.Count(result, "\n")
	if lines > 0 {
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

	// Count lines in result
	lines := strings.Count(result, "\n")
	if lines > 0 {
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

	// Count matches
	matches := strings.Count(result, "\n")
	if matches > 0 {
		return fmt.Sprintf("grep: %s (%d matches)", pattern, matches)
	}
	return fmt.Sprintf("grep: %s", pattern)
}

// renderDefaultSummary renders a summary for other tools.
func renderDefaultSummary(tool, args, result string, isError bool) string {
	if isError {
		return fmt.Sprintf("✗ %s", tool)
	}

	// Count lines in result
	lines := strings.Count(result, "\n")
	if lines > 0 {
		return fmt.Sprintf("%s (%d lines)", tool, lines)
	}
	return tool
}

// extractJSONString extracts a string value from a JSON object.
// Returns empty string if not found or not a string.
func extractJSONString(json, key string) string {
	// Simple extraction for common patterns
	// Look for "key": "value" or "key":"value"
	search := `"` + key + `"`
	idx := strings.Index(json, search)
	if idx < 0 {
		return ""
	}

	// Find the colon after the key
	colonIdx := strings.Index(json[idx+len(search):], ":")
	if colonIdx < 0 {
		return ""
	}
	colonIdx += idx + len(search)

	// Find the opening quote
	startIdx := strings.Index(json[colonIdx:], `"`)
	if startIdx < 0 {
		return ""
	}
	startIdx += colonIdx

	// Find the closing quote (handle escaped quotes)
	endIdx := startIdx + 1
	for endIdx < len(json) {
		if json[endIdx] == '"' && json[endIdx-1] != '\\' {
			break
		}
		endIdx++
	}

	if endIdx >= len(json) {
		return ""
	}

	return json[startIdx+1 : endIdx]
}
