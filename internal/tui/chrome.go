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
	"time"
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

	tickActive bool
}

// NewChrome returns an idle Chrome with no scheduled tick.
func NewChrome() *Chrome { return &Chrome{} }

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

// BeginTool transitions to the "calling <tool>…" caption.
func (c *Chrome) BeginTool(name string) {
	c.active = true
	c.phase = phaseTool
	c.activeTool = name
	c.startedAt = time.Now()
	c.tokens = 0
}

// UpdateTokens refreshes the rolling token estimate shown in the caption.
func (c *Chrome) UpdateTokens(n int) { c.tokens = n }

// Reset returns the band to idle — called on agent done.
func (c *Chrome) Reset() {
	c.active = false
	c.phase = ""
	c.activeTool = ""
	c.tokens = 0
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
		// Surface the FirstTokenTimeout reality the agent honors.
		if elapsed > 5.0 && c.tokens == 0 {
			caption += "  (reasoner cold start can take 30–45s)"
		}
	case phaseWriting:
		caption = fmt.Sprintf("writing… %.1fs · %d tokens", elapsed, c.tokens)
	case phaseTool:
		caption = fmt.Sprintf("calling %s… %.1fs", c.activeTool, elapsed)
	default:
		caption = fmt.Sprintf("working… %.1fs", elapsed)
	}
	return "  " + t.StatusModel.Render(c.spinner()) + " " +
		t.AssistantText.Render(caption) + "  " +
		t.Hint.Render("[^C cancel]")
}

func (c *Chrome) spinner() string {
	return spinnerFrames[c.frame%len(spinnerFrames)]
}
