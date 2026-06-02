// bench/cmd/cacheprobe/main.go
// cacheprobe sweeps a stable prefix across lengths and prints, for each, the
// prompt_cache_hit_tokens reported on a repeat request. The granularity at
// which hit-tokens jump reveals DeepSeek's effective cache unit, which feeds
// internal/cacheunit.AlignPadding. Manual, needs DEEPSEEK_API_KEY.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

func main() {
	model := flag.String("model", "deepseek-v4-flash", "model id")
	base := flag.String("base-url", "https://api.deepseek.com", "base url")
	minN := flag.Int("min", 900, "min prefix chars")
	maxN := flag.Int("max", 1300, "max prefix chars")
	step := flag.Int("step", 16, "char step")
	flag.Parse()

	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "needs DEEPSEEK_API_KEY")
		os.Exit(1)
	}
	c := llm.NewClient(key, *base)
	ctx := context.Background()

	for n := *minN; n <= *maxN; n += *step {
		sys := strings.Repeat("x", n)
		req := llm.Request{
			Model: *model,
			Messages: []llm.Message{
				{Role: "system", Blocks: []llm.ContentBlock{llm.TextBlock{Text: sys}}},
				{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: "hi"}}},
			},
			Stream:        true,
			StreamOptions: &llm.StreamOptions{IncludeUsage: true},
			Thinking:      llm.ThinkingEnabled(false),
		}
		drain(ctx, c, req)      // warm
		u := drain(ctx, c, req) // measure
		fmt.Printf("chars=%d hit=%d miss=%d\n", n, u.PromptCacheHitTokens, u.PromptCacheMissTokens)
	}
}

func drain(ctx context.Context, c *llm.Client, req llm.Request) llm.Usage {
	ch, err := c.Stream(ctx, req)
	if err != nil {
		return llm.Usage{}
	}
	var u llm.Usage
	for ev := range ch {
		if ev.Type == llm.EventFinish {
			u = ev.Usage
		}
	}
	return u
}
