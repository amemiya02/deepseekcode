// welcome.go renders the startup wordmark banner. Width-adaptive: a
// terminal narrower than the block-font wordmark falls back to a
// single-line greeting so nothing wraps mid-glyph.
package tui

import (
	"strings"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// wordmarkArt is the DEEPSEEKCODE wordmark in a clean block font.
// Width ≈ 78 cols, height 5.
var wordmarkArt = []string{
	` ____  _____ _____ ____  ____  _____ _____ _  __ ____ ___  ____  _____ `,
	`|  _ \| ____| ____|  _ \/ ___|| ____| ____| |/ // ___/ _ \|  _ \| ____|`,
	`| | | |  _| |  _| | |_) \___ \|  _| |  _| | ' /| |  | | | | | | |  _|  `,
	`| |_| | |___| |___|  __/ ___) | |___| |___| . \| |__| |_| | |_| | |___ `,
	`|____/|_____|_____|_|   |____/|_____|_____|_|\_\\____\___/|____/|_____|`,
}

// renderWelcome returns the styled welcome banner. Wordmark in bold
// cyan, tagline + key hints in dim.
func renderWelcome(t Theme, width int) string {
	const fullWidth = 80 // wordmark (78) + small margin

	if width < fullWidth {
		return t.ToolCall.Render("⏺ deepseekcode") + " " +
			t.Hint.Render("v"+version.Version+" · /help for commands") + "\n"
	}

	mark := t.StatusModel
	tag := t.Hint

	var b strings.Builder
	b.WriteByte('\n')
	for _, line := range wordmarkArt {
		b.WriteString(mark.Render(line))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(tag.Render("  terminal coding agent for DeepSeek · v" + version.Version))
	b.WriteByte('\n')
	b.WriteString(tag.Render("  ⏎ send · ⇧⏎ newline · /help · ^D quit"))
	b.WriteByte('\n')
	return b.String()
}
