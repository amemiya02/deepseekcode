// Package tui implements the Bubble Tea TUI for deepseekcode.
//
// Architecture in one paragraph: a single tea.Program owns the UI
// state. The agent runs in a goroutine; a separate consumer goroutine
// reads agent.Events() and wraps each event into a single
// agentEventMsg that is dispatched to the tea.Program. Update folds
// that one message type — type-switching on the inner Event — into
// the App model. Permission asks travel the same channel and carry
// their reply chan, so the agent goroutine blocks waiting on the
// user without any out-of-band wiring.
//
// Components: items.go owns rendered chat-item types; scrollback.go
// owns the chat buffer + stream cursors + visual selection; app.go
// owns the model + Update; status.go renders the status line;
// theme.go owns Lip Gloss styles; keymap.go owns keybindings.
package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/amemiya02/deepseekcode/internal/agent"
)

// agentEventMsg is the single envelope carrying any agent.Event into
// the tea.Program. The Update switch type-switches on Event to route
// to the right sub-module. Replaces the older "one tea.Msg type per
// callback" design (9 types) with one envelope + a type switch.
//
// agent.EventDone (deferred-emit terminator) is the only "this run
// is finished" signal — there is no separate agentDoneMsg. Routing
// completion through the same channel as the deltas is what keeps
// chrome.Reset() ordered AFTER any trailing tokens still in flight.
type agentEventMsg struct{ Event agent.Event }

// runStartMsg fires immediately after a prompt is submitted, before
// the agent goroutine has emitted anything. Lets the UI start the
// chrome spinner and redraw ticker so the user sees activity even
// during the cold-start gap before the first reasoning token.
type runStartMsg struct{}

// redrawMsg drives the coalesced re-render loop. While the agent is
// running, streaming deltas only bump scrollback.Seq(); the actual
// viewport rebuild happens on the redraw tick, capped at ~12 fps.
// Without this, fast token streams cause O(N²) renders that block
// scroll input.
type redrawMsg struct{}

// slashExpandedMsg carries the expanded template text from an async
// shell injection. The tea.Cmd closure runs Expand (which may block
// on !`cmd`) off the UI goroutine, then delivers this message back
// to Update for AppendUser + submitPromptCmd.
type slashExpandedMsg struct{ text string }

// submitPromptCmd starts a new agent Run with userPrompt. The agent
// goroutine emits agent.Events as work progresses; the App's event
// pump goroutine wraps each into an agentEventMsg. We return
// runStartMsg synchronously so the UI thread can spin up the chrome
// ticker before any agent event has fired.
func (a *App) submitPromptCmd(userPrompt string) tea.Cmd {
	return func() tea.Msg {
		go a.runAgent(userPrompt)
		return runStartMsg{}
	}
}
