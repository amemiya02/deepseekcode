package main

import (
	"flag"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

func runCache(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dsc cache explain TRACE.jsonl")
	}
	switch args[0] {
	case "explain":
		out, err := runCacheExplain(args[1:])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	default:
		return fmt.Errorf("unknown cache command %q (usage: dsc cache explain TRACE.jsonl)", args[0])
	}
}

func runCacheExplain(args []string) (string, error) {
	fs := flag.NewFlagSet("cache explain", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 1 {
		return "", fmt.Errorf("usage: dsc cache explain TRACE.jsonl")
	}
	ledger, err := traceinspect.ExplainFile(fs.Arg(0))
	if err != nil {
		return "", err
	}
	return traceinspect.RenderExplainText(ledger), nil
}
