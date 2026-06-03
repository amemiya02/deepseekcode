package traceinspect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/traceschema"
)

// EvictionThreshold is the cache_hit_tokens ceiling below which a turn > 1 is
// considered a full-body cache eviction. Derived from bench evidence (~9 k).
const EvictionThreshold = 9000

// Why labels for TurnLedger.Why.
const (
	WhyExpectedMiss = "expected-miss" // first turn of an epoch
	WhyCompaction   = "compaction"    // compaction record preceded this turn
	WhyDrift        = "drift"         // prefix hash changed since last turn
	WhyEviction     = "eviction"      // provider-side body eviction (no preceding event)
)

// TurnLedger is a single-turn cache row for the explain ledger.
type TurnLedger struct {
	Turn         int
	EpochID      string
	HitTokens    int
	MissTokens   int
	OutputTokens int
	CostCNY      float64
	Evicted      bool   // true if a full-body eviction is detected
	Why          string // human-readable eviction cause (empty when Evicted==false and not an expected miss)
}

// ExplainFile reads a JSONL trace and returns a per-turn ledger in emission order.
// It classifies each usage record with an eviction flag and a Why label.
func ExplainFile(path string) ([]TurnLedger, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type epochState struct {
		seenFirstUsage      bool
		lastPrefixHash      string
		pendingCompaction   bool
		pendingDriftBlocked bool
	}

	states := map[string]*epochState{}

	getState := func(epochID string) *epochState {
		if states[epochID] == nil {
			states[epochID] = &epochState{}
		}
		return states[epochID]
	}

	var ledger []TurnLedger

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r traceschema.Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}

		switch r.Type {
		case "prefix.snapshot":
			st := getState(r.EpochID)
			if r.StaticPrefixHash != "" {
				st.lastPrefixHash = r.StaticPrefixHash
			}

		case "compaction":
			st := getState(r.EpochID)
			st.pendingCompaction = true

		case "drift.blocked":
			st := getState(r.EpochID)
			st.pendingDriftBlocked = true

		case "usage":
			turn := 0
			if r.Turn != nil {
				turn = *r.Turn
			}
			hit := 0
			if r.CacheHitTokens != nil {
				hit = *r.CacheHitTokens
			}
			miss := 0
			if r.CacheMissTokens != nil {
				miss = *r.CacheMissTokens
			}
			out := 0
			if r.OutputTokens != nil {
				out = *r.OutputTokens
			}
			cost := 0.0
			if r.CostCNY != nil {
				cost = *r.CostCNY
			}

			st := getState(r.EpochID)

			row := TurnLedger{
				Turn:         turn,
				EpochID:      r.EpochID,
				HitTokens:    hit,
				MissTokens:   miss,
				OutputTokens: out,
				CostCNY:      cost,
			}

			// Turn 1 of an epoch is always an expected miss (cold start).
			if !st.seenFirstUsage {
				st.seenFirstUsage = true
				row.Why = WhyExpectedMiss
				// Not flagged Evicted — this is expected, not a regression.
			} else {
				// Subsequent turns: classify eviction cause if hit is low.
				// A turn is considered evicted when hit tokens are at or below
				// the absolute threshold.
				if hit <= EvictionThreshold {
					row.Evicted = true
					switch {
					case st.pendingCompaction:
						row.Why = WhyCompaction
					case st.pendingDriftBlocked:
						row.Why = WhyDrift
					default:
						row.Why = WhyEviction
					}
				}
			}

			// Reset pending flags after consumption.
			st.pendingCompaction = false
			st.pendingDriftBlocked = false

			ledger = append(ledger, row)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return ledger, nil
}

// RenderExplainText formats the ledger as a fixed-width text table.
func RenderExplainText(ledger []TurnLedger) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-6s %-24s %-8s %-8s %-6s %-10s %-6s %s\n",
		"TURN", "EPOCH", "HIT", "MISS", "OUT", "COST(¥)", "EVICT", "WHY")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 82))
	for _, row := range ledger {
		evict := "N"
		if row.Evicted {
			evict = "Y"
		}
		epochShort := row.EpochID
		if len(epochShort) > 22 {
			epochShort = epochShort[:10] + ".." + epochShort[len(epochShort)-10:]
		}
		fmt.Fprintf(&b, "%-6d %-24s %-8d %-8d %-6d %-10.6f %-6s %s\n",
			row.Turn, epochShort, row.HitTokens, row.MissTokens, row.OutputTokens,
			row.CostCNY, evict, row.Why)
	}
	return b.String()
}
