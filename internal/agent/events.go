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

// EventProtocolVersion is the current envelope version. Increment when
// the EventEnvelope shape changes in a backward-incompatible way.
const EventProtocolVersion = 1

// EventEnvelope wraps an Event with versioning, sequence number, and
// timestamp for multi-consumer fan-out on the Bus. It does NOT implement
// Event itself — it is a container, not an event.
type EventEnvelope struct {
	Version int       // = EventProtocolVersion
	Seq     uint64    // monotonic, assigned by Bus
	At      time.Time // publish moment
	Event   Event     // the concrete event
}

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

// EventStepFinish ends one ReAct step. The consumer updates its
// status counters / cost HUD here. Model is the model that produced the step
// (an escalated turn reports the stronger model); empty means the loop model.
type EventStepFinish struct {
	Reason StopReason
	Usage  llm.Usage
	Model  string
}

// EventInfo is an out-of-band notice (retry attempt, validator
// skipped, tool-call rate warning). Surfaced as a chat line.
type EventInfo struct{ Text string }

// Budget-gate event kinds (EventBudget.Kind), T1.3.
const (
	BudgetKindWarning  = "warning"  // crossed WarnCNY
	BudgetKindBlocked  = "blocked"  // crossed HardCNY — the turn is refused
	BudgetKindUnpriced = "unpriced" // model has no pricing table; gate can't price it
)

// EventBudget reports a session-budget gate decision (T1.3 — promoted from a
// stringly-typed EventInfo so "warned" vs "blocked" vs "unpriced" are
// distinguishable for analytics and programmatic gating). ProjectedCNY/SpentCNY
// are the gate's inputs (ProjectedCNY is 0 for the unpriced kind, where it
// cannot be computed); Model is the model being gated. Traced with type
// budget.warning / budget.blocked / budget.unpriced.
type EventBudget struct {
	Kind         string
	ProjectedCNY float64
	SpentCNY     float64
	Model        string
}

// EventPermissionDenied reports a tool call refused by the permission layer
// (T1.3 — promoted from EventInfo). ByRule distinguishes an explicit deny-rule
// match from a policy-tier denial; Reason is the human-readable cause. Traced
// with type permission.denied.
type EventPermissionDenied struct {
	Tool   string
	Reason string
	ByRule bool
}

// EventEscalated reports that the current turn was re-issued on a stronger
// model (the Two-Model escalation). Trigger is "marker" (the model emitted a
// <<<NEEDS_PRO>>> self-declaration) or "repair_errors" (the per-turn repair
// failure count crossed the threshold). FromModel/ToModel record the switch.
// Traced with type policy.escalated.
type EventEscalated struct {
	Trigger   string
	FromModel string
	ToModel   string
	Reason    string
}

// EventPermissionAsk requests user approval for a tool call. The
// consumer MUST send a PermissionResponse on Reply — the agent
// goroutine blocks on the receive. Reply is buffered (cap 1) so the
// consumer can send without serialization concerns.
type EventPermissionAsk struct {
	Check permissions.Check
	Reply chan<- PermissionResponse
}

// EventDone is the agent's "this Run is finished" signal. Emitted via
// defer from Run, so it travels the same channel as every other event
// and arrives in strict order AFTER the final EventStepFinish. This
// is load-bearing: routing the "done" signal through a separate
// goroutine + tea.Msg path used to race past trailing text deltas,
// leaving the UI's chrome stuck on "writing…" because a late delta
// would re-fire BeginWriting after the reset.
type EventDone struct {
	Reason StopReason
	Err    error
}

// EventCompaction reports that the agent collapsed messages
// [FromIdx, ToIdx) into a single Summary message, freeing
// RemovedCount slots from the live transcript. Wired in Phase 2.
type EventCompaction struct {
	FromIdx, ToIdx int
	Summary        string
	RemovedCount   int
}

// EventHookFired reports that a registered hook ran. Decision is one
// of allow / deny / continue / ask; Reason is the hook's free-form
// explanation. Surfaced so the UI can show a `[hook] …` chat line.
// Wired in Phase 3.
type EventHookFired struct {
	HookName string
	Event    string // PreToolUse / PostToolUse / ...
	Decision string // allow / deny / continue / ask
	Reason   string
	Dur      time.Duration
}

// EventQuestionAsk requests the user answer one or more questions.
// The consumer MUST send a QuestionResponse on Reply — the agent
// goroutine blocks on the receive. Reply is buffered (cap 1) so the
// consumer can send without serialization concerns.
type EventQuestionAsk struct {
	Questions []tools.Question
	Reply     chan<- tools.QuestionResponse
}

func (EventReasoningStart) isAgentEvent()   {}
func (EventReasoningDelta) isAgentEvent()   {}
func (EventReasoningEnd) isAgentEvent()     {}
func (EventTextDelta) isAgentEvent()        {}
func (EventToolCallStart) isAgentEvent()    {}
func (EventToolCallResult) isAgentEvent()   {}
func (EventStepFinish) isAgentEvent()       {}
func (EventInfo) isAgentEvent()             {}
func (EventBudget) isAgentEvent()           {}
func (EventPermissionDenied) isAgentEvent() {}
func (EventEscalated) isAgentEvent()        {}
func (EventPermissionAsk) isAgentEvent()    {}
func (EventQuestionAsk) isAgentEvent()      {}
func (EventDone) isAgentEvent()             {}
func (EventCompaction) isAgentEvent()       {}
func (EventHookFired) isAgentEvent()        {}

// EventSubagentStart signals that a sub-agent spawn has begun.
type EventSubagentStart struct {
	Agent       string
	Description string
}

// EventSubagentFinish signals that a sub-agent run completed
// (successfully or not).
type EventSubagentFinish struct {
	Agent  string
	Result SubResult
}

func (EventSubagentStart) isAgentEvent()  {}
func (EventSubagentFinish) isAgentEvent() {}

// EventBackgroundJobStart signals that a background job has started.
type EventBackgroundJobStart struct {
	ID   string
	Kind JobKind
}

// EventBackgroundJobFinish signals that a background job has completed.
type EventBackgroundJobFinish struct {
	ID      string
	State   JobState
	Summary string
}

func (EventBackgroundJobStart) isAgentEvent()  {}
func (EventBackgroundJobFinish) isAgentEvent() {}

// EventRepair reports a tool-call repair action (args completed, recovered,
// suppressed, or schema-complex). Published by the repair integration layer
// in runStep after model streaming finishes.
type EventRepair struct {
	Kind       string
	Tool       string
	CallID     string
	Message    string
	BeforeHash string
	AfterHash  string
}

func (EventRepair) isAgentEvent() {}

// EventEpochCreated signals that a new PrefixEpoch was created.
type EventEpochCreated struct {
	EpochID          string
	StaticPrefixHash string
	ToolsHash        string
	Reason           string
}

// EventEpochFrozen signals that the current PrefixEpoch was frozen
// after the first model request.
type EventEpochFrozen struct {
	EpochID string
}

// EventPendingChange signals that a component change was detected
// after the epoch was frozen. The change is recorded but not applied.
type EventPendingChange struct {
	EpochID     string
	Kind        PendingChangeKind
	Description string
}

// EventEpochSwitched signals an explicit epoch switch.
type EventEpochSwitched struct {
	OldEpochID       string
	NewEpochID       string
	StaticPrefixHash string
	ToolsHash        string
	Reason           string
}

// EventDriftBlocked signals that an unauthorized prefix drift was
// detected and blocked within a frozen epoch.
type EventDriftBlocked struct {
	EpochID string
	Which   string
}

func (EventEpochCreated) isAgentEvent()  {}
func (EventEpochFrozen) isAgentEvent()   {}
func (EventPendingChange) isAgentEvent() {}
func (EventEpochSwitched) isAgentEvent() {}
func (EventDriftBlocked) isAgentEvent()  {}

// EventCompactionWarning is emitted when context pressure crosses the
// warning threshold (default 75%). The UI can show a status indicator
// or prepare pinned facts for an upcoming compaction.
type EventCompactionWarning struct {
	Pressure  float64
	Threshold float64
}

// EventSemanticCompaction reports that semantic compaction ran.
// UsedSemantic is true when the LLM summary was used; false when
// deterministic fallback was used. SummaryCost is the cost of the
// LLM call (0 when fallback). FallbackReason is set when the LLM
// call failed and deterministic compaction was used instead.
//
// StaticPrefixHashBefore/After are the measured static-prefix fingerprints
// (system + tools) of the frozen baseline and of the request compaction
// actually fed the model. They are emitted into the trace so the benchmark
// can verify compaction did not move the prefix — instead of the agent
// asserting stability with a hardcoded boolean. When the freeze override is
// intact they are equal; a regression that summarized against the live
// (non-frozen) prompt makes them diverge and fails the gate.
type EventSemanticCompaction struct {
	FromIdx, ToIdx         int
	UsedSemantic           bool
	SummaryCost            float64
	FallbackReason         string
	StaticPrefixHashBefore string
	StaticPrefixHashAfter  string
}

func (EventCompactionWarning) isAgentEvent()  {}
func (EventSemanticCompaction) isAgentEvent() {}

// PlanItem is one entry of the agent's structured plan, decoupled from
// tools.TodoItem so consumers (gateway/SPA) need not import tools. Status is
// the Contract value set: pending | in_progress | done.
type PlanItem struct {
	Text   string
	Status string
}

// EventPlanUpdate reports the agent's plan (todo list) was replaced. Published
// by TodoWrite via its PlanPublisher hook (wired in Agent construction).
type EventPlanUpdate struct{ Items []PlanItem }

// EventRetry reports a transient mid-stream re-issue attempt (T1.4). Attempt is
// 1-based for the first re-issue; Max is the configured ceiling.
type EventRetry struct{ Attempt, Max int }

// EventToolCallDelta carries incremental tool stdout for a running call. Only
// published by tool runners that genuinely stream (e.g. bash stdout); tools
// that return whole results never emit it.
type EventToolCallDelta struct {
	CallID string
	Delta  string
}

func (EventPlanUpdate) isAgentEvent()    {}
func (EventRetry) isAgentEvent()         {}
func (EventToolCallDelta) isAgentEvent() {}
