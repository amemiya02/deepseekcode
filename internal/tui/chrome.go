// chrome.go renders the live activity band between the scrollback and
// the status line. It owns the streaming spinner, the phase caption,
// and the "new content below" indicator.
//
// The band is always reserved one row (in layout()) so its appearance
// and disappearance do not cause the viewport to reflow under the user.
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

// chromeState is the live activity indicator state. Updated by message
// handlers in app.go on reasoning/text/tool events.
type chromeState struct {
	active     bool
	phase      chromePhase
	activeTool string
	startedAt  time.Time
	tokens     int
	frame      int
}

// spinnerFrames is the 10-frame Braille spinner used by Claude Code,
// gh, and most modern CLIs. Each tick advances frame by one.
var spinnerFrames = []string{
	"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
}

func (c chromeState) spinner() string {
	return spinnerFrames[c.frame%len(spinnerFrames)]
}

// render returns the one-line activity caption. When inactive and the
// user has scrolled away from the bottom, it shows the "new content
// below" indicator instead. Returns "" when there is nothing to say —
// the reserved row will render as blank.
func (c chromeState) render(t Theme, showNewBelow bool) string {
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

// reset clears state to idle. Called on agentDoneMsg so the next run
// starts with a clean slate.
func (c *chromeState) reset() {
	c.active = false
	c.phase = ""
	c.activeTool = ""
	c.tokens = 0
}
