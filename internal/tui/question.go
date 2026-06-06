// question.go owns the modal question card: pending ask state,
// per-question option list, single/multi select, and rendering.
// Mirrors permission.go's Open/Resolve/Render pattern.
package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/i18n"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// QuestionFlow encapsulates the modal question card.
type QuestionFlow struct {
	pending  *agent.EventQuestionAsk
	qIndex   int        // current question index
	cursor   int        // current option cursor within the question
	selected [][]bool   // per-question, per-option selection (for multi-select)
	answers  [][]string // accumulated answers across questions
}

// NewQuestionFlow returns an inactive flow.
func NewQuestionFlow() *QuestionFlow { return &QuestionFlow{} }

// Active reports whether a question ask is pending.
func (q *QuestionFlow) Active() bool { return q.pending != nil }

// Open stores the ask and resets state. Caller switches to modeQuestion.
func (q *QuestionFlow) Open(ev agent.EventQuestionAsk) {
	q.pending = &ev
	q.qIndex = 0
	q.cursor = 0
	q.answers = nil
	q.selected = make([][]bool, len(ev.Questions))
	for i, qu := range ev.Questions {
		q.selected[i] = make([]bool, len(qu.Options))
	}
}

// current returns the current Question.
func (q *QuestionFlow) current() tools.Question {
	return q.pending.Questions[q.qIndex]
}

// MoveDelta moves the option cursor up (negative) or down (positive).
func (q *QuestionFlow) MoveDelta(delta int) {
	opts := q.current().Options
	if len(opts) == 0 {
		return
	}
	q.cursor += delta
	if q.cursor < 0 {
		q.cursor = 0
	}
	if q.cursor >= len(opts) {
		q.cursor = len(opts) - 1
	}
}

// Toggle flips the selection of the current option (multi-select).
func (q *QuestionFlow) Toggle() {
	sel := q.selected[q.qIndex]
	if q.cursor < len(sel) {
		sel[q.cursor] = !sel[q.cursor]
	}
}

// Next advances to the next question or finishes. Returns done=true
// when all questions are answered, along with the full response and
// the reply channel.
func (q *QuestionFlow) Next() (done bool, resp tools.QuestionResponse, reply chan<- tools.QuestionResponse) {
	qu := q.current()
	// For single-select, select exactly the cursor item.
	if !qu.Multiple && len(qu.Options) > 0 {
		q.selected[q.qIndex] = make([]bool, len(qu.Options))
		q.selected[q.qIndex][q.cursor] = true
	}
	// Collect answers for this question.
	var labels []string
	for i, sel := range q.selected[q.qIndex] {
		if sel && i < len(qu.Options) {
			labels = append(labels, qu.Options[i].Label)
		}
	}
	q.answers = append(q.answers, labels)

	q.qIndex++
	if q.qIndex >= len(q.pending.Questions) {
		reply = q.pending.Reply
		resp.Answers = q.answers
		q.pending = nil
		return true, resp, reply
	}
	q.cursor = 0
	return false, resp, nil
}

// Cancel clears the pending flow and returns the reply channel so the
// caller can send an empty response to unblock the agent.
func (q *QuestionFlow) Cancel() chan<- tools.QuestionResponse {
	if q.pending == nil {
		return nil
	}
	reply := q.pending.Reply
	q.pending = nil
	return reply
}

// Render returns the modal question card.
func (q *QuestionFlow) Render(t Theme, width int) string {
	if q.pending == nil {
		return ""
	}
	innerW := width - 4
	if innerW < 30 {
		innerW = 30
	}
	qu := q.current()

	// Gradient "/// question N/M" accent header.
	gradTitle := ApplyBoldForegroundGrad(lipgloss.NewStyle(),
		fmt.Sprintf("/// question %d/%d", q.qIndex+1, len(q.pending.Questions)),
		t.BrandDeep, t.BrandLight)
	title := t.PermPrompt.Render("? ") + gradTitle
	header := qu.Header
	if header == "" {
		header = qu.Question
	}
	headerLine := t.AssistantText.Render("  " + header)

	var lines []string
	lines = append(lines, title, headerLine)
	if qu.Question != "" && qu.Header != "" {
		lines = append(lines, t.Hint.Render("  "+qu.Question))
	}
	lines = append(lines, "")

	if len(qu.Options) == 0 {
		lines = append(lines, t.Hint.Render("  "+i18n.T("question.confirm")))
	} else {
		for i, opt := range qu.Options {
			prefix := "  "
			if q.cursor == i {
				prefix = "▸ "
			}
			label := opt.Label
			if opt.Description != "" {
				label += " — " + opt.Description
			}
			if qu.Multiple && q.selected[q.qIndex][i] {
				label = "✓ " + label
			} else if qu.Multiple {
				label = "  " + label
			}
			if q.cursor == i {
				lines = append(lines, t.PermFocus.Render(prefix+label))
			} else {
				lines = append(lines, prefix+label)
			}
		}
	}

	var hint string
	if len(qu.Options) == 0 {
		// Free-text input: the confirm line already added above suffices;
		// show the generic "type and press ⏎" hint here too so the hint
		// line is always present (no layout shift when toggling modes).
		hint = t.Hint.Render("  " + i18n.T("question.input.hint"))
	} else {
		multiHint := ""
		if qu.Multiple {
			multiHint = " · space toggle"
		}
		hint = t.Hint.Render("  " + i18n.T("question.nav.hint", multiHint))
	}
	lines = append(lines, "", hint)

	body := strings.Join(lines, "\n")

	// Raised in-slot pane: bgRaised surface, rounded border in the border
	// token, Padding(1,2). Degrades to fg-only when fills are disabled.
	pane := t.Panel(TierRaised).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderColor).
		Padding(1, 2).
		Width(width - 2)
	return pane.Render(body)
}
