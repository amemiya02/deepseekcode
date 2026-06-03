package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

// stdinForServe is the reader used by the ACP path. It defaults to os.Stdin
// and can be overridden in tests to simulate EOF without closing the real stdin.
var stdinForServe io.Reader = os.Stdin

// runServe is the entry point for `dsc serve`.
// args is os.Args[2:] (everything after "serve").
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	useACP := fs.Bool("acp", false, "run ACP JSON-RPC server over stdio")
	httpAddr := fs.String("http", "", "run HTTP+SSE gateway on this address (e.g. :8080)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if !*useACP && *httpAddr == "" {
		return fmt.Errorf("serve: specify --acp or --http :PORT")
	}
	if *useACP && *httpAddr != "" {
		return fmt.Errorf("serve: --acp and --http are mutually exclusive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sm := acp.NewSessionManager(acp.RealAgentFactory)

	if *useACP {
		srv := acp.NewACPServer(sm, stdinForServe, os.Stdout)
		srv.Serve(ctx)
		return nil
	}

	// HTTP+SSE mode.
	gw := acp.NewHTTPGateway(sm)
	server := &http.Server{Addr: *httpAddr, Handler: gw}
	fmt.Fprintf(os.Stderr, "dsc serve: listening on %s\n", *httpAddr)

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return server.Shutdown(context.Background())
	}
}
