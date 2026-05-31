package repair

import (
	"encoding/json"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// DSML XML envelope delimiters. ｜ is U+FF5C (full-width pipe), not ASCII |.
const (
	dsmlInvokeOpen  = "<｜DSML｜invoke name=\""
	dsmlInvokeClose = "</｜DSML｜invoke>"
	dsmlParamOpen   = "<｜DSML｜parameter name=\""
	dsmlParamClose  = "</｜DSML｜parameter>"
)

// parseDSMLToolCalls extracts tool calls from DeepSeek V4 DSML XML envelopes
// embedded anywhere in text. Returns calls in document order. Returns nil when
// no <｜DSML｜invoke ...> is present. ｜ is U+FF5C (full-width pipe).
//
// Each <｜DSML｜parameter name="K" string="true|false">V</...> becomes one entry in
// the reconstructed JSON arguments object: string="true" => "K":"<V verbatim>";
// string="false" => "K": <V parsed as JSON, or V verbatim if it fails to parse>.
func parseDSMLToolCalls(text string) []llm.ToolCall {
	var calls []llm.ToolCall
	remaining := text

	for {
		// Find next invoke open tag.
		invokeIdx := strings.Index(remaining, dsmlInvokeOpen)
		if invokeIdx == -1 {
			break
		}

		// Move past the invoke open tag to read the tool name.
		pos := invokeIdx + len(dsmlInvokeOpen)

		// Read tool name up to the closing '">'.
		nameEnd := strings.Index(remaining[pos:], "\">")
		if nameEnd == -1 {
			break // Malformed: no closing quote.
		}
		name := remaining[pos : pos+nameEnd]
		pos += nameEnd + 2 // Skip past '">'

		// Find the matching invoke close tag.
		closeIdx := strings.Index(remaining[pos:], dsmlInvokeClose)
		if closeIdx == -1 {
			break // Unterminated invoke: skip and stop.
		}
		invokeBody := remaining[pos : pos+closeIdx]
		pos += closeIdx + len(dsmlInvokeClose)

		// Parse parameters within the invoke body.
		args := make(map[string]any)
		paramRemain := invokeBody
		truncated := false
		for {
			paramIdx := strings.Index(paramRemain, dsmlParamOpen)
			if paramIdx == -1 {
				break
			}

			paramPos := paramIdx + len(dsmlParamOpen)

			// Read parameter name up to the next '"'.
			pNameEnd := strings.Index(paramRemain[paramPos:], "\"")
			if pNameEnd == -1 {
				truncated = true
				break
			}
			paramName := paramRemain[paramPos : paramPos+pNameEnd]
			paramPos += pNameEnd + 1 // Skip past '"'

			// Expect ' string="' prefix.
			const stringAttr = " string=\""
			if !strings.HasPrefix(paramRemain[paramPos:], stringAttr) {
				truncated = true
				break
			}
			paramPos += len(stringAttr)

			// Read the string attribute value ("true" or "false").
			strEnd := strings.Index(paramRemain[paramPos:], "\"")
			if strEnd == -1 {
				truncated = true
				break
			}
			isString := paramRemain[paramPos:paramPos+strEnd] == "true"
			paramPos += strEnd + 1 // Skip past '"'

			// Expect '>' after the attributes.
			if paramPos >= len(paramRemain) || paramRemain[paramPos] != '>' {
				truncated = true
				break
			}
			paramPos++ // Skip '>'

			// Read value up to dsmlParamClose.
			valueEnd := strings.Index(paramRemain[paramPos:], dsmlParamClose)
			if valueEnd == -1 {
				truncated = true
				break // Unterminated parameter.
			}
			value := paramRemain[paramPos : paramPos+valueEnd]
			paramPos += valueEnd + len(dsmlParamClose)

			// Store in args map.
			if isString {
				args[paramName] = value
			} else {
				var parsed any
				if err := json.Unmarshal([]byte(value), &parsed); err != nil {
					args[paramName] = value // Fallback to raw string.
				} else {
					args[paramName] = parsed
				}
			}

			paramRemain = paramRemain[paramPos:]
		}

		// If the invoke body was truncated (malformed params), skip this call.
		if truncated && len(args) == 0 {
			remaining = remaining[pos:]
			continue
		}

		// Marshal args to JSON.
		argsJSON := "{}"
		if len(args) > 0 {
			b, err := json.Marshal(args)
			if err == nil {
				argsJSON = string(b)
			}
		}

		// Build the tool call using the existing recovered_ ID scheme.
		call := llm.ToolCall{
			ID:   "recovered_" + generateCallID(),
			Type: "function",
			Function: llm.ToolCallFunc{
				Name:      name,
				Arguments: argsJSON,
			},
		}
		calls = append(calls, call)

		remaining = remaining[pos:]
	}

	if len(calls) == 0 {
		return nil
	}
	return calls
}
