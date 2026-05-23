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

// Callbacks is the table-driven event bus the loop pushes events through.
// The TUI subscribes by populating fields. Persistence (session.Store)
// also subscribes. Nil callbacks are silently skipped.
type Callbacks struct {
	OnTextDelta      func(text string)
	OnReasoningStart func()
	OnReasoningDelta func(text string)
	OnReasoningEnd   func()
	OnToolCallStart  func(call llm.ToolCall)
	OnToolCallResult func(callID string, result tools.Result, dur time.Duration)
	OnStepFinish     func(reason StopReason, usage llm.Usage)
	OnDuetValidation func(callID string, approved bool, reasoning string, dur time.Duration)
	OnPermissionAsk  func(check permissions.Check) PermissionResponse
	OnInfo           func(msg string) // out-of-band notices (skipped validator, etc.)
}

// Agent is one running ReAct loop. Construct with New, drive with Run.
//
// Agent is *not* safe for concurrent use within a single session. The
// TUI wraps it in a goroutine and pipes interaction through Callbacks.
type Agent struct {
	Client      *llm.Client
	Tools       *tools.Registry
	Permissions *permissions.Policy
	Validator   DuetValidator // nil = duet disabled
	Callbacks   Callbacks

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
func New(client *llm.Client, reg *tools.Registry, pol *permissions.Policy, model string) *Agent {
	return &Agent{
		Client:       client,
		Tools:        reg,
		Permissions:  pol,
		Validator:    NopValidator{}, // wave-5 replaces
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
func (a *Agent) Run(ctx context.Context, userPrompt string) (StopReason, error) {
	if userPrompt != "" {
		a.Messages = append(a.Messages, llm.Message{
			Role:    "user",
			Content: userPrompt,
		})
		if a.Persister != nil {
			_, _ = a.Persister.AppendUserMessage(ctx, userPrompt)
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
				a.fire(func() { a.Callbacks.OnInfo(fmt.Sprintf("step timed out after %s", a.StepTimeout)) })
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
				a.fire(func() { a.Callbacks.OnStepFinish(reason, step.Usage) })
				return reason, nil
			}
		}

		if !hasTools {
			stepCancel()
			a.fire(func() { a.Callbacks.OnStepFinish(StopModelDone, step.Usage) })
			return StopModelDone, nil
		}

		// Tool execution shares the per-step deadline so a stuck tool
		// can't run forever.
		toolErr := a.runToolCalls(stepCtx, step.ToolCalls)
		stepCancel()
		if toolErr != nil {
			return StopUnknown, toolErr
		}
		a.fire(func() { a.Callbacks.OnStepFinish(StopUnknown, step.Usage) })
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
		finish        string
		usage         llm.Usage
	)

	for ev := range events {
		switch ev.Type {
		case llm.EventTextDelta:
			text += ev.Text
			if inReasoning {
				a.fire(func() { a.Callbacks.OnReasoningEnd() })
				inReasoning = false
			}
			a.fire(func() { a.Callbacks.OnTextDelta(ev.Text) })
		case llm.EventReasoningDelta:
			if !inReasoning {
				a.fire(func() { a.Callbacks.OnReasoningStart() })
				inReasoning = true
			}
			reasoning += ev.Text
			a.fire(func() { a.Callbacks.OnReasoningDelta(ev.Text) })
		case llm.EventToolCallDelta:
			// tool-call deltas are aggregated by the client; we don't need
			// per-delta tracking here. The EventFinish carries the
			// assembled calls.
		case llm.EventFinish:
			if inReasoning {
				a.fire(func() { a.Callbacks.OnReasoningEnd() })
				inReasoning = false
			}
			finish = ev.FinishReason
			usage = ev.Usage
			assembledCall = ev.ToolCalls
		case llm.EventError:
			return StepRecord{}, fmt.Errorf("stream error: %w", ev.Err)
		}
	}

	// Persist the assistant turn (text + tool_calls; reasoning_content is
	// rendered via callbacks and stored separately by the session layer).
	a.Messages = append(a.Messages, llm.Message{
		Role:      "assistant",
		Content:   text,
		ToolCalls: assembledCall,
	})
	if a.Persister != nil {
		_, _ = a.Persister.AppendAssistant(context.Background(), text, reasoning, assembledCall, a.Model, usage)
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
	out = append(out, llm.Message{Role: "system", Content: a.System})
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
		var msg llm.Message
		msg.Role = "tool"
		msg.ToolCallID = r.callID
		if r.err != nil {
			// Infrastructure failure: surface as a tool error so the
			// model can react instead of crashing the run.
			msg.Content = "execution error: " + r.err.Error()
		} else {
			msg.Content = r.res.Content
		}
		a.Messages = append(a.Messages, msg)
		if a.Persister != nil {
			_, _ = a.Persister.AppendToolResult(ctx, r.callID, msg.Content)
		}
	}
	return nil
}

// executeOne dispatches a single tool call: permission check → Duet
// validator (if destructive) → execute. Returns the Result the model
// will see, plus an infrastructure error (which the caller serializes
// into a tool-result message rather than aborting the loop).
func (a *Agent) executeOne(ctx context.Context, call llm.ToolCall) (tools.Result, error) {
	a.fire(func() { a.Callbacks.OnToolCallStart(call) })

	// Tool-call rate limit. Warning fires exactly once when crossing 80%
	// of the cap; the hard cap blocks any call beyond MaxToolCalls.
	a.toolCallCount++
	if a.MaxToolCalls > 0 {
		threshold := int(float64(a.MaxToolCalls) * 0.8)
		if threshold > 0 && a.toolCallCount == threshold {
			a.fire(func() {
				a.Callbacks.OnInfo(fmt.Sprintf("tool call warning: %d/%d used", a.toolCallCount, a.MaxToolCalls))
			})
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
		if a.Callbacks.OnPermissionAsk == nil {
			return tools.Errf("permission required but no UI to ask"), nil
		}
		resp := a.Callbacks.OnPermissionAsk(permissions.Check{Tool: tool, Args: rawArgs})
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
			a.fire(func() { a.Callbacks.OnInfo("pro validation skipped: " + err.Error()) })
		} else {
			a.fire(func() { a.Callbacks.OnDuetValidation(call.ID, dec.Approve, dec.Reasoning, dur) })
			if !dec.Approve {
				return tools.Errf("blocked by pro validator: %s", dec.Reasoning), nil
			}
		}
	}

	t0 := time.Now()
	res, err := tool.Execute(ctx, rawArgs)
	dur := time.Since(t0)
	a.fire(func() { a.Callbacks.OnToolCallResult(call.ID, res, dur) })

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

// fire safely invokes a Callback closure, no-op'ing on nil.
func (a *Agent) fire(fn func()) {
	defer func() {
		// Swallow panics from user-supplied callbacks; they should never
		// take down the loop.
		_ = recover()
	}()
	if fn == nil {
		return
	}
	fn()
}
