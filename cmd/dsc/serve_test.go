package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
	"github.com/amemiya02/deepseekcode/internal/mcp"
)

func TestServeBadFlagReturnsError(t *testing.T) {
	err := runServe([]string{"--unknown-flag-xyz"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestServeNoFlagsReturnsError(t *testing.T) {
	// Neither --acp nor --http specified: should error.
	err := runServe([]string{})
	if err == nil {
		t.Fatal("expected error when neither --acp nor --http is given")
	}
}

func TestServeMutuallyExclusiveFlagsReturnsError(t *testing.T) {
	// --acp and --http together are mutually exclusive: should error.
	err := runServe([]string{"--acp", "--http", ":8080"})
	if err == nil {
		t.Fatal("expected error when both --acp and --http are given")
	}
}

// TestServeACPExitsOnEOF verifies that the ACP path reaches the Serve call and
// returns nil when stdin is closed immediately (EOF).  This exercises the
// implementation body (SessionManager + ACPServer construction) rather than
// just flag-parsing.
func TestServeACPExitsOnEOF(t *testing.T) {
	// A pipe whose write end is closed immediately looks like EOF to the reader.
	pr, pw := io.Pipe()
	pw.Close() // immediate EOF

	// Temporarily redirect os.Stdin to the pipe reader for the duration of the call.
	origStdin := stdinForServe
	stdinForServe = pr
	defer func() { stdinForServe = origStdin }()

	err := runServe([]string{"--acp"})
	if err != nil {
		t.Fatalf("expected nil error on EOF, got: %v", err)
	}
}

// TestServeLoopbackNoAuth verifies the serve auth model: a loopback bind serves
// the gateway WITHOUT a bearer token, so GET / must NOT return 401.
func TestServeLoopbackNoAuth(t *testing.T) {
	sm := acp.NewSessionManager(acp.RealAgentFactory)
	handler, token := buildServeHandler(sm, "127.0.0.1:8080", mcp.NewRegistry())
	if token != "" {
		t.Fatalf("loopback bind must not generate a token, got %q", token)
	}

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// No Authorization header.
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("loopback serve must not 401 on GET /, got %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on GET /, got %d", resp.StatusCode)
	}
}

// TestServeRemoteRequiresToken verifies that a non-loopback bind wraps the
// handler in the bearer-token middleware: GET / without a token returns 401,
// and with the correct token returns 200.
func TestServeRemoteRequiresToken(t *testing.T) {
	sm := acp.NewSessionManager(acp.RealAgentFactory)
	handler, token := buildServeHandler(sm, "192.168.1.5:8080", mcp.NewRegistry())
	if token == "" {
		t.Fatal("non-loopback bind must generate a bearer token")
	}

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Without a token: 401.
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("non-loopback serve must 401 without token, got %d", resp.StatusCode)
	}

	// With the correct token: not 401.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusUnauthorized {
		t.Fatalf("non-loopback serve must accept the correct token, got 401")
	}
}

// TestResolveBindAddr verifies the safe-by-default bind logic: wildcard and
// non-loopback hosts are forced to loopback unless --http-allow-remote is set.
func TestResolveBindAddr(t *testing.T) {
	cases := []struct {
		name        string
		addr        string
		allowRemote bool
		want        string
		wantErr     bool
	}{
		{name: "bare port forced to loopback", addr: ":8080", allowRemote: false, want: "127.0.0.1:8080"},
		{name: "wildcard ipv4 forced to loopback", addr: "0.0.0.0:8080", allowRemote: false, want: "127.0.0.1:8080"},
		{name: "wildcard ipv6 forced to loopback", addr: "[::]:8080", allowRemote: false, want: "127.0.0.1:8080"},
		{name: "explicit lan host forced to loopback", addr: "192.168.1.5:8080", allowRemote: false, want: "127.0.0.1:8080"},
		{name: "loopback preserved", addr: "127.0.0.1:9090", allowRemote: false, want: "127.0.0.1:9090"},
		{name: "localhost preserved", addr: "localhost:9090", allowRemote: false, want: "localhost:9090"},
		{name: "wildcard honored with allow-remote", addr: "0.0.0.0:8080", allowRemote: true, want: "0.0.0.0:8080"},
		{name: "bare port wildcard with allow-remote", addr: ":8080", allowRemote: true, want: ":8080"},
		{name: "lan host honored with allow-remote", addr: "192.168.1.5:8080", allowRemote: true, want: "192.168.1.5:8080"},
		{name: "empty addr errors", addr: "", allowRemote: false, wantErr: true},
		{name: "missing port errors", addr: "127.0.0.1", allowRemote: false, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveBindAddr(tc.addr, tc.allowRemote)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for addr %q, got %q", tc.addr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for addr %q: %v", tc.addr, err)
			}
			if got != tc.want {
				t.Fatalf("resolveBindAddr(%q, %v) = %q, want %q", tc.addr, tc.allowRemote, got, tc.want)
			}
		})
	}
}
