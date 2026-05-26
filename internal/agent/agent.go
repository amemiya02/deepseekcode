package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/gitctx"
	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/repair"
	"github.com/amemiya02/deepseekcode/internal/sandbox"
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

	// eventsCompat is the agent-lifetime event channel for the Events()
	// compatibility wrapper. The consumer (TUI or CLI) ranges over
	// Events() and type-switches on the concrete Event type. Buffered so
	// streaming token bursts don't block the model goroutine; capacity
	// matched to ~4 seconds of fast SSE token rate.
	eventsCompat chan Event

	// bus is the multi-consumer fan-out hub. The agent publishes all
	// events through the bus; Events() returns a compatibility channel
	// that unwraps EventEnvelope back to bare Event. Additional
	// consumers subscribe via Bus().Subscribe.
	bus *Bus

	// Persister, if non-nil, receives session and snapshot bookkeeping
	// alongside the in-memory Messages list. nil = ephemeral session
	// (the -p one-shot mode runs this way).
	Persister Persister

	// Model is the active main-loop model (e.g. deepseek-v4-flash).
	// Changed mid-session via /models.
	Model    string
	Thinking bool

	// AutoReasoning enables per-turn thinking selection via
	// llm.SelectThinking. When true, runStep calls SelectThinking
	// with the last user message text to decide thinking on/off.
	AutoReasoning bool

	prefixMon *llm.PrefixMonitor

	// System is the system prompt. Cache-stable across turns by design.
	System string

	// PromptBuilder, when non-nil, overrides System with the builder's
	// output at the start of Run. The builder owns the static + dynamic
	// split (see internal/prompt); the agent just calls Build() to get
	// the assembled string. nil → System stays as configured.
	PromptBuilder *prompt.SystemPromptBuilder

	// CompactionCfg controls when the running message list gets
	// collapsed into a synthetic summary. Initialized to
	// DefaultCompactionConfig in New; override fields before Run.
	CompactionCfg CompactionConfig

	// HookRunner dispatches lifecycle hooks (PreToolUse, PostToolUse,
	// SessionStart, SessionEnd). nil = hooks disabled.
	HookRunner *hooks.Runner

	// StopWhen runs after each step; first match wins. Defaults below.
	StopWhen []StopCondition

	// Messages is the conversation. The agent appends user messages,
	// assistant turns, and tool results here.
	Messages []llm.Message

	// StepTimeout, if non-zero, caps the duration of a single step
	// (one model turn + tool execution). 0 = no per-step limit.
	StepTimeout time.Duration

	// MaxToolCalls is the hard cap on total tool calls per session.
	// Warns at 80% via OnInfo. 0 = unlimited.
	MaxToolCalls int

	// IsSubagent is true when this agent was spawned by a parent
	// via LoopSpawner. It disables thinking in sub-agents (via
	// SelectThinking) and may be used to adjust other behaviors.
	IsSubagent bool

	// Spawner, when non-nil, enables sub-agent dispatch from slash
	// commands that declare agent: or subtask: true. Set by the
	// assembly layer (cmd/dsc or TUI) after construction.
	Spawner tools.Spawner

	// Jobs manages background jobs (async sub-agents and background_bash).
	// Initialized in New; closed via defer in Run.
	Jobs *JobRegistry

	// Plan-mode state. inPlan is true while the agent is in plan mode;
	// savedTools/savedPolicy hold the originals so ExitPlan can restore.
	inPlan      bool
	savedTools  *tools.Registry
	savedPolicy *permissions.Policy

	// stormBreaker suppresses repeated identical read-only tool calls.
	stormBreaker *repair.StormBreaker

	toolCallCount int
	steps         []StepRecord
	gitReader     *gitctx.Reader // lazily constructed per cwd
}

// New returns an Agent with sensible defaults for v0.1.
//
// The Events channel is buffered at 256: roughly 4 seconds at a 60 tok/s
// burst rate. Streaming deltas don't block the model goroutine unless
// the consumer falls more than that behind, which would only happen if
// the UI goroutine were stuck — an upstream bug we'd want to surface.
func New(client *llm.Client, reg *tools.Registry, pol *permissions.Policy, model string) *Agent {
	a := &Agent{
		Client:        client,
		Tools:         reg,
		Permissions:   pol,
		eventsCompat:  make(chan Event, 256),
		Model:         model,
		Thinking:      true,
		prefixMon:     llm.NewPrefixMonitor(),
		System:        DefaultSystemPrompt,
		MaxToolCalls:  200,
		CompactionCfg: DefaultCompactionConfig(),
		Jobs:          NewJobRegistry(),
		stormBreaker:  repair.NewStormBreaker(6, 3),
		StopWhen: []StopCondition{
			MaxSteps(50),
			LoopDetection(5, 3),
		},
	}

	a.bus = NewBus()
	def := a.bus.Subscribe(256)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("agent: unpack goroutine panic: %v", r)
			}
		}()
		for env := range def.C {
			a.eventsCompat <- env.Event
		}
	}()

	return a
}

// Events returns the receive end of the agent-lifetime event stream.
// Consume from one goroutine; the agent guarantees in-order delivery.
// The channel is never closed by the agent — multiple Run calls share
// it. Consumers should select against their own ctx.Done() to exit
// cleanly during shutdown.
func (a *Agent) Events() <-chan Event { return a.eventsCompat }

// Bus returns the agent's event bus. Additional consumers (loggers,
// parity recorders, future daemons) subscribe via Bus().Subscribe
// to receive versioned EventEnvelope values. The primary consumer
// (TUI/CLI) should continue using Events() for backward compatibility.
func (a *Agent) Bus() *Bus { return a.bus }

// EmitInfo pushes an out-of-band notice onto the event stream. Used by
// adjacent components (e.g. llm.Client.OnRetry) that don't otherwise
// hold the event channel but want to surface user-visible status.
func (a *Agent) EmitInfo(msg string) {
	a.bus.Publish(EventInfo{Text: msg})
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
		a.bus.Publish(EventDone{Reason: reason, Err: err})
	}()

	defer func() {
		if a.HookRunner != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			sessionID := ""
			if a.Persister != nil {
				sessionID = a.Persister.SessionID()
			}
			a.HookRunner.Run(ctx, hooks.EventSessionEnd, hooks.HookInput{
				Event:     hooks.EventSessionEnd,
				CWD:       a.Permissions.Cwd,
				SessionID: sessionID,
			})
		}
	}()

	if a.PromptBuilder != nil {
		a.System = a.PromptBuilder.Build()
	}

	// SessionStart hook fires at the beginning of each Run.
	if a.HookRunner != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		a.HookRunner.Run(ctx, hooks.EventSessionStart, hooks.HookInput{
			Event: hooks.EventSessionStart,
			CWD:   a.Permissions.Cwd,
		})
	}

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
				a.bus.Publish(EventInfo{Text: fmt.Sprintf("step timed out after %s", a.StepTimeout)})
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
				a.bus.Publish(EventStepFinish{Reason: reason, Usage: step.Usage})
				return reason, nil
			}
		}

		if !hasTools {
			stepCancel()
			a.bus.Publish(EventStepFinish{Reason: StopModelDone, Usage: step.Usage})
			return StopModelDone, nil
		}

		// Tool execution shares the per-step deadline so a stuck tool
		// can't run forever.
		toolErr := a.runToolCalls(stepCtx, step.ToolCalls)
		stepCancel()
		if toolErr != nil {
			return StopUnknown, toolErr
		}
		a.bus.Publish(EventStepFinish{Reason: StopUnknown, Usage: step.Usage})

		// Compaction check runs between turns (the next iteration's
		// runStep will rebuild the wire request). A failure here is
		// reported but never aborts the loop — compaction is an
		// optimization, not a correctness requirement.
		a.maybeCompact(ctx)
	}
}

// ForceCompact runs maybeCompact with a temporarily-lowered token
// threshold so the user's /compact slash command can fire even when
// the message list hasn't hit the auto threshold. The preserve
// count is honored — too-short transcripts still no-op.
func (a *Agent) ForceCompact(ctx context.Context) {
	saved := a.CompactionCfg
	defer func() { a.CompactionCfg = saved }()
	a.CompactionCfg.AutoCompactInputTokens = 1
	a.maybeCompact(ctx)
}

// maybeCompact runs the compaction pipeline and, if it produced a
// summary, swaps the in-memory message list and persists the
// collapse. Errors surface via EventInfo so the user sees them
// without crashing the Run.
func (a *Agent) maybeCompact(ctx context.Context) {
	res := CompactSession(a.Messages, a.CompactionCfg)
	if res.Summary == "" {
		return
	}
	if a.Persister != nil {
		if _, err := a.Persister.ReplaceWithCompaction(ctx, res.FromIdx, res.ToIdx, res.Summary); err != nil {
			a.bus.Publish(EventInfo{Text: "compaction persistence failed: " + err.Error()})
			return
		}
	}
	a.Messages = append([]llm.Message{res.SummaryMessage}, res.KeptMessages...)
	a.bus.Publish(EventCompaction{
		FromIdx:      res.FromIdx,
		ToIdx:        res.ToIdx,
		Summary:      res.Summary,
		RemovedCount: res.RemovedCount,
	})
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
	a.refreshGitContext(ctx)

	thinking := a.Thinking
	if a.AutoReasoning {
		thinking = llm.SelectThinking(a.IsSubagent, a.lastUserText(), a.Thinking)
	}

	req := llm.Request{
		Model:    a.Model,
		Messages: a.fullMessages(),
		Tools:    a.Tools.AsLLMTools(),
		Thinking: llm.ThinkingEnabled(thinking),
	}

	staticSys := a.System
	if i := strings.Index(a.System, prompt.DynamicContextBoundary); i >= 0 {
		staticSys = a.System[:i]
	}
	if changed, which := a.prefixMon.Check(staticSys, req.Tools); changed {
		a.bus.Publish(EventInfo{Text: "prefix cache invalidated: " + which})
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
				a.bus.Publish(EventReasoningEnd{})
				inReasoning = false
			}
			a.bus.Publish(EventTextDelta{Text: ev.Text})
		case llm.EventReasoningDelta:
			if !inReasoning {
				a.bus.Publish(EventReasoningStart{})
				inReasoning = true
			}
			reasoning += ev.Text
			a.bus.Publish(EventReasoningDelta{Text: ev.Text})
		case llm.EventToolUseDelta:
			// tool-call deltas are aggregated by the client; we don't need
			// per-delta tracking here. The EventFinish carries the
			// assembled calls.
		case llm.EventFinish:
			if inReasoning {
				a.bus.Publish(EventReasoningEnd{})
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

	// Repair pipeline: scavenge, repair args, storm-breaker filter.
	assembledCall = a.repairToolCalls(ctx, reasoning, text, assembledCall, &blocks)

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

// refreshGitContext repopulates the builder's GitStatus / GitDiff /
// ActiveBranch fields with a fresh snapshot and rebuilds a.System.
// No-op when PromptBuilder or its Project is nil. Failures keep the
// previous values so the prompt never flickers between empty and
// populated mid-session.
func (a *Agent) refreshGitContext(ctx context.Context) {
	if a.PromptBuilder == nil || a.PromptBuilder.Project == nil {
		return
	}
	if a.PromptBuilder.Project.CWD == "" {
		return
	}
	if a.gitReader == nil {
		a.gitReader = &gitctx.Reader{CWD: a.PromptBuilder.Project.CWD}
	}
	snap, err := a.gitReader.Read(ctx)
	if err != nil {
		return
	}
	a.PromptBuilder.Project.GitStatus = snap.Status
	a.PromptBuilder.Project.GitDiff = snap.Diff
	if snap.ActiveBranch != "" {
		a.PromptBuilder.Project.ActiveBranch = snap.ActiveBranch
	}
	a.System = a.PromptBuilder.Build()
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
			block.IsError = r.res.IsError
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
	a.bus.Publish(EventToolCallStart{Call: call})

	// Tool-call rate limit. Warning fires exactly once when crossing 80%
	// of the cap; the hard cap blocks any call beyond MaxToolCalls.
	a.toolCallCount++
	if a.MaxToolCalls > 0 {
		threshold := int(float64(a.MaxToolCalls) * 0.8)
		if threshold > 0 && a.toolCallCount == threshold {
			a.bus.Publish(EventInfo{Text: fmt.Sprintf("tool call warning: %d/%d used", a.toolCallCount, a.MaxToolCalls)})
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
	dec, reason := a.Permissions.Decide(permissions.Check{Tool: tool, Args: rawArgs})
	switch dec {
	case permissions.Deny:
		if strings.HasPrefix(reason, "matched deny rule:") {
			a.bus.Publish(EventInfo{Text: "denied by rule: " + reason})
			return tools.Errf("denied by rule: %s", reason), nil
		}
		a.bus.Publish(EventInfo{Text: "denied by permissions policy: " + reason})
		return tools.Errf("denied by permissions policy: %s", reason), nil
	case permissions.Ask:
		// Emit a permission ask carrying its own reply channel, and
		// park until the consumer answers. Buffered cap=1 so the UI
		// can send without waiting on us to receive.
		reply := make(chan PermissionResponse, 1)
		a.bus.Publish(EventPermissionAsk{
			Check: permissions.Check{Tool: tool, Args: rawArgs},
			Reply: reply,
		})
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

	// Pre-tool hook runs after permission gate, before Duet / execution.
	if a.HookRunner != nil {
		sessionID := ""
		if a.Persister != nil {
			sessionID = a.Persister.SessionID()
		}
		hookIn := hooks.HookInput{
			Event:     hooks.EventPreToolUse,
			ToolName:  call.Function.Name,
			ToolInput: rawArgs,
			CWD:       a.Permissions.Cwd,
			SessionID: sessionID,
		}
		t0 := time.Now()
		out, _ := a.HookRunner.Run(ctx, hooks.EventPreToolUse, hookIn)
		a.bus.Publish(EventHookFired{
			HookName: "PreToolUse",
			Event:    string(hooks.EventPreToolUse),
			Decision: out.Decision,
			Reason:   out.Reason,
			Dur:      time.Since(t0),
		})
		if out.Decision == "deny" {
			return tools.Errf("blocked by hook: %s", out.Reason), nil
		}
	}

	t0 := time.Now()
	res, err := tool.Execute(ctx, rawArgs)
	dur := time.Since(t0)
	a.bus.Publish(EventToolCallResult{CallID: call.ID, Result: res, Dur: dur})

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return tools.Result{Content: "user cancelled"}, nil
		}
		// Fire PostToolUseFailure even on infra error so hooks see every tool outcome.
		a.firePostHook(ctx, call, rawArgs, true)
		return tools.Result{}, err
	}

	// Post-tool hook fires after execution. Failure variant runs when
	// the tool result indicates an error; decision is informational only
	// (cannot undo completed tool execution).
	a.firePostHook(ctx, call, rawArgs, res.IsError)

	return res.Truncate(tools.DefaultMaxResultBytes), nil
}

// firePostHook runs the PostToolUse or PostToolUseFailure hook and
// emits an EventHookFired. Extracted so the err-early-return path
// and the success path share one implementation.
func (a *Agent) firePostHook(ctx context.Context, call llm.ToolCall, rawArgs json.RawMessage, isError bool) {
	if a.HookRunner == nil {
		return
	}
	hookEvent := hooks.EventPostToolUse
	if isError {
		hookEvent = hooks.EventPostToolUseFailure
	}
	sessionID := ""
	if a.Persister != nil {
		sessionID = a.Persister.SessionID()
	}
	hookIn := hooks.HookInput{
		Event:     hookEvent,
		ToolName:  call.Function.Name,
		ToolInput: rawArgs,
		CWD:       a.Permissions.Cwd,
		SessionID: sessionID,
	}
	t0 := time.Now()
	out, _ := a.HookRunner.Run(ctx, hookEvent, hookIn)
	a.bus.Publish(EventHookFired{
		HookName: "PostToolUse",
		Event:    string(hookEvent),
		Decision: out.Decision,
		Reason:   out.Reason,
		Dur:      time.Since(t0),
	})
}

// Transcript returns a compact wire-format snapshot of recent messages
// for the Duet builtin hook. Bounded so we don't blow up pro's context
// uselessly; for v0.1 we send the last 8 messages.
func (a *Agent) Transcript() []byte {
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

// lastUserText returns the text of the most recent user message.
// Used by SelectThinking to decide per-turn thinking mode.
// Returns "" if there are no user messages.
func (a *Agent) lastUserText() string {
	for i := len(a.Messages) - 1; i >= 0; i-- {
		if a.Messages[i].Role == "user" {
			for _, b := range a.Messages[i].Blocks {
				if tb, ok := b.(llm.TextBlock); ok {
					return tb.Text
				}
			}
		}
	}
	return ""
}

// repairToolCalls runs the repair pipeline on tool calls after streaming.
// It scavenges hidden calls, repairs args, and applies storm-breaker filtering.
// Returns the kept calls and updates blocks in-place to reflect repaired/kept calls.
func (a *Agent) repairToolCalls(ctx context.Context, reasoning, content string, declared []llm.ToolCall, blocks *[]llm.ContentBlock) []llm.ToolCall {
	// Build allowed tool-name map from the registry
	allowed := make(map[string]struct{})
	for _, t := range a.Tools.AsLLMTools() {
		allowed[t.Function.Name] = struct{}{}
	}

	// Step 1: Scavenge hidden tool calls from reasoning and content
	scavengeResult := repair.ScavengeToolCalls(reasoning, content, allowed, repair.ScavengeOptions{})
	for _, r := range scavengeResult.Reports {
		a.bus.Publish(EventRepair{
			Kind:       string(r.Kind),
			Tool:       r.Tool,
			CallID:     r.CallID,
			Message:    r.Message,
			BeforeHash: r.BeforeHash,
			AfterHash:  r.AfterHash,
		})
	}

	// Step 2: Merge declared and recovered calls (copy to avoid slice aliasing)
	allCalls := make([]llm.ToolCall, 0, len(declared)+len(scavengeResult.Calls))
	allCalls = append(allCalls, declared...)
	allCalls = append(allCalls, scavengeResult.Calls...)

	// Step 3: Repair arguments for all calls
	repairedCalls := make([]llm.ToolCall, 0, len(allCalls))
	for _, call := range allCalls {
		repairResult := repair.RepairJSONArgs(call.Function.Arguments)
		if repairResult.Changed {
			call.Function.Arguments = repairResult.Repaired
			a.bus.Publish(EventRepair{
				Kind:       string(repair.KindArgsCompleted),
				Tool:       call.Function.Name,
				CallID:     call.ID,
				Message:    "arguments repaired",
				BeforeHash: repair.HashArgs(repairResult.Raw),
				AfterHash:  repair.HashArgs(repairResult.Repaired),
			})
		}
		if !repairResult.Valid {
			a.bus.Publish(EventRepair{
				Kind:    string(repair.KindArgsInvalid),
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "arguments invalid after repair",
			})
		}
		repairedCalls = append(repairedCalls, call)
	}

	// Step 4: Build tool kinds map from registry using ReadOnlyHint
	kinds := make(map[string]repair.ToolKind)
	for _, t := range a.Tools.All() {
		kind := repair.ToolMutating // safe default
		if ro, ok := t.(tools.ReadOnlyHint); ok && ro.IsReadOnly() {
			kind = repair.ToolReadOnly
		}
		kinds[t.Name()] = kind
	}

	// Step 5: Apply storm-breaker filter
	filtered := a.stormBreaker.Filter(repairedCalls, kinds)
	for _, r := range filtered.Reports {
		a.bus.Publish(EventRepair{
			Kind:       string(r.Kind),
			Tool:       r.Tool,
			CallID:     r.CallID,
			Message:    r.Message,
			BeforeHash: r.BeforeHash,
			AfterHash:  r.AfterHash,
		})
	}

	// Step 6: Update blocks to reflect kept calls only
	// Rebuild the blocks slice: preserve text/thinking blocks, replace tool-use blocks
	var newBlocks []llm.ContentBlock
	for _, b := range *blocks {
		switch b.(type) {
		case llm.TextBlock, llm.ThinkingBlock:
			newBlocks = append(newBlocks, b)
		case llm.ToolUseBlock:
			// Skip original tool-use blocks; we'll add kept ones below
		}
	}
	// Add ToolUseBlocks for kept calls
	for _, call := range filtered.Calls {
		newBlocks = append(newBlocks, llm.ToolUseBlock{
			ID:    call.ID,
			Name:  call.Function.Name,
			Input: json.RawMessage(call.Function.Arguments),
		})
	}
	*blocks = newBlocks

	return filtered.Calls
}

// AskQuestion implements tools.Questioner. It emits an EventQuestionAsk
// and blocks until the consumer replies or ctx is cancelled.
func (a *Agent) AskQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	reply := make(chan tools.QuestionResponse, 1)
	a.bus.Publish(EventQuestionAsk{Questions: req.Questions, Reply: reply})
	select {
	case resp := <-reply:
		return resp, nil
	case <-ctx.Done():
		return tools.QuestionResponse{}, ctx.Err()
	}
}

var _ tools.Questioner = (*Agent)(nil)

// StartBashJob implements tools.JobController. It starts a background bash
// job and returns immediately with the job ID.
func (a *Agent) StartBashJob(ctx context.Context, command string, usePTY bool, timeoutMs int, sb sandbox.Sandbox, profile sandbox.Profile) (string, error) {
	if a.Jobs == nil {
		return "", fmt.Errorf("no job registry configured")
	}

	job, jobCtx := a.Jobs.Start(context.Background(), JobBackgroundBash, command)

	a.bus.Publish(EventBackgroundJobStart{
		ID:   job.ID,
		Kind: JobBackgroundBash,
	})

	go func() {
		var finalState JobState
		var finalSummary string

		defer func() {
			// Read final state under lock before publishing
			job.mu.Lock()
			finalState = job.State
			finalSummary = job.Summary
			job.mu.Unlock()

			a.bus.Publish(EventBackgroundJobFinish{
				ID:      job.ID,
				State:   finalState,
				Summary: finalSummary,
			})
		}()

		var exitCode int
		var execErr error

		if usePTY {
			// Use PTY runner - pass job directly as OutputAppender
			_, exitCode, execErr = tools.RunPTYForJobWithSandbox(jobCtx, command, time.Duration(timeoutMs)*time.Millisecond, job, sb, profile)
		} else {
			// Use regular bash runner
			exitCode, execErr = runBashCommand(jobCtx, command, timeoutMs, job, sb, profile)
		}

		if execErr != nil {
			if jobCtx.Err() == context.Canceled {
				a.Jobs.Finish(job.ID, JobCanceled, "canceled by agent shutdown")
			} else {
				summary := fmt.Sprintf("error: %v", execErr)
				if tail, _, _ := job.Tail(10_000); tail != "" {
					if blocked, body := tools.ClassifySandboxBlocked(sb, tail); blocked {
						summary = body
					}
				}
				a.Jobs.Finish(job.ID, JobFailed, summary)
			}
			return
		}

		state := JobSucceeded
		if exitCode != 0 {
			state = JobFailed
		}

		summary := fmt.Sprintf("exit %d", exitCode)
		if exitCode != 0 {
			if tail, _, _ := job.Tail(10_000); tail != "" {
				if blocked, body := tools.ClassifySandboxBlocked(sb, tail); blocked {
					summary = body
				}
			}
		}
		a.Jobs.Finish(job.ID, state, summary)
	}()

	return job.ID, nil
}

// jobWriter wraps a Job to implement io.Writer for streaming output.
type jobWriter struct {
	job *Job
}

func (w jobWriter) Write(p []byte) (int, error) {
	w.job.AppendOutput(p)
	return len(p), nil
}

// runBashCommand runs a shell command and streams output to the job.
// Returns exit code and error.
func runBashCommand(ctx context.Context, command string, timeoutMs int, job *Job, sb sandbox.Sandbox, profile sandbox.Profile) (int, error) {
	shell := "/bin/sh"
	if s := os.Getenv("SHELL"); s != "" {
		shell = s
	}

	// Create timeout context
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Env = append(os.Environ(), "TERM=dumb")

	// Use a single writer to merge stdout/stderr
	cmd.Stdout = jobWriter{job: job}
	cmd.Stderr = jobWriter{job: job}

	if sb != nil {
		switch {
		case !sb.Available():
			job.AppendOutput([]byte("(sandbox unavailable; running unsandboxed)\n"))
		default:
			if err := sb.Wrap(ctx, profile, cmd); err != nil {
				job.AppendOutput([]byte("(sandbox unavailable; running unsandboxed: " + err.Error() + ")\n"))
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	waitErr := cmd.Wait()

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, waitErr
		}
	}

	return exitCode, nil
}

// JobStatus implements tools.JobStatusController.
func (a *Agent) JobStatus(id string, tailLines int) (tools.Status, error) {
	if a.Jobs == nil {
		return tools.Status{}, fmt.Errorf("no job registry configured")
	}
	status, err := a.Jobs.JobStatus(id, tailLines)
	if err != nil {
		return tools.Status{}, err
	}
	// Convert our Status to tools.Status
	return tools.Status{
		ID:           status.ID,
		Kind:         status.Kind,
		State:        status.State,
		StartedAt:    status.StartedAt,
		FinishedAt:   status.FinishedAt,
		Summary:      status.Summary,
		Tail:         status.Tail,
		DroppedBytes: status.DroppedBytes,
		TotalLines:   status.TotalLines,
		Truncated:    status.Truncated,
	}, nil
}

// CancelJob implements tools.JobStatusController.
func (a *Agent) CancelJob(id string) error {
	if a.Jobs == nil {
		return fmt.Errorf("no job registry configured")
	}
	if !a.Jobs.Cancel(id) {
		return fmt.Errorf("job not found or already finished")
	}
	return nil
}

// Close releases agent resources. It cancels all running background jobs
// and should be called when the session ends (not per prompt turn).
func (a *Agent) Close() {
	if a.Jobs != nil {
		a.Jobs.Close()
	}
}

var _ tools.JobController = (*Agent)(nil)
var _ tools.JobStatusController = (*Agent)(nil)
