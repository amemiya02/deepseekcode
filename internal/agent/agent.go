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
	"sync/atomic"
	"time"

	"github.com/amemiya02/deepseekcode/internal/agents"
	"github.com/amemiya02/deepseekcode/internal/cacheunit"
	"github.com/amemiya02/deepseekcode/internal/gitctx"
	"github.com/amemiya02/deepseekcode/internal/hooks"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/mcp"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/prompt"
	"github.com/amemiya02/deepseekcode/internal/repair"
	"github.com/amemiya02/deepseekcode/internal/routing"
	"github.com/amemiya02/deepseekcode/internal/sandbox"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/skills"
	"github.com/amemiya02/deepseekcode/internal/tokenizer"
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

	// traceSink is the root JSONL trace sink set by AttachTraceSink, retained
	// so spawned subagents can tee their epoch/usage events into the same
	// trace via AttachChildTraceSink. nil when no trace is being emitted.
	traceSink *TraceSink

	// childTraceHandles tracks subagent trace handles so a one-shot run can
	// wait for in-flight (async) subagent traces to flush before it closes the
	// root trace and exits — otherwise an async `task` whose completion the
	// model never polled would be killed at process exit and its child epoch
	// lost. Guarded by childTraceMu.
	childTraceMu      sync.Mutex
	childTraceHandles []*TraceSinkHandle

	// stopRequested records an explicit user stop (e.g. the TUI's ctrl+c)
	// so Run can report StopUserRequested instead of an ambient
	// StopContextCancel. Reset at the start of each Run. Set from the UI
	// goroutine, read from the agent goroutine, hence atomic.
	stopRequested atomic.Bool

	// checkpoints is the named-checkpoint index for this session.
	// Reset at the start of each Run so branch/resume works on the live step list.
	checkpoints *CheckpointIndex

	// Persister, if non-nil, receives session and snapshot bookkeeping
	// alongside the in-memory Messages list. nil = ephemeral session
	// (the -p one-shot mode runs this way).
	Persister Persister

	// Model is the active main-loop model (e.g. deepseek-v4-flash).
	// Changed mid-session via /models.
	Model    string
	Thinking bool

	// ThinkingMode, when set (on|off|adaptive), overrides per-turn thinking
	// selection for cost experiments (env DEEPSEEKCODE_THINKING_MODE). Empty
	// keeps the legacy Thinking/AutoReasoning behavior. "off" never thinks;
	// "adaptive" reasons on the first turn (plan) or a repair turn, terse
	// otherwise — cutting the 2x-priced reasoning_content tax on routine turns.
	ThinkingMode string
	// turnsSeen counts runStep invocations this session; selectTurnThinking
	// reads it so "adaptive" mode can reason on the first turn only.
	turnsSeen int

	// Temperature and TopP, when non-nil, are sent on every model request as
	// the OpenAI-shaped sampling controls. nil (the default) omits the field
	// entirely so the main-loop wire bytes — and thus the cache fingerprint —
	// are unchanged. Sub-agents set these from their def frontmatter (T7.1).
	Temperature *float64
	TopP        *float64

	// EscalationModel, when non-empty and different from Model, enables
	// model-driven escalation (T2.3): a turn is re-issued once on this model
	// when the assistant emits a <<<NEEDS_PRO>>> self-declaration or the
	// per-turn repair-error count crosses escalationRepairThreshold. Empty (the
	// default) disables escalation entirely — the mechanism is a no-op and adds
	// no wire bytes. The marker *contract* (telling the model the marker exists)
	// is a separate, opt-in system-prompt addition (see escalationContract); the
	// detection here works regardless and never moves the Prefix Fingerprint.
	EscalationModel string

	// AutoReasoning enables per-turn thinking selection via
	// llm.SelectThinking. When true, runStep calls SelectThinking
	// with the last user message text to decide thinking on/off.
	AutoReasoning bool

	// ReasoningEffort is the configured DeepSeek V4 reasoning effort
	// level (low, medium, high, max). Carried into every main-loop
	// request when thinking is enabled. Empty means omit from wire.
	ReasoningEffort llm.ReasoningEffort

	// AutoRoute enables the pre-turn cost-aware router (internal/routing):
	// per-turn model + effort chosen from the user message and repair signal.
	// It never moves the Prefix Fingerprint (model/effort are not in the static
	// prefix). Requires EscalationModel set for the pro tier to be reachable.
	AutoRoute bool

	// AutoClarify gates vague prompts through internal/routing.NeedsClarification
	// before spending a (possibly pro/max) turn.
	AutoClarify bool

	// lastRoute carries routing stickiness across turns.
	lastRoute routing.Decision

	// UserID is an optional DeepSeek field for abuse monitoring and
	// enterprise attribution. Empty means omitted from wire.
	UserID string

	prefixMon *llm.PrefixMonitor
	epochMgr  *EpochManager

	// DisablePrefixEpoch disables the PrefixEpoch feature (for benchmarking).
	DisablePrefixEpoch bool

	// DisableSemanticCompaction disables semantic (LLM) compaction (for benchmarking).
	DisableSemanticCompaction bool

	// System is the system prompt. Cache-stable across turns by design.
	System string

	// cacheUnit, when > 0, pads the static system prompt so the frozen prefix
	// ends on a DeepSeek cache-unit boundary (§3.4). The padding is injected at
	// the EpochComponents build site, BEFORE the epoch is created/hashed, so it
	// lives inside the frozen, fingerprinted prefix and stays byte-stable across
	// turns. The value comes from a bench/cmd/cacheprobe measurement; 0 (the
	// default) disables padding entirely — zero behavior change, no wire bytes.
	cacheUnit int

	// PromptBuilder, when non-nil, overrides System with the builder's
	// output at the start of Run. The builder owns the static + dynamic
	// split (see internal/prompt); the agent just calls Build() to get
	// the assembled string. nil → System stays as configured.
	PromptBuilder *prompt.SystemPromptBuilder

	// CompactionCfg controls when the running message list gets
	// collapsed into a synthetic summary. Initialized to
	// DefaultCompactionConfig in New; override fields before Run.
	CompactionCfg CompactionConfig

	// SemanticCfg controls semantic (LLM-powered) compaction.
	// Initialized to defaultSemanticCompactionConfig in New;
	// override fields before Run. Zero value disables semantic
	// compaction (falls back to deterministic only).
	SemanticCfg SemanticCompactionConfig

	// MaxContextTokens is the maximum context window size for
	// context pressure computation. Default: 128_000.
	MaxContextTokens int

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

	// BudgetPolicy and BudgetState control session cost gating.
	// Zero values (default) disable budget checks entirely.
	BudgetPolicy BudgetPolicy
	BudgetState  BudgetState

	// Skills is the skill metadata store. nil = no skills loaded.
	Skills *skills.Store

	// PostEditDiagnostics, when non-nil, is called after each step that
	// includes mutating tool calls. It receives the deduplicated list of
	// affected file paths and returns a formatted diagnostics string. An
	// empty return means no feedback is appended. The returned string is
	// injected as a synthetic user message so the model sees it on its
	// next turn, following the same pattern as injectLoopBreakNudge.
	PostEditDiagnostics func(ctx context.Context, paths []string) string

	// Verify, when non-nil, runs a shell command after each step that
	// includes mutating tool calls. When the command exits non-zero, the
	// synthesized feedback is injected as a synthetic user message so the
	// model can fix the reported errors on its next turn.
	//
	// Verify is also consulted at model-stop time (when the model emits no
	// tool calls): if the hook passes, the stop reason is promoted from
	// StopModelDone to StopVerifiedDone; if it fails, the feedback is
	// injected into a.Messages before returning so the caller (or a
	// re-entered loop) has context about the failure. This is the single
	// wiring point for verification — do not set a separate VerifyCmd field.
	Verify *VerifyHook

	// MCPRegistry is the MCP tool registry. nil = no MCP servers.
	// Its SchemaHash feeds the epoch's mcp_schema_hash, so startup MCP
	// discovery is part of the frozen prefix and mid-session schema
	// changes surface as pending changes rather than live drift.
	MCPRegistry *mcp.Registry

	// Profile is the active first-class agent profile. nil means the
	// implicit "default" profile. The profile name feeds the epoch's
	// agent_profile_hash; switching profiles via SwitchProfile creates a
	// new epoch (one expected cache miss) rather than mutating the live one.
	Profile *agents.AgentProfile

	// ActiveTiers controls which tool tiers are sent to the model. The
	// agent uses Tools.AsLLMToolsFiltered(ActiveTiers...) when building
	// requests; a nil/empty slice means "no filter" (all registered tools
	// are exposed). The constructor defaults this to [TierCore] (see New).
	ActiveTiers []tools.ToolTier

	toolCallCount int
	steps         []StepRecord
	gitReader     *gitctx.Reader // lazily constructed per cwd

	// compactionFloor is the step index at/after which StepRecord.MessageCount
	// boundaries are valid for /undo (T3.5). A compaction renumbers a.Messages,
	// invalidating boundaries recorded by earlier steps, so it is raised to
	// len(a.steps) whenever a compaction swaps the message slice; ReconcileUndo
	// refuses to undo to a step below it. Session-scoped (NOT reset per Run).
	compactionFloor int

	// loopNudged records that the one-shot loop-break nudge has been spent in
	// the current Run. loopFloor is the step index from which loop detection
	// counts *after* a nudge: without it the pre-nudge repeats stay inside the
	// detection window and re-trip the detector on the very next step, robbing
	// the model of the single recovery turn the nudge is meant to grant. Both
	// are per-Run, touched only on the single Run/runStep goroutine, and reset
	// at the top of Run.
	loopNudged bool
	loopFloor  int

	// charsPerToken is the per-session calibrated chars-per-token ratio (T4.1),
	// learned from provider usage frames and consumed by the compaction trigger
	// and the budget projection. 0 means uncalibrated → the cold-start char/4
	// prior is used. Touched only on the single Run/runStep goroutine (like
	// loopFloor), so it needs no lock.
	charsPerToken float64

	// staticPrefixResidual is the learned server-side static-prefix token cost
	// (system+tool-schema template) not visible to the local tokenizer. Learned
	// once from the first real usage.PromptTokens frame, then cached. Zero until
	// learned — only a small turn-1 undercount. Touched only on the run goroutine.
	staticPrefixResidual  int
	staticResidualLearned bool

	// schemaComplexEmitted tracks which tools have had schema-complexity
	// telemetry emitted in the current prefix epoch, keyed by
	// epochID + "\x00" + toolName. Prevents repeated noise per epoch.
	schemaComplexEmitted map[string]bool

	// schemaAdapters maps tool names to their SchemaAdapter for
	// rehydrating flattened arguments back to nested form before execution.
	// Populated by runStep when BuildSchemaAdapters flattens complex schemas.
	schemaAdapters map[string]repair.SchemaAdapter
}

// New returns an Agent with sensible defaults for v0.1.
//
// The Events channel is buffered at 256: roughly 4 seconds at a 60 tok/s
// burst rate. Streaming deltas don't block the model goroutine unless
// the consumer falls more than that behind, which would only happen if
// the UI goroutine were stuck — an upstream bug we'd want to surface.
func New(client *llm.Client, reg *tools.Registry, pol *permissions.Policy, model string) *Agent {
	a := &Agent{
		Client:               client,
		Tools:                reg,
		Permissions:          pol,
		eventsCompat:         make(chan Event, 256),
		Model:                model,
		Thinking:             true,
		prefixMon:            llm.NewPrefixMonitor(),
		epochMgr:             NewEpochManager(),
		System:               DefaultSystemPrompt,
		MaxToolCalls:         200,
		ActiveTiers:          []tools.ToolTier{tools.TierCore},
		CompactionCfg:        DefaultCompactionConfig(),
		SemanticCfg:          defaultSemanticCompactionConfig(),
		MaxContextTokens:     MaxContextTokens,
		Jobs:                 NewJobRegistry(),
		stormBreaker:         repair.NewStormBreaker(6, 3),
		schemaComplexEmitted: make(map[string]bool),
		schemaAdapters:       make(map[string]repair.SchemaAdapter),
	}

	// ThinkingMode (on|off|adaptive) is env-settable for cost A/Bs; empty keeps
	// the legacy default behavior so the shipped default is unchanged.
	if m := os.Getenv("DEEPSEEKCODE_THINKING_MODE"); m != "" {
		a.ThinkingMode = m
	}

	a.checkpoints = newCheckpointIndex()

	// loopDetection is a method (it reads a.loopFloor), so StopWhen is wired
	// after the literal rather than inside it.
	a.StopWhen = []StopCondition{
		MaxSteps(50),
		a.loopDetection(5, 3),
	}

	a.bus = NewBus()
	a.epochMgr.SetBus(a.bus)
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

// ToolIsReadOnly reports whether the named tool declares itself read-only via
// the tools.ReadOnlyHint interface. Returns false for unknown tools.
func (a *Agent) ToolIsReadOnly(name string) bool {
	if t, ok := a.Tools.Get(name); ok {
		if ro, ok := t.(tools.ReadOnlyHint); ok {
			return ro.IsReadOnly()
		}
	}
	return false
}

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

// RequestStop marks the current run as explicitly stopped by the user, so a
// subsequent context cancellation is reported as StopUserRequested rather than
// the ambient StopContextCancel. Callers invoke it immediately before
// cancelling the run's context (e.g. the TUI's ctrl+c handler). Safe to call
// from a goroutine other than the one driving Run.
func (a *Agent) RequestStop() {
	a.stopRequested.Store(true)
}

// RecordCheckpoint implements tools.CheckpointRecorder. It associates name
// with the current step count so /branch and --resume-at can find it.
func (a *Agent) RecordCheckpoint(name string) int {
	step := len(a.steps)
	a.checkpoints.Record(name, step)
	return step
}

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
	// Clear any stop request from a previous run so a stale ctrl+c can't
	// mislabel this run's termination.
	a.stopRequested.Store(false)
	// Each Run gets its own single loop-break nudge and a fresh detection floor.
	a.loopNudged = false
	a.loopFloor = 0
	a.checkpoints = newCheckpointIndex()

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

	// Intent-clarify gate (off by default via AutoClarify): if the fresh prompt
	// is too vague to act on, ask one clarifying question and yield the turn
	// rather than spending a (possibly pro/max) model call on a guess. Pre-loop,
	// so no model request is issued. shouldClarify returns false unless
	// AutoClarify is enabled, so default behavior is unchanged.
	if userPrompt != "" && a.shouldClarify(userPrompt) {
		_, questions := routing.NeedsClarification(userPrompt)
		q := "Before I start: " + questions[0]
		blocks := []llm.ContentBlock{llm.TextBlock{Text: q}}
		a.bus.Publish(EventTextDelta{Text: q})
		a.Messages = append(a.Messages, llm.Message{Role: "assistant", Blocks: blocks})
		if a.Persister != nil {
			_, _ = a.Persister.AppendAssistant(ctx, blocks, a.Model, llm.Usage{})
		}
		return StopModelDone, nil
	}

	// overflowRetried gates the context-overflow recovery to a single attempt
	// per fresh overflow: a 400 "context too long" triggers one compaction and
	// re-attempt, and is reset after any step that completes, so a later
	// overflow in a long run gets its own recovery rather than failing hard.
	overflowRetried := false

agentLoop:
	for {
		select {
		case <-ctx.Done():
			if a.stopRequested.Load() {
				return StopUserRequested, ctx.Err()
			}
			return StopContextCancel, ctx.Err()
		default:
		}

		// Per-step deadline covers BOTH the model turn and tool execution.
		// stepCancel is always non-nil so we can defer it unconditionally.
		stepCtx, stepCancel := stepContext(ctx, a.StepTimeout)

		// Transcript boundary this step starts from (before its model turn
		// appends anything), recorded on the step for /undo reconcile (T3.5).
		preStepMsgCount := len(a.Messages)
		step, err := a.runStep(stepCtx)
		if err != nil {
			stepCancel()
			// Context overflow can never succeed on a blind retry — the only
			// recovery is to shrink the conversation. Compact once (deterministically,
			// so the summarizer call cannot itself overflow) and re-attempt the
			// step. This is distinct from the transient mid-stream re-issue inside
			// runStep: that re-sends the same request, this changes it.
			if llm.IsContextOverflow(err) && !overflowRetried {
				overflowRetried = true
				a.bus.Publish(EventInfo{Text: "context overflow; compacting and re-attempting turn"})
				a.compactForOverflow(ctx)
				continue
			}
			switch {
			case errors.Is(err, context.Canceled):
				// Parent context cancelled mid-step. Distinguish an explicit
				// user stop from an ambient cancellation for honest analytics.
				if a.stopRequested.Load() {
					return StopUserRequested, err
				}
				return StopContextCancel, err
			case a.StepTimeout > 0 && errors.Is(err, context.DeadlineExceeded):
				// The per-step deadline fired. This is a non-success exit, not
				// a clean finish: surface it as StopStepTimeout with a non-nil
				// error so callers and the TUI never render it as a done row
				// (the old path returned StopUnknown,nil — a silent success).
				return StopStepTimeout, fmt.Errorf("step timed out after %s", a.StepTimeout)
			default:
				return StopUnknown, err
			}
		}
		// The step completed; allow a future overflow its own recovery attempt.
		overflowRetried = false
		step.MessageCount = preStepMsgCount
		a.steps = append(a.steps, step)

		// finish-reason override: even if the model said finish_reason=stop,
		// if it emitted tool calls we keep looping. See docs/design.md §6.4.
		hasTools := len(step.ToolCalls) > 0

		// Run user-provided stop conditions BEFORE the natural loop exit
		// check so loop-detection can fire on tool-emitting steps too.
		for _, sc := range a.StopWhen {
			if stop, reason := sc(a.steps); stop {
				// First loop detection in this Run: give the model exactly one
				// corrective nudge to change approach before hard-stopping. The
				// offending assistant tool_calls are already in a.Messages but
				// have no results yet (this check runs before runToolCalls), so
				// the nudge synthesizes a result per dangling call (keeping the
				// tool_call/tool_result pairing valid) and then appends a single
				// user message. loopFloor is raised so the repeats that tripped
				// the detector are forgiven for the recovery turn. The nudge text
				// rides the message tail only — it never enters the system prompt
				// or any tool schema, so the cache-stable prefix is untouched.
				if reason == StopLoopDetected && !a.loopNudged {
					a.loopNudged = true
					a.loopFloor = len(a.steps)
					a.injectLoopBreakNudge(step.ToolCalls)
					a.bus.Publish(EventInfo{Text: "loop detected; nudging the model to change approach (one chance before hard stop)"})
					a.bus.Publish(EventStepFinish{Reason: StopUnknown, Usage: step.Usage, Model: step.Model})
					stepCancel()
					continue agentLoop
				}
				stepCancel()
				a.bus.Publish(EventStepFinish{Reason: reason, Usage: step.Usage, Model: step.Model})
				return reason, nil
			}
		}

		if !hasTools {
			// Promote StopModelDone to StopVerifiedDone when a verify hook is
			// configured and the final check passes. On failure, inject the
			// feedback into a.Messages before returning so the caller (or a
			// re-entered loop) has context about why the tree is dirty.
			reason := StopModelDone
			if a.Verify != nil && a.Verify.Cmd != "" {
				if fb, ok := a.Verify.Run(stepCtx); ok {
					reason = StopVerifiedDone
				} else {
					msg := []llm.ContentBlock{llm.TextBlock{Text: fb}}
					a.Messages = append(a.Messages, llm.Message{
						Role:   "user",
						Blocks: msg,
					})
					if a.Persister != nil {
						_, _ = a.Persister.AppendUserMessage(stepCtx, msg)
					}
				}
			}
			stepCancel()
			a.bus.Publish(EventStepFinish{Reason: reason, Usage: step.Usage, Model: step.Model})
			return reason, nil
		}

		// Tool execution shares the per-step deadline so a stuck tool
		// can't run forever. runToolCalls never fails the loop: a tool
		// error (infra or model-facing) is serialized into a tool-result
		// block for the model to react to, so there is no abort branch here.
		a.runToolCalls(stepCtx, step.ToolCalls)
		a.maybeFeedPostEditDiagnostics(stepCtx, step.ToolCalls)
		a.maybeRunVerifyHook(stepCtx, step.ToolCalls)
		stepCancel()
		a.bus.Publish(EventStepFinish{Reason: StopUnknown, Usage: step.Usage, Model: step.Model})

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

// Compact forces an immediate compaction (honoring the preserve count) and
// reports whether a compaction was performed. It is the exported companion to
// ForceCompact, intended for CLI paths such as `dsc --compact` and for
// testing: callers that need to distinguish "nothing to compact" from a real
// compaction can inspect the bool; errors surface as a non-nil second return.
//
// Internally it borrows ForceCompact's threshold-lowering trick, but wraps a
// snapshot of the message-list length so it can report whether the list
// actually shrank.
func (a *Agent) Compact(ctx context.Context) (compacted bool, err error) {
	before := len(a.Messages)
	// Catch any panic from the internal path and convert to an error so
	// CLI callers get a clean failure rather than a crash.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("compaction panic: %v", r)
		}
	}()
	a.ForceCompact(ctx)
	return len(a.Messages) != before, nil
}

// compactForOverflow runs a forced compaction for context-overflow recovery.
// It pins the deterministic path for the duration of the call: a semantic
// (LLM) summary would itself send much of the already-too-large conversation
// and could overflow again, so overflow recovery must never depend on a model
// call. The original semantic setting is restored on return.
func (a *Agent) compactForOverflow(ctx context.Context) {
	savedSemantic := a.DisableSemanticCompaction
	a.DisableSemanticCompaction = true
	defer func() { a.DisableSemanticCompaction = savedSemantic }()
	a.ForceCompact(ctx)
}

// emaAlpha weights the latest observed ratio when updating the running
// chars-per-token EMA. 0.3 lets the estimate track a shift between CJK-heavy
// and code-heavy phases of a session without a single huge paste dominating it.
const emaAlpha = 0.3

// calibrateCharsPerToken folds one turn's authoritative usage.PromptTokens into
// the per-session chars-per-token ratio (T4.1), correlating it against the
// character count of the exact request that produced it. A non-positive
// PromptTokens (no usage frame — a partial/failed turn) or a zero char count
// leaves the prior ratio intact, so a corrupt frame never poisons the estimate.
// The observed ratio is clamped to a sane band before being folded in via an
// EMA; the first frame seeds the EMA directly, and until then char/4
// (charsPerToken==0) remains the cold-start prior.
func (a *Agent) calibrateCharsPerToken(promptMessages []llm.Message, usage llm.Usage) {
	if usage.PromptTokens <= 0 {
		return
	}
	chars, _ := estimateChars(promptMessages)
	if chars <= 0 {
		return
	}
	observed := float64(chars) / float64(usage.PromptTokens)
	// Clamp to reject corrupt frames. DeepSeek V4 sits ~1.0-1.7 on pure CJK,
	// ~3-3.5 on code/JSON, ~4 on ASCII prose; whitespace-heavy pastes can run
	// higher. [1.0, 12.0] admits every legitimate mix and rejects garbage.
	if observed < 1.0 {
		observed = 1.0
	} else if observed > 12.0 {
		observed = 12.0
	}
	if a.charsPerToken <= 0 {
		a.charsPerToken = observed // seed the EMA with the first usable frame
		return
	}
	a.charsPerToken = emaAlpha*observed + (1-emaAlpha)*a.charsPerToken
}

// learnStaticResidual records, once, the server-side static-prefix token cost
// (system+tool-schema template) not visible to the local tokenizer:
//
//	residual = max(0, usage.PromptTokens - tokenizer.Count(system) - convTokens)
//
// where convTokens is the tokenizer's CountMessages of the request that produced usage.
// No-op when the tokenizer is unavailable or residual already learned.
func (a *Agent) learnStaticResidual(usagePromptTokens int, sentMessages []llm.Message) {
	if a.staticResidualLearned || usagePromptTokens <= 0 {
		return
	}
	if !tokenizer.Available() {
		return
	}
	sys := tokenizer.Count(a.System)
	conv, _ := tokenizer.CountMessages(sentMessages)
	residual := usagePromptTokens - sys - conv
	if residual < 0 {
		residual = 0
	}
	a.staticPrefixResidual = residual
	a.staticResidualLearned = true
}

// maybeCompact runs the compaction pipeline and, if it produced a
// summary, swaps the in-memory message list and persists the
// collapse. Errors surface via EventInfo so the user sees them
// without crashing the Run.
func (a *Agent) maybeCompact(ctx context.Context) {
	maxCtx := a.MaxContextTokens
	if maxCtx <= 0 {
		maxCtx = MaxContextTokens
	}

	if !a.DisableSemanticCompaction {
		// Check context pressure for semantic compaction thresholds.
		pressure := ContextPressure(a.Messages, maxCtx, a.charsPerToken)
		action := ShouldSemanticCompact(pressure, a.SemanticCfg)

		if action == "warn" {
			a.bus.Publish(EventCompactionWarning{
				Pressure:  pressure,
				Threshold: a.SemanticCfg.WarnThreshold,
			})
		}

		if action == "compact" || action == "protect" {
			systemPrompt := a.System
			toolSpecs := a.Tools.AsLLMToolsFiltered(a.ActiveTiers...)
			// Capture the frozen baseline (what the model has been receiving)
			// as the authoritative "before" prefix, then apply the freeze
			// override that the next request will also use.
			beforeSystem, beforeTools := a.staticSystem(), toolSpecs
			if epoch := a.epochMgr.CurrentEpoch(); epoch != nil && a.epochMgr.IsFrozen() {
				beforeSystem, beforeTools = epoch.FrozenSystem, epoch.FrozenTools
				systemPrompt = epoch.FrozenSystem
				toolSpecs = epoch.FrozenTools
			}
			res := SemanticCompact(ctx, a.Messages, a.Client, systemPrompt, toolSpecs, a.SemanticCfg)
			if res.Summary != "" {
				if a.Persister != nil {
					if _, err := a.Persister.ReplaceWithCompaction(ctx, res.FromIdx, res.ToIdx, res.Summary); err != nil {
						a.bus.Publish(EventInfo{Text: "semantic compaction persistence failed: " + err.Error()})
						return
					}
				}
				a.Messages = append([]llm.Message{res.SummaryMessage}, res.KeptMessages...)
				a.compactionFloor = len(a.steps) // boundaries before here are now stale (T3.5)
				// Folded read_file results are gone from the live transcript, so
				// the model no longer holds those files' contents — force a
				// re-read before any edit (T3.2). nil-safe on both calls.
				a.Tools.FileTracker().Clear()
				a.bus.Publish(EventCompaction{
					FromIdx:      res.FromIdx,
					ToIdx:        res.ToIdx,
					Summary:      res.Summary,
					RemovedCount: res.RemovedCount,
				})
				a.bus.Publish(EventSemanticCompaction{
					FromIdx:                res.FromIdx,
					ToIdx:                  res.ToIdx,
					UsedSemantic:           res.UsedSemantic,
					SummaryCost:            res.SummaryCost,
					FallbackReason:         res.FallbackReason,
					StaticPrefixHashBefore: staticPrefixHash(beforeSystem, beforeTools),
					StaticPrefixHashAfter:  staticPrefixHash(staticPrefixOf(systemPrompt), toolSpecs),
				})
				if res.UsedSemantic && res.SummaryCost > 0 {
					a.BudgetState.SpentCNY += res.SummaryCost
				}
				return
			}
			// SemanticCompact returned empty (too few messages or no compaction needed).
			// Fall through to deterministic path below.
		}
	}

	// Deterministic fallback path. When semantic compaction is live, reconcile
	// the absolute trigger with the ratio-of-max trigger so the two cannot
	// disagree on the firing point (T4.4); when it is disabled, the absolute
	// trigger stands alone (no ratio trigger to reconcile against).
	detCfg := a.CompactionCfg
	if !a.DisableSemanticCompaction {
		detCfg.AutoCompactInputTokens = reconcileCompactThreshold(detCfg.AutoCompactInputTokens, a.SemanticCfg.CompactThreshold, maxCtx)
	}
	res := CompactSession(a.Messages, detCfg, a.charsPerToken)
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
	a.compactionFloor = len(a.steps) // boundaries before here are now stale (T3.5)
	// See the semantic path above: invalidate read stamps on fold (T3.2).
	a.Tools.FileTracker().Clear()
	a.bus.Publish(EventCompaction{
		FromIdx:      res.FromIdx,
		ToIdx:        res.ToIdx,
		Summary:      res.Summary,
		RemovedCount: res.RemovedCount,
	})
	// Emit a MEASURED compaction record on the deterministic path too, so a
	// task that requires compaction evidence (require_compaction_record) sees a
	// real record rather than passing on the absence of one. Deterministic
	// compaction only collapses messages — it never rebuilds the static
	// prefix — so before/after are computed from the same frozen baseline and
	// equal by construction. The benchmark still MEASURES them; a regression
	// that let the live prompt drift away from the frozen baseline would make
	// them diverge and fail the gate.
	before, after := a.compactionPrefixHashes()
	a.bus.Publish(EventSemanticCompaction{
		FromIdx:                res.FromIdx,
		ToIdx:                  res.ToIdx,
		UsedSemantic:           false,
		StaticPrefixHashBefore: before,
		StaticPrefixHashAfter:  after,
	})
}

// compactionPrefixHashes returns the measured static-prefix fingerprints for a
// compaction record: "before" is the frozen baseline the model has been
// receiving; "after" is the prefix the next request will use. When the epoch is
// frozen both come from the freeze override and are equal by construction; the
// benchmark fails the gate if a regression makes them diverge.
func (a *Agent) compactionPrefixHashes() (before, after string) {
	system := a.staticSystem()
	var toolSpecs []llm.Tool
	if a.Tools != nil {
		toolSpecs = a.Tools.AsLLMToolsFiltered(a.ActiveTiers...)
	}
	beforeSystem, beforeTools := system, toolSpecs
	afterSystem, afterTools := system, toolSpecs
	if epoch := a.epochMgr.CurrentEpoch(); epoch != nil && a.epochMgr.IsFrozen() {
		beforeSystem, beforeTools = epoch.FrozenSystem, epoch.FrozenTools
		afterSystem, afterTools = epoch.FrozenSystem, epoch.FrozenTools
	}
	return staticPrefixHash(beforeSystem, beforeTools), staticPrefixHash(afterSystem, afterTools)
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

// staticSystem returns the cache-stable portion of the system prompt
// (everything before the dynamic-context boundary). This is what feeds the
// epoch's system hash; per-turn git/date context after the boundary is
// excluded so it never counts as prefix drift.
func (a *Agent) staticSystem() string {
	return staticPrefixOf(a.System)
}

// staticPrefixOf returns the cache-stable portion of a system prompt
// (everything before the dynamic-context boundary). Operating on an arbitrary
// string lets compaction measure the prefix of whatever prompt it actually
// fed the model, not just a.System.
func staticPrefixOf(system string) string {
	if i := strings.Index(system, prompt.DynamicContextBoundary); i >= 0 {
		return system[:i]
	}
	return system
}

// staticPrefixHash fingerprints the static prefix (system + tools) the same
// way the prefix monitor and epoch hashing do. Compaction emits a before/after
// pair built with this so the benchmark can verify the prefix did not move,
// rather than the agent asserting it with a hardcoded boolean.
func staticPrefixHash(system string, toolSpecs []llm.Tool) string {
	return llm.StaticPrefix{System: system, Tools: toolSpecs}.Fingerprint().CombinedSHA256
}

// StaticPrefixFingerprint returns the combined SHA-256 fingerprint of the
// agent's current static prefix (system prompt + tool schemas). It is the
// same value that feeds the DeepSeek cache key and the prefix-epoch hash, so
// it can be persisted cross-session to detect whether the cache is still warm.
// Returns "" before the first Run (no tools resolved yet) — callers must
// treat an empty string as "unknown / cold".
func (a *Agent) StaticPrefixFingerprint() string {
	var toolSpecs []llm.Tool
	if a.Tools != nil {
		toolSpecs = a.Tools.AsLLMToolsFiltered(a.ActiveTiers...)
	}
	return llm.StaticPrefix{System: a.staticSystem(), Tools: toolSpecs}.Fingerprint().CombinedSHA256
}

// CurrentEpochID returns the current epoch's ID, or "" when no epoch has been
// initialized yet. Used to stamp a subagent's child trace with the parent
// epoch it ran under.
func (a *Agent) CurrentEpochID() string {
	if e := a.epochMgr.CurrentEpoch(); e != nil {
		return e.EpochID
	}
	return ""
}

// currentProfileID returns the active agent-profile name, or "default"
// when no profile has been set. It feeds the epoch's agent_profile_hash.
func (a *Agent) currentProfileID() string {
	if a.Profile != nil && a.Profile.Name != "" {
		return a.Profile.Name
	}
	return "default"
}

// buildEpochComponents snapshots the current prefix-determining state
// (static system, active tool tier, skill directory, MCP schema, profile)
// into EpochComponents. Shared by runStep (initial freeze + drift detection)
// and SwitchProfile (explicit epoch switch) so both compute the same hash
// from the same inputs.
//
// Complex tool schemas (depth > 2) are flattened here so that ToolSpecs,
// FrozenTools, and the wire-format request all carry the same flat schemas.
// The adapter map is stored on a.schemaAdapters for repairToolCalls to
// rehydrate flat arguments back to nested form before execution.
func (a *Agent) buildEpochComponents() EpochComponents {
	rawTools := a.Tools.AsLLMToolsFiltered(a.ActiveTiers...)

	// Flatten complex tool schemas for DeepSeek compatibility.
	// This MUST happen here (not downstream) so that ToolSpecs → FrozenTools
	// → fingerprint → wire all carry the flat schemas. If flattening were
	// applied to req.Tools after epoch freeze, the epoch would overwrite it
	// with the original unflattened tools on every turn.
	var epochTools []llm.Tool
	flattenedTools, adapters, err := repair.BuildSchemaAdapters(rawTools)
	if err == nil && len(adapters) > 0 {
		epochTools = flattenedTools
		a.schemaAdapters = adapters
	} else {
		epochTools = rawTools
		// Reset adapters when no flattening is needed (e.g. after a profile
		// switch that removes the complex-schema tool).
		a.schemaAdapters = make(map[string]repair.SchemaAdapter)
	}

	// §3.4: pad the static system so the prefix ends on a DeepSeek cache-unit
	// boundary. Applied HERE, before the epoch is created/hashed, so the padding
	// is inside the frozen, fingerprinted prefix and stays byte-stable across
	// turns. unit comes from a cacheprobe measurement; 0 (default) = disabled, in
	// which case staticSystem is exactly a.staticSystem() — zero behavior change.
	staticSystem := a.staticSystem()
	if a.cacheUnit > 0 {
		pre := tokenizer.Count(staticSystem)
		staticSystem += cacheunit.PadText(pre, a.cacheUnit, tokenizer.Count)
	}

	return EpochComponents{
		StaticSystem:   staticSystem,
		ToolSpecs:      epochTools,
		Model:          a.Model,
		AgentProfileID: a.currentProfileID(),
		Capability:     a.buildCapabilitySet(),
	}
}

// toolTiersFromStrings maps profile tier names to tools.ToolTier values.
// Unknown names are skipped.
func toolTiersFromStrings(names []string) []tools.ToolTier {
	var out []tools.ToolTier
	for _, n := range names {
		switch strings.ToLower(strings.TrimSpace(n)) {
		case "core":
			out = append(out, tools.TierCore)
		case "profile":
			out = append(out, tools.TierProfile)
		case "lazy":
			out = append(out, tools.TierLazy)
		}
	}
	return out
}

// SwitchProfile makes p the active agent profile. It applies the profile's
// tool tiers and model, then creates a new PrefixEpoch via the epoch
// manager. The first turn of the new epoch is expected to miss cache
// (ExpectedCacheMiss); subsequent same-epoch turns stay cache-stable.
// Returns the new epoch. A no-op when an epoch hasn't been initialized yet
// (the first runStep will pick up the profile when it creates epoch #1).
func (a *Agent) SwitchProfile(p agents.AgentProfile) *PrefixEpoch {
	a.Profile = &p
	if tiers := toolTiersFromStrings(p.ToolTiers); len(tiers) > 0 {
		a.ActiveTiers = tiers
	}
	if p.Model != "" {
		a.Model = p.Model
	}
	if a.epochMgr.CurrentEpoch() == nil {
		// No epoch yet: the initial runStep will fold the profile into
		// epoch #1, so there is nothing to switch.
		return nil
	}
	return a.epochMgr.SwitchEpoch("profile_switch:"+p.Name, a.buildEpochComponents())
}

// runStep streams one model turn and aggregates events into a StepRecord.
// It assembles the assistant message and appends it to a.Messages.
func (a *Agent) runStep(ctx context.Context) (StepRecord, error) {
	a.refreshGitContext(ctx)

	lastUserText := a.lastUserText()
	turnModel, turnThinking, turnEffort := a.routeTurn(lastUserText, 0)
	a.turnsSeen++ // after routeTurn so the first turn reads turnsSeen==0 (adaptive plan turn)

	req := llm.Request{
		Model:           turnModel,
		Messages:        a.fullMessages(),
		Tools:           a.Tools.AsLLMToolsFiltered(a.ActiveTiers...),
		Thinking:        llm.ThinkingEnabled(turnThinking),
		ReasoningEffort: turnEffort,
		UserID:          a.UserID,
		Temperature:     a.Temperature,
		TopP:            a.TopP,
	}

	staticSys := a.staticSystem()
	epochComps := a.buildEpochComponents()

	// Emit telemetry for schema-flattened tools. buildEpochComponents
	// already flattened and stored adapters; this just publishes events
	// for observability. Must happen after buildEpochComponents so that
	// a.schemaAdapters is populated.
	for name := range a.schemaAdapters {
		a.publishRepairEvent(EventRepair{
			Kind:    string(repair.KindSchemaFlattened),
			Tool:    name,
			Message: "schema flattened for DeepSeek compatibility",
		})
	}

	epoch := a.epochMgr.CurrentEpoch()
	if epoch == nil {
		epoch = a.epochMgr.InitEpoch("session_start", epochComps)
		a.bus.Publish(EventEpochCreated{
			EpochID:          epoch.EpochID,
			StaticPrefixHash: epoch.StaticPrefixHash,
			ToolsHash:        epoch.ComponentHashes["tools"],
			Reason:           "session_start",
		})
	}

	if !a.DisablePrefixEpoch {
		if !a.epochMgr.IsFrozen() {
			driftChanges := a.epochMgr.DetectDrift(epochComps)
			for _, ch := range driftChanges {
				a.bus.Publish(EventPendingChange{
					EpochID:     epoch.EpochID,
					Kind:        ch.Kind,
					Description: ch.Description,
				})
			}
			a.epochMgr.FreezeEpoch()
			a.bus.Publish(EventEpochFrozen{EpochID: epoch.EpochID})
		} else {
			driftChanges := a.epochMgr.DetectDrift(epochComps)
			for _, ch := range driftChanges {
				a.bus.Publish(EventPendingChange{
					EpochID:     epoch.EpochID,
					Kind:        ch.Kind,
					Description: ch.Description,
				})
			}
		}

		// CRITICAL-1: When the epoch is frozen, use the frozen snapshot
		// for tools and system prompt. This guarantees cache-stable
		// prefixes even when live tools/system drift between turns.
		if a.epochMgr.IsFrozen() {
			epoch = a.epochMgr.CurrentEpoch()
			req.Tools = epoch.FrozenTools
			req.Messages = a.fullMessagesWithFrozenSystem(epoch.FrozenSystem)
			staticSys = epoch.FrozenSystem
		}
	}

	// Schema-complexity telemetry: emit once per tool per epoch so future
	// schema-flattening decisions are backed by data (evidence only, never
	// mutates the schema or blocks the request).
	if a.schemaComplexEmitted == nil {
		a.schemaComplexEmitted = make(map[string]bool)
	}
	for _, rpt := range repair.AnalyzeToolSchemas(req.Tools, 4, 80) {
		key := epoch.EpochID + "\x00" + rpt.Tool
		if a.schemaComplexEmitted[key] {
			continue
		}
		a.schemaComplexEmitted[key] = true
		a.publishRepairEvent(EventRepair{
			Kind:    string(repair.KindSchemaComplex),
			Tool:    rpt.Tool,
			Message: rpt.Message,
		})
	}

	expectedCacheMiss := a.epochMgr.ExpectedCacheMiss()

	prefixDiff, prefixChanged := a.prefixMon.Check(llm.StaticPrefix{System: staticSys, Tools: req.Tools})
	prefixWhich := prefixDiff.Which()
	if prefixChanged {
		a.bus.Publish(EventInfo{Text: "prefix cache invalidated: " + prefixWhich})
		if a.epochMgr.IsFrozen() {
			a.bus.Publish(EventDriftBlocked{
				EpochID: epoch.EpochID,
				Which:   prefixWhich,
			})
		}
	}

	// Session budget gate: check before model streaming starts. The projection
	// discounts input by the rolling session cache-hit rate (T4.2); at cold
	// start that rate is 0, so the first turn is priced all-miss like before.
	// A model with no pricing table can't be cost-gated — surface that once
	// rather than letting it slip the gate silently.
	if !llm.CostKnown(a.Model) && !a.BudgetState.UnknownModelWarned {
		a.BudgetState.UnknownModelWarned = true
		a.bus.Publish(EventBudget{Kind: BudgetKindUnpriced, Model: a.Model, SpentCNY: a.BudgetState.SpentCNY})
	}
	projectedCNY := ProjectedTurnCostCNY(a.Model, req, a.charsPerToken, a.BudgetState.SessionCacheHitRate(), a.staticPrefixResidual)
	allow, warn := CheckBudget(a.BudgetPolicy, a.BudgetState, projectedCNY)
	if warn {
		a.BudgetState.Warned = true
		a.bus.Publish(EventBudget{Kind: BudgetKindWarning, ProjectedCNY: projectedCNY, SpentCNY: a.BudgetState.SpentCNY, Model: a.Model})
	}
	if !allow {
		a.bus.Publish(EventBudget{Kind: BudgetKindBlocked, ProjectedCNY: projectedCNY, SpentCNY: a.BudgetState.SpentCNY, Model: a.Model})
		return StepRecord{FinishReason: "budget_blocked"}, nil
	}

	// Stream the model turn (with the bounded mid-stream re-issue, T1.4).
	sr, err := a.streamWithReissue(ctx, req)
	if err != nil {
		return StepRecord{}, err
	}

	// Snapshot storm-breaker history before the repair so an escalated-away
	// (discarded) flash turn cannot pollute the suppression state the committed
	// turn is judged against. Only needed when escalation is possible.
	escalating := a.escalationEnabled()
	var stormSnap repair.StormSnapshot
	if escalating {
		stormSnap = a.stormBreaker.Snapshot()
	}

	// Repair pipeline: scavenge, repair args, storm-breaker filter.
	assembledCall, repairErrors := a.repairToolCalls(ctx, sr.reasoning, sr.text, sr.toolCalls, &sr.blocks)

	// respModel is the model that actually produced the turn being committed.
	// Escalation may swap it to the stronger model below; it then drives
	// persistence, cost, and trace attribution so none is mis-recorded.
	respModel := a.Model

	// Model-driven escalation (T2.3): re-issue the SAME turn once on the
	// stronger model when the assistant self-declares out of depth
	// (<<<NEEDS_PRO>>>) or the per-turn repair-error count crosses the
	// threshold. The flash turn is discarded — only the escalated turn is
	// appended / persisted / committed to suppression history — so escalation
	// never duplicates a persisted turn. It composes with the mid-stream
	// re-issue (the escalated turn gets the same stall-resilience) and runs at
	// most once per step. Changing only req.Model keeps the system+tools prefix
	// bytes identical, so the Prefix Fingerprint does not move for the re-issue.
	if escalating {
		if trigger, reason := a.escalationTrigger(sr.text, repairErrors); trigger != "" {
			proReq := req
			proReq.Model = a.EscalationModel

			// Re-gate the budget: the pro re-issue is genuine extra spend on top
			// of the flash turn (which streamed and was billed). Price both the
			// flash turn (now realized) and the pro re-issue (projected all-miss,
			// like the main gate) against the session cap. When it would not fit,
			// skip escalation and commit the flash turn rather than overshoot.
			flashCost := llm.Cost(a.Model, sr.usage)
			gateState := a.BudgetState
			gateState.SpentCNY += flashCost
			allow, _ := CheckBudget(a.BudgetPolicy, gateState, ProjectedTurnCostCNY(a.EscalationModel, proReq, a.charsPerToken, a.BudgetState.SessionCacheHitRate(), a.staticPrefixResidual))
			if !allow {
				a.bus.Publish(EventInfo{Text: "escalation skipped: projected cost would exceed the session budget"})
			} else {
				// Charge the realized flash turn (it is about to be discarded) and
				// rewind its storm-history pollution before the committed pro turn.
				if flashCost > 0 {
					a.BudgetState.SpentCNY += flashCost
				}
				a.stormBreaker.Restore(stormSnap)
				a.bus.Publish(EventEscalated{
					Trigger:   trigger,
					FromModel: a.Model,
					ToModel:   a.EscalationModel,
					Reason:    reason,
				})
				proSR, proErr := a.streamWithReissue(ctx, proReq)
				if proErr != nil {
					return StepRecord{}, proErr
				}
				sr = proSR
				respModel = a.EscalationModel
				assembledCall, _ = a.repairToolCalls(ctx, sr.reasoning, sr.text, sr.toolCalls, &sr.blocks)
			}
		}
	}

	// The wire flatten layer turns Blocks back into DeepSeek's
	// {content, reasoning_content, tool_calls} shape on the next
	// request, so reasoning_content still round-trips and the
	// thinking-mode 400 stays away.
	a.Messages = append(a.Messages, llm.Message{
		Role:   "assistant",
		Blocks: sr.blocks,
	})
	if a.Persister != nil {
		_, _ = a.Persister.AppendAssistant(context.Background(), sr.blocks, respModel, sr.usage)

		if rp, ok := a.Persister.(ReceiptAppender); ok {
			fp := llm.StaticPrefix{System: staticSys, Tools: req.Tools}.Fingerprint()
			payload, _ := json.Marshal(map[string]any{
				"model":                respModel,
				"usage":                sr.usage,
				"prefix_hash":          fp.CombinedSHA256,
				"prefix_system_hash":   fp.SystemSHA256,
				"prefix_tools_hash":    fp.ToolsSHA256,
				"prefix_cache_changed": prefixChanged,
				"prefix_cache_reason":  prefixWhich,
				"epoch_id":             epoch.EpochID,
				"static_prefix_hash":   epoch.StaticPrefixHash,
				"expected_cache_miss":  expectedCacheMiss,
			})
			_, _ = rp.AppendReceipt(context.Background(), session.ReceiptModelFinal, payload)
		}
		if rp, ok := a.Persister.(ReceiptAppender); ok {
			if len(a.epochMgr.PendingChanges()) > 0 {
				for _, pc := range a.epochMgr.PendingChanges() {
					pcpayload, _ := json.Marshal(map[string]any{
						"epoch_id":    epoch.EpochID,
						"kind":        string(pc.Kind),
						"description": pc.Description,
					})
					_, _ = rp.AppendReceipt(context.Background(), session.ReceiptEpoch, pcpayload)
				}
			}
		}
	}

	// Accumulate observed model cost into session budget state. respModel
	// reflects an escalation so the stronger model's pricing is charged.
	if cost := llm.Cost(respModel, sr.usage); cost > 0 {
		a.BudgetState.SpentCNY += cost
	}

	// Fold this committed turn's realized cache hit/miss into the rolling
	// session rate so the next turn's projection discounts by actual cache
	// performance (T4.2). The discarded flash turn of an escalation is not
	// folded — only what was committed, matching how cost is charged.
	a.BudgetState.FoldCacheUsage(sr.usage.PromptCacheHitTokens, sr.usage.PromptCacheMissTokens)

	// Calibrate the chars-per-token ratio from this turn's authoritative
	// prompt-token count (T4.1). req.Messages is the exact model-visible payload
	// (including the frozen system prompt when an epoch is frozen), so its char
	// count correlates with usage.PromptTokens. A zero/absent usage frame leaves
	// the prior ratio untouched.
	a.calibrateCharsPerToken(req.Messages, sr.usage)
	a.learnStaticResidual(sr.usage.PromptTokens, req.Messages)

	return StepRecord{
		FinishReason:      sr.finish,
		Usage:             sr.usage,
		ToolCalls:         assembledCall,
		EpochID:           epoch.EpochID,
		StaticPrefixHash:  epoch.StaticPrefixHash,
		ExpectedCacheMiss: expectedCacheMiss,
		Model:             respModel,
	}, nil
}

// partialBlocks assembles the content blocks captured before a stream broke
// mid-turn: the reasoning channel (if any) followed by the visible text (if
// any), in DeepSeek's logical emission order. Tool calls are deliberately
// excluded — a mid-stream failure means the client never assembled them, and
// emitting a half-formed ToolUseBlock would risk a dangling tool_call on the
// next request. Returns nil when nothing was streamed.
func partialBlocks(reasoning, text string) []llm.ContentBlock {
	var blocks []llm.ContentBlock
	if reasoning != "" {
		blocks = append(blocks, llm.ThinkingBlock{Text: reasoning})
	}
	if text != "" {
		blocks = append(blocks, llm.TextBlock{Text: text})
	}
	return blocks
}

// streamWithReissue streams one model turn and, on a transient mid-stream stall
// (first-token / chunk-stall timeout), re-issues the identical request once
// before giving up (T1.4). The request is sent verbatim, so a re-issue is
// cache-cheap. A parse error or a context cancellation / step deadline is NOT
// re-issued and propagates. On give-up it salvages and persists the partial
// assistant turn (T1.1) — keyed to req.Model so an escalated re-issue's partial
// is attributed correctly — then returns the error. On success it returns the
// assembled streamResult with a nil error; the caller does the repair, append,
// and persistence so only the FINAL (possibly escalated) turn is committed.
func (a *Agent) streamWithReissue(ctx context.Context, req llm.Request) (streamResult, error) {
	const maxStreamReissues = 1
	for attempt := 0; ; attempt++ {
		events, err := a.Client.Stream(ctx, req)
		if err != nil {
			// Connect-phase failure (the client already retried transient
			// 429/5xx). A context-overflow 400 is handled by Run, which
			// compacts and re-attempts; everything else propagates as-is.
			return streamResult{}, err
		}
		sr := a.consumeStream(events)
		if sr.err == nil {
			return sr, nil // clean turn
		}
		if isReissuableStreamErr(sr.err) && attempt < maxStreamReissues {
			a.bus.Publish(EventInfo{Text: fmt.Sprintf("stream stalled (%v); re-issuing turn", sr.err)})
			a.bus.Publish(EventRetry{Attempt: attempt + 1, Max: maxStreamReissues})
			continue
		}
		// Give up. Persist the partial assistant turn (reasoning + visible text
		// accumulated before the final break). EventFinish never fired, so no
		// tool calls were assembled — the partial carries only thinking/text and
		// cannot leave a dangling tool_call; SanitizeForDeepSeek still guards the
		// next request. Nothing is appended when no content arrived (e.g. a
		// first-token timeout on attempt one).
		if partial := partialBlocks(sr.reasoning, sr.text); len(partial) > 0 {
			a.Messages = append(a.Messages, llm.Message{Role: "assistant", Blocks: partial})
			if a.Persister != nil {
				_, _ = a.Persister.AppendAssistant(context.Background(), partial, req.Model, sr.usage)
			}
		}
		return streamResult{}, fmt.Errorf("stream error: %w", sr.err)
	}
}

// streamResult holds everything consumeStream accumulates from one model
// stream attempt: the visible text, the reasoning channel, the assembled
// content blocks and tool calls, the finish reason, and the usage frame. A
// non-nil err means the stream broke before EventFinish (timeout, parse error,
// or context cancellation); the caller decides whether to re-issue, salvage,
// or proceed.
type streamResult struct {
	text      string
	reasoning string
	blocks    []llm.ContentBlock
	toolCalls []llm.ToolCall
	finish    string
	usage     llm.Usage
	err       error
}

// consumeStream drains one model stream, republishing reasoning/text deltas on
// the bus and accumulating the assistant turn into a streamResult. It returns
// when the stream closes (EventFinish) or breaks (EventError). The client
// closes the channel right after EventError, so the range exits on the next
// receive; the error is captured rather than returned early so the caller can
// salvage the partial turn. A reasoning block left open by a mid-stream break
// is closed before returning so the UI never stalls on an unterminated fold.
func (a *Agent) consumeStream(events <-chan llm.Event) streamResult {
	var (
		sr          streamResult
		inReasoning bool
	)
	for ev := range events {
		switch ev.Type {
		case llm.EventTextDelta:
			sr.text += ev.Text
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
			sr.reasoning += ev.Text
			a.bus.Publish(EventReasoningDelta{Text: ev.Text})
		case llm.EventToolUseDelta:
			// tool-call deltas are aggregated by the client; we don't need
			// per-delta tracking here. The EventFinish carries the assembled
			// calls.
		case llm.EventFinish:
			if inReasoning {
				a.bus.Publish(EventReasoningEnd{})
				inReasoning = false
			}
			sr.finish = ev.FinishReason
			sr.usage = ev.Usage
			sr.toolCalls = ev.ToolCalls
			sr.blocks = ev.Blocks
		case llm.EventError:
			sr.err = ev.Err
		}
	}
	if inReasoning {
		a.bus.Publish(EventReasoningEnd{})
	}
	return sr
}

// isReissuableStreamErr reports whether a mid-stream error is a transient stall
// that re-sending the identical request may clear. Only the two stream-timeout
// tiers qualify. A malformed-chunk parse error would likely repeat, and a
// context cancellation or step deadline (context.Canceled /
// context.DeadlineExceeded) must propagate — re-issuing would either ignore the
// user's stop or busy-loop against an already-expired deadline.
func isReissuableStreamErr(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, llm.ErrFirstTokenTimeout) || errors.Is(err, llm.ErrChunkStall)
}

// escalationRepairThreshold is the per-turn count of unrecoverable repair
// reports (invalid args, storm-suppressed calls) that auto-escalates the turn
// to the stronger model, even without an explicit <<<NEEDS_PRO>>> marker.
const escalationRepairThreshold = 3

// Escalation marker delimiters. The model self-declares it is out of depth by
// opening its reply with <<<NEEDS_PRO>>> or <<<NEEDS_PRO: brief reason>>>.
const (
	needsProOpen  = "<<<NEEDS_PRO"
	needsProClose = ">>>"
)

// escalationEnabled reports whether model-driven escalation is active: a target
// model is configured and the loop is not already running on it. The
// already-on-target guard mirrors the Duet's modelFn()=="deepseek-v4-pro"
// self-skip, so a pro main loop never escalates to itself.
func (a *Agent) escalationEnabled() bool {
	return a.EscalationModel != "" && a.Model != a.EscalationModel
}

// escalationTrigger decides whether the just-completed turn should be re-issued
// on the stronger model and why. Marker wins over the repair-count threshold so
// an explicit self-declaration is attributed as such. Returns ("","") when no
// escalation is warranted.
func (a *Agent) escalationTrigger(text string, repairErrors int) (trigger, reason string) {
	if reason, ok := needsProMarker(text); ok {
		if reason == "" {
			reason = "model requested a stronger model"
		}
		return "marker", reason
	}
	if repairErrors >= escalationRepairThreshold {
		return "repair_errors", fmt.Sprintf("%d unrecoverable repair errors this turn", repairErrors)
	}
	return "", ""
}

// needsProMarker reports whether the assistant's visible text opens with the
// escalation marker, returning the optional reason. Only the first non-empty
// line is inspected; the marker must be a complete <<<NEEDS_PRO ... >>> token.
func needsProMarker(text string) (reason string, ok bool) {
	line := firstNonEmptyLine(text)
	if !strings.HasPrefix(line, needsProOpen) {
		return "", false
	}
	rest := line[len(needsProOpen):]
	end := strings.Index(rest, needsProClose)
	if end < 0 {
		return "", false
	}
	// The marker must be the WHOLE first line — nothing after the closing
	// >>> — so a line that merely quotes the token mid-sentence (e.g. echoing
	// a log) does not spuriously escalate. The contract asks for the marker as
	// the entire first line.
	if strings.TrimSpace(rest[end+len(needsProClose):]) != "" {
		return "", false
	}
	mid := strings.TrimSpace(rest[:end])
	mid = strings.TrimSpace(strings.TrimPrefix(mid, ":"))
	return mid, true
}

// firstNonEmptyLine returns the first line of s that is not blank after
// trimming surrounding whitespace, or "" when there is none.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// escalationContract is the system-prompt instruction that teaches the model
// the <<<NEEDS_PRO>>> marker. It is OPT-IN: callers add it to the static system
// prompt only when escalation is configured, so the default DefaultSystemPrompt
// (and thus the committed cache-stable golden) is untouched. When added, the
// MODEL NAME is the only interpolant, so the Prefix Fingerprint stays
// byte-stable across turns for a given escalation target. It must be inserted
// before prompt.DynamicContextBoundary so it lands in the fingerprinted prefix.
func escalationContract(model string) string {
	return "\n\nEscalation: if a task is genuinely beyond your depth — it needs deeper " +
		"reasoning or you are stuck after repeated failed attempts — make the FIRST line of " +
		"your reply exactly `<<<NEEDS_PRO: brief reason>>>` (or `<<<NEEDS_PRO>>>`). The turn " +
		"will be re-issued on " + model + ". Use this sparingly, only when truly warranted."
}

// EnableEscalation turns on model-driven escalation to the given model and adds
// the marker contract to the static system prompt so the model knows to emit
// <<<NEEDS_PRO>>>. Call it BEFORE Run (before epoch #1 freezes) so the contract
// is part of the frozen, fingerprinted prefix; it is inserted just before
// prompt.DynamicContextBoundary (when present) so per-turn dynamic context still
// follows it. Adding the contract deliberately moves the Prefix Fingerprint for
// this session — the model name is the only interpolant, so it stays byte-stable
// across turns — while the default (escalation off) leaves DefaultSystemPrompt
// and the committed cache-stable golden untouched. No-op when model is empty or
// already the active model. NOTE: when a PromptBuilder is set it rebuilds
// a.System each turn, overwriting this injection; such assemblies must add the
// contract through the builder's static section instead.
func (a *Agent) EnableEscalation(model string) {
	if model == "" || model == a.Model {
		return
	}
	a.EscalationModel = model
	// When a PromptBuilder owns the system prompt it rebuilds a.System every
	// turn (Run / refreshGitContext), so injecting the contract here would be
	// silently wiped on the first turn — leaving a live-vs-frozen system byte
	// divergence that llm.PrefixMonitor would flag as drift (EventDriftBlocked).
	// (DetectDrift would not catch it; it compares only the Capability Set, not
	// system bytes.) Such assemblies must add the contract through the builder's
	// static section; we only inject for the direct-System case.
	if a.PromptBuilder != nil {
		return
	}
	contract := escalationContract(model)
	if i := strings.Index(a.System, prompt.DynamicContextBoundary); i >= 0 {
		a.System = a.System[:i] + contract + a.System[i:]
	} else {
		a.System += contract
	}
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

func (a *Agent) fullMessagesWithFrozenSystem(frozenSystem string) []llm.Message {
	if frozenSystem == "" {
		return a.Messages
	}
	fullSystem := frozenSystem
	if a.PromptBuilder != nil && a.PromptBuilder.Project != nil {
		fullSystem += prompt.DynamicContextBoundary + prompt.RenderProjectContext(*a.PromptBuilder.Project)
	}
	out := make([]llm.Message, 0, len(a.Messages)+1)
	out = append(out, llm.Message{
		Role:   "system",
		Blocks: []llm.ContentBlock{llm.TextBlock{Text: fullSystem}},
	})
	out = append(out, a.Messages...)
	return out
}

// loopBreakToolResultText and loopBreakNudgeText are the message-tail strings
// the one-shot loop-break nudge injects. They live ONLY in tool-result and
// user messages on the conversation tail — never in DefaultSystemPrompt or any
// tool schema — so they cannot move the Prefix Fingerprint or invalidate the
// prompt cache.
const (
	loopBreakToolResultText = "Stopped: this tool call has repeated several times without making progress (possible loop)."
	loopBreakNudgeText      = "You appear to be repeating the same action without making progress. Briefly summarize what you were trying to accomplish and why it isn't working, then try a different approach — do not repeat the previous tool call."
)

// injectLoopBreakNudge appends, to the conversation tail, one synthetic error
// tool-result for each still-dangling tool call from the offending step,
// followed by a single user-role nudge asking the model to change approach.
// Loop detection fires before runToolCalls has produced any results, so the
// offending assistant tool_calls sit in a.Messages unpaired; synthesizing a
// result per call.ID is what keeps the next request's tool_call/tool_result
// pairing valid for DeepSeek. Both the results and the nudge are persisted so
// a Replay reconstructs the same paired transcript.
func (a *Agent) injectLoopBreakNudge(calls []llm.ToolCall) {
	for _, call := range calls {
		a.Messages = append(a.Messages, llm.Message{
			Role:   "tool",
			Blocks: []llm.ContentBlock{llm.ToolResultBlock{ToolUseID: call.ID, Content: loopBreakToolResultText, IsError: true}},
		})
		if a.Persister != nil {
			_, _ = a.Persister.AppendToolResult(context.Background(), call.ID, loopBreakToolResultText, true)
		}
	}
	nudge := []llm.ContentBlock{llm.TextBlock{Text: loopBreakNudgeText}}
	a.Messages = append(a.Messages, llm.Message{Role: "user", Blocks: nudge})
	if a.Persister != nil {
		_, _ = a.Persister.AppendUserMessage(context.Background(), nudge)
	}
}

// maybeFeedPostEditDiagnostics collects affected file paths from mutating
// tool calls in the just-completed step, calls PostEditDiagnostics if set,
// and appends a synthetic user message when the callback returns non-empty.
// Read-only tools are skipped. Paths are deduplicated.
//
// LSP timing note: this reads only diagnostics already cached by the LSP
// client at the time of the call. It does not wait for a fresh
// publishDiagnostics notification from the server. If the LSP server
// reports diagnostics slightly after the file write, they will be visible
// through the normal lsp tool or a later edit cycle. This is a deliberate
// no-wait design to avoid adding latency to every mutating step.
func (a *Agent) maybeFeedPostEditDiagnostics(ctx context.Context, calls []llm.ToolCall) {
	if a.PostEditDiagnostics == nil {
		return
	}

	// Collect affected paths from mutating tools only, reusing
	// AffectedPathsFor so the path logic stays in sync with the
	// snapshot mechanism.
	seen := make(map[string]bool)
	var paths []string
	for _, call := range calls {
		tool, ok := a.Tools.Get(call.Function.Name)
		if !ok {
			continue
		}
		// Skip read-only tools.
		if ro, ok := tool.(tools.ReadOnlyHint); ok && ro.IsReadOnly() {
			continue
		}
		for _, p := range AffectedPathsFor(a.Tools, call) {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	if len(paths) == 0 {
		return
	}

	text := a.PostEditDiagnostics(ctx, paths)
	if text == "" {
		return
	}

	msg := []llm.ContentBlock{llm.TextBlock{Text: text}}
	a.Messages = append(a.Messages, llm.Message{Role: "user", Blocks: msg})
	if a.Persister != nil {
		_, _ = a.Persister.AppendUserMessage(ctx, msg)
	}
}

// maybeRunVerifyHook runs the configured VerifyHook after steps that include
// mutating tool calls. When the hook reports a failure, the feedback is
// injected as a synthetic user message so the model can fix reported errors on
// its next turn. No-op when Verify is nil, Cmd is empty, or the step had no
// mutating tool calls.
func (a *Agent) maybeRunVerifyHook(ctx context.Context, calls []llm.ToolCall) {
	if a.Verify == nil || a.Verify.Cmd == "" {
		return
	}
	// Only run after steps that include at least one mutating tool call.
	hasMutating := false
	for _, call := range calls {
		tool, ok := a.Tools.Get(call.Function.Name)
		if !ok {
			continue
		}
		if ro, ok := tool.(tools.ReadOnlyHint); ok && ro.IsReadOnly() {
			continue
		}
		hasMutating = true
		break
	}
	if !hasMutating {
		return
	}

	feedback, passed := a.Verify.Run(ctx)
	if passed {
		return
	}

	msg := []llm.ContentBlock{llm.TextBlock{Text: feedback}}
	a.Messages = append(a.Messages, llm.Message{Role: "user", Blocks: msg})
	if a.Persister != nil {
		_, _ = a.Persister.AppendUserMessage(ctx, msg)
	}
}

// runToolCalls dispatches all tool calls from one step, respecting
// permissions and the Duet validator. Calls without dependencies (i.e.
// all of them — we don't model deps) run in parallel.
//
// Snapshots are taken serially before the parallel execution kicks off
// so a /undo can revert all files touched by one step in a single
// operation.
func (a *Agent) runToolCalls(ctx context.Context, calls []llm.ToolCall) {
	// Pre-tool snapshot: union of all statically-affected paths.
	if a.Persister != nil {
		var paths []string
		for _, call := range calls {
			paths = append(paths, AffectedPathsFor(a.Tools, call)...)
		}
		if len(paths) > 0 {
			_, _ = a.Persister.TakeSnapshot(len(a.steps), paths)
			// Mark the current step (already appended) as snapshotted so /undo
			// reconcile can count snapshots, not steps (T3.5).
			if len(a.steps) > 0 {
				a.steps[len(a.steps)-1].Snapshotted = true
			}
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
		// Persist permission decision receipt for automatic deny.
		if a.Persister != nil {
			if rp, ok := a.Persister.(ReceiptAppender); ok {
				payload, _ := json.Marshal(map[string]any{
					"tool":     tool.Name(),
					"decision": "deny",
					"reason":   reason,
				})
				_, _ = rp.AppendReceipt(ctx, session.ReceiptPermission, payload)
			}
		}
		if strings.HasPrefix(reason, "matched deny rule:") {
			a.bus.Publish(EventPermissionDenied{Tool: tool.Name(), Reason: reason, ByRule: true})
			return tools.Errf("denied by rule: %s", reason), nil
		}
		a.bus.Publish(EventPermissionDenied{Tool: tool.Name(), Reason: reason, ByRule: false})
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

		// Persist permission decision receipt with final user response.
		if a.Persister != nil {
			if rp, ok := a.Persister.(ReceiptAppender); ok {
				decision := "allow"
				if !resp.Allow {
					decision = "deny"
				}
				payload, _ := json.Marshal(map[string]any{
					"tool":            tool.Name(),
					"policy_decision": dec.String(),
					"decision":        decision,
					"persist_pattern": resp.PersistPattern,
				})
				_, _ = rp.AppendReceipt(ctx, session.ReceiptPermission, payload)
			}
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
	default:
		// Automatic Allow — persist receipt.
		if a.Persister != nil {
			if rp, ok := a.Persister.(ReceiptAppender); ok {
				payload, _ := json.Marshal(map[string]any{
					"tool":     tool.Name(),
					"decision": "allow",
					"reason":   reason,
				})
				_, _ = rp.AppendReceipt(ctx, session.ReceiptPermission, payload)
			}
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

// effectiveReasoningEffort returns the effort to send on the wire.
// When thinking is disabled, returns empty (field omitted). When
// thinking is enabled, returns the configured effort if valid,
// otherwise defaults to max.
func (a *Agent) effectiveReasoningEffort(thinking bool) llm.ReasoningEffort {
	if !thinking {
		return ""
	}
	if a.ReasoningEffort.Valid() {
		return a.ReasoningEffort
	}
	return llm.ReasoningEffortMax
}

// selectTurnThinking resolves whether to think this turn. ThinkingMode (set via
// DEEPSEEKCODE_THINKING_MODE for cost A/Bs) overrides the legacy path:
//   "off"      -> never think
//   "adaptive" -> think on the first turn (plan) or a repair turn, else terse
//   "on"/""    -> legacy: AutoReasoning ? SelectThinking : Thinking
//
// Thinking on/off changes only the wire reasoning_effort field (and whether the
// model emits reasoning_content); it does NOT touch the Static Prefix, so it
// cannot cause prefix drift.
func (a *Agent) selectTurnThinking(userText string, repairErrorsLastTurn int) bool {
	switch a.ThinkingMode {
	case "off":
		return false
	case "adaptive":
		return a.turnsSeen == 0 || repairErrorsLastTurn > 0
	default:
		if a.AutoReasoning {
			return llm.SelectThinking(a.IsSubagent, userText, a.Thinking)
		}
		return a.Thinking
	}
}

// routeTurn returns the per-turn (model, thinking, effort) to use. When
// AutoRoute is off (or no escalation target) it returns the loop defaults
// unchanged. It updates a.lastRoute for stickiness. Model/effort never affect
// the Static Prefix, so this cannot cause prefix drift.
func (a *Agent) routeTurn(userText string, repairErrorsLastTurn int) (string, bool, llm.ReasoningEffort) {
	if !a.AutoRoute || a.EscalationModel == "" {
		thinking := a.selectTurnThinking(userText, repairErrorsLastTurn)
		return a.Model, thinking, a.effectiveReasoningEffort(thinking)
	}
	d := routing.Classify(
		routing.Signals{UserText: userText, RepairErrorsLastTurn: repairErrorsLastTurn},
		routing.Config{FlashModel: a.Model, ProModel: a.EscalationModel, StickyTurns: 2},
		a.lastRoute,
	)
	a.lastRoute = d
	effort, _ := llm.ParseReasoningEffort(d.Effort) // "" stays invalid → field omitted
	return d.Model, d.Thinking, effort
}

// shouldClarify reports whether the agent should ask a clarifying question
// before acting on userText. Returns false when AutoClarify is disabled
// (the default) so legacy behavior is unchanged.
func (a *Agent) shouldClarify(userText string) bool {
	if !a.AutoClarify {
		return false
	}
	need, _ := routing.NeedsClarification(userText)
	return need
}

// publishRepairEvent publishes an EventRepair and persists a repair receipt
// if the persister supports ReceiptAppender.
func (a *Agent) publishRepairEvent(ev EventRepair) {
	a.bus.Publish(ev)
	if a.Persister != nil {
		if rp, ok := a.Persister.(ReceiptAppender); ok {
			payload, _ := json.Marshal(ev)
			_, _ = rp.AppendReceipt(context.Background(), session.ReceiptRepair, payload)
		}
	}
}

// repairToolCalls runs the repair pipeline on tool calls after streaming.
// It scavenges hidden calls, repairs args, and applies storm-breaker filtering.
// Returns the kept calls, updates blocks in-place to reflect repaired/kept
// calls, and reports the per-turn count of UNRECOVERABLE repair signals
// (arguments that stayed invalid after repair + storm-suppressed calls). That
// count drives the T2.3 auto-escalation threshold; KindArgsCompleted (a
// successful repair) is deliberately not counted — only failures escalate.
func (a *Agent) repairToolCalls(ctx context.Context, reasoning, content string, declared []llm.ToolCall, blocks *[]llm.ContentBlock) ([]llm.ToolCall, int) {
	repairErrors := 0
	// Build allowed tool-name map from the registry
	allowed := make(map[string]struct{})
	for _, t := range a.Tools.AsLLMToolsFiltered(a.ActiveTiers...) {
		allowed[t.Function.Name] = struct{}{}
	}

	// Step 1: Scavenge hidden tool calls from reasoning and content
	scavengeResult := repair.ScavengeToolCalls(reasoning, content, allowed, repair.ScavengeOptions{})
	for _, r := range scavengeResult.Reports {
		a.publishRepairEvent(EventRepair{
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

		// When truncation is inside a string, the repair is unsafe —
		// we'd be inventing string content the model never finished.
		// Drop the call and ask the model to re-emit complete arguments.
		if repairResult.NeedMore {
			repairErrors++
			a.publishRepairEvent(EventRepair{
				Kind:    string(repair.KindArgsNeedMore),
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "arguments truncated inside string — requesting continuation",
			})
			continue
		}

		if repairResult.Changed {
			call.Function.Arguments = repairResult.Repaired
			a.publishRepairEvent(EventRepair{
				Kind:       string(repair.KindArgsCompleted),
				Tool:       call.Function.Name,
				CallID:     call.ID,
				Message:    "arguments repaired",
				BeforeHash: repair.HashArgs(repairResult.Raw),
				AfterHash:  repair.HashArgs(repairResult.Repaired),
			})
		}
		if !repairResult.Valid {
			repairErrors++
			a.publishRepairEvent(EventRepair{
				Kind:    string(repair.KindArgsInvalid),
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "arguments invalid after repair",
			})
		}
		repairedCalls = append(repairedCalls, call)
	}

	// Step 3b: Rehydrate flattened arguments back to nested form.
	// When BuildSchemaAdapters flattened complex schemas for the model,
	// the model returns flat arguments (e.g. "target__path" → "target.path").
	// We must reconstruct the nested structure before execution.
	// Only emit a repair event when rehydration actually changed the args
	// (i.e. flat keys were found and nested). When the model returns nested
	// args (no flat keys to rehydrate), Rehydrate is a no-op passthrough —
	// no event, no re-serialization.
	for i, call := range repairedCalls {
		adapter, hasAdapter := a.schemaAdapters[call.Function.Name]
		if !hasAdapter || len(adapter.FieldMap) == 0 {
			continue
		}
		rehydrated, err := adapter.Rehydrate(json.RawMessage(call.Function.Arguments))
		if err != nil {
			// Rehydration failed — log and skip (keep flat args, which
			// will likely fail validation, but don't silently drop).
			a.publishRepairEvent(EventRepair{
				Kind:    string(repair.KindArgsInvalid),
				Tool:    call.Function.Name,
				CallID:  call.ID,
				Message: "schema rehydration failed: " + err.Error(),
			})
			continue
		}
		before := call.Function.Arguments
		after := string(rehydrated)
		if before == after {
			// Rehydration was a no-op (no flat keys found in args).
			// Don't emit a misleading event or re-serialize.
			continue
		}
		repairedCalls[i].Function.Arguments = after
		a.publishRepairEvent(EventRepair{
			Kind:       string(repair.KindArgsCompleted),
			Tool:       call.Function.Name,
			CallID:     call.ID,
			Message:    "arguments rehydrated from flattened schema",
			BeforeHash: repair.HashArgs(before),
			AfterHash:  repair.HashArgs(after),
		})
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
		if r.Kind == repair.KindSuppressed {
			repairErrors++
		}
		a.publishRepairEvent(EventRepair{
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

	return filtered.Calls, repairErrors
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
