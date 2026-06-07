// Package cache provides pure diagnostics for prefix-cache attribution.
// It contains no cache logic of its own; it classifies why a given turn
// resulted in a cache miss (or hit) based on the observed prefix state
// and the current compaction epoch.
package cache

import "github.com/amemiya02/deepseekcode/internal/llm"

// Epoch is a monotonic prefix-rewrite version, bumped ONLY at compaction.
// When CurEpoch != PrevEpoch the receipt is attributed CauseCompactReset
// regardless of prefix content — a compacted prefix forces a full re-prefill.
type Epoch int

// Cause labels why a turn consumed cache-miss tokens (or, for Steady, mostly hits).
type Cause string

const (
	// CauseColdFirst is set when there is no prior prefix pin — the very first
	// turn or a fresh session always pays the full prefill cost.
	CauseColdFirst Cause = "cold_first"

	// CausePrefixMut is set when the system prompt or tool set changed between
	// the previous turn and this one.
	CausePrefixMut Cause = "prefix_mut"

	// CauseResidualTail is a structural floor: the V4 incomplete tail block is
	// always recomputed. The number of wasted tokens is ResidualEst.
	CauseResidualTail Cause = "residual_tail"

	// CauseCompactReset is set when the compaction epoch bumped, forcing a full
	// re-prefill regardless of prefix content.
	CauseCompactReset Cause = "compact_reset"

	// CauseSteady is the happy path: stable prefix, same epoch, cache hits
	// dominate.
	CauseSteady Cause = "steady"
)

// Input holds the observed state for one turn that the attribution decision
// tree consumes.
type Input struct {
	Turn       int
	Model      string
	Prev       *llm.StaticPrefix // nil => cold first
	Cur        llm.StaticPrefix
	PrevEpoch  Epoch
	CurEpoch   Epoch
	Unit       int // measured cache block size; <=0 disables residual estimate
	TailTokens int // newly appended post-prefix tokens this turn
	Usage      llm.Usage
}

// CacheReceipt is the attribution verdict for one turn: why the cache behaved
// as it did, plus the cost/savings breakdown.
type CacheReceipt struct {
	Turn        int
	Epoch       Epoch
	Dominant    Cause
	Drift       llm.PrefixDiff // populated when Dominant == CausePrefixMut
	HitTokens   int
	MissTokens  int
	ResidualEst int     // structural floor: incomplete tail block = TailTokens % Unit
	CostCNY     float64 // llm.Cost
	SavedCNY    float64 // llm.CacheSavings
}

// Attribute classifies why the cache behaved as it did for a given turn.
//
// Decision tree (evaluated in order; first match wins):
//
//   1. Prev == nil            → CauseColdFirst
//   2. CurEpoch != PrevEpoch  → CauseCompactReset
//   3. Diff(*Prev, Cur).Changed() → CausePrefixMut (Drift populated)
//   4. Otherwise              → CauseSteady
//      If Unit > 0: ResidualEst = TailTokens % Unit
func Attribute(in Input) CacheReceipt {
	r := CacheReceipt{
		Turn:       in.Turn,
		Epoch:      in.CurEpoch,
		HitTokens:  in.Usage.PromptCacheHitTokens,
		MissTokens: in.Usage.PromptCacheMissTokens,
		CostCNY:    llm.Cost(in.Model, in.Usage),
		SavedCNY:   llm.CacheSavings(in.Model, in.Usage),
	}

	switch {
	case in.Prev == nil:
		r.Dominant = CauseColdFirst
	case in.CurEpoch != in.PrevEpoch:
		r.Dominant = CauseCompactReset
	default:
		drift := llm.Diff(*in.Prev, in.Cur)
		if drift.Changed() {
			r.Dominant = CausePrefixMut
			r.Drift = drift
		} else {
			r.Dominant = CauseSteady
			if in.Unit > 0 {
				r.ResidualEst = in.TailTokens % in.Unit
			}
		}
	}

	return r
}
