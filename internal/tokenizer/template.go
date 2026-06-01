package tokenizer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// Special tokens (exact bytes; ｜ is U+FF5C full-width pipe).
const (
	tokBOS        = "<｜begin▁of▁sentence｜>"
	tokEOS        = "<｜end▁of▁sentence｜>"
	tokUser       = "<｜User｜>"
	tokAssistant  = "<｜Assistant｜>"
	tokThinkStart = "<think>"
	tokThinkEnd   = "</think>"
	dsmlPipe      = "｜DSML｜"
	dsmlToolCalls = "<｜DSML｜tool_calls>"
	dsmlTCEnd     = "</｜DSML｜tool_calls>"
	dsmlInvokePre = "<｜DSML｜invoke name=\""
	dsmlInvokeEnd = "</｜DSML｜invoke>"
	dsmlParamPre  = "<｜DSML｜parameter name=\""
	dsmlParamEnd  = "</｜DSML｜parameter>"
	toolResultPre = "<tool_result>"
	toolResultEnd = "</tool_result>"
)

// messageParts is the intermediate representation for template rendering.
type messageParts struct {
	role             string // "system", "user", "assistant"
	content          string
	reasoningContent string
	toolCalls        []toolCallParts
	toolResults      []string // <tool_result>...</tool_result> blocks to merge into user
}

type toolCallParts struct {
	name string
	args string // JSON arguments string
}

// renderDsmlToolCalls renders tool calls in the DSML XML format.
func renderDsmlToolCalls(tcs []toolCallParts) string {
	var invokes []string
	for _, tc := range tcs {
		invokes = append(invokes, dsmlInvokePre+tc.name+"\">\n"+encodeArgsToDsml(tc.args)+"\n"+dsmlInvokeEnd)
	}
	return "\n\n" + dsmlToolCalls + "\n" + strings.Join(invokes, "\n") + "\n" + dsmlTCEnd
}

// encodeArgsToDsml converts a JSON arguments string into DSML parameter blocks.
// Non-string values are JSON-encoded. Keys are sorted for deterministic output.
func encodeArgsToDsml(argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args = map[string]any{"arguments": argsJSON}
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		v := args[k]
		isStr := "false"
		var val string
		switch sv := v.(type) {
		case string:
			isStr = "true"
			val = sv
		default:
			j, err := json.Marshal(v)
			if err != nil {
				val = fmt.Sprintf("%v", sv)
			} else {
				val = string(j)
			}
		}
		parts = append(parts, dsmlParamPre+k+"\" string=\""+isStr+"\">"+val+dsmlParamEnd)
	}
	return strings.Join(parts, "\n")
}

// extractContent returns the concatenated TextBlock content from blocks.
func extractContent(blocks []llm.ContentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if tb, ok := blk.(llm.TextBlock); ok {
			b.WriteString(tb.Text)
		}
	}
	return b.String()
}

// extractReasoning returns the concatenated ThinkingBlock text from blocks.
func extractReasoning(blocks []llm.ContentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if tb, ok := blk.(llm.ThinkingBlock); ok {
			b.WriteString(tb.Text)
		}
	}
	return b.String()
}

// extractToolCalls returns ToolUseBlock info from blocks.
func extractToolCalls(blocks []llm.ContentBlock) []toolCallParts {
	var tcs []toolCallParts
	for _, blk := range blocks {
		if tb, ok := blk.(llm.ToolUseBlock); ok {
			args := string(tb.Input)
			if args == "" || args == "null" {
				args = "{}"
			}
			tcs = append(tcs, toolCallParts{name: tb.Name, args: args})
		}
	}
	return tcs
}

// extractToolResults returns <tool_result>...</tool_result> strings from blocks.
func extractToolResults(blocks []llm.ContentBlock) []string {
	var results []string
	for _, blk := range blocks {
		if tb, ok := blk.(llm.ToolResultBlock); ok {
			results = append(results, toolResultPre+tb.Content+toolResultEnd)
		}
	}
	return results
}

// FormatPrompt renders messages through the V4 chat template. dropThinking
// strips reasoning from assistant turns before the last user turn.
func FormatPrompt(messages []llm.Message, dropThinking bool) string {
	if len(messages) == 0 {
		return tokAssistant + tokThinkEnd
	}

	// Convert to messageParts.
	parts := make([]messageParts, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			parts = append(parts, messageParts{
				role:    "system",
				content: extractContent(msg.Blocks),
			})
		case "user":
			parts = append(parts, messageParts{
				role:    "user",
				content: extractContent(msg.Blocks),
			})
		case "assistant":
			parts = append(parts, messageParts{
				role:             "assistant",
				content:          extractContent(msg.Blocks),
				reasoningContent: extractReasoning(msg.Blocks),
				toolCalls:        extractToolCalls(msg.Blocks),
			})
		case "tool":
			// Tool results get merged into the preceding user message.
			results := extractToolResults(msg.Blocks)
			// Also check for content blocks (some tool messages use TextBlock).
			if content := extractContent(msg.Blocks); content != "" {
				results = append(results, toolResultPre+content+toolResultEnd)
			}
			// Find or create a preceding user message to merge into.
			merged := false
			for j := len(parts) - 1; j >= 0; j-- {
				if parts[j].role == "user" {
					parts[j].toolResults = append(parts[j].toolResults, results...)
					merged = true
					break
				}
			}
			if !merged {
				parts = append(parts, messageParts{
					role:        "user",
					toolResults: results,
				})
			}
		default:
			// Treat unknown roles as user.
			parts = append(parts, messageParts{
				role:    "user",
				content: extractContent(msg.Blocks),
			})
		}
	}

	// Drop thinking from assistant messages before the last user message.
	if dropThinking {
		lastUserIdx := -1
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i].role == "user" {
				lastUserIdx = i
				break
			}
		}
		if lastUserIdx >= 0 {
			for i := 0; i < lastUserIdx; i++ {
				if parts[i].role == "assistant" {
					parts[i].reasoningContent = ""
				}
			}
		}
	}

	// Render the prompt.
	var b strings.Builder
	b.WriteString(tokBOS)

	for i, mp := range parts {
		nextRole := ""
		if i+1 < len(parts) {
			nextRole = parts[i+1].role
		}

		switch mp.role {
		case "system":
			b.WriteString(mp.content)
		case "user":
			b.WriteString(tokUser)
			b.WriteString(mp.content)
			// Merge tool results into user content.
			for _, tr := range mp.toolResults {
				b.WriteString("\n\n")
				b.WriteString(tr)
			}
			if nextRole == "assistant" || nextRole == "" {
				b.WriteString(tokAssistant)
				b.WriteString(tokThinkEnd)
			}
		case "assistant":
			if mp.reasoningContent != "" {
				b.WriteString(tokThinkStart)
				b.WriteString(mp.reasoningContent)
				b.WriteString(tokThinkEnd)
			}
			b.WriteString(mp.content)
			if len(mp.toolCalls) > 0 {
				b.WriteString(renderDsmlToolCalls(mp.toolCalls))
			}
			b.WriteString(tokEOS)
		}
	}

	return b.String()
}

// CountMessages returns the EXACT token count of the rendered conversation
// (no sampling) and ok=true when the tokenizer is available; (0,false)
// otherwise. Uses CountExact internally — exact at any conversation size.
func CountMessages(messages []llm.Message) (n int, ok bool) {
	if !Available() {
		return 0, false
	}
	prompt := FormatPrompt(messages, false)
	return CountExact(prompt), true
}
