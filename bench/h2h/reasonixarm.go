package h2h

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amemiya02/deepseekcode/bench/h2h/acpclient"
	"github.com/amemiya02/deepseekcode/bench/h2h/usageproxy"
)

// reasonixUpstream is the provider endpoint the usage proxy forwards to.
const reasonixUpstream = "https://api.deepseek.com"

// writeReasonixConfig drops a workspace-local reasonix.toml that pins
// the bench model and points its base_url at the usage-harvest proxy.
// ./reasonix.toml takes precedence over the operator's global config
// (resolution order verified against the live binary 2026-06-10),
// which both routes traffic through the proxy and isolates the run
// from local config drift. The file is git-excluded so the agent
// sees a clean `git status`.
func writeReasonixConfig(dir, baseURL string) error {
	cfg := fmt.Sprintf(`# h2h bench: pin model + route API via the usage-harvest proxy.
default_model = "deepseek-flash"

[[providers]]
name        = "deepseek-flash"
kind        = "openai"
base_url    = %q
model       = "deepseek-v4-flash"
api_key_env = "DEEPSEEK_API_KEY"
context_window = 1000000
`, baseURL)
	if err := os.WriteFile(filepath.Join(dir, "reasonix.toml"), []byte(cfg), 0o644); err != nil {
		return err
	}
	// Fail-soft: keep the agent's `git status` clean in real clones.
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.WriteString("reasonix.toml\n")
		f.Close()
	}
	return nil
}

// RunReasonix executes one task via `reasonix acp` in ws.Dir.
// Env REASONIX_BENCH_API_KEY supplies the second account's key
// (fairness §3.3(1)). Usage comes from the wire: reasonix v1.0.0
// emits no token usage over ACP, in transcripts, or on stderr
// (verified live 2026-06-10), so a loopback proxy harvests the
// provider-reported usage object from each chat completion — the
// same provider counters dsc's -trace-jsonl records.
func RunReasonix(ctx context.Context, bin string, task TaskSpec, ws *Workspace) (ArmResult, error) {
	res := ArmResult{Arm: "reasonix", TaskID: task.ID}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(task.WallclockCapMin)*time.Minute)
	defer cancel()
	px, err := usageproxy.Start(reasonixUpstream)
	if err != nil {
		res.Err = "usage proxy: " + err.Error()
		res.DNF = true
		return res, nil
	}
	defer px.Close()
	if err := writeReasonixConfig(ws.Dir, px.URL()); err != nil {
		res.Err = "write reasonix.toml: " + err.Error()
		res.DNF = true
		return res, nil
	}
	c, err := acpclient.Start(ctx, bin,
		[]string{"acp"},
		[]string{"DEEPSEEK_API_KEY=" + os.Getenv("REASONIX_BENCH_API_KEY")},
		ws.Dir)
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
	acpUsage, perr := c.Prompt(sid, task.Prompt)
	if perr != nil {
		res.Err = "prompt: " + perr.Error()
		res.DNF = true // timeout, crash, or error all count as DNF
	}
	// Primary: provider-reported usage harvested on the wire.
	for _, u := range px.Usages() {
		res.Turns = append(res.Turns, TurnUsage{HitTokens: u.HitTokens, MissTokens: u.MissTokens, OutTokens: u.OutTokens})
	}
	// Secondary: ACP session/update usage, kept for future reasonix
	// versions that may start emitting it.
	if len(res.Turns) == 0 {
		for _, u := range acpUsage {
			res.Turns = append(res.Turns, TurnUsage{HitTokens: u.HitTokens, MissTokens: u.MissTokens, OutTokens: u.OutTokens})
		}
	}
	// Zero usage on a finished run is missing data, never a free win —
	// surface it instead of letting billable=0 pass silently (§3.3(5)).
	if len(res.Turns) == 0 && res.Err == "" {
		res.Err = "no usage captured: proxy saw no API responses with a usage object"
	}
	return res, nil
}
