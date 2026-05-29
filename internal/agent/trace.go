package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/traceschema"
)

// traceRecord aliases the single canonical agent-trace record
// (traceschema.Record). The emitter here, internal/traceinspect, and the
// benchmark harness (bench/cmd/benchrunner) all share that one type, so a
// field rename is a compile error in every consumer rather than a silent
// reader breakage (T6.1). Every emitted record is stamped with
// schema_version = traceschema.Version in encode.
type traceRecord = traceschema.Record

// traceWriter is the synchronized JSONL sink shared by a root TraceSink and
// any subagent child sinks. Sharing one mutex+encoder keeps records from the
// parent and child goroutines from interleaving into corrupt half-lines.
type traceWriter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (w *traceWriter) encode(r traceRecord) {
	// Stamp the schema version on every record at the single write chokepoint
	// so no emit site can forget it (T6.1).
	r.SchemaVersion = traceschema.Version
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.enc.Encode(r)
}

// TraceSink converts an agent's event stream into JSONL trace records.
// It subscribes to a bus and writes one record per epoch lifecycle
// event, per turn (prefix snapshot + usage), per compaction, and per
// blocked drift. The trace is the source of truth for the benchmark's
// cache-reliability gate.
//
// Construct the root via Agent.AttachTraceSink (wires the subscription, a
// drain goroutine, and a handle the caller waits on after Run). Subagent
// child sinks are derived via newChildTraceSink and share the root's writer.
type TraceSink struct {
	w     *traceWriter
	model string
	runID string
	role  string
	// parentEpochID is set only on a subagent child sink; it stamps every
	// child record with the epoch the parent was running under, so the
	// benchmark can attribute the child epoch to its parent.
	parentEpochID string

	turn          int
	curEpochID    string
	curPrefixHash string
	curToolsHash  string
}

// NewTraceSink builds a root sink writing JSONL to w. model is used to price
// usage records (cost_cny). Every record is stamped with a per-run run_id and
// agent_role="root" so the benchmark can distinguish the root epoch from any
// subagent epochs when judging parent/subagent cache pollution.
func NewTraceSink(w io.Writer, model string) *TraceSink {
	return &TraceSink{
		w:     &traceWriter{enc: json.NewEncoder(w)},
		model: model,
		runID: fmt.Sprintf("run_%d", time.Now().UnixNano()),
		role:  "root",
	}
}

// newChildTraceSink derives a subagent sink that shares the parent's
// synchronized writer and run_id but stamps every record with
// agent_role="subagent" and parentEpochID. The child agent's own EpochManager
// mints a distinct epoch_id, so a child epoch never reuses the parent's — which
// is exactly what the benchmark's parent/subagent isolation check verifies.
func newChildTraceSink(parent *TraceSink, parentEpochID string) *TraceSink {
	return &TraceSink{
		w:             parent.w,
		model:         parent.model,
		runID:         parent.runID,
		role:          "subagent",
		parentEpochID: parentEpochID,
	}
}

func (s *TraceSink) write(r traceRecord) {
	if r.RunID == "" {
		r.RunID = s.runID
	}
	if r.AgentRole == "" {
		r.AgentRole = s.role
	}
	if r.ParentEpochID == "" {
		r.ParentEpochID = s.parentEpochID
	}
	s.w.encode(r)
}

// Handle processes a single agent event, emitting trace records. Exported
// so it can be unit-tested without a live bus.
func (s *TraceSink) Handle(ev Event) {
	switch e := ev.(type) {
	case EventEpochCreated:
		s.curEpochID = e.EpochID
		s.curPrefixHash = e.StaticPrefixHash
		s.curToolsHash = e.ToolsHash
		s.write(traceRecord{
			Type:             "prefix.snapshot",
			EpochID:          e.EpochID,
			StaticPrefixHash: e.StaticPrefixHash,
			ToolsHash:        e.ToolsHash,
			Reason:           e.Reason,
		})
	case EventEpochSwitched:
		s.curEpochID = e.NewEpochID
		s.curPrefixHash = e.StaticPrefixHash
		s.curToolsHash = e.ToolsHash
		s.write(traceRecord{
			Type:             "epoch.switched",
			OldEpochID:       e.OldEpochID,
			EpochID:          e.NewEpochID,
			StaticPrefixHash: e.StaticPrefixHash,
			ToolsHash:        e.ToolsHash,
			Reason:           e.Reason,
		})
		// An epoch switch is also a fresh prefix snapshot for the new epoch.
		s.write(traceRecord{
			Type:             "prefix.snapshot",
			EpochID:          e.NewEpochID,
			StaticPrefixHash: e.StaticPrefixHash,
			ToolsHash:        e.ToolsHash,
			Reason:           "epoch_switched",
		})
	case EventEpochFrozen:
		s.write(traceRecord{Type: "epoch.frozen", EpochID: e.EpochID})
	case EventPendingChange:
		s.write(traceRecord{
			Type:        "pending_change",
			EpochID:     e.EpochID,
			Kind:        string(e.Kind),
			Description: e.Description,
		})
	case EventDriftBlocked:
		// Unauthorized drift inside a frozen epoch — the gate counts these.
		s.write(traceRecord{Type: "drift.blocked", EpochID: e.EpochID, Which: e.Which})
	case EventStepFinish:
		s.turn++
		turn := s.turn
		hit := e.Usage.PromptCacheHitTokens
		miss := e.Usage.PromptCacheMissTokens
		out := e.Usage.CompletionTokens
		// Attribute usage to the model that actually produced the turn — an
		// escalated turn (T2.3) carries the stronger model — falling back to the
		// sink's loop model for steps that don't stamp one. This keeps the trace
		// usage record consistent with the ReceiptModelFinal payload.
		stepModel := e.Model
		if stepModel == "" {
			stepModel = s.model
		}
		cost := llm.Cost(stepModel, e.Usage)
		// One snapshot per turn lets the harness prove the static prefix
		// hash is identical across every turn of an epoch.
		s.write(traceRecord{
			Type:             "prefix.snapshot",
			Turn:             &turn,
			EpochID:          s.curEpochID,
			StaticPrefixHash: s.curPrefixHash,
			ToolsHash:        s.curToolsHash,
		})
		s.write(traceRecord{
			Type:            "usage",
			Turn:            &turn,
			EpochID:         s.curEpochID,
			Model:           stepModel,
			CacheHitTokens:  &hit,
			CacheMissTokens: &miss,
			OutputTokens:    &out,
			CostCNY:         &cost,
			Reason:          e.Reason.String(),
		})
	case EventSemanticCompaction:
		// Emit the measured static-prefix fingerprints the agent computed for
		// the frozen baseline (before) and for the request compaction actually
		// fed the model (after). The harness compares them — there is no
		// hardcoded "unchanged" claim. Equal hashes prove compaction reused
		// the frozen prefix; a divergence (e.g. summarizing against the live,
		// non-frozen prompt) fails the gate.
		kind := "semantic"
		if !e.UsedSemantic {
			kind = "deterministic"
		}
		cost := e.SummaryCost
		s.write(traceRecord{
			Type:                   "compaction",
			EpochID:                s.curEpochID,
			Kind:                   kind,
			StaticPrefixHash:       s.curPrefixHash,
			BeforeStaticPrefixHash: e.StaticPrefixHashBefore,
			AfterStaticPrefixHash:  e.StaticPrefixHashAfter,
			SummaryCostCNY:         &cost,
		})
	case EventEscalated:
		// The turn was re-issued on a stronger model. Kind carries the trigger
		// (marker / repair_errors); Model is the model escalated TO.
		s.write(traceRecord{
			Type:        "policy.escalated",
			EpochID:     s.curEpochID,
			Kind:        e.Trigger,
			Model:       e.ToModel,
			Description: e.FromModel + " -> " + e.ToModel + ": " + e.Reason,
		})
	case EventBudget:
		// Session-budget gate decision (T1.3). The Type encodes the kind
		// (budget.warning / budget.blocked / budget.unpriced); Model is the
		// model gated.
		t := "budget.warning"
		switch e.Kind {
		case BudgetKindBlocked:
			t = "budget.blocked"
		case BudgetKindUnpriced:
			t = "budget.unpriced"
		}
		rec := traceRecord{Type: t, Model: e.Model}
		// Carry the projected turn cost so the inspector can show realized vs
		// projected (T6.4). Unpriced gates have no computable projection.
		if e.Kind != BudgetKindUnpriced {
			pc := e.ProjectedCNY
			rec.ProjectedCNY = &pc
		}
		s.write(rec)
	case EventPermissionDenied:
		// Tool call refused by the permission layer (T1.3). Kind is rule|policy;
		// Description carries the tool name; Reason the cause.
		kind := "policy"
		if e.ByRule {
			kind = "rule"
		}
		s.write(traceRecord{Type: "permission.denied", Kind: kind, Description: e.Tool, Reason: e.Reason})
	case EventDone:
		// A terminal marker per agent. The root stamps agent_role="root"; a
		// subagent stamps "subagent" + parent_epoch_id. The benchmark requires
		// a child agent.done (alongside a child usage turn) before it trusts a
		// subagent's isolation evidence — a child trace cut off before EventDone
		// is partial and must not pass.
		s.write(traceRecord{
			Type:    "agent.done",
			EpochID: s.curEpochID,
			Reason:  e.Reason.String(),
		})
	}
}

// TraceSinkHandle lets the caller wait for the sink to finish processing
// after Run returns (EventDone is the terminator) and unsubscribe.
type TraceSinkHandle struct {
	bus  *Bus
	sub  *Subscription
	done chan struct{}
}

// Wait blocks until the agent's terminating EventDone has been processed.
func (h *TraceSinkHandle) Wait() { <-h.done }

// WaitTimeout blocks until EventDone is processed or d elapses, whichever is
// first. It returns true when EventDone was processed (the agent finished
// cleanly) and false on timeout — a timed-out child trace is partial, which the
// caller must surface rather than silently close.
func (h *TraceSinkHandle) WaitTimeout(d time.Duration) bool {
	select {
	case <-h.done:
		return true
	case <-time.After(d):
		return false
	}
}

// Close unsubscribes the sink from the bus. Safe to call after Wait.
func (h *TraceSinkHandle) Close() { h.bus.Unsubscribe(h.sub) }

// AttachTraceSink subscribes a root TraceSink to the agent's bus and starts a
// drain goroutine. The returned handle's Wait blocks until the run's
// EventDone is processed, so callers can flush a JSONL file before exit. The
// sink is also retained on the agent so spawned subagents can tee their own
// epoch/usage events into the same trace (see AttachChildTraceSink).
func (a *Agent) AttachTraceSink(w io.Writer) *TraceSinkHandle {
	sink := NewTraceSink(w, a.Model)
	a.traceSink = sink
	return attachSinkToBus(a.bus, sink)
}

// AttachChildTraceSink wires a subagent's event bus into the parent's trace
// writer, stamping every child record with agent_role="subagent" and the
// parent's current epoch_id. It is a no-op (returns nil) when the parent has no
// trace sink attached, so normal interactive/CLI runs are unaffected. The
// returned handle's Wait blocks until the child's EventDone is processed; the
// caller closes it after the subagent's Run returns.
func (a *Agent) AttachChildTraceSink(child *Agent) *TraceSinkHandle {
	if a.traceSink == nil || child == nil {
		return nil
	}
	sink := newChildTraceSink(a.traceSink, a.CurrentEpochID())
	h := attachSinkToBus(child.bus, sink)
	a.childTraceMu.Lock()
	a.childTraceHandles = append(a.childTraceHandles, h)
	a.childTraceMu.Unlock()
	return h
}

// WaitChildTraces blocks until every tracked subagent trace handle has flushed
// its child's EventDone, or the shared deadline elapses, then closes them. A
// one-shot run calls this before closing the root trace so an async (`task`
// with async:true) subagent's child epoch is flushed instead of being lost when
// the process exits. No-op when no subagent trace was attached.
//
// If a handle times out the child never reached EventDone — its trace is
// partial. Rather than close it silently, a `child_trace_incomplete` record is
// written so the gate fails closed instead of trusting a cut-off child.
func (a *Agent) WaitChildTraces(timeout time.Duration) {
	a.childTraceMu.Lock()
	handles := append([]*TraceSinkHandle(nil), a.childTraceHandles...)
	a.childTraceMu.Unlock()

	deadline := time.Now().Add(timeout)
	for _, h := range handles {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		if !h.WaitTimeout(remaining) && a.traceSink != nil {
			a.traceSink.write(traceRecord{Type: "child_trace_incomplete"})
		}
		h.Close()
	}
}

// attachSinkToBus subscribes sink to bus and drains it in a goroutine,
// returning a handle whose done channel closes once EventDone is processed.
func attachSinkToBus(bus *Bus, sink *TraceSink) *TraceSinkHandle {
	sub := bus.Subscribe(512)
	h := &TraceSinkHandle{bus: bus, sub: sub, done: make(chan struct{})}
	go func() {
		closed := false
		for env := range sub.C {
			sink.Handle(env.Event)
			if _, ok := env.Event.(EventDone); ok && !closed {
				closed = true
				close(h.done)
			}
		}
		if !closed {
			close(h.done)
		}
	}()
	return h
}
