package traceinspect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
	"github.com/amemiya02/deepseekcode/internal/traceschema"
)

// record aliases the canonical agent-trace record (traceschema.Record), shared
// with the emitter (internal/agent) and the benchmark harness, so a field
// rename breaks this reader at compile time instead of silently (T6.1).
type record = traceschema.Record

type EpochSummary struct {
	EpochID     string
	Role        string
	ParentEpoch string
	ShortHash   string
	UsageTurns  int
	Done        bool
}

type Report struct {
	Path            string
	TotalUsageTurns int
	CacheHitTokens  int
	CacheMissTokens int
	OutputTokens    int
	CostCNY         float64
	CacheHitRate    float64
	RootEpochs      int
	SubagentEpochs  int

	// UniquePrefixHashes counts distinct static_prefix_hash values seen
	// across prefix.snapshot records. A stable run has exactly 1; drift
	// produces 2+.
	UniquePrefixHashes int

	// ExpectedCacheMisses counts prefix.snapshot records where the epoch
	// was newly created (first snapshot for that epoch), indicating an
	// expected cache miss for the first turn of that epoch.
	ExpectedCacheMisses int

	// CacheSavingsCNY is the difference between what the run would have
	// cost at full cache-miss pricing and what it actually cost, computed
	// from the usage records' cache hit/miss token split.
	CacheSavingsCNY float64

	// T6.4: lifecycle records the gate inspects. A clean summary that omits
	// these would mask exactly the events the Cache Reliability gate fails on.
	CompactionCount    int
	DriftBlockedCount  int
	PendingChangeCount int

	// Realized (sum of usage cost_cny) vs the budget gate's projected cost
	// (sum of projected_cny on budget.* records). BudgetEvents counts how many
	// budget records contributed a projection.
	ProjectedCNY float64
	BudgetEvents int

	Epochs []EpochSummary
}

func InspectFile(path string) (Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()

	rep := Report{Path: path}
	epochs := map[string]*EpochSummary{}
	seenEpochs := map[string]bool{}   // first prefix.snapshot per epoch = expected miss
	uniqueHashes := map[string]bool{} // distinct static_prefix_hash values
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return Report{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
		role := r.AgentRole
		if role == "" {
			role = "root"
		}
		if r.EpochID != "" {
			e := epochs[r.EpochID]
			if e == nil {
				e = &EpochSummary{EpochID: r.EpochID, Role: role, ParentEpoch: r.ParentEpochID}
				epochs[r.EpochID] = e
			}
			if r.StaticPrefixHash != "" {
				e.ShortHash = short(r.StaticPrefixHash)
			}
			if r.ParentEpochID != "" {
				e.ParentEpoch = r.ParentEpochID
			}
		}
		switch r.Type {
		case "prefix.snapshot":
			if r.StaticPrefixHash != "" {
				uniqueHashes[r.StaticPrefixHash] = true
			}
			if r.EpochID != "" && !seenEpochs[r.EpochID] {
				seenEpochs[r.EpochID] = true
				rep.ExpectedCacheMisses++
			}
		case "usage":
			rep.TotalUsageTurns++
			if r.CacheHitTokens != nil {
				rep.CacheHitTokens += *r.CacheHitTokens
			}
			if r.CacheMissTokens != nil {
				rep.CacheMissTokens += *r.CacheMissTokens
			}
			if r.OutputTokens != nil {
				rep.OutputTokens += *r.OutputTokens
			}
			if r.CostCNY != nil {
				rep.CostCNY += *r.CostCNY
			}
			// Compute cache savings from the hit/miss token split.
			// We reconstruct a minimal Usage to reuse the pricing math.
			if r.CacheHitTokens != nil && r.CacheMissTokens != nil {
				u := llm.Usage{
					PromptCacheHitTokens:  *r.CacheHitTokens,
					PromptCacheMissTokens: *r.CacheMissTokens,
				}
				if r.OutputTokens != nil {
					u.CompletionTokens = *r.OutputTokens
				}
				rep.CacheSavingsCNY += llm.CacheSavings(r.Model, u)
			}
			if e := epochs[r.EpochID]; e != nil {
				e.UsageTurns++
			}
		case "agent.done":
			if e := epochs[r.EpochID]; e != nil {
				e.Done = true
			}
		case "compaction":
			rep.CompactionCount++
		case "drift.blocked":
			rep.DriftBlockedCount++
		case "pending_change":
			rep.PendingChangeCount++
		case "budget.warning", "budget.blocked", "budget.unpriced":
			if r.ProjectedCNY != nil {
				rep.ProjectedCNY += *r.ProjectedCNY
				rep.BudgetEvents++
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Report{}, err
	}

	rep.UniquePrefixHashes = len(uniqueHashes)

	totalInput := rep.CacheHitTokens + rep.CacheMissTokens
	if totalInput > 0 {
		rep.CacheHitRate = float64(rep.CacheHitTokens) / float64(totalInput)
	}
	for _, e := range epochs {
		if e.Role == "subagent" {
			rep.SubagentEpochs++
		} else {
			rep.RootEpochs++
		}
		rep.Epochs = append(rep.Epochs, *e)
	}
	sort.Slice(rep.Epochs, func(i, j int) bool {
		return rep.Epochs[i].EpochID < rep.Epochs[j].EpochID
	})
	return rep, nil
}

func RenderText(rep Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "trace %s\n", rep.Path)
	fmt.Fprintf(&b, "usage turns %d | cache %.1f%% | in hit/miss %d/%d | out %d | cost ¥%.6f\n",
		rep.TotalUsageTurns, rep.CacheHitRate*100, rep.CacheHitTokens, rep.CacheMissTokens, rep.OutputTokens, rep.CostCNY)
	fmt.Fprintf(&b, "cache %.1f%% | hit %d | miss %d | saved CNY %.2f | prefixes %d | expected_miss %d\n",
		rep.CacheHitRate*100, rep.CacheHitTokens, rep.CacheMissTokens, rep.CacheSavingsCNY, rep.UniquePrefixHashes, rep.ExpectedCacheMisses)
	fmt.Fprintf(&b, "epochs root %d | subagents %d\n", rep.RootEpochs, rep.SubagentEpochs)
	fmt.Fprintf(&b, "lifecycle: compaction %d | drift.blocked %d | pending %d\n",
		rep.CompactionCount, rep.DriftBlockedCount, rep.PendingChangeCount)
	if rep.BudgetEvents > 0 {
		// Δ = projected (conservative all-miss) − realized (cache-discounted);
		// a positive Δ is the cache savings the gate's projection didn't assume.
		fmt.Fprintf(&b, "cost realized ¥%.6f vs projected ¥%.6f (Δ ¥%.6f over %d budget event(s))\n",
			rep.CostCNY, rep.ProjectedCNY, rep.ProjectedCNY-rep.CostCNY, rep.BudgetEvents)
	}
	for _, e := range rep.Epochs {
		done := "open"
		if e.Done {
			done = "done"
		}
		parent := ""
		if e.ParentEpoch != "" {
			parent = " parent=" + e.ParentEpoch
		}
		fmt.Fprintf(&b, "- %s role=%s hash=%s turns=%d %s%s\n", e.EpochID, e.Role, e.ShortHash, e.UsageTurns, done, parent)
	}
	return b.String()
}

func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}
