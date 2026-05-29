package traceinspect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

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
	Epochs          []EpochSummary
}

func InspectFile(path string) (Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()

	rep := Report{Path: path}
	epochs := map[string]*EpochSummary{}
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
			if e := epochs[r.EpochID]; e != nil {
				e.UsageTurns++
			}
		case "agent.done":
			if e := epochs[r.EpochID]; e != nil {
				e.Done = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Report{}, err
	}

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
	fmt.Fprintf(&b, "epochs root %d | subagents %d\n", rep.RootEpochs, rep.SubagentEpochs)
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
