// welcome.go renders the startup banner. Width-adaptive: a terminal
// narrower than the letterform wordmark falls back to a single-line
// greeting so nothing wraps mid-glyph. The wide banner is composed by
// logo.go (letterforms + diagonal fields + brand gradient).
package tui

import (
	"strings"

	"github.com/amemiya02/deepseekcode/internal/i18n"
	"github.com/amemiya02/deepseekcode/internal/version"
)

// renderWelcome returns the styled welcome banner. Wide terminals get the
// gradient letterform logo; narrow ones get a one-line greeting.
func renderWelcome(t Theme, width int) string {
	const fullWidth = 80 // letterform wordmark + diagonal fields need room

	if width < fullWidth {
		return t.ToolCall.Render(i18n.T("welcome.narrow")) + " " +
			t.Hint.Render(i18n.T("welcome.narrow.hint", version.Display())) + "\n"
	}

	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString(renderLogo(t, "deepseek", "deepseekcode", version.Display(), width))
	b.WriteByte('\n')
	b.WriteString(t.Hint.Render("  " + i18n.T("welcome.tagline")))
	b.WriteByte('\n')
	b.WriteString(t.Hint.Render("  " + i18n.T("welcome.hint.send")))
	b.WriteByte('\n')
	return b.String()
}
