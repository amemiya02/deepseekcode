package tui

import (
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

// CacheDoctorPanel is a read-only Bubbletea-compatible view component that
// renders the per-turn cache ledger produced by traceinspect.ExplainFile.
// It does NOT implement tea.Model itself (no Cmd loop needed for a pure view);
// it is embedded or called from the parent App model's View method.
type CacheDoctorPanel struct {
	rows  []traceinspect.TurnLedger
	Theme Theme // set by NewCacheDoctorPanel; zero-value falls back to DefaultTheme()
}

// NewCacheDoctorPanel constructs the panel with an initial (possibly nil) ledger
// and the active Theme (so light/midnight/nebula/aurora users get correct styling).
func NewCacheDoctorPanel(ledger []traceinspect.TurnLedger, theme Theme) CacheDoctorPanel {
	return CacheDoctorPanel{rows: ledger, Theme: theme}
}

// Append returns a new panel with one additional row appended. Callers use this
// when streaming live usage records from the agent.
func (p CacheDoctorPanel) Append(row traceinspect.TurnLedger) CacheDoctorPanel {
	rows := make([]traceinspect.TurnLedger, len(p.rows)+1)
	copy(rows, p.rows)
	rows[len(p.rows)] = row
	return CacheDoctorPanel{rows: rows, Theme: p.Theme}
}

// View renders the panel as a plain string using the Theme threaded in at
// construction time (falls back to DefaultTheme if the panel was zero-constructed).
func (p CacheDoctorPanel) View() string {
	t := p.Theme
	if t.Name == "" {
		t = DefaultTheme()
	}
	headerStyle := t.Panel(TierSurface).Bold(true)
	evictStyle := t.Badge(BadgeErr)
	okStyle := t.Badge(BadgeOk)

	var b strings.Builder
	b.WriteString(headerStyle.Render("Cache Doctor") + "\n")

	if len(p.rows) == 0 {
		b.WriteString("  (no usage turns yet)\n")
		return b.String()
	}

	fmt.Fprintf(&b, "  %-5s %-8s %-8s %-6s %-10s %-6s %s\n",
		"TURN", "HIT", "MISS", "OUT", "COST(¥)", "EVICT", "WHY")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("─", 64))

	for _, row := range p.rows {
		// Pad the text to a fixed visual width BEFORE applying ANSI styling so
		// that the surrounding fmt.Fprintf %s verb counts only the ANSI bytes,
		// not mixing visual cells with escape-code bytes.
		evictCell := okStyle.Render(fmt.Sprintf("%-6s", "N"))
		if row.Evicted {
			evictCell = evictStyle.Render(fmt.Sprintf("%-6s", "Y"))
		}
		fmt.Fprintf(&b, "  %-5d %-8d %-8d %-6d %-10.6f %s %s\n",
			row.Turn, row.HitTokens, row.MissTokens, row.OutputTokens,
			row.CostCNY, evictCell, row.Why)
	}
	return b.String()
}
