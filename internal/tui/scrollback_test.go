package tui

import (
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestStreamTextSegmentsAcrossTurnBoundary pins the bug we fixed
// when streamText was a pointer-into-slice: a new turn's first text
// delta must NOT extend the previous turn's assistant-text block.
func TestStreamTextSegmentsAcrossTurnBoundary(t *testing.T) {
	s := NewScrollback()
	// Turn 1.
	s.AppendUser("hello")
	s.AppendText("world")
	s.AppendStepFinish("stop", llm.Usage{}, "x")
	// Turn 2.
	s.AppendUser("what next")
	s.AppendText("answer")

	items := s.Items()
	// Find both asst text items; the second must NOT have absorbed
	// the first one's content.
	var asstTexts []string
	for _, it := range items {
		if it.kind == itemAssistantText {
			asstTexts = append(asstTexts, it.text)
		}
	}
	if len(asstTexts) != 2 {
		t.Fatalf("want 2 separate assistant-text items, got %d (%v)", len(asstTexts), asstTexts)
	}
	if asstTexts[0] != "world" || asstTexts[1] != "answer" {
		t.Errorf("text bled across turns: %v", asstTexts)
	}
}

// TestStreamTextSegmentsAcrossToolCall confirms a tool call inside
// the same step splits the text stream — the next text delta after
// the tool result starts a fresh segment.
func TestStreamTextSegmentsAcrossToolCall(t *testing.T) {
	s := NewScrollback()
	s.AppendUser("do X")
	s.AppendText("starting")
	s.AppendToolCall("c1", "read_file", `{"path":"a"}`)
	s.AppendToolResult("c1", tools.Result{Content: "hi"}, 0)
	s.AppendText("finishing")

	var asst []string
	for _, it := range s.Items() {
		if it.kind == itemAssistantText {
			asst = append(asst, it.text)
		}
	}
	if len(asst) != 2 || asst[0] != "starting" || asst[1] != "finishing" {
		t.Errorf("tool call should split text segments, got %v", asst)
	}
}

// TestStreamReasoningLifecycle covers start → deltas → end → restart.
func TestStreamReasoningLifecycle(t *testing.T) {
	s := NewScrollback()
	s.StartReasoning()
	s.AppendReasoning("first ")
	s.AppendReasoning("thought")
	s.EndReasoning()

	items := s.Items()
	if len(items) != 1 || items[0].kind != itemReasoning {
		t.Fatalf("expected one reasoning item, got %+v", items)
	}
	if items[0].reasoning != "first thought" {
		t.Errorf("reasoning bytes lost: %q", items[0].reasoning)
	}

	// A second StartReasoning opens a new block, not extends the old one.
	s.StartReasoning()
	s.AppendReasoning("second")
	items = s.Items()
	if len(items) != 2 {
		t.Fatalf("expected two reasoning items, got %d", len(items))
	}
	if items[1].reasoning != "second" {
		t.Errorf("second reasoning bled into first: %q", items[1].reasoning)
	}
}

// TestAppendTextLazilyCreatesBlock confirms AppendText creates a new
// chatItem on first call and returns created=true; subsequent calls
// extend it and return created=false.
func TestAppendTextLazilyCreatesBlock(t *testing.T) {
	s := NewScrollback()
	if created, _ := s.AppendText("hi"); !created {
		t.Errorf("first AppendText should report created=true")
	}
	if created, _ := s.AppendText(" there"); created {
		t.Errorf("second AppendText should report created=false")
	}
	if s.Items()[0].text != "hi there" {
		t.Errorf("text lost on second delta: %q", s.Items()[0].text)
	}
}

// TestSelectionInvalidatesOnSeqDrift covers the design choice that a
// selection captured at seq N must invalidate when items mutate
// beneath it. Otherwise visual-mode indices would silently drift.
func TestSelectionInvalidatesOnSeqDrift(t *testing.T) {
	s := NewScrollback()
	s.AppendInfo("line1")
	s.AppendInfo("line2")
	s.AppendInfo("line3")
	// Render so FullLines is populated and selection can anchor.
	_ = s.Render(DarkTheme(), 80)
	s.BeginSelection(0)
	if !s.SelectionActive() {
		t.Fatalf("BeginSelection failed to activate")
	}
	// Stream a token: bumps seq.
	s.AppendText("delta")
	// Next Render must auto-invalidate the selection.
	_ = s.Render(DarkTheme(), 80)
	if s.SelectionActive() {
		t.Errorf("selection should auto-invalidate when seq drifts")
	}
}

// TestClearResetsEverything pins /clear semantics: empty items, no
// in-progress stream, no selection.
func TestClearResetsEverything(t *testing.T) {
	s := NewScrollback()
	s.AppendUser("hi")
	s.StartReasoning()
	s.AppendReasoning("...")
	_ = s.Render(DarkTheme(), 80)
	s.BeginSelection(0)

	s.Clear()
	if s.Len() != 0 {
		t.Errorf("Clear should empty items; got %d", s.Len())
	}
	if s.SelectionActive() {
		t.Errorf("Clear should drop selection")
	}
	// Next AppendText must create fresh (no carried-over streamTextIdx).
	if created, _ := s.AppendText("after"); !created {
		t.Errorf("Clear should reset streamTextIdx; AppendText after Clear must report created=true")
	}
}

// TestLastToolResult and TestLastAssistantText guard the small
// accessors used by the pager + yank commands.
func TestLastToolResult(t *testing.T) {
	s := NewScrollback()
	if _, _, ok := s.LastToolResult(); ok {
		t.Errorf("empty scrollback should have no LastToolResult")
	}
	s.AppendToolCall("c1", "ls", "{}")
	s.AppendToolResult("c1", tools.Result{Content: "out"}, 0)
	tool, content, ok := s.LastToolResult()
	if !ok || tool != "ls" || content != "out" {
		t.Errorf("LastToolResult mismatch: tool=%q content=%q ok=%v", tool, content, ok)
	}
}

func TestLastAssistantText(t *testing.T) {
	s := NewScrollback()
	if got := s.LastAssistantText(); got != "" {
		t.Errorf("empty scrollback should have empty LastAssistantText")
	}
	s.AppendText("one")
	s.AppendUser("u")
	s.AppendText("two")
	if got := s.LastAssistantText(); got != "two" {
		t.Errorf("LastAssistantText should pick the most recent; got %q", got)
	}
}

// TestRenderCachesByWidthAndSeq confirms Render is cheap on a repeat
// call with the same (width, seq).
func TestRenderCachesByWidthAndSeq(t *testing.T) {
	s := NewScrollback()
	s.AppendUser("hi")
	first := s.Render(DarkTheme(), 80)
	second := s.Render(DarkTheme(), 80)
	if first != second {
		t.Errorf("cached Render should be identical: first=%q second=%q", first, second)
	}
	// Strip ANSI for an easier substring check.
	if !strings.Contains(stripANSI(first), "hi") {
		t.Errorf("render missing user text: %q", first)
	}
}

// TestExpandLastResultMatchesRenderThreshold pins the fix for the broken `e`
// affordance: a tool result longer than maxBodyLines (so the renderer shows
// "… press e to expand") MUST be expandable, while a short one is a no-op. The
// old hard-coded > 30 gate left 11..30-line results stuck — hint shown,
// expansion refused.
func TestExpandLastResultMatchesRenderThreshold(t *testing.T) {
	s := NewScrollback()
	s.AppendToolCall("c1", "bash", "echo")
	// > maxBodyLines (10) but well under the old 30-line gate.
	s.AppendToolResult("c1", tools.Result{Content: strings.Repeat("x\n", maxBodyLines+5)}, 0)
	if !s.ExpandLastResult() {
		t.Fatalf("a %d-line result (> maxBodyLines=%d) must be expandable via e", maxBodyLines+5, maxBodyLines)
	}
	if s.ExpandLastResult() {
		t.Error("an already-expanded result must not re-expand")
	}

	// A short result is never truncated, so e is a no-op.
	short := NewScrollback()
	short.AppendToolCall("c2", "bash", "echo")
	short.AppendToolResult("c2", tools.Result{Content: "a\nb\nc"}, 0)
	if short.ExpandLastResult() {
		t.Error("a result <= maxBodyLines must not report as expandable")
	}
}

// TestFinishedPrefixReuseOnTokenDelta verifies that appending a token
// delta (AppendText) reuses the finishedPrefix, while appending a new
// structural item (AppendToolCall) forces a rebuild.
func TestFinishedPrefixReuseOnTokenDelta(t *testing.T) {
	s := NewScrollback()
	s.AppendUser("hello")
	s.AppendText("streaming")
	_ = s.Render(DarkTheme(), 80)

	// Record the structureSeq after item creation (which does bump).
	structBefore := s.structureSeq

	// Token delta: should NOT bump structureSeq.
	s.AppendText(" more")
	_ = s.Render(DarkTheme(), 80)
	if s.structureSeq != structBefore {
		t.Errorf("AppendText delta should not bump structureSeq: before=%d after=%d", structBefore, s.structureSeq)
	}

	// Structural change: should bump structureSeq.
	s.AppendToolCall("c1", "bash", "{}")
	_ = s.Render(DarkTheme(), 80)
	if s.structureSeq <= structBefore {
		t.Errorf("AppendToolCall should bump structureSeq: before=%d after=%d", structBefore, s.structureSeq)
	}
}

// TestEndStreamsBumpsSeq verifies that EndStreams invalidates the (width, seq)
// render cache by bumping seq, so the next Render recomputes through the
// finalized item path (no streaming branch).
func TestEndStreamsBumpsSeq(t *testing.T) {
	s := NewScrollback()
	s.AppendText("streaming text")
	seqBefore := s.Seq()
	s.EndStreams()
	if s.Seq() <= seqBefore {
		t.Errorf("EndStreams must bump seq: before=%d after=%d", seqBefore, s.Seq())
	}
}

// TestAppendTextCreationBumpsStructureSeq distinguishes first-token
// creation (item identity change → bump) from later token deltas (no bump).
func TestAppendTextCreationBumpsStructureSeq(t *testing.T) {
	s := NewScrollback()
	before := s.structureSeq
	created, _ := s.AppendText("first")
	if !created {
		t.Fatal("first AppendText should report created=true")
	}
	if s.structureSeq != before+1 {
		t.Fatalf("AppendText creation must bump structureSeq: before=%d after=%d", before, s.structureSeq)
	}
	// Subsequent delta must NOT bump.
	before = s.structureSeq
	s.AppendText(" second")
	if s.structureSeq != before {
		t.Fatalf("AppendText delta must not bump structureSeq: before=%d after=%d", before, s.structureSeq)
	}
}

// TestRenderOutputIdenticalToConcat verifies that Render output is
// identical to a from-scratch concat of all item renders (no caching
// artifact changes the output).
func TestRenderOutputIdenticalToConcat(t *testing.T) {
	s := NewScrollback()
	s.AppendUser("user text")
	s.AppendToolCall("c1", "bash", "{}")
	s.AppendToolResult("c1", tools.Result{Content: "output"}, 0)
	s.AppendText("assistant text")
	th := DarkTheme()
	w := 80

	got := s.Render(th, w)

	// Build expected from scratch.
	var b strings.Builder
	for _, it := range s.Items() {
		b.WriteString(it.render(th, w))
	}
	want := b.String()

	if got != want {
		t.Errorf("Render output differs from concat:\ngot=%q\nwant=%q", got, want)
	}
}

// TestFinishedPrefixCacheInvalidationByStructureSeq verifies that the
// finished prefix cache is invalidated when structureSeq changes.
func TestFinishedPrefixCacheInvalidationByStructureSeq(t *testing.T) {
	s := NewScrollback()
	s.AppendUser("hello")
	s.AppendText("streaming")
	_ = s.Render(DarkTheme(), 80)

	// The finishedPrefix should contain the user item.
	if s.finishedPrefix == "" {
		t.Fatal("expected non-empty finishedPrefix after first render")
	}

	// Append a structural item — should invalidate cache.
	s.AppendInfo("notice")
	_ = s.Render(DarkTheme(), 80)

	// The finishedPrefix should now include the info item.
	if !strings.Contains(s.finishedPrefix, "notice") {
		t.Errorf("finishedPrefix should include info item after structural change: %q", s.finishedPrefix)
	}
}

func TestInvalidateRenderCacheKeepsItems(t *testing.T) {
	s := NewScrollback()
	s.AppendText("hello world")
	s.AppendToolCall("tc1", "bash", "echo hi")
	s.AppendToolResult("tc1", tools.Result{Content: "output"}, 0)
	before := len(s.Items())
	if before == 0 {
		t.Fatal("expected items before invalidation")
	}

	s.InvalidateRenderCache()

	after := len(s.Items())
	if after != before {
		t.Errorf("items count changed: before=%d after=%d", before, after)
	}
	// Verify items are still accessible and correct.
	if s.Items()[0].text != "hello world" {
		t.Errorf("text lost after invalidation: %q", s.Items()[0].text)
	}
}
