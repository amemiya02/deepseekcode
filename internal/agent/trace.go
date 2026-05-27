package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// traceRecord is one JSONL line in the agent trace. Field names mirror the
// benchrunner's parser (bench/cmd/benchrunner) so the harness can read this
// trace directly and compute the Cache Reliability gate from real epoch,
// usage, compaction, and drift evidence — no placeholders.
type traceRecord struct {
	Type string `json:"type"`
	Turn *int   `json:"turn,omitempty"`

	// run_id/agent_role/parent_epoch_id identify which agent emitted the
	// record. The root one-shot stamps agent_role="root" with no parent; a
	// subagent (once child-trace emission is wired) stamps "subagent" plus
	// the parent epoch it ran under. The benchmark uses these to tell a
	// child epoch from the root epoch when judging parent/subagent cache
	// pollution — instead of hardcoding a pass.
	RunID         string `json:"run_id,omitempty"`
	AgentRole     string `json:"agent_role,omitempty"`
	ParentEpochID string `json:"parent_epoch_id,omitempty"`

	EpochID          string `json:"epoch_id,omitempty"`
	OldEpochID       string `json:"old_epoch_id,omitempty"`
	Model            string `json:"model,omitempty"`
	StaticPrefixHash string `json:"static_prefix_hash,omitempty"`
	ToolsHash        string `json:"tools_hash,omitempty"`
	Reason           string `json:"reason,omitempty"`

	// usage
	CacheHitTokens  *int     `json:"cache_hit_tokens,omitempty"`
	CacheMissTokens *int     `json:"cache_miss_tokens,omitempty"`
	OutputTokens    *int     `json:"output_tokens,omitempty"`
	CostCNY         *float64 `json:"cost_cny,omitempty"`

	// pending_change / drift.blocked
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
	Which       string `json:"which,omitempty"`

	// compaction — the measured static-prefix fingerprints before and after
	// compaction. The benchmark compares them itself; there is no hardcoded
	// "changed" boolean. Equal means compaction reused the frozen prefix.
	BeforeStaticPrefixHash string   `json:"before_static_prefix_hash,omitempty"`
	AfterStaticPrefixHash  string   `json:"after_static_prefix_hash,omitempty"`
	SummaryCostCNY         *float64 `json:"summary_cost_cny,omitempty"`
}

// TraceSink converts the agent's event stream into JSONL trace records.
// It subscribes to the bus and writes one record per epoch lifecycle
// event, per turn (prefix snapshot + usage), per compaction, and per
// blocked drift. The trace is the source of truth for the benchmark's
// cache-reliability gate.
//
// Construct via Agent.AttachTraceSink, which wires the subscription and a
// drain goroutine and returns a handle the caller waits on after Run.
type TraceSink struct {
	mu    sync.Mutex
	enc   *json.Encoder
	model string
	runID string
	role  string

	turn          int
	curEpochID    string
	curPrefixHash string
	curToolsHash  string
}

// NewTraceSink builds a sink writing JSONL to w. model is used to price
// usage records (cost_cny). Every record is stamped with a per-run run_id and
// agent_role="root" so the benchmark can distinguish the root epoch from any
// subagent epochs when judging parent/subagent cache pollution.
func NewTraceSink(w io.Writer, model string) *TraceSink {
	return &TraceSink{
		enc:   json.NewEncoder(w),
		model: model,
		runID: fmt.Sprintf("run_%d", time.Now().UnixNano()),
		role:  "root",
	}
}

func (s *TraceSink) write(r traceRecord) {
	if r.RunID == "" {
		r.RunID = s.runID
	}
	if r.AgentRole == "" {
		r.AgentRole = s.role
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(r)
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
		cost := llm.Cost(s.model, e.Usage)
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
			Model:           s.model,
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

// WaitTimeout blocks until EventDone is processed or d elapses, whichever
// is first. The timeout guards against the (unlikely) case where a burst of
// high-frequency events overflowed the subscription buffer and EventDone was
// dropped — the caller still makes progress and flushes whatever was written.
func (h *TraceSinkHandle) WaitTimeout(d time.Duration) {
	select {
	case <-h.done:
	case <-time.After(d):
	}
}

// Close unsubscribes the sink from the bus. Safe to call after Wait.
func (h *TraceSinkHandle) Close() { h.bus.Unsubscribe(h.sub) }

// AttachTraceSink subscribes a TraceSink to the agent's bus and starts a
// drain goroutine. The returned handle's Wait blocks until the run's
// EventDone is processed, so callers can flush a JSONL file before exit.
func (a *Agent) AttachTraceSink(w io.Writer) *TraceSinkHandle {
	sink := NewTraceSink(w, a.Model)
	sub := a.bus.Subscribe(512)
	h := &TraceSinkHandle{bus: a.bus, sub: sub, done: make(chan struct{})}
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
