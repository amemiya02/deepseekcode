package main

import (
	"io"
	"testing"
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
