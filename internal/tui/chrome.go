// chrome.go renders the live activity band between the scrollback and
// the status line, and owns the redraw-ticker lifecycle that drives
// it. The band is always reserved one row (in App.layout) so its
// appearance and disappearance never reflow the viewport.
//
// The Chrome module encapsulates: spinner phase + caption (thinking /
// writing / tool), the "new content below" hint, and the boolean
// tickActive flag that keeps the App from scheduling redundant
// coalescing ticks.
package tui

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// chromePhase enumerates what the agent is currently doing so the
// caption can match. "" means idle.
type chromePhase string

const (
	phaseThinking chromePhase = "thinking"
	phaseWriting  chromePhase = "writing"
	phaseTool     chromePhase = "tool"
)

// spinnerFrames is the 10-frame Braille spinner used by Claude Code,
// gh, and most modern CLIs. Each tick advances frame by one.
var spinnerFrames = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

// Chrome owns the live activity band — spinner, phase caption, and
// the tick-active flag that the App uses to avoid scheduling
// redundant redraw timers.
type Chrome struct {
	active     bool
	phase      chromePhase
	activeTool string
	startedAt  time.Time
	tokens     int
	frame      int
	ramp       []color.Color // gradient ramp, lazily initialized

	// Tool-batch progress. A "batch" is the set of tool calls the model
	// emitted in one turn (runToolCalls executes them in parallel); the
	// caption surfaces "N ready" so a multi-tool turn shows live progress.
	// batchClosed is set at each step boundary (EndToolBatch, driven by
	// EventStepFinish) and consumed by the next BeginTool so a fresh batch
	// resets even when the intervening model turn emitted no reasoning/text
	// (a pure tool-call turn never leaves phaseTool, so phase alone can't
	// delimit batches).
	toolsStarted int
	toolsReady   int
	batchClosed  bool

	// firstTokenTimeout mirrors llm.Client.FirstTokenTimeout so the
	// cold-start caption quotes the real timeout the agent honors rather
	// than a hardcoded range. Zero → caption suppressed.
	firstTokenTimeout time.Duration

	tickActive bool
}

// NewChrome returns an idle Chrome with no scheduled tick.
func NewChrome() *Chrome { return &Chrome{} }

// SetFirstTokenTimeout records the agent's real first-token timeout so the
// cold-start caption can quote it. Called once at App construction.
func (c *Chrome) SetFirstTokenTimeout(d time.Duration) { c.firstTokenTimeout = d }

// Active reports whether an activity is in progress (spinner shown).
func (c *Chrome) Active() bool { return c.active }

// TickActive reports whether a redraw ticker is currently scheduled.
func (c *Chrome) TickActive() bool { return c.tickActive }

// MarkTickStarted / MarkTickStopped flip the tick-scheduled flag.
// Idempotent — handlers call MarkTickStarted defensively.
func (c *Chrome) MarkTickStarted() { c.tickActive = true }

// MarkTickStopped is called when the redraw tick concludes its
// follow-up scheduling — agent idle AND chrome idle.
func (c *Chrome) MarkTickStopped() { c.tickActive = false }

// AdvanceFrame moves the spinner one tick forward.
func (c *Chrome) AdvanceFrame() { c.frame++ }

// BeginThinking transitions the band to the "thinking…" caption.
func (c *Chrome) BeginThinking() {
	c.active = true
	c.phase = phaseThinking
	c.activeTool = ""
	c.startedAt = time.Now()
	c.tokens = 0
}

// BeginWriting transitions to the "writing…" caption.
func (c *Chrome) BeginWriting() {
	c.active = true
	c.phase = phaseWriting
	c.activeTool = ""
	c.startedAt = time.Now()
	c.tokens = 0
}

// BeginTool transitions to the tool caption. It is called once per tool
// call; a turn's parallel tool calls each call it. The first call of a
// fresh batch (phase was not already tool) resets the batch counters and
// the elapsed clock; subsequent calls in the same batch only bump the
// started count so the caption can render "N ready" against the batch size.
func (c *Chrome) BeginTool(name string) {
	if c.phase != phaseTool || c.batchClosed {
		c.toolsStarted = 0
		c.toolsReady = 0
		c.startedAt = time.Now()
	}
	c.batchClosed = false
	c.active = true
	c.phase = phaseTool
	c.activeTool = name
	c.tokens = 0
	c.toolsStarted++
}

// EndToolBatch marks the current tool batch closed at a step boundary
// (called on EventStepFinish, once per step). The counters are NOT zeroed
// here — that is deferred to the next BeginTool so the completed "N ready"
// stays visible through the inter-step gap rather than flashing "0 ready".
func (c *Chrome) EndToolBatch() { c.batchClosed = true }

// ToolReady records that one tool call in the current batch has produced a
// result. Clamped to the started count. No-op outside a tool batch.
func (c *Chrome) ToolReady() {
	if c.toolsReady < c.toolsStarted {
		c.toolsReady++
	}
}

// UpdateTokens refreshes the rolling token estimate shown in the caption.
func (c *Chrome) UpdateTokens(n int) { c.tokens = n }

// Reset returns the band to idle — called on agent done.
func (c *Chrome) Reset() {
	c.active = false
	c.phase = ""
	c.activeTool = ""
	c.tokens = 0
	c.toolsStarted = 0
	c.toolsReady = 0
	c.batchClosed = false
}

// Render returns the one-line band content. When idle and the user
// has scrolled away from the bottom, it surfaces the "new content
// below" indicator instead. Returns "" when there's nothing to say —
// the reserved row renders as blank.
func (c *Chrome) Render(t Theme, showNewBelow bool) string {
	if !c.active {
		if showNewBelow {
			return t.Hint.Render("  ⏷ new content below (G to jump)")
		}
		return ""
	}
	elapsed := time.Since(c.startedAt).Seconds()
	var caption string
	switch c.phase {
	case phaseThinking:
		caption = fmt.Sprintf("thinking… %.1fs · %d tokens", elapsed, c.tokens)
		// Surface the real FirstTokenTimeout the agent honors, not a
		// hardcoded range, so the caption matches the configured timeout.
		if elapsed > 5.0 && c.tokens == 0 && c.firstTokenTimeout > 0 {
			caption += fmt.Sprintf("  (reasoner cold start; first token within %.0fs)", c.firstTokenTimeout.Seconds())
		}
	case phaseWriting:
		caption = fmt.Sprintf("writing… %.1fs · %d tokens", elapsed, c.tokens)
	case phaseTool:
		// Multi-tool turn: show batch progress ("N ready"); single tool:
		// name it.
		if c.toolsStarted > 1 {
			caption = fmt.Sprintf("running %d tools… %.1fs · %d ready", c.toolsStarted, elapsed, c.toolsReady)
		} else {
			caption = fmt.Sprintf("calling %s… %.1fs", c.activeTool, elapsed)
		}
	default:
		caption = fmt.Sprintf("working… %.1fs", elapsed)
	}
	// Caption rides a phase-offset gradient (shimmer) so the active
	// caption feels alive — the gradient sweep advances with each tick.
	// Static content (idle, cancel hint) uses flat tokens.
	c.ensureRamp(t)
	var captionRendered string
	if len(c.ramp) >= 2 {
		base := lipgloss.NewStyle()
		captionRendered = ApplyForegroundGradPhase(base, caption, c.ramp[0], c.ramp[len(c.ramp)/2], c.frame)
	} else {
		// Degraded: no gradient ramp (non-truecolor). Flat fgBase.
		captionRendered = lipgloss.NewStyle().Foreground(t.FgBase).Render(caption)
	}
	hintStyle := lipgloss.NewStyle().Foreground(t.FgFaint)
	return "  " + c.spinner(t) + " " +
		captionRendered + "  " +
		hintStyle.Render("[^C cancel]")
}

func (c *Chrome) spinner(t Theme) string {
	c.ensureRamp(t)
	ch := spinnerFrames[c.frame%len(spinnerFrames)]
	if len(c.ramp) == 0 {
		// Ramp unavailable (degraded color) — fall back to a flat brand color
		// so the spinner still reads as live activity.
		return lipgloss.NewStyle().Foreground(t.BrandLight).Render(ch)
	}
	return lipgloss.NewStyle().
		Foreground(c.ramp[c.frame%len(c.ramp)]).
		Render(ch)
}

func (c *Chrome) ensureRamp(t Theme) {
	if c.ramp != nil {
		return
	}
	c.ramp = makeGradientRamp(16, t.BrandDeep, t.BrandLight, t.BrandDeep)
}

// makeGradientRamp returns a slice of colors blended between the given
// stops using HCL interpolation.
func makeGradientRamp(size int, stops ...color.Color) []color.Color {
	if len(stops) < 2 {
		return nil
	}
	points := make([]colorful.Color, len(stops))
	for i, k := range stops {
		points[i], _ = colorful.MakeColor(k)
	}
	numSegments := len(stops) - 1
	if numSegments == 0 {
		return nil
	}
	blended := make([]color.Color, 0, size)
	segmentSizes := make([]int, numSegments)
	baseSize := size / numSegments
	remainder := size % numSegments
	for i := range numSegments {
		segmentSizes[i] = baseSize
		if i < remainder {
			segmentSizes[i]++
		}
	}
	for i := range numSegments {
		c1 := points[i]
		c2 := points[i+1]
		segmentSize := segmentSizes[i]
		for j := range segmentSize {
			if segmentSize == 0 {
				continue
			}
			t := float64(j) / float64(segmentSize)
			c := c1.BlendHcl(c2, t)
			blended = append(blended, c)
		}
	}
	return blended
}
