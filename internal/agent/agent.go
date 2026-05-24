package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// PermissionResponse is what the UI returns from OnPermissionAsk.
type PermissionResponse struct {
	Allow          bool
	PersistPattern bool // when true (bash + "always"), persist to allowlist
}

// Agent is one running ReAct loop. Construct with New, drive with Run.
//
// Agent is *not* safe for concurrent use within a single session. The
// TUI wraps it in a goroutine and a consumer reads events from
// Events() to drive the UI.
type Agent struct {
	Client      *llm.Client
	Tools       *tools.Registry
	Permissions *permissions.Policy
	Validator   DuetValidator // nil = duet disabled

	// events is the agent-lifetime event channel. The consumer
	// (TUI or CLI) ranges over Events() and type-switches on the
	// concrete Event type. Buffered so streaming token bursts don't
	// block the model goroutine; capacity matched to ~4 seconds of
	// fast SSE token rate.
	events chan Event

	// Persister, if non-nil, receives session and snapshot bookkeeping
	// alongside the in-memory Messages list. nil = ephemeral session
	// (the -p one-shot mode runs this way).
	Persister Persister

	// Model is the active main-loop model (e.g. deepseek-v4-flash).
	// Changed mid-session via /models.
	Model    string
	Thinking bool

	// System is the system prompt. Cache-stable across turns by design.
	System string

	// StopWhen runs after each step; first match wins. Defaults below.
	StopWhen []StopCondition

	// Messages is the conversation. The agent appends user messages,
	// assistant turns, and tool results here.
	Messages []llm.Message

	// DuetExtraDestructive is the user's extra destructive regex list.
	DuetExtraDestructive []string

	// StepTimeout, if non-zero, caps the duration of a single step
	// (one model turn + tool execution). 0 = no per-step limit.
	StepTimeout time.Duration

	// MaxToolCalls is the hard cap on total tool calls per session.
	// Warns at 80% via OnInfo. 0 = unlimited.
	MaxToolCalls int

	toolCallCount int
	steps         []StepRecord
}

// New returns an Agent with sensible defaults for v0.1.
//
// The Events channel is buffered at 256: roughly 4 seconds at a 60 tok/s
// burst rate. Streaming deltas don't block the model goroutine unless
// the consumer falls more than that behind, which would only happen if
// the UI goroutine were stuck — an upstream bug we'd want to surface.
func New(client *llm.Client, reg *tools.Registry, pol *permissions.Policy, model string) *Agent {
	return &Agent{
		Client:       client,
		Tools:        reg,
		Permissions:  pol,
		Validator:    NopValidator{}, // wave-5 replaces
		events:       make(chan Event, 256),
		Model:        model,
		Thinking:     true,
		System:       DefaultSystemPrompt,
		MaxToolCalls: 200,
		StopWhen: []StopCondition{
			MaxSteps(50),
			LoopDetection(5, 3),
		},
	}
}

// Events returns the receive end of the agent-lifetime event stream.
// Consume from one goroutine; the agent guarantees in-order delivery.
// The channel is never closed by the agent — multiple Run calls share
// it. Consumers should select against their own ctx.Done() to exit
// cleanly during shutdown.
func (a *Agent) Events() <-chan Event { return a.events }

// EmitInfo pushes an out-of-band notice onto the event stream. Used by
// adjacent components (e.g. llm.Client.OnRetry) that don't otherwise
// hold the event channel but want to surface user-visible status.
func (a *Agent) EmitInfo(msg string) {
	a.events <- EventInfo{Text: msg}
}

// DefaultSystemPrompt is the cache-stable system prompt. It must not
// change between turns; that would invalidate the prompt cache and
// blow the cost story. Versioned by binary release, not by session.
const DefaultSystemPrompt = `You are deepseekcode, a terminal coding agent powered by DeepSeek.

You work directly with the user's repository through tool calls.
Behavioral rules:
- Prefer small, surgical edits. Use edit_file with unique old_string
  snippets; use write_file for new files or full rewrites.
- Before changing code, read enough to understand the surrounding
  context. Use grep for usages, read_file for definitions.
- Use bash for git, tests, build tools, and ad-hoc inspection. Always
  pass a timeout_ms if the command may run long.
- Use todo_write to track multi-step work. One item in_progress at a time.
- When uncertain, ask the user. Don't guess at file paths or invent APIs.
- Be concise. The user can see the diff; you don't need to narrate it.`

// Run drives the loop until a stop condition fires or context cancels.
// Returns the StopReason and any infrastructure error.
//
// The userPrompt is appended as a user message. To resume without a
// new user prompt (e.g. after a tool result the model needs to react
// to), pass "".
//
// Run defers an EventDone emit so the consumer sees a strict
// terminator AFTER every other event from this turn. Bypassing the
// events channel for the "done" signal used to race trailing deltas
// and leave the UI's chrome stuck on "writing…" — never do that.
func (a *Agent) Run(ctx context.Context, userPrompt string) (reason StopReason, err error) {
	defer func() {
		a.events <- EventDone{Reason: reason, Err: err}
	}()

	if userPrompt != "" {
		userBlocks := []llm.ContentBlock{llm.TextBlock{Text: userPrompt}}
		a.Messages = append(a.Messages, llm.Message{
			Role:   "user",
			Blocks: userBlocks,
		})
		if a.Persister != nil {
			_, _ = a.Persister.AppendUserMessage(ctx, userBlocks)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return StopContextCancel, ctx.Err()
		default:
		}

		// Per-step deadline covers BOTH the model turn and tool execution.
		// stepCancel is always non-nil so we can defer it unconditionally.
		stepCtx, stepCancel := stepContext(ctx, a.StepTimeout)

		step, err := a.runStep(stepCtx)
		if err != nil {
			stepCancel()
			if a.StepTimeout > 0 && errors.Is(err, context.DeadlineExceeded) {
				a.events <- EventInfo{Text: fmt.Sprintf("step timed out after %s", a.StepTimeout)}
				return StopUnknown, nil
			}
			return StopUnknown, err
		}
		a.steps = append(a.steps, step)

		// finish-reason override: even if the model said finish_reason=stop,
		// if it emitted tool calls we keep looping. See docs/design.md §6.4.
		hasTools := len(step.ToolCalls) > 0

		// Run user-provided stop conditions BEFORE the natural loop exit
		// check so loop-detection can fire on tool-emitting steps too.
		for _, sc := range a.StopWhen {
			if stop, reason := sc(a.steps); stop {
				stepCancel()
				a.events <- EventStepFinish{Reason: reason, Usage: step.Usage}
				return reason, nil
			}
		}

		if !hasTools {
			stepCancel()
			a.events <- EventStepFinish{Reason: StopModelDone, Usage: step.Usage}
			return StopModelDone, nil
		}

		// Tool execution shares the per-step deadline so a stuck tool
		// can't run forever.
		toolErr := a.runToolCalls(stepCtx, step.ToolCalls)
		stepCancel()
		if toolErr != nil {
			return StopUnknown, toolErr
		}
		a.events <- EventStepFinish{Reason: StopUnknown, Usage: step.Usage}
	}
}

// stepContext returns (ctx, cancel) for one step. When timeout is zero
// the parent context is reused and cancel is a no-op so the caller can
// invoke it unconditionally.
func stepContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

// runStep streams one model turn and aggregates events into a StepRecord.
// It assembles the assistant message and appends it to a.Messages.
func (a *Agent) runStep(ctx context.Context) (StepRecord, error) {
	req := llm.Request{
		Model:    a.Model,
		Messages: a.fullMessages(),
		Tools:    a.Tools.AsLLMTools(),
		Thinking: llm.ThinkingEnabled(a.Thinking),
	}

	events, err := a.Client.Stream(ctx, req)
	if err != nil {
		return StepRecord{}, err
	}

	var (
		text          string
		reasoning     string
		inReasoning   bool
		assembledCall []llm.ToolCall
		blocks        []llm.ContentBlock
		finish        string
		usage         llm.Usage
	)

	for ev := range events {
		switch ev.Type {
		case llm.EventTextDelta:
			text += ev.Text
			if inReasoning {
				a.events <- EventReasoningEnd{}
				inReasoning = false
			}
			a.events <- EventTextDelta{Text: ev.Text}
		case llm.EventReasoningDelta:
			if !inReasoning {
				a.events <- EventReasoningStart{}
				inReasoning = true
			}
			reasoning += ev.Text
			a.events <- EventReasoningDelta{Text: ev.Text}
		case llm.EventToolCallDelta:
			// tool-call deltas are aggregated by the client; we don't need
			// per-delta tracking here. The EventFinish carries the
			// assembled calls.
		case llm.EventFinish:
			if inReasoning {
				a.events <- EventReasoningEnd{}
				inReasoning = false
			}
			finish = ev.FinishReason
			usage = ev.Usage
			assembledCall = ev.ToolCalls
			blocks = ev.Blocks
		case llm.EventError:
			return StepRecord{}, fmt.Errorf("stream error: %w", ev.Err)
		}
	}

	// The wire flatten layer turns Blocks back into DeepSeek's
	// {content, reasoning_content, tool_calls} shape on the next
	// request, so reasoning_content still round-trips and the
	// thinking-mode 400 stays away.
	a.Messages = append(a.Messages, llm.Message{
		Role:   "assistant",
		Blocks: blocks,
	})
	if a.Persister != nil {
		_, _ = a.Persister.AppendAssistant(context.Background(), blocks, a.Model, usage)
	}

	return StepRecord{
		FinishReason: finish,
		Usage:        usage,
		ToolCalls:    assembledCall,
	}, nil
}

// fullMessages returns the wire-format message list with the system
// prompt prepended.
func (a *Agent) fullMessages() []llm.Message {
	if a.System == "" {
		return a.Messages
	}
	out := make([]llm.Message, 0, len(a.Messages)+1)
	out = append(out, llm.Message{
		Role:   "system",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: a.System}},
	})
	out = append(out, a.Messages...)
	return out
}

// runToolCalls dispatches all tool calls from one step, respecting
// permissions and the Duet validator. Calls without dependencies (i.e.
// all of them — we don't model deps) run in parallel.
//
// Snapshots are taken serially before the parallel execution kicks off
// so a /undo can revert all files touched by one step in a single
// operation.
func (a *Agent) runToolCalls(ctx context.Context, calls []llm.ToolCall) error {
	// Pre-tool snapshot: union of all statically-affected paths.
	if a.Persister != nil {
		var paths []string
		for _, call := range calls {
			paths = append(paths, AffectedPathsFor(a.Tools, call)...)
		}
		if len(paths) > 0 {
			_, _ = a.Persister.TakeSnapshot(len(a.steps), paths)
		}
	}

	type out struct {
		idx    int
		callID string
		res    tools.Result
		err    error
	}
	results := make([]out, len(calls))

	var wg sync.WaitGroup
	for i, call := range calls {
		i, call := i, call
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := a.executeOne(ctx, call)
			results[i] = out{idx: i, callID: call.ID, res: res, err: err}
		}()
	}
	wg.Wait()

	// Append results in original order so the conversation transcript
	// stays deterministic.
	for _, r := range results {
		block := llm.ToolResultBlock{ToolUseID: r.callID}
		if r.err != nil {
			// Infrastructure failure: surface as a tool error so the
			// model can react instead of crashing the run.
			block.Content = "execution error: " + r.err.Error()
			block.IsError = true
		} else {
			block.Content = r.res.Content
		}
		a.Messages = append(a.Messages, llm.Message{
			Role:   "tool",
			Blocks: []llm.ContentBlock{block},
		})
		if a.Persister != nil {
			_, _ = a.Persister.AppendToolResult(ctx, r.callID, block.Content, block.IsError)
		}
	}
	return nil
}

// executeOne dispatches a single tool call: permission check → Duet
// validator (if destructive) → execute. Returns the Result the model
// will see, plus an infrastructure error (which the caller serializes
// into a tool-result message rather than aborting the loop).
func (a *Agent) executeOne(ctx context.Context, call llm.ToolCall) (tools.Result, error) {
	a.events <- EventToolCallStart{Call: call}

	// Tool-call rate limit. Warning fires exactly once when crossing 80%
	// of the cap; the hard cap blocks any call beyond MaxToolCalls.
	a.toolCallCount++
	if a.MaxToolCalls > 0 {
		threshold := int(float64(a.MaxToolCalls) * 0.8)
		if threshold > 0 && a.toolCallCount == threshold {
			a.events <- EventInfo{Text: fmt.Sprintf("tool call warning: %d/%d used", a.toolCallCount, a.MaxToolCalls)}
		}
		if a.toolCallCount > a.MaxToolCalls {
			return tools.Errf("tool call limit reached (%d/%d)", a.toolCallCount, a.MaxToolCalls), nil
		}
	}

	tool, ok := a.Tools.Get(call.Function.Name)
	if !ok {
		return tools.Errf("unknown tool: %s", call.Function.Name), nil
	}

	rawArgs := json.RawMessage(call.Function.Arguments)
	if len(rawArgs) == 0 {
		rawArgs = json.RawMessage("{}")
	}

	// Validate tool-call arguments against the tool's JSON Schema.
	if err := validateToolArgs(tool, rawArgs); err != nil {
		return tools.Errf("invalid arguments for %s: %v", call.Function.Name, err), nil
	}

	// Permission gate.
	dec := a.Permissions.Decide(permissions.Check{Tool: tool, Args: rawArgs})
	switch dec {
	case permissions.Deny:
		return tools.Errf("denied by permissions policy"), nil
	case permissions.Ask:
		// Emit a permission ask carrying its own reply channel, and
		// park until the consumer answers. Buffered cap=1 so the UI
		// can send without waiting on us to receive.
		reply := make(chan PermissionResponse, 1)
		a.events <- EventPermissionAsk{
			Check: permissions.Check{Tool: tool, Args: rawArgs},
			Reply: reply,
		}
		var resp PermissionResponse
		select {
		case resp = <-reply:
		case <-ctx.Done():
			return tools.Errf("cancelled while awaiting permission"), nil
		}
		if !resp.Allow {
			return tools.Errf("user denied tool call"), nil
		}
		if resp.PersistPattern && tool.Name() == "bash" {
			var ba struct {
				Command string `json:"command"`
			}
			_ = json.Unmarshal(rawArgs, &ba)
			a.Permissions.AllowBashPattern(ba.Command)
		}
	}

	// Duet validator on destructive operations.
	if a.Validator != nil && a.isDestructive(call, rawArgs) && !a.duetSelfValidates() {
		t0 := time.Now()
		dec, err := a.Validator.Validate(ctx, call.Function.Name, rawArgs, a.transcript())
		dur := time.Since(t0)
		if err != nil {
			a.events <- EventInfo{Text: "pro validation skipped: " + err.Error()}
		} else {
			a.events <- EventDuet{CallID: call.ID, Approved: dec.Approve, Reasoning: dec.Reasoning, Dur: dur}
			if !dec.Approve {
				return tools.Errf("blocked by pro validator: %s", dec.Reasoning), nil
			}
		}
	}

	t0 := time.Now()
	res, err := tool.Execute(ctx, rawArgs)
	dur := time.Since(t0)
	a.events <- EventToolCallResult{CallID: call.ID, Result: res, Dur: dur}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return tools.Result{Content: "user cancelled"}, nil
		}
		return tools.Result{}, err
	}

	return res.Truncate(tools.DefaultMaxResultBytes), nil
}

// isDestructive returns true if this tool call should be reviewed by
// the Pro validator. See docs/design.md §11.2.
func (a *Agent) isDestructive(call llm.ToolCall, args json.RawMessage) bool {
	if call.Function.Name == "bash" {
		var ba struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(args, &ba)
		return permissions.IsDestructiveBash(ba.Command, a.DuetExtraDestructive)
	}
	cwd := a.Permissions.Cwd
	return permissions.IsDestructiveToolCall(call.Function.Name, args, cwd, a.Permissions.SecretPathPatterns)
}

// duetSelfValidates returns true when the user has switched the main-loop
// model to pro via /models — in which case the Duet validator becomes a
// silent no-op (pro can't meaningfully validate itself).
// See docs/design.md §11.7.
func (a *Agent) duetSelfValidates() bool {
	return a.Model == "deepseek-v4-pro"
}

// transcript returns a compact wire-format snapshot of recent messages
// for the Duet validator. Bounded so we don't blow up pro's context
// uselessly; for v0.1 we send the last 8 messages.
func (a *Agent) transcript() []byte {
	const tail = 8
	start := 0
	if len(a.Messages) > tail {
		start = len(a.Messages) - tail
	}
	b, _ := json.Marshal(a.Messages[start:])
	return b
}

// validateToolArgs performs lightweight validation of tool-call arguments
// against the tool's JSON Schema. It checks: (1) valid JSON, (2) is an
// object, (3) required fields are present. This catches the most common
// model mistakes without a full JSON Schema validator dependency.
func validateToolArgs(t tools.Tool, args json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil {
		return fmt.Errorf("arguments must be a JSON object: %w", err)
	}

	// Parse the tool's parameter schema to extract required fields.
	schema := t.Parameters()
	if len(schema) == 0 {
		return nil
	}
	var s struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil // schema parsing failure is not the model's fault
	}
	for _, field := range s.Required {
		if _, ok := m[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}
	return nil
}

