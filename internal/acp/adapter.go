package acp

import (
	"context"
	"sync"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// agentIface is the minimal surface of *agent.Agent that AgentAdapter needs.
// Keeping it unexported lets tests inject a stub without exposing it as a
// public contract. Production code always passes *agent.Agent.
type agentIface interface {
	Bus() *agent.Bus
	Run(ctx context.Context, userPrompt string) (agent.StopReason, error)
	ToolIsReadOnly(name string) bool
	Steer(text string)
}

// AgentAdapter adapts internal/agent.Agent to the AgentRunner interface.
// It translates Agent event bus events (EventTextDelta, EventInfo, EventDone)
// into AgentEvent values and calls onEvent for each.
//
// The adapter subscribes to the agent's event bus before calling Agent.Run
// and unsubscribes after Run returns.
type AgentAdapter struct {
	a agentIface

	// real is the concrete agent when one was supplied (nil for test stubs).
	// ApplySettings mutates it directly — the same between-turns field-write
	// pattern the TUI's /models switch uses (cmd/dsc: a.Model = m).
	real *agent.Agent
}

// NewAgentAdapter wraps an *agent.Agent as an AgentRunner.
func NewAgentAdapter(a *agent.Agent) *AgentAdapter {
	return &AgentAdapter{a: a, real: a}
}

// ApplySettings implements SettingsApplier: it pushes the composer's
// model/effort selection and permission mode onto the wrapped agent. Empty or
// unrecognised values keep the agent's current state. Must only be called
// while no run is active (the gateway applies it before launching a turn).
func (ad *AgentAdapter) ApplySettings(ts TurnSettings) {
	if ad.real == nil {
		return
	}
	if ts.Model != "" {
		ad.real.Model = ts.Model
	}
	if ts.Effort != "" {
		if e, ok := llm.ParseReasoningEffort(ts.Effort); ok {
			ad.real.ReasoningEffort = e
		}
	}
	if ts.PermissionMode != "" && ad.real.Permissions != nil {
		if mode, confirmEdits, ok := permissionModeFrom(ts.PermissionMode); ok {
			ad.real.Permissions.Mode = mode
			ad.real.Permissions.ConfirmEdits = confirmEdits
		}
	}
}

// permissionModeFrom maps the GUI composer vocabulary to a policy mode.
// ask = tiered defaults with per-edit confirmation; auto-edit = plain tiered
// defaults (safe cwd edits auto-apply); plan = read-only; yolo = allow all.
func permissionModeFrom(ui string) (mode permissions.Mode, confirmEdits, ok bool) {
	switch ui {
	case "ask":
		return permissions.ModeDefault, true, true
	case "auto-edit":
		return permissions.ModeDefault, false, true
	case "plan":
		return permissions.ModePlan, false, true
	case "yolo":
		return permissions.ModeYolo, false, true
	}
	return 0, false, false
}

// Steer forwards a mid-turn instruction to the wrapped agent.
func (ad *AgentAdapter) Steer(text string) { ad.a.Steer(text) }

// Run implements AgentRunner. It subscribes to the agent bus, calls Agent.Run
// in a goroutine, and forwards events to onEvent. The subscription uses the
// real agent.Bus API: Subscribe(bufferSize) returns a *Subscription whose .C
// field is <-chan EventEnvelope; events are unwrapped via env.Event.
// Unsubscribe is called via Bus().Unsubscribe(sub) after Run returns.
func (ad *AgentAdapter) Run(ctx context.Context, userPrompt string, onEvent func(AgentEvent)) error {
	bus := ad.a.Bus()

	// Subscribe before launching Run so no events are missed.
	sub := bus.Subscribe(256)
	defer bus.Unsubscribe(sub)

	done := make(chan error, 1)
	go func() {
		_, err := ad.a.Run(ctx, userPrompt)
		done <- err
	}()

	for {
		select {
		case env, ok := <-sub.C:
			if !ok {
				// Bus was closed before EventDone arrived (e.g. agent panic).
				// Drain the done channel so we return whatever error Agent.Run produced.
				select {
				case err := <-done:
					return err
				default:
					return nil
				}
			}
			switch e := env.Event.(type) {
			case agent.EventTextDelta:
				onEvent(AgentEvent{Kind: EventKindTextDelta, Text: e.Text})
			case agent.EventInfo:
				onEvent(AgentEvent{Kind: EventKindInfo, Text: e.Text})
			case agent.EventToolCallStart:
				onEvent(AgentEvent{
					Kind:         EventKindToolStart,
					ToolCallID:   e.Call.ID,
					ToolName:     e.Call.Function.Name,
					ToolArgs:     e.Call.Function.Arguments,
					ToolReadOnly: ad.a.ToolIsReadOnly(e.Call.Function.Name),
				})
			case agent.EventToolCallResult:
				onEvent(AgentEvent{
					Kind:       EventKindToolEnd,
					ToolCallID: e.CallID,
					ToolResult: e.Result.Content,
					ToolIsErr:  e.Result.IsError,
				})
			case agent.EventPermissionAsk:
				reply := e.Reply
				var once sync.Once
				toolName := ""
				if e.Check.Tool != nil {
					toolName = e.Check.Tool.Name()
				}
				onEvent(AgentEvent{
					Kind:     EventKindPermission,
					ToolName: toolName,
					ToolArgs: string(e.Check.Args),
					Respond: func(d PermissionDecision) {
						once.Do(func() { reply <- agent.PermissionResponse{Allow: d.Allowed()} })
					},
				})
			case agent.EventQuestionAsk:
				reply := e.Reply
				qs := make([]AskQuestion, len(e.Questions))
				for i, q := range e.Questions {
					opts := make([]AskOption, len(q.Options))
					for j, o := range q.Options {
						opts[j] = AskOption{Label: o.Label, Description: o.Description}
					}
					qs[i] = AskQuestion{Question: q.Question, Header: q.Header, Multiple: q.Multiple, Options: opts}
				}
				var once sync.Once
				onEvent(AgentEvent{
					Kind:      EventKindAsk,
					Questions: qs,
					Answer: func(answers [][]string) {
						once.Do(func() { reply <- tools.QuestionResponse{Answers: answers} })
					},
				})
			case agent.EventDone:
				onEvent(AgentEvent{
					Kind:       EventKindDone,
					StopReason: e.Reason.String(),
					Err:        e.Err,
				})
				// EventDone is published by Agent.Run's defer, so Run has not
				// returned yet when we receive it here. Blocking on done is safe:
				// Run will write to done momentarily after the defer completes.
				return <-done
			case agent.EventReasoningDelta:
				onEvent(AgentEvent{Kind: EventKindThinking, Text: e.Text})
			case agent.EventStepFinish:
				onEvent(AgentEvent{
					Kind:     EventKindCache,
					TurnPct:  llm.CacheHitRate(e.Usage),
					Prefixes: 1, // gateway overwrites with its session epoch count
					Eviction: false,
				})
				onEvent(AgentEvent{
					Kind:         EventKindCost,
					TurnCNY:      llm.Cost(e.Model, e.Usage),
					OutputTokens: e.Usage.CompletionTokens,
				})
			case agent.EventEscalated:
				onEvent(AgentEvent{Kind: EventKindRouting, From: e.FromModel, To: e.ToModel, Reason: e.Reason})
			case agent.EventBackgroundJobStart:
				onEvent(AgentEvent{Kind: EventKindJob, Running: +1}) // delta; gateway accumulates
			case agent.EventBackgroundJobFinish:
				onEvent(AgentEvent{Kind: EventKindJob, Running: -1})
			case agent.EventRetry:
				onEvent(AgentEvent{Kind: EventKindRetry, Attempt: e.Attempt, Max: e.Max})
			case agent.EventToolCallDelta:
				onEvent(AgentEvent{Kind: EventKindToolDelta, ToolCallID: e.CallID, ToolDelta: e.Delta})
			case agent.EventCompaction, agent.EventSemanticCompaction:
				onEvent(AgentEvent{Kind: EventKindCache, Eviction: true})
			case agent.EventCompactionWarning:
				onEvent(AgentEvent{Kind: EventKindCompactionWarning, Pressure: e.Pressure, Threshold: e.Threshold})
			case agent.EventPlanUpdate:
				items := make([]PlanItem, len(e.Items))
				for i, it := range e.Items {
					items[i] = PlanItem{Text: it.Text, Status: it.Status}
				}
				onEvent(AgentEvent{Kind: EventKindPlan, Plan: items})
			case agent.EventHookFired:
				// Surface the Duet pro-validation result. The published decision is the
				// merged PreToolUse-hook outcome; Duet is the pre-tool validator, and it
				// only emits allow/deny WITH a reasoning when it actually adjudicated a
				// destructive call. Pass-through allow(no reason)/continue is dropped.
				if e.HookName == "PreToolUse" && e.Reason != "" && (e.Decision == "allow" || e.Decision == "deny") {
					onEvent(AgentEvent{Kind: EventKindDuet, Decision: e.Decision, Reason: e.Reason})
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
