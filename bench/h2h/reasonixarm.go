package h2h

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/amemiya02/deepseekcode/bench/h2h/acpclient"
)

// ParseReasonixSession extracts usage from a reasonix .session.jsonl
// archive — the fallback when session/update carries no usage.
func ParseReasonixSession(path string) ([]TurnUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []TurnUsage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var line struct {
			Usage *TurnUsage `json:"usage"`
		}
		if json.Unmarshal(sc.Bytes(), &line) == nil && line.Usage != nil {
			out = append(out, *line.Usage)
		}
	}
	return out, sc.Err()
}

// newestSession returns the most recently modified *.session.jsonl
// under the reasonix archive dir.
func newestSession(archiveDir string) (string, bool) {
	matches, _ := filepath.Glob(filepath.Join(archiveDir, "*.session.jsonl"))
	if len(matches) == 0 {
		return "", false
	}
	sort.Slice(matches, func(i, j int) bool {
		a, _ := os.Stat(matches[i])
		b, _ := os.Stat(matches[j])
		if a == nil || b == nil {
			return false
		}
		return a.ModTime().After(b.ModTime())
	})
	return matches[0], true
}

// RunReasonix executes one task via `reasonix acp --yolo --dir <ws>`.
// Env REASONIX_BENCH_API_KEY supplies the second account's key
// (fairness §3.3(1)); REASONIX_ARCHIVE_DIR (default
// ~/.config/reasonix/archive) locates the usage fallback.
func RunReasonix(ctx context.Context, bin string, task TaskSpec, ws *Workspace) (ArmResult, error) {
	res := ArmResult{Arm: "reasonix", TaskID: task.ID}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(task.WallclockCapMin)*time.Minute)
	defer cancel()
	c, err := acpclient.Start(ctx, bin,
		[]string{"acp", "--yolo", "--dir", ws.Dir},
		[]string{"DEEPSEEK_API_KEY=" + os.Getenv("REASONIX_BENCH_API_KEY")})
	if err != nil {
		res.Err = err.Error()
		res.DNF = true
		return res, nil
	}
	defer c.Close()
	if err := c.Initialize(); err != nil {
		res.Err = "initialize: " + err.Error()
		res.DNF = true
		return res, nil
	}
	sid, err := c.NewSession(ws.Dir)
	if err != nil {
		res.Err = "session/new: " + err.Error()
		res.DNF = true
		return res, nil
	}
	usage, perr := c.Prompt(sid, task.Prompt)
	if perr != nil {
		res.Err = "prompt: " + perr.Error()
		res.DNF = ctx.Err() != nil
	}
	for _, u := range usage {
		res.Turns = append(res.Turns, TurnUsage{HitTokens: u.HitTokens, MissTokens: u.MissTokens, OutTokens: u.OutTokens})
	}
	if len(res.Turns) == 0 { // fallback: session archive
		dir := os.Getenv("REASONIX_ARCHIVE_DIR")
		if dir == "" {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".config", "reasonix", "archive")
		}
		if p, ok := newestSession(dir); ok {
			if turns, err := ParseReasonixSession(p); err == nil {
				res.Turns = turns
			}
		}
	}
	return res, nil
}
