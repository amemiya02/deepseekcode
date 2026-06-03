package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
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
	allowRemote := fs.Bool("http-allow-remote", false, "allow the HTTP gateway to bind a non-loopback address (exposes the agent to the network)")
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
	bindAddr, err := resolveBindAddr(*httpAddr, *allowRemote)
	if err != nil {
		return err
	}

	gw := acp.NewHTTPGateway(sm)
	server := &http.Server{Addr: bindAddr, Handler: gw}
	if *allowRemote && !isLoopbackHost(hostOf(bindAddr)) {
		fmt.Fprintf(os.Stderr, "dsc serve: WARNING binding to non-loopback address %s — the agent gateway is reachable from the network\n", bindAddr)
	}
	fmt.Fprintf(os.Stderr, "dsc serve: listening on %s\n", bindAddr)
	fmt.Fprintf(os.Stderr, "Gateway auth token: %s\n", gw.Token())

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

// resolveBindAddr decides the concrete listen address for the HTTP gateway.
//
// By default the gateway is bound to loopback so a network host cannot drive an
// agent that executes tools on the operator's machine. A wildcard host
// (empty, "0.0.0.0", or "::") or any explicit non-loopback host is rewritten to
// 127.0.0.1 unless allowRemote is set. When allowRemote is set, a wildcard host
// is left as-is and explicit hosts are honoured.
func resolveBindAddr(addr string, allowRemote bool) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("serve: empty --http address")
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Accept the bare ":PORT" form and anything else SplitHostPort rejects
		// only if it looks like a lone port; otherwise surface the error.
		return "", fmt.Errorf("serve: invalid --http address %q: %w", addr, err)
	}
	if port == "" {
		return "", fmt.Errorf("serve: --http address %q is missing a port", addr)
	}

	wildcard := host == "" || host == "0.0.0.0" || host == "::"

	if allowRemote {
		// Operator explicitly opted in. Honour the requested host as-is,
		// preserving a wildcard bind.
		return net.JoinHostPort(host, port), nil
	}

	// Safe-by-default: a wildcard or any non-loopback host is forced to
	// loopback so the gateway is not exposed to the network.
	if wildcard || !isLoopbackHost(host) {
		return net.JoinHostPort("127.0.0.1", port), nil
	}

	return net.JoinHostPort(host, port), nil
}

// hostOf returns the host portion of a host:port address, or "" if it cannot be
// parsed (best-effort, used only for a warning message).
func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return host
}

// isLoopbackHost reports whether host refers to the loopback interface. A
// non-IP host (e.g. "localhost") is treated as loopback.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Unknown hostname: do not assume loopback.
		return false
	}
	return ip.IsLoopback()
}
