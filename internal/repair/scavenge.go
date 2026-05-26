package repair

import (
	"encoding/json"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// ScavengeOptions configures the tool-call scavenger.
type ScavengeOptions struct {
	MaxBytes int // max bytes to scan per source (default 100 KiB)
	MaxCalls int // max calls to recover (default 4)
}

// ScavengeToolCalls recovers explicit registry-known tool calls from reasoning
// or content text when DeepSeek fails to emit structured tool_calls.
func ScavengeToolCalls(reasoning, content string, allowed map[string]struct{}, opts ScavengeOptions) Result {
	// Apply defaults
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 100 * 1024 // 100 KiB
	}
	if opts.MaxCalls <= 0 {
		opts.MaxCalls = 4
	}

	result := Result{}

	// Scan reasoning first, then content
	sources := []string{reasoning, content}
	seen := make(map[string]struct{}) // dedup by name + canonical args

	for _, src := range sources {
		if len(result.Calls) >= opts.MaxCalls {
			break
		}
		if src == "" {
			continue
		}
		// Truncate to MaxBytes
		if len(src) > opts.MaxBytes {
			src = src[:opts.MaxBytes]
		}

		// Find and extract tool calls from this source
		calls := extractToolCalls(src, allowed, &result.Reports, opts.MaxCalls-len(result.Calls))

		for _, call := range calls {
			// Deduplicate
			key := call.Function.Name + ":" + CanonicalArgsKey(call.Function.Arguments)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			result.Calls = append(result.Calls, call)
			if len(result.Calls) >= opts.MaxCalls {
				break
			}
		}
	}

	return result
}

// CanonicalArgsKey returns a canonical key for deduplication.
func CanonicalArgsKey(args string) string {
	canonical, _ := CanonicalArgs(args)
	return canonical
}

// extractToolCalls finds JSON-shaped tool calls in text and returns valid ones.
func extractToolCalls(text string, allowed map[string]struct{}, reports *[]Report, maxCalls int) []llm.ToolCall {
	var calls []llm.ToolCall

	// Scan for JSON objects that look like tool calls
	for i := 0; i < len(text) && len(calls) < maxCalls; i++ {
		// Look for opening brace
		if text[i] != '{' {
			continue
		}

		// Find matching closing brace, or use rest of string for partial JSON
		end := findMatchingBrace(text, i)
		var candidate string
		if end == -1 || end <= i {
			// No matching brace - try to repair partial JSON at end
			candidate = text[i:]
		} else {
			candidate = text[i : end+1]
		}

		// Try to parse as a tool call
		if call, ok := parseToolCall(candidate, allowed, reports); ok {
			calls = append(calls, call)
		}

		if end != -1 && end > i {
			i = end // continue after this object
		} else {
			break // no more complete objects possible
		}
	}

	return calls
}

// findMatchingBrace finds the position of the closing } for the { at pos.
func findMatchingBrace(s string, pos int) int {
	if pos >= len(s) || s[pos] != '{' {
		return -1
	}
	depth := 0
	inString := false

	for i := pos; i < len(s); i++ {
		ch := s[i]

		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

// parseToolCall attempts to parse a JSON object as a tool call.
// Returns the call and true if successful, or empty call and false otherwise.
// Even if arguments are invalid, returns the call if tool name is explicit and allowed.
func parseToolCall(jsonStr string, allowed map[string]struct{}, reports *[]Report) (llm.ToolCall, bool) {
	// Try to repair the JSON first if needed
	repair := RepairJSONArgs(jsonStr)
	candidate := repair.Repaired

	// Try DeepSeek/DSML shape: {"name":"tool","arguments":{...}}
	var dsmlShape struct {
		Name      string `json:"name"`
		Arguments any    `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(candidate), &dsmlShape); err == nil {
		if dsmlShape.Name != "" && isAllowed(dsmlShape.Name, allowed) {
			return buildToolCall(dsmlShape.Name, dsmlShape.Arguments, reports)
		}
	}

	// Try OpenAI shape: {"type":"function","function":{"name":"tool","arguments":"{...}"}}
	var openAIShape struct {
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(candidate), &openAIShape); err == nil {
		if openAIShape.Type == "function" && openAIShape.Function.Name != "" && isAllowed(openAIShape.Function.Name, allowed) {
			return buildToolCallFromString(openAIShape.Function.Name, openAIShape.Function.Arguments, reports)
		}
	}

	// Even if full parse fails, try to extract just the tool name with regex-like approach
	// This handles cases like {"name":"read_file","arguments":{invalid}}
	name := extractToolName(jsonStr)
	if name != "" && isAllowed(name, allowed) {
		// Try to extract arguments too
		args := extractToolArguments(jsonStr)
		return buildToolCallFromString(name, args, reports)
	}

	return llm.ToolCall{}, false
}

// extractToolName extracts the "name" field value from malformed JSON.
func extractToolName(s string) string {
	// Look for "name":"value" pattern
	idx := findKey(s, "name")
	if idx == -1 {
		return ""
	}
	// Find the string value after "name":
	valStart, valEnd := extractStringValue(s, idx)
	if valStart == -1 || valEnd == -1 {
		return ""
	}
	return s[valStart+1 : valEnd]
}

// extractToolArguments extracts the "arguments" field value from malformed JSON.
func extractToolArguments(s string) string {
	idx := findKey(s, "arguments")
	if idx == -1 {
		return "{}"
	}
	// Skip past "arguments":
	i := idx
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == ':' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	if i >= len(s) {
		return "{}"
	}
	// Arguments can be an object or string
	if s[i] == '{' {
		// Object - find matching brace
		end := findMatchingBrace(s, i)
		if end != -1 {
			return s[i : end+1]
		}
		// Partial object - return as-is
		return s[i:]
	} else if s[i] == '"' {
		// String - extract it
		valStart, valEnd := extractStringValue(s, i)
		if valStart != -1 && valEnd != -1 {
			return s[valStart+1 : valEnd]
		}
	}
	return "{}"
}

// findKey finds the position of "key": in s starting from beginning.
func findKey(s, key string) int {
	pattern := `"` + key + `"`
	idx := 0
	for {
		pos := indexSubstring(s, pattern, idx)
		if pos == -1 {
			return -1
		}
		// Check if followed by :
		after := pos + len(pattern)
		for after < len(s) && (s[after] == ' ' || s[after] == '\t' || s[after] == '\n' || s[after] == '\r') {
			after++
		}
		if after < len(s) && s[after] == ':' {
			return after + 1
		}
		idx = pos + 1
	}
}

// indexSubstring finds substring starting from pos.
func indexSubstring(s, sub string, start int) int {
	if start >= len(s) {
		return -1
	}
	for i := start; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// extractStringValue extracts a quoted string value starting at pos.
func extractStringValue(s string, pos int) (start, end int) {
	if pos >= len(s) || s[pos] != '"' {
		return -1, -1
	}
	start = pos
	for i := pos + 1; i < len(s); i++ {
		if s[i] == '"' && s[i-1] != '\\' {
			return start, i
		}
	}
	return -1, -1 // unterminated string
}

// isAllowed checks if a tool name is in the allowed set.
func isAllowed(name string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return false // empty allowed means nothing is allowed
	}
	_, ok := allowed[name]
	return ok
}

// buildToolCall constructs a ToolCall from name and arguments (any type).
func buildToolCall(name string, args any, reports *[]Report) (llm.ToolCall, bool) {
	var argsStr string
	switch v := args.(type) {
	case string:
		argsStr = v
	case map[string]any, []any, nil:
		if v == nil {
			argsStr = "{}"
		} else {
			b, err := json.Marshal(v)
			if err != nil {
				argsStr = "{}"
			} else {
				argsStr = string(b)
			}
		}
	default:
		b, err := json.Marshal(args)
		if err != nil {
			argsStr = "{}"
		} else {
			argsStr = string(b)
		}
	}
	return buildToolCallFromString(name, argsStr, reports)
}

// buildToolCallFromString constructs a ToolCall from name and arguments string.
func buildToolCallFromString(name, argsStr string, reports *[]Report) (llm.ToolCall, bool) {
	call := llm.ToolCall{
		ID:   fmt.Sprintf("recovered_%s", generateCallID()),
		Type: "function",
		Function: llm.ToolCallFunc{
			Name:      name,
			Arguments: argsStr,
		},
	}

	// Repair args if needed
	repair := RepairJSONArgs(argsStr)
	if repair.Changed {
		call.Function.Arguments = repair.Repaired
		*reports = append(*reports, Report{
			Kind:    KindRecovered,
			Tool:    name,
			CallID:  call.ID,
			Message: "arguments repaired",
		})
	}

	if !repair.Valid {
		// Invalid args - still return the call but mark it in reports
		*reports = append(*reports, Report{
			Kind:    KindArgsInvalid,
			Tool:    name,
			CallID:  call.ID,
			Message: "arguments invalid after repair attempt",
		})
	}

	return call, true
}

// generateCallID generates a simple unique suffix for recovered call IDs.
var callIDCounter int

func generateCallID() string {
	callIDCounter++
	return fmt.Sprintf("%d", callIDCounter)
}
