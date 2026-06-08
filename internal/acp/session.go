package acp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrSessionNotFound is returned by SessionManager lookups when no session
// exists for the requested id. Callers route on it with errors.Is rather than
// matching error strings.
var ErrSessionNotFound = errors.New("acp: session not found")

// EventKind identifies what kind of event came from an AgentRunner.
type EventKind int

const (
	EventKindTextDelta EventKind = iota
	EventKindInfo
	EventKindDone
	EventKindPermission // a tool call awaits user approval (carries Respond)
	EventKindAsk        // the agent asks the user question(s) (carries Answer)
	EventKindToolStart  // a tool call started executing
	EventKindToolEnd    // a tool call finished executing
	EventKindCache      // live cache_update {turn_pct, avg_pct, prefixes, eviction}
	EventKindCost       // live cost_update {turn_cny, session_cny, output_tokens}
	EventKindRouting    // live routing {from, to, reason}
	EventKindJob        // live job_update {running}
	EventKindRetry      // live retry {attempt, max}
	EventKindThinking   // live thinking_delta {text}
	EventKindToolDelta  // live tool_delta {id, delta}
	EventKindPlan       // live plan_update {items}
	EventKindDuet       // live duet {decision, reason} — pro-validation result
)

// PermissionDecision is the user's answer to a permission request, mirroring
// the SPA's 4-tier choice. Only Allow* decisions permit the tool call; the acp
// layer maps every Allow* to agent.PermissionResponse{Allow:true} (the agent
// has no session/always concept — persistence of "session"/"always" is the
// gateway/UI's concern, layered on later waves).
type PermissionDecision int

const (
	PermissionDeny PermissionDecision = iota
	PermissionAllowOnce
	PermissionAllowSession
	PermissionAllowAlways
)

// Allowed reports whether the decision lets the tool call proceed.
func (d PermissionDecision) Allowed() bool { return d != PermissionDeny }

// AgentEvent is the unified event from an AgentRunner.
type AgentEvent struct {
	Kind       EventKind
	Text       string // for TextDelta and Info
	StopReason string // for Done
	Err        error  // for Done

	// Permission (EventKindPermission): the tool call awaiting approval and the
	// closure that delivers the user's decision back to the parked agent.
	PermID   string
	ToolName string
	ToolArgs string
	Respond  func(PermissionDecision)

	// Ask (EventKindAsk): the questions and the closure that delivers the
	// per-question selected labels back to the parked agent.
	AskID     string
	Questions []AskQuestion
	Answer    func(answers [][]string)

	// Tool (EventKindToolStart / EventKindToolEnd): structured tool-call info.
	ToolCallID   string
	ToolResult   string
	ToolIsErr    bool
	ToolReadOnly bool // set for EventKindToolStart; true if the tool declares IsReadOnly()

	// Cache (EventKindCache): 0–1 ratios per Contract 2 (NOT percents).
	TurnPct  float64
	AvgPct   float64
	Prefixes int
	Eviction bool

	// Cost (EventKindCost).
	TurnCNY      float64
	SessionCNY   float64
	OutputTokens int

	// Routing (EventKindRouting).
	From, To, Reason string

	// Job (EventKindJob).
	Running int

	// Retry (EventKindRetry).
	Attempt, Max int

	// ToolDelta (EventKindToolDelta): ToolCallID reused; ToolDelta is the chunk.
	ToolDelta string

	// Plan (EventKindPlan).
	Plan []PlanItem

	// Duet (EventKindDuet): the pro-validation decision ("allow"|"deny") + reasoning
	// (Reason is reused for the human-readable text).
	Decision string
}

// PlanItem is one plan entry for plan_update. Status ∈ {pending,in_progress,done}.
type PlanItem struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

// AskQuestion is a UI-facing copy of one tools.Question, decoupled from the
// tools package so the gateway can serialize it without importing tools.
type AskQuestion struct {
	Question string      `json:"question"`
	Header   string      `json:"header"`
	Multiple bool        `json:"multiple"`
	Options  []AskOption `json:"options"`
}

// AskOption is one selectable answer.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AgentRunner is the interface the acp layer uses to drive an agent.
// It wraps internal/agent.Agent without importing it directly so the
// acp package stays decoupled and testable with stubs.
type AgentRunner interface {
	// Run executes the agent with the given prompt. It calls onEvent
	// synchronously for each event and returns when the agent is done.
	Run(ctx context.Context, userPrompt string, onEvent func(AgentEvent)) error
	// Steer injects a mid-turn instruction into the in-flight Run. No-op if no
	// run is active. Safe to call concurrently with Run.
	Steer(text string)
}

// AgentFactory creates a new AgentRunner for the given working directory.
type AgentFactory func(workingDir string) (AgentRunner, error)

// TurnSettings carries the per-turn runtime knobs the UI can change between
// turns: the active model, reasoning effort, and the composer's permission
// mode ("ask" / "auto-edit" / "plan" / "yolo"). Empty fields mean "keep the
// agent's current value".
type TurnSettings struct {
	Model          string
	Effort         string
	PermissionMode string
}

// SettingsApplier is an optional interface an AgentRunner may implement to
// accept TurnSettings. Callers must only apply settings while no run is
// active for the session (the gateway does so right before launching a turn),
// so implementations may use plain field writes.
type SettingsApplier interface {
	ApplySettings(TurnSettings)
}

// session holds the state for a single ACP session.
//
// ctx/cancel are owned by the SessionManager and parented to the manager's
// long-lived base context (NOT to the per-request/per-connection context that
// created the session). The only intentional cancellation path is Cancel();
// a connection closing must never cancel a logically-alive session.
type session struct {
	id     string
	runner AgentRunner
	ctx    context.Context
	cancel context.CancelFunc
}

// SessionManager manages the lifecycle of ACP sessions.
type SessionManager struct {
	factory  AgentFactory
	base     context.Context    // long-lived parent for all session contexts
	baseStop context.CancelFunc // cancels every session; called by Shutdown
	mu       sync.Mutex
	sessions map[string]*session
	counter  atomic.Int64
}

// NewSessionManager creates a SessionManager backed by the given factory.
// Session contexts are parented to a manager-owned context derived from
// context.Background(), so a closing request/connection never cancels a live
// session.
func NewSessionManager(factory AgentFactory) *SessionManager {
	base, stop := context.WithCancel(context.Background())
	return &SessionManager{
		factory:  factory,
		base:     base,
		baseStop: stop,
		sessions: make(map[string]*session),
	}
}

// Shutdown cancels every live session by cancelling the manager-owned base
// context. After Shutdown the manager should not be reused.
func (sm *SessionManager) Shutdown() {
	sm.baseStop()
}

// NewSession creates a new session and returns its id.
//
// The ctx argument is used only for factory creation; it is deliberately NOT
// the parent of the session's lifetime context. The session context is
// parented to the manager-owned base context so the session stays alive after
// the creating request/connection closes. Cancel() is the only path that
// cancels it.
func (sm *SessionManager) NewSession(ctx context.Context, workingDir string) (string, error) {
	runner, err := sm.factory(workingDir)
	if err != nil {
		return "", fmt.Errorf("acp: create agent: %w", err)
	}
	n := sm.counter.Add(1)
	id := fmt.Sprintf("sess-%d", n)
	sCtx, cancel := context.WithCancel(sm.base)
	sm.mu.Lock()
	sm.sessions[id] = &session{id: id, runner: runner, ctx: sCtx, cancel: cancel}
	sm.mu.Unlock()
	return id, nil
}

// Lookup returns the session for the given id, or ErrSessionNotFound if no
// session exists. Callers route on the error with errors.Is.
func (sm *SessionManager) Lookup(id string) (*session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return s, nil
}

// Has reports whether a session with the given id exists.
func (sm *SessionManager) Has(id string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.sessions[id]
	return ok
}

// Cancel cancels and removes the session.
func (sm *SessionManager) Cancel(id string) {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	if ok {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()
	if ok {
		s.cancel()
	}
}

// Steer injects a mid-turn instruction into the session's in-flight run.
// No-op if the session does not exist. Mirrors Cancel's lookup pattern but
// does NOT remove or cancel the session — the turn stays alive.
func (sm *SessionManager) Steer(id, text string) {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	sm.mu.Unlock()
	if ok {
		s.runner.Steer(text)
	}
}

// ApplySettings forwards TurnSettings to the session's runner when it
// implements SettingsApplier; otherwise (stub runners) it is a no-op. Callers
// must ensure no run is active for the session (see SettingsApplier).
func (sm *SessionManager) ApplySettings(id string, ts TurnSettings) {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	sm.mu.Unlock()
	if !ok {
		return
	}
	if ap, ok := s.runner.(SettingsApplier); ok {
		ap.ApplySettings(ts)
	}
}

// SessionCtx returns the context associated with the session. If the session
// does not exist (already cancelled/removed), it returns an already-cancelled
// context rather than context.Background(), so a caller that races a Cancel
// does not launch an uncancellable agent goroutine.
func (sm *SessionManager) SessionCtx(id string) context.Context {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	sm.mu.Unlock()
	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return s.ctx
}

// Prompt runs the agent for the session with the given prompt, calling
// onEvent for each event. It blocks until the agent is done. If the session
// does not exist it returns ErrSessionNotFound (wrapped).
func (sm *SessionManager) Prompt(ctx context.Context, id, prompt string, onEvent func(AgentEvent)) error {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	sm.mu.Unlock()
	if !ok {
		return fmt.Errorf("acp: prompt session %q: %w", id, ErrSessionNotFound)
	}
	return s.runner.Run(ctx, prompt, onEvent)
}
