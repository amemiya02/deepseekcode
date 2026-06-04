package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/gateway"
	"github.com/amemiya02/deepseekcode/internal/session"
	"github.com/amemiya02/deepseekcode/internal/snapshots"
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

	// HTTP+SSE mode: serve the web gateway (SPA + /v1 API), not the bare ACP
	// gateway, so a browser can drive the agent end-to-end.
	bindAddr, err := resolveBindAddr(*httpAddr, *allowRemote)
	if err != nil {
		return err
	}

	// Build the gateway handler with the auth model applied for this bind. On a
	// loopback bind (the default) it is served WITHOUT a bearer token so a local
	// browser works out of the box; on a non-loopback bind the token is required
	// and printed.
	handler, token := buildServeHandler(sm, bindAddr)
	if token != "" {
		fmt.Fprintf(os.Stderr, "dsc serve: WARNING binding to non-loopback address %s — the agent gateway is reachable from the network\n", bindAddr)
		fmt.Fprintf(os.Stderr, "Gateway auth token: %s\n", token)
	}

	server := &http.Server{Addr: bindAddr, Handler: handler}
	fmt.Fprintf(os.Stderr, "dsc serve: listening on %s\n", bindAddr)

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

// buildServeHandler constructs the gateway handler for bindAddr and applies the
// serve auth model. On a loopback bind it returns the bare gateway handler and
// an empty token (no auth). On a non-loopback bind it wraps the handler in the
// bearer-token middleware and returns the generated token so the caller can
// print it. Factored out of runServe so the auth decision is unit-testable
// without binding a socket or driving a signal.
func buildServeHandler(sm *acp.SessionManager, bindAddr string) (http.Handler, string) {
	// Wire the same capabilities the in-process desktop gateway (DefaultHandler)
	// gets, so `dsc serve` is not a degraded shell: the process working dir as the
	// workspace root (makes /v1/files, /v1/file and /v1/add-to-chat work instead of
	// 400 "no workspace root"), a session.Store and a snapshots.Manager (enable the
	// checkpoint/rewind/fork endpoints). The session store is best-effort: if it
	// cannot open we warn and keep serving — file/agent features still work, only
	// checkpoint/rewind degrade to 501.
	wd, _ := os.Getwd()
	opts := []gateway.Option{
		gateway.WithWorkspaceRoot(wd),
		gateway.WithSnapshots(snapshots.New("")),
	}
	if store, err := session.Open(""); err != nil {
		fmt.Fprintf(os.Stderr, "dsc serve: session store unavailable (%v) — checkpoint/rewind disabled\n", err)
	} else {
		opts = append(opts, gateway.WithStore(store))
	}

	handler := gateway.NewHandler(sm, os.Getenv("DEEPSEEKCODE_TRACE_JSONL"), opts...)
	if isLoopbackHost(hostOf(bindAddr)) {
		return handler, ""
	}
	token := newServeToken()
	return requireBearer(token, handler), token
}

// newServeToken returns a cryptographically random 32-byte token, hex-encoded.
// It is used to gate a non-loopback HTTP gateway bind. crypto/rand.Read never
// returns an error on supported platforms; if it somehow does, panic rather
// than fall back to a predictable token.
func newServeToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("serve: generating gateway token: %v", err))
	}
	return hex.EncodeToString(b)
}

// requireBearer wraps next so every request must present
// "Authorization: Bearer <token>" (constant-time compared). Used only on a
// non-loopback bind; a loopback bind serves next unwrapped.
func requireBearer(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, prefix) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got := strings.TrimPrefix(h, prefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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
