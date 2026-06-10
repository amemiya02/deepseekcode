package h2h

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// traceFrame is the JSON shape of one line in a dsc -trace-jsonl file.
// Usage counters are flat top-level fields, not nested under a "usage" object.
type traceFrame struct {
	Type            string `json:"type"`
	CacheHitTokens  int    `json:"cache_hit_tokens"`
	CacheMissTokens int    `json:"cache_miss_tokens"`
	OutputTokens    int    `json:"output_tokens"`
}

// ParseDscTrace extracts per-turn usage from a dsc -trace-jsonl file.
// Lines without type "usage" are skipped; zero-usage frames are kept
// (a fully-cached turn can legitimately have miss=0).
func ParseDscTrace(path string) ([]TurnUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []TurnUsage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var frame traceFrame
		if err := json.Unmarshal(sc.Bytes(), &frame); err != nil {
			continue // tolerate non-JSON lines
		}
		if frame.Type == "usage" {
			out = append(out, TurnUsage{
				HitTokens:  frame.CacheHitTokens,
				MissTokens: frame.CacheMissTokens,
				OutTokens:  frame.OutputTokens,
			})
		}
	}
	return out, sc.Err()
}

// RunDsc executes one task in the workspace with the dsc binary and
// returns the parsed per-turn usage. The API key comes from env
// DSC_BENCH_API_KEY (fairness 3.3(1): a dedicated account).
func RunDsc(ctx context.Context, dscBin string, task TaskSpec, ws *Workspace) (ArmResult, error) {
	res := ArmResult{Arm: "dsc", TaskID: task.ID}
	trace := ws.Dir + "/.h2h-trace.jsonl"
	ctx, cancel := context.WithTimeout(ctx, time.Duration(task.WallclockCapMin)*time.Minute)
	defer cancel()
	// Flag spellings verified in Task 4 Step 1.
	cmd := exec.CommandContext(ctx, dscBin, "-yolo", "-trace-jsonl", trace, "-p", task.Prompt)
	cmd.Dir = ws.Dir
	cmd.Env = append(os.Environ(), "DEEPSEEK_API_KEY="+os.Getenv("DSC_BENCH_API_KEY"))
	if err := cmd.Run(); err != nil {
		res.DNF = true // timeout, crash, or non-zero exit all count as DNF
		res.Err = fmt.Sprintf("dsc run: %v", err)
		// fall through: partial trace still parses (fail-soft, spec 5)
	}
	turns, perr := ParseDscTrace(trace)
	if perr != nil && res.Err == "" {
		return res, perr
	}
	res.Turns = turns
	return res, nil
}
