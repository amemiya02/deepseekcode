// welcome.go renders the startup banner. Width-adaptive: a terminal
// narrower than the letterform wordmark falls back to a single-line
// greeting so nothing wraps mid-glyph. The wide banner is composed by
// logo.go (letterforms + diagonal fields + brand gradient).
package tui

import (
	"strings"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// renderWelcome returns the styled welcome banner. Wide terminals get the
// gradient letterform logo; narrow ones get a one-line greeting.
func renderWelcome(t Theme, width int) string {
	const fullWidth = 80 // letterform wordmark + diagonal fields need room

	if width < fullWidth {
		return t.ToolCall.Render("⏺ deepseekcode") + " " +
			t.Hint.Render("v"+version.Version+" · /help for commands") + "\n"
	}

	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString(renderLogo(t, "deepseek", "deepseekcode", "v"+version.Version, width))
	b.WriteByte('\n')
	b.WriteString(t.Hint.Render("  Terminal coding agent for DeepSeek"))
	b.WriteByte('\n')
	b.WriteString(t.Hint.Render("  ⏎ send · ⇧⏎ newline · /help · ^D quit"))
	b.WriteByte('\n')
	return b.String()
}
