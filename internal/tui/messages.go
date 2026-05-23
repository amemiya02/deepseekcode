// Package tui implements the Bubble Tea TUI for deepseekcode.
//
// Architecture in one paragraph: a single tea.Program owns the UI
// state. The agent runs in a goroutine; its Callbacks translate events
// into typed tea.Msg values (defined here) and Send() them to the
// program. Update() folds those messages into the App model. Permission
// asks use a request/response channel so the agent goroutine can block
// while the UI waits for input.
//
// Components: items.go owns rendered chat-item types; app.go owns the
// model + Update; status.go renders the status line; theme.go owns
// Lip Gloss styles; keymap.go owns keybindings.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// agent → UI events
type (
	// reasoningStartMsg starts a new reasoning block.
	reasoningStartMsg struct{}

	// reasoningDeltaMsg appends to the active reasoning block.
	reasoningDeltaMsg struct{ Text string }

	// reasoningEndMsg closes the active reasoning block.
	reasoningEndMsg struct{}

	// textDeltaMsg appends to the active assistant text block.
	textDeltaMsg struct{ Text string }

	// toolCallStartMsg announces a tool call.
	toolCallStartMsg struct{ Call llm.ToolCall }

	// toolCallResultMsg carries the result.
	toolCallResultMsg struct {
		CallID string
		Result tools.Result
		Dur    time.Duration
	}

	// duetMsg renders an inline ◆ pro check line.
	duetMsg struct {
		CallID    string
		Approved  bool
		Reasoning string
		Dur       time.Duration
	}

	// stepFinishMsg ends the current step; status line updates.
	stepFinishMsg struct {
		Reason agent.StopReason
		Usage  llm.Usage
	}

	// infoMsg renders an out-of-band notice (skipped validator, etc.).
	infoMsg struct{ Text string }

	// permissionAskMsg requests a user decision. The response is sent
	// back on Reply (buffered channel of 1).
	permissionAskMsg struct {
		Check permissions.Check
		Reply chan<- agent.PermissionResponse
	}

	// agentDoneMsg signals the agent finished one Run cycle.
	agentDoneMsg struct {
		Reason agent.StopReason
		Err    error
	}

	// agentErrMsg surfaces an infrastructure error from the agent.
	agentErrMsg struct{ Err error }

	// runStartMsg fires immediately after a prompt is submitted, before
	// the agent goroutine has emitted anything. Lets the UI start the
	// chrome spinner and redraw ticker so the user sees activity even
	// during the cold-start gap before the first reasoning token.
	runStartMsg struct{}

	// redrawMsg drives the coalesced re-render loop. While the agent is
	// running, deltas (text/reasoning tokens) only mark a.dirty=true;
	// the actual viewport rebuild happens on the redraw tick, capped at
	// ~12 fps. Without this, fast token streams cause O(N²) renders
	// that block scroll input.
	redrawMsg struct{}
)

// UI → agent commands

// submitPromptCmd starts a new agent Run with userPrompt. The agent
// goroutine emits agent → UI messages as work progresses. We return
// runStartMsg so the UI thread can spin up the chrome ticker before
// any agent callback has fired.
func (a *App) submitPromptCmd(userPrompt string) tea.Cmd {
	return func() tea.Msg {
		go a.runAgent(userPrompt)
		return runStartMsg{}
	}
}
