// Package taubench — the dsc arm.
//
// runDSCArm drives ONE tau-bench task through dsc's REAL in-process agent loop
// with AutoRoute enabled (Model=deepseek-v4-flash, EscalationModel=deepseek-v4-pro),
// exactly as the differential cost-per-solved-task design (plan 4) requires: a
// capability pass-rate can't separate two harnesses that run the same model, so
// the dsc arm must exercise the routing + cache machinery, not a stub.
//
// The per-turn loop is the Go port of runner.ts runAgentLoop:
//
//	while turns < maxTurns:
//	  userMsg := sim.Next(transcript); if nil -> break
//	  push user; a.Run(ctx, userMsg); push tool events (+toolCalls); push agent
//	  turns++
//	truncated = (turns == maxTurns)
//	pass = task.Check({db, finalAgentMessage, transcript})
//
// Token/cost accounting is dsc-side (plan 4 §"dsc-side API facts"): we sum every
// EventStepFinish.Usage off a.Events() across the whole run, then derive costUsd
// via llm.Cost and cacheHitRatio via llm.CacheHitRate over that aggregate.
//
// Live API is gated: callers building the offline unit/smoke path inject a
// scripted simClient and a mock agentClient (internal/llmtest), so no test or
// `go build` touches the network. RunDSCArmLive is the only path that dials the
// real DeepSeek endpoint, and it is reached solely from the -live CLI flow.
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/permissions"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// Model identifiers for the dsc arm. AutoRoute starts on flash and escalates to
// pro per the plan-1 routing wiring.
const (
	dscFlashModel = "deepseek-v4-flash"
	dscProModel   = "deepseek-v4-pro"
)

// cnyPerUsd converts ¥ (the unit llm.Cost returns) to USD for the RunResult.
// The Reasonix harness reports costUsd, so the dsc arm matches that unit. Held
// at a single constant so both arms share one conversion.
const cnyPerUsd = 7.2

// Claude Sonnet reference pricing in USD per 1M tokens, mirroring the Reasonix
// CLAUDE_SONNET_PRICING constant (telemetry/stats.ts). claudeEquivalentCost
// values prompt tokens at input rate and completion tokens at output rate with
// no cache discount — the synthetic "what would Claude have cost" gauge the
// report surfaces alongside the real DeepSeek costUsd.
const (
	claudeInputUsdPerMTok  = 3.0
	claudeOutputUsdPerMTok = 15.0
)

// claudeEquivalentUsd prices one aggregate usage record at Claude Sonnet rates
// in USD. It is linear in prompt/completion tokens, so summing per-turn (as the
// TS reference does) and pricing the aggregate once yield the same figure.
func claudeEquivalentUsd(u llm.Usage) float64 {
	const million = 1_000_000.0
	return (float64(u.PromptTokens)*claudeInputUsdPerMTok +
		float64(u.CompletionTokens)*claudeOutputUsdPerMTok) / million
}

// RunResult is the per-run record. Field names / JSON tags are byte-compatible
// with the Reasonix types.ts RunResult so both arms aggregate into one report.
type RunResult struct {
	TaskID              string  `json:"taskId"`
	Mode                string  `json:"mode"`
	Pass                bool    `json:"pass"`
	Turns               int     `json:"turns"`
	ToolCalls           int     `json:"toolCalls"`
	CacheHitRatio       float64 `json:"cacheHitRatio"`
	CostUsd             float64 `json:"costUsd"`
	ClaudeEquivalentUsd float64 `json:"claudeEquivalentUsd"`
	PromptTokens        int     `json:"promptTokens"`
	CompletionTokens    int     `json:"completionTokens"`
	Truncated           bool    `json:"truncated"`
	FinalAgentMessage   string  `json:"finalAgentMessage"`
	ErrorMessage        string  `json:"errorMessage,omitempty"`
}

// dscArmConfig is the dependency-injected input to runDSCArm. The agentClient
// drives dsc's in-process loop; the simClient drives the user simulator. Both
// are injected so the offline test can supply mocks and the live path can
// supply real DeepSeek clients.
type dscArmConfig struct {
	agentClient *llm.Client
	simClient   chatClient
	task        TaskDef
	cwd         string // working directory for the yolo permissions policy
}

// runDSCArm runs one task through the in-process dsc agent loop and returns a
// RunResult. It never dials the network itself — networking lives entirely in
// the injected clients.
func runDSCArm(ctx context.Context, cfg dscArmConfig) (RunResult, error) {
	// Fresh world per run so mutations never leak across tasks/repeats.
	db := cloneDb(retailSeed())

	// Register the 6 retail tools bound to this run's world.
	reg := tools.New()
	for _, t := range newRetailTools(&db) {
		reg.RegisterWithTier(t, tools.TierCore)
	}

	// Yolo policy so retail tool calls are auto-approved (no human gate in a
	// benchmark loop).
	pol := permissions.New(permissions.ModeYolo, cfg.cwd, nil, nil, nil)

	// Build the agent on flash with AutoRoute + pro escalation — the routing
	// machinery this benchmark is built to exercise.
	a := agent.New(cfg.agentClient, reg, pol, dscFlashModel)
	a.System = cfg.task.SystemPrompt
	a.AutoRoute = true
	a.EscalationModel = dscProModel

	// Drain the agent event stream concurrently, summing EventStepFinish.Usage
	// and counting executed tool calls across the whole run. a.Events() is a
	// shared, never-closed channel fed by an async bus→compat goroutine, so
	// trailing events (the final turn's EventStepFinish, then its EventDone)
	// arrive AFTER a.Run returns. We therefore drain until we have seen one
	// EventDone per completed Run — the strict per-Run terminator the agent
	// guarantees to emit last — rather than racing a bare stop signal, which
	// would drop the last turn's usage.
	usageCh := make(chan dscUsageTally, 1)
	wantDone := make(chan int, 1)
	go func() {
		var tally dscUsageTally
		var doneSeen, doneTarget int
		targetKnown := false
		events := a.Events()
		for {
			select {
			case n := <-wantDone:
				doneTarget = n
				targetKnown = true
				if targetKnown && doneSeen >= doneTarget {
					usageCh <- tally
					return
				}
			case ev := <-events:
				switch e := ev.(type) {
				case agent.EventStepFinish:
					tally.usage.PromptTokens += e.Usage.PromptTokens
					tally.usage.CompletionTokens += e.Usage.CompletionTokens
					tally.usage.TotalTokens += e.Usage.TotalTokens
					tally.usage.PromptCacheHitTokens += e.Usage.PromptCacheHitTokens
					tally.usage.PromptCacheMissTokens += e.Usage.PromptCacheMissTokens
					// Price this step at the model that actually served it
					// (e.Model is the post-escalation model when AutoRoute
					// promoted the turn to pro), not a flat flash rate.
					tally.costCNY += llm.Cost(e.Model, e.Usage)
				case agent.EventToolCallResult:
					tally.toolCalls++
				case agent.EventDone:
					doneSeen++
					if targetKnown && doneSeen >= doneTarget {
						usageCh <- tally
						return
					}
				}
			}
		}
	}()

	sim := NewUserSimulator(cfg.simClient, cfg.task.Persona)

	maxTurns := cfg.task.MaxTurns
	if maxTurns == 0 {
		maxTurns = 8
	}

	var (
		transcript   []Turn
		turns        int
		runsStarted  int
		lastAgentMsg string
		truncated    bool
		errMessage   string
	)

	for turns < maxTurns {
		userMsg, err := sim.Next(ctx, transcript)
		if err != nil {
			errMessage = err.Error()
			break
		}
		if userMsg == nil {
			break
		}
		transcript = append(transcript, Turn{Role: "user", Content: *userMsg})

		nBefore := len(a.Messages)
		// Count the run as started BEFORE a.Run: the agent's EventDone is
		// emitted from a defer, so it fires once even if a.Run panics (the
		// defer runs as the panic unwinds), keeping the drainer's doneTarget
		// accurate. safeRun recovers any panic into an error so the orderly
		// drain shutdown below (wantDone <- runsStarted; <-usageCh) still runs
		// — a bare a.Run panic would skip it and deadlock the drain goroutine.
		runsStarted++
		if err := safeRun(ctx, a, *userMsg); err != nil {
			errMessage = err.Error()
			break
		}

		// Fold this turn's new assistant/tool messages into the transcript so
		// the user-sim sees them on its next call (mirrors runner.ts: push tool
		// events, then the agent's final message).
		for _, te := range toolTurns(a.Messages[nBefore:]) {
			transcript = append(transcript, te)
		}
		lastAgentMsg = finalAgentText(a.Messages)
		transcript = append(transcript, Turn{Role: "agent", Content: lastAgentMsg})

		turns++
	}
	if turns == maxTurns {
		truncated = true
	}

	// Tell the drainer how many EventDone terminators to expect (one per Run we
	// started), then collect the summed usage once they have all arrived.
	wantDone <- runsStarted
	tally := <-usageCh

	pass := false
	if errMessage == "" {
		pass = safeCheck(cfg.task, CheckArgs{
			DB:                db,
			FinalAgentMessage: lastAgentMsg,
			Transcript:        transcriptStrings(transcript),
		})
	}

	// costCNY was summed per step at each serving model's price (flash, or pro
	// after an escalation), so it already reflects AutoRoute promotions — unlike
	// a flat llm.Cost(dscFlashModel, …) over the aggregate, which would price
	// escalated turns at flash rates and understate cost.
	rr := RunResult{
		TaskID:              cfg.task.ID,
		Mode:                "dsc",
		Pass:                pass,
		Turns:               turns,
		ToolCalls:           tally.toolCalls,
		CacheHitRatio:       llm.CacheHitRate(tally.usage),
		CostUsd:             tally.costCNY / cnyPerUsd,
		ClaudeEquivalentUsd: claudeEquivalentUsd(tally.usage),
		PromptTokens:        tally.usage.PromptTokens,
		CompletionTokens:    tally.usage.CompletionTokens,
		Truncated:           truncated,
		FinalAgentMessage:   lastAgentMsg,
		ErrorMessage:        errMessage,
	}
	return rr, nil
}

// dscUsageTally accumulates the per-run usage and tool-call count drained off
// the agent event stream. costCNY is summed per EventStepFinish at the price of
// the model that actually served that step (deepseek-v4-pro after an AutoRoute
// escalation, deepseek-v4-flash otherwise), mirroring the TS reference's
// loop.stats.totalCost which prices each call at its serving model rather than
// flat-rating every token at flash.
type dscUsageTally struct {
	usage     llm.Usage
	costCNY   float64
	toolCalls int
}

// safeRun calls a.Run and recovers any panic into an error so a single panicky
// turn can't unwind past runDSCArm's drain-shutdown (wantDone <- runsStarted;
// <-usageCh) and leave the drain goroutine blocked forever. The agent's
// EventDone is emitted from a defer, so it still fires as the panic unwinds —
// the drainer therefore sees one EventDone for this run regardless, and the
// caller treats the panic exactly like any other run error.
func safeRun(ctx context.Context, a *agent.Agent, userMsg string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("agent run panicked: %v", r)
		}
	}()
	_, err = a.Run(ctx, userMsg)
	return err
}

// safeCheck runs task.Check, treating a panic as a failed check (mirrors
// runner.ts safeCheck — a check that throws must not crash the run).
func safeCheck(task TaskDef, args CheckArgs) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return task.Check(args)
}

// toolTurns extracts the tool-result turns from a slice of newly-appended
// agent messages, in order. Each tool result becomes a Turn{Role:"tool"} so the
// user-sim transcript matches the runner.ts shape.
func toolTurns(msgs []llm.Message) []Turn {
	var out []Turn
	// Map tool_use id -> tool name so the result turn can carry the name.
	names := map[string]string{}
	for _, m := range msgs {
		for _, b := range m.Blocks {
			switch blk := b.(type) {
			case llm.ToolUseBlock:
				names[blk.ID] = blk.Name
			case llm.ToolResultBlock:
				out = append(out, Turn{
					Role:     "tool",
					Content:  blk.Content,
					ToolName: names[blk.ToolUseID],
				})
			}
		}
	}
	return out
}

// finalAgentText returns the concatenated text of the last assistant message,
// or "" if none has text. Mirrors agent.extractFinalText (unexported there).
func finalAgentText(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "assistant" {
			continue
		}
		var sb strings.Builder
		for _, b := range msgs[i].Blocks {
			if tb, ok := b.(llm.TextBlock); ok {
				sb.WriteString(tb.Text)
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return ""
}

// transcriptStrings renders the transcript into the []string form CheckArgs
// expects (one line per turn, same format as transcriptToString).
func transcriptStrings(turns []Turn) []string {
	out := make([]string, 0, len(turns))
	for _, t := range turns {
		switch t.Role {
		case "user":
			out = append(out, "USER: "+t.Content)
		case "agent":
			out = append(out, "AGENT: "+t.Content)
		case "tool":
			out = append(out, fmt.Sprintf("(tool %s returned: %s)", t.ToolName, t.Content))
		}
	}
	return out
}

// realChatClient adapts an *llm.Client to the chatClient interface the
// UserSimulator depends on. It issues a single non-tool chat completion and
// drains the stream into one text string. This is the LIVE user-sim path; the
// offline tests inject a scripted client instead.
type realChatClient struct{ c *llm.Client }

// Chat issues one completion and returns the assembled assistant text.
func (r realChatClient) Chat(ctx context.Context, req simChatRequest) (string, error) {
	msgs := make([]llm.Message, 0, len(req.messages))
	for _, m := range req.messages {
		msgs = append(msgs, llm.Message{
			Role:   m.role,
			Blocks: []llm.ContentBlock{llm.TextBlock{Text: m.content}},
		})
	}
	temp := req.temperature
	stream, err := r.c.Stream(ctx, llm.Request{
		Model:       req.model,
		Messages:    msgs,
		Temperature: &temp,
		MaxTokens:   req.maxTokens,
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for ev := range stream {
		switch ev.Type {
		case llm.EventTextDelta:
			sb.WriteString(ev.Text)
		case llm.EventError:
			return "", ev.Err
		}
	}
	return sb.String(), nil
}

// RunDSCArmLive runs one task through the dsc arm against the REAL DeepSeek
// endpoint. It is the only network-dialing entry point and is reached solely
// from the -live CLI flow; unit tests and `go build` never call it.
func RunDSCArmLive(ctx context.Context, client *llm.Client, task TaskDef, cwd string) (RunResult, error) {
	return runDSCArm(ctx, dscArmConfig{
		agentClient: client,
		simClient:   realChatClient{c: client},
		task:        task,
		cwd:         cwd,
	})
}
