package llm

import (
	"strings"
	"unicode/utf8"
)

// trivialMaxRunes bounds a "trivial" message — a short greeting or acknowledgement
// ("你好", "hi", "thanks", "好的") that carries no task. At or below this rune count,
// and with no high-effort keyword, a turn should NOT force thinking: a non-reasoning
// turn comes back with reasoning_content ≈ the answer, which renders a redundant
// "Thought for <1s" block holding a duplicate of the reply. Kept small so a short
// real instruction ("read a file") stays above it and still thinks.
const trivialMaxRunes = 8

// IsTrivialMessage reports whether userText is a short greeting/ack that should not
// force thinking. A high-effort keyword (debug/error/调试/报错…) always makes a
// message non-trivial, regardless of length, so "报错" still thinks.
func IsTrivialMessage(userText string) bool {
	trimmed := strings.TrimSpace(userText)
	lower := strings.ToLower(trimmed)
	for _, kw := range highEffortKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}
	return utf8.RuneCountInString(trimmed) <= trivialMaxRunes
}

// highEffortKeywords: hitting any of these forces thinking ON.
// Copied verbatim from CodeWhale (Rust) §3.1.
var highEffortKeywords = []string{
	"debug", "error",
	"调试", "错误", "报错", "出错", "崩溃", // zh-Hans
	"調試", "錯誤", // zh-Hant
	"デバッグ", "エラー", "バグ", // ja
}

// lowEffortKeywords: hitting any of these forces thinking OFF.
var lowEffortKeywords = []string{
	"search", "lookup",
	"搜索", "查找", "查询", // zh
	"検索", // ja
}

// SelectThinking decides whether to enable thinking for this turn.
//   - isSubagent == true → false (short-cut; no sub-agents in v0.1)
//   - hit highEffortKeywords → true
//   - hit lowEffortKeywords → false
//   - otherwise → defaultOn
//
// Matching: lowercases ASCII in lastUserMsg, then substring-contains.
// CJK has no case, so ToLower is a no-op on those runes.
// High keywords take priority over low (same as CodeWhale).
func SelectThinking(isSubagent bool, lastUserMsg string, defaultOn bool) bool {
	if isSubagent {
		return false
	}
	lower := strings.ToLower(lastUserMsg)
	for _, kw := range highEffortKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	for _, kw := range lowEffortKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}
	return defaultOn
}
