// events.go defines the tagged-union the agent pushes through its
// Events() channel. Replaces the old Callbacks struct — instead of
// the UI registering function pointers and the agent calling them
// synchronously on its goroutine, the UI consumes a typed event
// stream and translates each to its own representation (tea.Msg in
// the TUI, fprintf in the CLI).
//
// Why a sealed interface over a struct-of-functions:
//   - Order is explicit: events arrive in the order the agent emits
//     them, so the consumer never has to reason about callback
//     interleaving across goroutines.
//   - Adding a new event kind is one type declaration plus a
//     consumer case, with the compiler flagging missing handling
//     anywhere a switch lists known variants.
//   - The CLI and TUI adapters are just two consumers of the same
//     channel — no parallel implementations of the same function
//     table to keep in sync.
//
// Permission ask is special. The agent must block until the user
// answers, so EventPermissionAsk carries a buffered Reply channel.
// The consumer writes a PermissionResponse on Reply; the agent
// goroutine is parked on that receive.
package agent

import (
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Event is the sealed interface implemented by every event the agent
// emits. Type-switch on the concrete type at the consumer.
type Event interface{ isAgentEvent() }

// EventReasoningStart opens a new reasoning block.
type EventReasoningStart struct{}

// EventReasoningDelta appends to the active reasoning block.
type EventReasoningDelta struct{ Text string }

// EventReasoningEnd closes the active reasoning block.
type EventReasoningEnd struct{}

// EventTextDelta appends to the active assistant-text block.
type EventTextDelta struct{ Text string }

// EventToolCallStart announces a tool call (model decided to run a
// tool; permission gates haven't fired yet).
type EventToolCallStart struct{ Call llm.ToolCall }

// EventToolCallResult carries the result of an executed tool call.
type EventToolCallResult struct {
	CallID string
	Result tools.Result
	Dur    time.Duration
}

// EventDuet carries a pro-validator decision on a destructive call.
type EventDuet struct {
	CallID    string
	Approved  bool
	Reasoning string
	Dur       time.Duration
}

// EventStepFinish ends one ReAct step. The consumer updates its
// status counters / cost HUD here.
type EventStepFinish struct {
	Reason StopReason
	Usage  llm.Usage
}

// EventInfo is an out-of-band notice (retry attempt, validator
// skipped, tool-call rate warning). Surfaced as a chat line.
type EventInfo struct{ Text string }

// EventPermissionAsk requests user approval for a tool call. The
// consumer MUST send a PermissionResponse on Reply — the agent
// goroutine blocks on the receive. Reply is buffered (cap 1) so the
// consumer can send without serialization concerns.
type EventPermissionAsk struct {
	Check permissions.Check
	Reply chan<- PermissionResponse
}

func (EventReasoningStart) isAgentEvent()  {}
func (EventReasoningDelta) isAgentEvent()  {}
func (EventReasoningEnd) isAgentEvent()    {}
func (EventTextDelta) isAgentEvent()       {}
func (EventToolCallStart) isAgentEvent()   {}
func (EventToolCallResult) isAgentEvent()  {}
func (EventDuet) isAgentEvent()            {}
func (EventStepFinish) isAgentEvent()      {}
func (EventInfo) isAgentEvent()            {}
func (EventPermissionAsk) isAgentEvent()   {}
