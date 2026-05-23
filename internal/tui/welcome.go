// welcome.go renders the startup ASCII banner: a stylized whale (the
// DeepSeek mascot) above the DEEPSEEKCODE wordmark and a short hint
// line. The banner adapts to terminal width — narrow terminals get a
// single-line greeting so nothing wraps mid-glyph.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// whaleArt is the multi-line ASCII whale. Kept narrow (≤ 28 cols) so
// it can sit comfortably beside the wordmark on standard terminals.
var whaleArt = []string{
	`               .--.`,
	`              /    \____`,
	`             |  ●        \___`,
	`              \              \`,
	`               \______________\`,
	`               ~~~~~~~~~~~~~~~~`,
}

// wordmarkArt is the DEEPSEEKCODE wordmark in a clean block font.
// Width ≈ 78 cols, height 5. Drops to the narrow fallback when the
// terminal can't fit it.
var wordmarkArt = []string{
	` ____  _____ _____ ____  ____  _____ _____ _  __ ____ ___  ____  _____ `,
	`|  _ \| ____| ____|  _ \/ ___|| ____| ____| |/ // ___/ _ \|  _ \| ____|`,
	`| | | |  _| |  _| | |_) \___ \|  _| |  _| | ' /| |  | | | | | | |  _|  `,
	`| |_| | |___| |___|  __/ ___) | |___| |___| . \| |__| |_| | |_| | |___ `,
	`|____/|_____|_____|_|   |____/|_____|_____|_|\_\\____\___/|____/|_____|`,
}

// renderWelcome returns the multi-line ASCII welcome banner styled
// for the current theme. Falls back to a compact single-line greeting
// when the terminal is too narrow for the full art.
func renderWelcome(t Theme, width int) string {
	const fullWidth = 80 // wordmark (78) + small margin

	// Compact fallback for narrow terminals — keep one short line so the
	// initial paint never wraps a glyph mid-character.
	if width < fullWidth {
		return t.ToolCall.Render("⏺ deepseekcode") + " " +
			t.Hint.Render("v"+version.Version+" · /help for commands") + "\n"
	}

	whaleStyle := t.ToolCall                            // cyan, our flash accent
	markStyle := t.StatusModel                          // cyan + bold
	tagStyle := t.Hint                                  // dim
	dotStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5f87d7")) // cool blue accent for the eye

	// Re-style the whale's eye dot separately so it pops without
	// repainting every line — split on the literal "●" we placed in
	// whaleArt.
	whale := make([]string, len(whaleArt))
	for i, line := range whaleArt {
		if idx := strings.Index(line, "●"); idx >= 0 {
			whale[i] = whaleStyle.Render(line[:idx]) +
				dotStyle.Render("●") +
				whaleStyle.Render(line[idx+len("●"):])
		} else {
			whale[i] = whaleStyle.Render(line)
		}
	}

	mark := make([]string, len(wordmarkArt))
	for i, line := range wordmarkArt {
		mark[i] = markStyle.Render(line)
	}

	var b strings.Builder
	b.WriteByte('\n')
	for _, line := range whale {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	for _, line := range mark {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(tagStyle.Render("  terminal coding agent for DeepSeek · v" + version.Version))
	b.WriteByte('\n')
	b.WriteString(tagStyle.Render("  ⏎ send · ⇧⏎ newline · /help · ^D quit"))
	b.WriteByte('\n')
	return b.String()
}
