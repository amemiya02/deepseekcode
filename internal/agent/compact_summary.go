// compact_summary.go owns the deterministic local summarizer that
// CompactSession folds removed messages through. No LLM call — the
// summary is rules-based so compaction is fast, free, and
// reproducible across restarts of the same input.
package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

const (
	recentRequestPreviewChars = 160
	currentWorkPreviewChars   = 200
	timelinePreviewChars      = 80
	maxRecentRequests         = 3
)

var (
	keyFileRegexp = regexp.MustCompile(`[\w./\-]+\.(go|rs|py|js|ts|tsx|jsx|md|toml|json|yaml|yml|sh)\b`)
	pendingTerms  = []string{"todo", "next", "pending", "follow up", "follow-up", "remain"}
)

// summarizeMessages produces a deterministic <summary> XML-ish block
// from the messages being compacted. Empty input returns "" so the
// caller can treat "no work to summarize" as a no-op compaction.
//
// Output sections:
//   - messages: total + per-role counts
//   - tools_used: unique tool names invoked, sorted
//   - recent_requests: up to 3 most-recent user messages, truncated
//   - pending_work: sentences containing pending-work signal words
//   - key_files: paths extracted from any text by regex, sorted, unique
//   - current_work: last assistant TextBlock, truncated
//   - timeline: one line per message with role/turn/preview
func summarizeMessages(removed []llm.Message) string {
	if len(removed) == 0 {
		return ""
	}

	var (
		userCount, asstCount, toolCount int
		toolsUsed                       = map[string]struct{}{}
		recentRequests                  []string
		pending                         = map[string]struct{}{}
		keyFiles                        = map[string]struct{}{}
		currentWork                     string
		timeline                        []string
	)

	for i, m := range removed {
		switch m.Role {
		case "user":
			userCount++
		case "assistant":
			asstCount++
		case "tool":
			toolCount++
		}
		preview := messagePreview(m, timelinePreviewChars)
		timeline = append(timeline, fmt.Sprintf("[%s][%d] %s", m.Role, i+1, preview))

		for _, b := range m.Blocks {
			switch v := b.(type) {
			case llm.TextBlock:
				collectKeyFiles(v.Text, keyFiles)
				collectPending(v.Text, pending)
				if m.Role == "assistant" {
					currentWork = truncateRunes(v.Text, currentWorkPreviewChars)
				}
				if m.Role == "user" {
					recentRequests = appendRecent(recentRequests, truncateRunes(v.Text, recentRequestPreviewChars))
				}
			case llm.ThinkingBlock:
				collectKeyFiles(v.Text, keyFiles)
				collectPending(v.Text, pending)
			case llm.ToolUseBlock:
				if v.Name != "" {
					toolsUsed[v.Name] = struct{}{}
				}
				collectKeyFiles(string(v.Input), keyFiles)
			case llm.ToolResultBlock:
				collectKeyFiles(v.Content, keyFiles)
				collectPending(v.Content, pending)
			}
		}
	}

	var b strings.Builder
	b.WriteString("<summary>\n")
	fmt.Fprintf(&b, "- messages: %d total (%d user, %d assistant, %d tool)\n",
		len(removed), userCount, asstCount, toolCount)
	fmt.Fprintf(&b, "- tools_used: [%s]\n", strings.Join(sortedKeys(toolsUsed), ", "))
	b.WriteString("- recent_requests:\n")
	for _, r := range recentRequests {
		fmt.Fprintf(&b, "  - %s\n", r)
	}
	b.WriteString("- pending_work:\n")
	for _, p := range sortedKeys(pending) {
		fmt.Fprintf(&b, "  - %s\n", p)
	}
	fmt.Fprintf(&b, "- key_files: [%s]\n", strings.Join(sortedKeys(keyFiles), ", "))
	fmt.Fprintf(&b, "- current_work: %s\n", currentWork)
	b.WriteString("- timeline:\n")
	for _, line := range timeline {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteString("</summary>")
	return b.String()
}

func appendRecent(buf []string, item string) []string {
	buf = append(buf, item)
	if len(buf) > maxRecentRequests {
		buf = buf[len(buf)-maxRecentRequests:]
	}
	return buf
}

func collectKeyFiles(s string, into map[string]struct{}) {
	for _, m := range keyFileRegexp.FindAllString(s, -1) {
		into[m] = struct{}{}
	}
}

// collectPending records sentences containing any pending-work term.
// A "sentence" is anything separated by . ! ? or newline; we keep
// the trimmed sentence (capped at 200 chars) deduped via the map.
func collectPending(s string, into map[string]struct{}) {
	lower := strings.ToLower(s)
	for _, term := range pendingTerms {
		if !strings.Contains(lower, term) {
			continue
		}
		for _, sentence := range splitSentences(s) {
			if strings.Contains(strings.ToLower(sentence), term) {
				trimmed := strings.TrimSpace(sentence)
				if trimmed != "" {
					into[truncateRunes(trimmed, 200)] = struct{}{}
				}
			}
		}
	}
}

func splitSentences(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})
}

func messagePreview(m llm.Message, max int) string {
	for _, b := range m.Blocks {
		switch v := b.(type) {
		case llm.TextBlock:
			return truncateRunes(singleLine(v.Text), max)
		case llm.ThinkingBlock:
			return "(thinking) " + truncateRunes(singleLine(v.Text), max)
		case llm.ToolUseBlock:
			return fmt.Sprintf("(tool_use %s)", v.Name)
		case llm.ToolResultBlock:
			return truncateRunes(singleLine(v.Content), max)
		}
	}
	return ""
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// mergeCompactSummaries folds an earlier <summary> block (from a
// previous compaction round) into a fresh one, deduplicating the
// list fields (tools_used, key_files) and taking current_work from
// the new summary. Empty previousSummary returns newSummary
// verbatim. If either side fails to parse as the expected XML-ish
// format we fall back to a string concatenation with a visible
// separator so no information is silently lost.
func mergeCompactSummaries(previousSummary, newSummary string) string {
	if previousSummary == "" {
		return newSummary
	}
	prev, prevOK := parseSummaryFields(previousSummary)
	next, nextOK := parseSummaryFields(newSummary)
	if !prevOK || !nextOK {
		return previousSummary + "\n\n--- merged with newer summary ---\n\n" + newSummary
	}

	merged := summaryFields{
		messages:       next.messages, // newer count wins
		toolsUsed:      dedupSort(append(prev.toolsUsed, next.toolsUsed...)),
		recentRequests: next.recentRequests, // newer is more relevant
		pending:        dedupSort(append(prev.pending, next.pending...)),
		keyFiles:       dedupSort(append(prev.keyFiles, next.keyFiles...)),
		currentWork:    next.currentWork,
		timeline:       next.timeline, // newer timeline only
	}
	return renderSummary(merged)
}

type summaryFields struct {
	messages       string
	toolsUsed      []string
	recentRequests []string
	pending        []string
	keyFiles       []string
	currentWork    string
	timeline       []string
}

// parseSummaryFields recognizes the layout summarizeMessages emits.
// Returns ok=false if the body lacks a <summary> ... </summary>
// wrap so the caller can fall back to a safe concat.
func parseSummaryFields(s string) (summaryFields, bool) {
	if !strings.Contains(s, "<summary>") || !strings.Contains(s, "</summary>") {
		return summaryFields{}, false
	}
	var f summaryFields
	var section string
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, "- messages:"):
			f.messages = strings.TrimSpace(strings.TrimPrefix(line, "- messages:"))
			section = ""
		case strings.HasPrefix(line, "- tools_used:"):
			f.toolsUsed = parseBracketList(strings.TrimPrefix(line, "- tools_used:"))
			section = ""
		case strings.HasPrefix(line, "- recent_requests:"):
			section = "recent_requests"
		case strings.HasPrefix(line, "- pending_work:"):
			section = "pending_work"
		case strings.HasPrefix(line, "- key_files:"):
			f.keyFiles = parseBracketList(strings.TrimPrefix(line, "- key_files:"))
			section = ""
		case strings.HasPrefix(line, "- current_work:"):
			f.currentWork = strings.TrimSpace(strings.TrimPrefix(line, "- current_work:"))
			section = ""
		case strings.HasPrefix(line, "- timeline:"):
			section = "timeline"
		case strings.HasPrefix(line, "  - "):
			item := strings.TrimSpace(strings.TrimPrefix(line, "  -"))
			switch section {
			case "recent_requests":
				f.recentRequests = append(f.recentRequests, item)
			case "pending_work":
				f.pending = append(f.pending, item)
			}
		case strings.HasPrefix(line, "  ["):
			if section == "timeline" {
				f.timeline = append(f.timeline, strings.TrimSpace(line))
			}
		}
	}
	return f, true
}

func parseBracketList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func dedupSort(in []string) []string {
	seen := map[string]struct{}{}
	for _, v := range in {
		if v != "" {
			seen[v] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

func renderSummary(f summaryFields) string {
	var b strings.Builder
	b.WriteString("<summary>\n")
	if f.messages != "" {
		fmt.Fprintf(&b, "- messages: %s\n", f.messages)
	}
	fmt.Fprintf(&b, "- tools_used: [%s]\n", strings.Join(f.toolsUsed, ", "))
	b.WriteString("- recent_requests:\n")
	for _, r := range f.recentRequests {
		fmt.Fprintf(&b, "  - %s\n", r)
	}
	b.WriteString("- pending_work:\n")
	for _, p := range f.pending {
		fmt.Fprintf(&b, "  - %s\n", p)
	}
	fmt.Fprintf(&b, "- key_files: [%s]\n", strings.Join(f.keyFiles, ", "))
	fmt.Fprintf(&b, "- current_work: %s\n", f.currentWork)
	b.WriteString("- timeline:\n")
	for _, line := range f.timeline {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteString("</summary>")
	return b.String()
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
