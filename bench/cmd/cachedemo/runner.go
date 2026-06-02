// bench/cmd/cachedemo/runner.go
package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// runArmLive issues each request and drains its event stream, capturing the
// authoritative usage frame DeepSeek emits on EventFinish. Network-only; not
// unit-tested (exercised by `make demo-cache`).
func runArmLive(ctx context.Context, c *llm.Client, reqs []llm.Request) ([]llm.Usage, error) {
	out := make([]llm.Usage, 0, len(reqs))
	for _, req := range reqs {
		ch, err := c.Stream(ctx, req)
		if err != nil {
			return nil, err
		}
		var u llm.Usage
		for ev := range ch {
			if ev.Type == llm.EventError && ev.Err != nil {
				return nil, ev.Err
			}
			if ev.Type == llm.EventFinish {
				u = ev.Usage
			}
		}
		out = append(out, u)
	}
	return out, nil
}

// loadUsageFixture replays a committed []llm.Usage so the demo (and its numbers)
// are reproducible without an API key.
func loadUsageFixture(path string) ([]llm.Usage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var u []llm.Usage
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return u, nil
}
