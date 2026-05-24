// scrollback.go owns the chat history: items, in-progress stream
// cursors, the rendered line cache, and visual-mode selection.
//
// Why a dedicated module. The bug-prone pattern this replaces was
// "raw pointers into the items slice" (streamText / streamThink),
// which dangled after slice realloc and needed a backward-walking
// rebind heuristic that itself had a cross-turn bleed bug. Indices
// stay valid because the items slice only ever appends: nothing
// reorders, nothing removes.
//
// Selection invariant. visual-mode selection is captured at the
// rendered-line index space; if the underlying items mutate during
// a selection (e.g. streaming tokens extend the buffer), the seq
// number drifts and the selection auto-invalidates on the next
// Render. Keeps the indexing model honest without needing to
// re-map selections across content changes.
package tui

import (
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// noStream is the sentinel for "no in-progress stream of this kind."
// Picking -1 (vs 0 or len) makes "is there a stream?" a single sign
// check rather than a bounds compare against an ever-changing slice.
const noStream = -1

// Scrollback is the deep module that owns the chat scrollback.
// Mutations only through methods. Indices into items are stable —
// callers may take them away and reuse them later.
type Scrollback struct {
	items []chatItem

	streamTextIdx  int       // noStream if no in-progress assistant text
	streamThinkIdx int       // noStream if no in-progress reasoning
	thinkStart     time.Time // wall-clock for the in-progress reasoning

	// Rendered-line cache. Re-computed when (seq, width) drifts.
	rendered  string
	fullLines []string
	renderW   int
	renderSeq uint64

	// seq bumps on every mutation. Used both to skip recomputing the
	// render cache and to invalidate visual selection if the index
	// space shifts under it.
	seq uint64

	// Visual selection. visSeq is the seq value the selection was
	// started against; if seq drifts, the selection clears.
	vis    visualState
	visSeq uint64
}

// NewScrollback returns an empty Scrollback with no in-progress
// streams and no selection.
func NewScrollback() *Scrollback {
	return &Scrollback{
		streamTextIdx:  noStream,
		streamThinkIdx: noStream,
	}
}

// --- one-shot appenders -----------------------------------------------------

// AppendUser closes any in-progress stream (turn boundary) and adds
// the user prompt line.
func (s *Scrollback) AppendUser(text string) {
	s.EndStreams()
	s.appendItem(chatItem{kind: itemUser, text: text, timestamp: time.Now()})
}

// AppendInfo adds an out-of-band notice. Closes streams: an info
// line interleaved with streaming would visually split the stream.
func (s *Scrollback) AppendInfo(text string) {
	s.EndStreams()
	s.appendItem(chatItem{kind: itemInfo, text: text, timestamp: time.Now()})
}

// AppendError adds an error line and closes any in-progress stream.
func (s *Scrollback) AppendError(text string) {
	s.EndStreams()
	s.appendItem(chatItem{kind: itemError, text: text, timestamp: time.Now()})
}

// AppendWelcome adds the startup banner. Does not touch streams (no
// stream is in progress at startup).
func (s *Scrollback) AppendWelcome() {
	s.appendItem(chatItem{kind: itemWelcome})
}

// AppendToolCall records a tool invocation and closes any in-progress
// text stream — tool calls partition assistant text into segments.
func (s *Scrollback) AppendToolCall(callID, tool, args string) {
	s.EndStreams()
	s.appendItem(chatItem{
		kind:       itemToolCall,
		tool:       tool,
		args:       args,
		toolCallID: callID,
		timestamp:  time.Now(),
	})
}

// AppendToolResult records the matching result for callID. Walks
// items backward to recover the tool name from the prior toolCall.
func (s *Scrollback) AppendToolResult(callID string, result tools.Result, dur time.Duration) {
	s.EndStreams()
	var tool string
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].kind == itemToolCall && s.items[i].toolCallID == callID {
			tool = s.items[i].tool
			break
		}
	}
	s.appendItem(chatItem{
		kind:       itemToolResult,
		tool:       tool,
		toolCallID: callID,
		result:     result,
		duration:   dur,
		timestamp:  time.Now(),
	})
}

// AppendHookFired adds a hook-execution line. Deny decisions are
// rendered in red; other decisions are informational.
func (s *Scrollback) AppendHookFired(hookName, event, decision, reason string, dur time.Duration) {
	s.EndStreams()
	s.appendItem(chatItem{
		kind:         itemHookFired,
		tool:         event,
		hookDecision: decision,
		hookReason:   reason,
		duration:     dur,
		timestamp:    time.Now(),
	})
}

// AppendStepFinish closes the step's footer line with usage and
// stop-reason. Ends any in-progress stream.
func (s *Scrollback) AppendStepFinish(stopReason string, usage llm.Usage, model string) {
	s.EndStreams()
	s.appendItem(chatItem{
		kind:       itemStepFinish,
		stopReason: stopReason,
		usage:      usage,
		model:      model,
		timestamp:  time.Now(),
	})
}

// --- streaming aggregators --------------------------------------------------

// StartReasoning marks the start of a reasoning block. The next
// AppendReasoning calls extend that block until EndReasoning.
func (s *Scrollback) StartReasoning() {
	s.thinkStart = time.Now()
	s.appendItem(chatItem{kind: itemReasoning, folded: true, timestamp: time.Now()})
	s.streamThinkIdx = len(s.items) - 1
}

// AppendReasoning extends the in-progress reasoning block. Defends
// against deltas without a prior Start by lazily starting one. Returns
// the rough token estimate (chars/4) for chrome counters.
func (s *Scrollback) AppendReasoning(delta string) int {
	if s.streamThinkIdx == noStream {
		s.StartReasoning()
	}
	it := &s.items[s.streamThinkIdx]
	it.reasoning += delta
	it.tokens = len(it.reasoning) / 4
	it.duration = time.Since(s.thinkStart)
	s.bump()
	return it.tokens
}

// EndReasoning closes the in-progress reasoning block. No-op if none.
func (s *Scrollback) EndReasoning() {
	if s.streamThinkIdx == noStream {
		return
	}
	s.items[s.streamThinkIdx].duration = time.Since(s.thinkStart)
	s.streamThinkIdx = noStream
	s.bump()
}

// AppendText extends the in-progress assistant-text block; creates
// one if none is in progress. Returns (createdNewBlock, runningTokens).
// Callers (chrome) flip to "writing" phase when createdNewBlock is true.
func (s *Scrollback) AppendText(delta string) (created bool, tokens int) {
	if s.streamTextIdx == noStream {
		s.appendItem(chatItem{kind: itemAssistantText, timestamp: time.Now()})
		s.streamTextIdx = len(s.items) - 1
		created = true
	}
	it := &s.items[s.streamTextIdx]
	it.text += delta
	s.bump()
	return created, len(it.text) / 4
}

// EndStreams closes any in-progress stream cursors. Idempotent —
// safe to call at turn boundaries (agent done) and from every
// one-shot appender as a defensive reset.
func (s *Scrollback) EndStreams() {
	if s.streamTextIdx != noStream {
		s.streamTextIdx = noStream
	}
	if s.streamThinkIdx != noStream {
		s.streamThinkIdx = noStream
	}
}

// --- read & render ----------------------------------------------------------

// Items returns a read-only snapshot of the items slice. The slice
// header may be retained for the caller's read but must not be
// mutated; mutations during streaming would race.
func (s *Scrollback) Items() []chatItem {
	return s.items
}

// Len returns the number of items.
func (s *Scrollback) Len() int { return len(s.items) }

// Seq returns the mutation counter. Bumped on any state change that
// affects rendering. Useful for tests and for dirty-detection.
func (s *Scrollback) Seq() uint64 { return s.seq }

// Render returns the ANSI-styled scrollback at the given width. The
// underlying line-split is cached on (width, seq); selection highlight
// is re-applied on every call since cursor moves don't bump seq.
//
// Side-effect: if the active selection's seq has drifted (the items
// changed underneath it), the selection clears here. See file header.
func (s *Scrollback) Render(t Theme, width int) string {
	if width <= 0 {
		return ""
	}
	if width != s.renderW || s.renderSeq != s.seq || s.rendered == "" {
		var b strings.Builder
		for _, it := range s.items {
			b.WriteString(it.render(t, width))
		}
		s.rendered = b.String()
		s.fullLines = strings.Split(s.rendered, "\n")
		s.renderW = width
		s.renderSeq = s.seq
		if s.vis.active && s.visSeq != s.seq {
			s.vis.active = false
		}
	}
	if !s.vis.active {
		return s.rendered
	}
	return strings.Join(applyVisualHighlightLines(s.fullLines, s.vis), "\n")
}

// FullLines returns the line split from the last Render call. Returns
// nil if Render hasn't been called yet. Used by mouse/visual cursor
// math to clamp into a valid index range.
func (s *Scrollback) FullLines() []string { return s.fullLines }

// --- toggles ---------------------------------------------------------------

// ToggleLastReasoning expands/collapses the most recent reasoning
// block. No-op if none.
func (s *Scrollback) ToggleLastReasoning() {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].kind == itemReasoning {
			s.items[i].folded = !s.items[i].folded
			s.bump()
			return
		}
	}
}

// ToggleAllReasoning flips every reasoning block to a single target
// state — collapse if any are open, else expand.
func (s *Scrollback) ToggleAllReasoning() {
	anyOpen := false
	for _, it := range s.items {
		if it.kind == itemReasoning && !it.folded {
			anyOpen = true
			break
		}
	}
	changed := false
	for i := range s.items {
		if s.items[i].kind == itemReasoning && s.items[i].folded == anyOpen {
			s.items[i].folded = !anyOpen
			changed = true
		}
	}
	if changed {
		s.bump()
	}
}

// ExpandLastResult expands the most recent collapsed (and currently
// truncated) tool result. Returns true if it found one.
func (s *Scrollback) ExpandLastResult() bool {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].kind == itemToolResult && !s.items[i].expanded && lineCount(s.items[i].result.Content) > 30 {
			s.items[i].expanded = true
			s.bump()
			return true
		}
	}
	return false
}

// LastToolResult returns the (tool, content) of the most recent
// non-empty tool result, or ("","",false) when none exists. Used by
// the pager overlay.
func (s *Scrollback) LastToolResult() (tool, content string, ok bool) {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].kind == itemToolResult && s.items[i].result.Content != "" {
			return s.items[i].tool, s.items[i].result.Content, true
		}
	}
	return "", "", false
}

// LastAssistantText returns the text of the most recent assistant
// text item, or "" when none.
func (s *Scrollback) LastAssistantText() string {
	for i := len(s.items) - 1; i >= 0; i-- {
		if s.items[i].kind == itemAssistantText && s.items[i].text != "" {
			return s.items[i].text
		}
	}
	return ""
}

// Clear empties the scrollback and resets stream cursors + selection.
func (s *Scrollback) Clear() {
	s.items = nil
	s.streamTextIdx = noStream
	s.streamThinkIdx = noStream
	s.vis = visualState{}
	s.bump()
}

// --- selection -------------------------------------------------------------

// BeginSelection starts a visual-mode selection anchored at line.
// Captures the current seq; subsequent index-space mutations
// invalidate the selection on the next Render.
func (s *Scrollback) BeginSelection(line int) {
	line = clampLine(line, len(s.fullLines))
	s.vis = visualState{active: true, anchor: line, cursor: line}
	s.visSeq = s.seq
}

// ExtendSelection moves the selection cursor to line. No-op if no
// selection is active.
func (s *Scrollback) ExtendSelection(line int) {
	if !s.vis.active {
		return
	}
	s.vis.cursor = clampLine(line, len(s.fullLines))
}

// SwapSelectionEnds swaps anchor and cursor, mirroring Vim's `o`.
func (s *Scrollback) SwapSelectionEnds() {
	if s.vis.active {
		s.vis.anchor, s.vis.cursor = s.vis.cursor, s.vis.anchor
	}
}

// SelectionCursor returns the current cursor line, or -1 if no
// selection is active. Used to drive viewport auto-scroll.
func (s *Scrollback) SelectionCursor() int {
	if !s.vis.active {
		return -1
	}
	return s.vis.cursor
}

// SelectionRange returns (lo, hi, active). lo ≤ hi.
func (s *Scrollback) SelectionRange() (lo, hi int, active bool) {
	if !s.vis.active {
		return 0, 0, false
	}
	lo, hi = s.vis.anchor, s.vis.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, true
}

// EndSelection returns the selected text (ANSI-stripped) and clears
// the selection. Returns "" if nothing was selected.
func (s *Scrollback) EndSelection() string {
	lo, hi, active := s.SelectionRange()
	s.vis.active = false
	if !active || len(s.fullLines) == 0 {
		return ""
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(s.fullLines) {
		hi = len(s.fullLines) - 1
	}
	if lo > hi {
		return ""
	}
	return stripANSI(strings.Join(s.fullLines[lo:hi+1], "\n"))
}

// CancelSelection drops any active selection without yanking.
func (s *Scrollback) CancelSelection() { s.vis.active = false }

// SelectionActive reports whether a selection is in progress.
func (s *Scrollback) SelectionActive() bool { return s.vis.active }

// --- internal --------------------------------------------------------------

func (s *Scrollback) appendItem(it chatItem) {
	s.items = append(s.items, it)
	s.bump()
}

func (s *Scrollback) bump() { s.seq++ }

func clampLine(line, total int) int {
	if line < 0 {
		return 0
	}
	if total > 0 && line >= total {
		return total - 1
	}
	return line
}
