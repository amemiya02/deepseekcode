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
