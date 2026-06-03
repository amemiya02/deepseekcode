package main

import (
	"flag"
	"fmt"

	"github.com/amemiya02/deepseekcode/internal/traceinspect"
)

func runTrace(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dsc trace <inspect|diff-body> ...")
	}
	switch args[0] {
	case "inspect":
		out, err := runTraceInspect(args[1:])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	case "diff-body":
		out, err := runTraceDiffBody(args[1:])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	default:
		return fmt.Errorf("unknown trace command %q (usage: dsc trace <inspect|diff-body> ...)", args[0])
	}
}

func runTraceInspect(args []string) (string, error) {
	fs := flag.NewFlagSet("trace inspect", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 1 {
		return "", fmt.Errorf("usage: dsc trace inspect TRACE.jsonl")
	}
	rep, err := traceinspect.InspectFile(fs.Arg(0))
	if err != nil {
		return "", err
	}
	return traceinspect.RenderText(rep), nil
}

func runTraceDiffBody(args []string) (string, error) {
	fs := flag.NewFlagSet("trace diff-body", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 2 {
		return "", fmt.Errorf("usage: dsc trace diff-body TURN_A.json TURN_B.json")
	}
	d, err := traceinspect.DiffBodyFiles(fs.Arg(0), fs.Arg(1))
	if err != nil {
		return "", err
	}
	return traceinspect.RenderBodyDiff(d), nil
}
