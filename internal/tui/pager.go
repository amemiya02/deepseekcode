// pager.go implements a fullscreen overlay for viewing tool results
// that would otherwise dominate the chat scrollback. The pager owns
// its own viewport so j/k/G/gg navigate the long output independently
// of the chat scroll position.
//
// Activated by `p` in Normal mode (opens the last tool result) and by
// future callers like /paste, /diff. Dismissed by `q` or esc, which
// returns the user to Insert mode with input refocused.
package tui

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pagerState owns the pager overlay. nil when not active.
type pagerState struct {
	title   string
	content string
	vp      viewport.Model
}

// openPager replaces the main chat view with a pager showing content.
// Caller is responsible for any state validation; this just shows what
// it is given.
func (a *App) openPager(title, content string) {
	h := a.height - 2
	if h < 5 {
		h = 5
	}
	vp := viewport.New(a.width, h)
	vp.MouseWheelEnabled = true
	vp.SetContent(content)
	a.pager = &pagerState{
		title:   title,
		content: content,
		vp:      vp,
	}
	_ = a.setMode(modePager)
}

// closePager dismisses the pager and returns focus to the textarea.
func (a *App) closePager() tea.Cmd {
	a.pager = nil
	return a.setMode(modeInsert)
}

// render draws the pager: a one-line title bar plus the viewport body.
// Sized to (width, height) so the caller controls the overall budget.
func (p *pagerState) render(t Theme, width, height int) string {
	p.vp.Width = width
	bodyH := height - 1
	if bodyH < 1 {
		bodyH = 1
	}
	p.vp.Height = bodyH
	titleBar := t.StatusModel.Render(" pager · "+p.title) +
		t.Hint.Render("  · j/k scroll · G/gg jump · q close")
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, p.vp.View())
}

// openExternalPager dumps the rendered scrollback to a temp file and
// spawns the user's pager ($PAGER, default `less -R`) on it. The
// pager owns the terminal completely while running — native mouse
// scroll, native drag-select across the full content, and the
// terminal's own search. On `q` (or whatever quits the pager), control
// returns to the TUI.
//
// Why this exists: alt-screen mode means there is no scrollback above
// our current frame, so dragging the mouse upward past the visible
// area can't reach earlier content. Visual mode (esc → v → j/k → y)
// solves this TUI-natively; this command gives users the *terminal*-
// native answer when they want native copy/scroll behavior.
func (a *App) openExternalPager() tea.Cmd {
	content := a.scrollback.Render(a.theme, a.width)

	f, err := os.CreateTemp("", "dsc-scrollback-*.txt")
	if err != nil {
		return func() tea.Msg {
			return infoMsg{Text: "external pager: cannot create temp file: " + err.Error()}
		}
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return func() tea.Msg {
			return infoMsg{Text: "external pager: write failed: " + err.Error()}
		}
	}
	_ = f.Close()

	// Prefer $PAGER, fall back to `less -R` (preserves ANSI colors).
	// We tokenize on spaces so `PAGER="less -RS"` works without a shell.
	pagerEnv := os.Getenv("PAGER")
	var argv []string
	if pagerEnv == "" {
		argv = []string{"less", "-R"}
	} else {
		argv = strings.Fields(pagerEnv)
	}
	argv = append(argv, f.Name())

	cmd := exec.Command(argv[0], argv[1:]...)
	// tea.ExecProcess releases the TTY to the subprocess and restores
	// the TUI when it exits. The callback receives the subprocess's
	// exit error (nil on clean q-quit).
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		_ = os.Remove(f.Name())
		if err != nil {
			return infoMsg{Text: "external pager exited with error: " + err.Error()}
		}
		return nil
	})
}

// handlePagerKey processes a keystroke while the pager overlay is up.
// Always intercepts — the pager is modal.
func (a *App) handlePagerKey(km tea.KeyMsg) (tea.Cmd, bool) {
	if a.pager == nil {
		return nil, false
	}
	switch km.String() {
	case "esc", "q":
		return a.closePager(), true
	case "j", "down":
		a.pager.vp.LineDown(1)
	case "k", "up":
		a.pager.vp.LineUp(1)
	case "g":
		a.pager.vp.GotoTop()
	case "G":
		a.pager.vp.GotoBottom()
	case "ctrl+d", "pgdown":
		a.pager.vp.HalfViewDown()
	case "ctrl+u", "pgup":
		a.pager.vp.HalfViewUp()
	}
	return nil, true
}
