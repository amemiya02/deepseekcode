// permission.go owns the modal permission card: pending ask state,
// 2×2 button grid, key resolution, and rendering. App composes one
// PermissionFlow instead of carrying pendingPerm + permCursor as
// flat fields plus a 130-line renderPermissionPrompt method.
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/permissions"
)

// PermissionFlow encapsulates the modal permission card. Mutations
// only through methods.
type PermissionFlow struct {
	pending *agent.EventPermissionAsk
	cursor  int // 0=allow once, 1=session, 2=always, 3=deny
}

// NewPermissionFlow returns an inactive flow (no pending ask).
func NewPermissionFlow() *PermissionFlow { return &PermissionFlow{} }

// Active reports whether a permission ask is pending user input.
func (p *PermissionFlow) Active() bool { return p.pending != nil }

// Open stores the ask and resets the cursor to the safe default
// (allow once). Caller is responsible for switching App.mode to
// modePermission and re-laying out the screen.
func (p *PermissionFlow) Open(ev agent.EventPermissionAsk) {
	p.pending = &ev
	p.cursor = 0
}

// Tool returns the tool name behind the pending ask, or "" when
// no ask is pending.
func (p *PermissionFlow) Tool() string {
	if p.pending == nil {
		return ""
	}
	return p.pending.Check.Tool.Name()
}

// Nav — 2×2 grid keyed as:
//
//	0 1
//	2 3
func (p *PermissionFlow) MoveUp() {
	if p.cursor >= 2 {
		p.cursor -= 2
	}
}

func (p *PermissionFlow) MoveDown() {
	if p.cursor < 2 {
		p.cursor += 2
	}
}

func (p *PermissionFlow) MoveLeft() {
	if p.cursor%2 == 1 {
		p.cursor--
	}
}

func (p *PermissionFlow) MoveRight() {
	if p.cursor%2 == 0 && p.cursor < 3 {
		p.cursor++
	}
}

func (p *PermissionFlow) Tab()      { p.cursor = (p.cursor + 1) % 4 }
func (p *PermissionFlow) ShiftTab() { p.cursor = (p.cursor + 3) % 4 }

// Resolve maps a key to a button decision. Returns (resp, reply,
// check, ok=true) when the key resolved a button; ok=false means
// the key wasn't a button (caller should swallow). On ok=true the
// flow is cleared (Active becomes false) — the caller is responsible
// for sending resp on reply, restoring mode, and re-layouting.
func (p *PermissionFlow) Resolve(key string) (resp agent.PermissionResponse, reply chan<- agent.PermissionResponse, check permissions.Check, ok bool) {
	if p.pending == nil {
		return resp, nil, check, false
	}
	btn := -1
	switch strings.ToLower(key) {
	case "enter":
		btn = p.cursor
	case "y", "o":
		btn = 0
	case "s":
		btn = 1
	case "a":
		btn = 2
	case "n", "d", "esc":
		btn = 3
	}
	if btn < 0 {
		return resp, nil, check, false
	}
	switch btn {
	case 0, 1:
		resp = agent.PermissionResponse{Allow: true}
	case 2:
		resp = agent.PermissionResponse{Allow: true, PersistPattern: true}
	case 3:
		resp = agent.PermissionResponse{Allow: false}
	}
	reply = p.pending.Reply
	check = p.pending.Check
	p.pending = nil
	return resp, reply, check, true
}

// Render returns the modal permission card. Returns "" when there's
// nothing pending.
func (p *PermissionFlow) Render(t Theme, width int) string {
	if p.pending == nil {
		return ""
	}
	innerW := width - 4
	if innerW < 30 {
		innerW = 30
	}
	tool := p.pending.Check.Tool.Name()
	args := oneline(string(p.pending.Check.Args), innerW-10)

	title := t.PermPrompt.Render("⚠ permission required")
	toolLine := t.AssistantText.Render("  tool  ") + t.StatusModel.Render(tool)
	argsLine := t.AssistantText.Render("  args  ") + t.Hint.Render(args)

	const btnW = 20
	btns := []struct{ key, label string }{
		{"Y", "allow once"},
		{"S", "session"},
		{"A", "always"},
		{"N", "deny"},
	}
	renderBtn := func(idx int) string {
		b := btns[idx]
		text := " [" + b.key + "] " + b.label
		pad := btnW - len(text)
		if pad > 0 {
			text += strings.Repeat(" ", pad)
		}
		if idx == p.cursor {
			return t.PermFocus.Render(text)
		}
		return t.PermButton.Render(text)
	}

	row1 := "  " + renderBtn(0) + "  " + renderBtn(1)
	row2 := "  " + renderBtn(2) + "  " + renderBtn(3)
	hint := t.Hint.Render("  ←↑↓→ / tab move · ⏎ select · esc deny · y/s/a/n direct")

	body := strings.Join([]string{
		title,
		toolLine,
		argsLine,
		"",
		row1,
		row2,
		hint,
	}, "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.PermPrompt.GetForeground()).
		Padding(0, 1).
		Width(width - 2).
		Render(body)
}
