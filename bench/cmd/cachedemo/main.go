// bench/cmd/cachedemo/main.go
// cachedemo runs a controlled A/B that proves deepseekcode keeps DeepSeek's
// prompt cache hot. Default is offline (replays committed fixtures); -live
// makes real (cheap) deepseek-v4-flash calls and reports the cache hit-rate
// and ¥ savings from DeepSeek's own prompt_cache_hit_tokens.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func main() {
	var (
		model   = flag.String("model", "deepseek-v4-flash", "model id")
		turns   = flag.Int("turns", 12, "number of turns per arm")
		live    = flag.Bool("live", false, "make real API calls (needs DEEPSEEK_API_KEY)")
		baseURL = flag.String("base-url", "https://api.deepseek.com", "DeepSeek base URL")
		out     = flag.String("out", "bench/cache-demo/results.json", "results JSON path")
		fixture = flag.String("fixture", "bench/cache-demo/results.sample.json", "offline usage fixture")
		driftAt = flag.Int("drift-at", 0, "turn at which the drift arm appends a tool (0 = turns/2)")
	)
	flag.Parse()

	if err := run(*model, *turns, *live, *baseURL, *out, *fixture, *driftAt); err != nil {
		fmt.Fprintln(os.Stderr, "cachedemo:", err)
		os.Exit(1)
	}
}

func run(model string, turns int, live bool, baseURL, out, fixture string, driftAt int) error {
	script := demoTurns(turns)

	// Resolve driftAt: 0 means turns/2.
	if driftAt == 0 {
		driftAt = turns / 2
	}

	var naiveUsage, stableUsage, driftUsage []llm.Usage
	if live {
		key := os.Getenv("DEEPSEEK_API_KEY")
		if key == "" {
			return fmt.Errorf("-live requires DEEPSEEK_API_KEY")
		}
		c := llm.NewClient(key, baseURL)
		ctx := context.Background()

		naiveReqs := make([]llm.Request, len(script))
		stableReqs := make([]llm.Request, len(script))
		driftReqs := make([]llm.Request, len(script))
		for i, u := range script {
			naiveReqs[i] = buildRequest(model, naivePrefix(i+1), u)
			stableReqs[i] = buildRequest(model, stablePrefix(), u)
			driftReqs[i] = buildRequest(model, driftPrefix(i+1, driftAt), u)
		}
		var err error
		if naiveUsage, err = runArmLive(ctx, c, naiveReqs); err != nil {
			return fmt.Errorf("naive arm: %w", err)
		}
		if stableUsage, err = runArmLive(ctx, c, stableReqs); err != nil {
			return fmt.Errorf("stable arm: %w", err)
		}
		if driftUsage, err = runArmLive(ctx, c, driftReqs); err != nil {
			return fmt.Errorf("drift arm: %w", err)
		}
	} else {
		// Offline: replay the committed sample so the demo runs without a key.
		// The sample stores the naive turns first, then the stable turns.
		// A third drift arm is optional; skip gracefully if the fixture has only 2 arms.
		all, err := loadUsageFixture(fixture)
		if err != nil {
			return fmt.Errorf("offline fixture (%s): %w", fixture, err)
		}
		if len(all)%2 != 0 && len(all)%3 != 0 {
			return fmt.Errorf("fixture has entry count %d (expected multiple of 2 or 3)", len(all))
		}
		if len(all)%3 == 0 && len(all)/3*3 == len(all) && len(all) >= 3 {
			third := len(all) / 3
			naiveUsage = all[:third]
			stableUsage = all[third : 2*third]
			driftUsage = all[2*third:]
		} else {
			half := len(all) / 2
			naiveUsage = all[:half]
			stableUsage = all[half:]
		}
	}

	arms := []ArmResult{
		aggregate("naive", model, naiveUsage),
		aggregate("stable", model, stableUsage),
	}
	if len(driftUsage) > 0 {
		arms = append(arms, aggregate("drift", model, driftUsage))
	}
	fmt.Print(renderComparison(arms))
	if err := writeJSON(out, arms); err != nil {
		return err
	}
	fmt.Printf("\nwrote %s\n", out)
	return nil
}
