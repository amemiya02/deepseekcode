package main

import (
	"flag"
	"os"

	"github.com/amemiya02/deepseekcode/internal/bootstrap"
)

func runInit() error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite existing files")
	fs.Parse(os.Args[2:])

	return bootstrap.Run(bootstrap.InitOptions{
		Force: *force,
		CWD:   "",
	})
}
