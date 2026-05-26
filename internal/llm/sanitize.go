package llm

// OmittedReasoningPlaceholder is the stable placeholder text inserted when
// an assistant message with tool calls is missing its thinking block.
// DeepSeek V4 thinking mode can reject replayed assistant messages that
// have tool_calls but no reasoning_content.
const OmittedReasoningPlaceholder = "(reasoning omitted)"

// SanitizeForDeepSeek returns a sanitized copy of the request suitable for
// DeepSeek V4 thinking mode. If thinking is disabled or absent, the request
// is returned unchanged. Otherwise, assistant messages containing tool calls
// but no thinking block have a placeholder thinking block prepended.
//
// The returned Request is a shallow copy with a copied Messages slice;
// individual Message and ContentBlock values are shared.
func (r Request) SanitizeForDeepSeek() Request {
	// Fast path: thinking disabled or absent
	if r.Thinking == nil || r.Thinking.Type != "enabled" {
		return r
	}

	// Check if any assistant message needs sanitization
	needsSanitize := false
	for _, m := range r.Messages {
		if m.Role != "assistant" {
			continue
		}
		if hasToolUseButNoThinking(m.Blocks) {
			needsSanitize = true
			break
		}
	}
	if !needsSanitize {
		return r
	}

	// Shallow copy the request and copy the Messages slice
	out := r
	out.Messages = make([]Message, len(r.Messages))
	copy(out.Messages, r.Messages)

	// Sanitize each assistant message in place
	for i := range out.Messages {
		if out.Messages[i].Role != "assistant" {
			continue
		}
		if hasToolUseButNoThinking(out.Messages[i].Blocks) {
			out.Messages[i].Blocks = prependThinkingBlock(out.Messages[i].Blocks)
		}
	}

	return out
}

// hasToolUseButNoThinking returns true if blocks contains at least one
// ToolUseBlock and no ThinkingBlock.
func hasToolUseButNoThinking(blocks []ContentBlock) bool {
	hasToolUse := false
	hasThinking := false
	for _, b := range blocks {
		switch b.(type) {
		case ToolUseBlock:
			hasToolUse = true
		case ThinkingBlock:
			hasThinking = true
		}
	}
	return hasToolUse && !hasThinking
}

// prependThinkingBlock returns a new slice with a placeholder ThinkingBlock
// prepended to the existing blocks.
func prependThinkingBlock(blocks []ContentBlock) []ContentBlock {
	out := make([]ContentBlock, 1, len(blocks)+1)
	out[0] = ThinkingBlock{Text: OmittedReasoningPlaceholder}
	out = append(out, blocks...)
	return out
}
