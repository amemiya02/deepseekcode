package acp

import (
	"context"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// agentIface is the minimal surface of *agent.Agent that AgentAdapter needs.
// Keeping it unexported lets tests inject a stub without exposing it as a
// public contract. Production code always passes *agent.Agent.
type agentIface interface {
	Bus() *agent.Bus
	Run(ctx context.Context, userPrompt string) (agent.StopReason, error)
}

// AgentAdapter adapts internal/agent.Agent to the AgentRunner interface.
// It translates Agent event bus events (EventTextDelta, EventInfo, EventDone)
// into AgentEvent values and calls onEvent for each.
//
// The adapter subscribes to the agent's event bus before calling Agent.Run
// and unsubscribes after Run returns.
type AgentAdapter struct {
	a agentIface
}

// NewAgentAdapter wraps an *agent.Agent as an AgentRunner.
func NewAgentAdapter(a *agent.Agent) *AgentAdapter {
	return &AgentAdapter{a: a}
}

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
			case agent.EventPermissionAsk:
				// The adapter has no UI layer; deny the permission to unblock the agent.
				e.Reply <- agent.PermissionResponse{Allow: false}
			case agent.EventQuestionAsk:
				// The adapter has no UI layer; send an empty response to unblock the agent.
				e.Reply <- tools.QuestionResponse{}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
