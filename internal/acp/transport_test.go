package acp_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

func TestTransportWriteRead(t *testing.T) {
	var buf bytes.Buffer
	w := acp.NewFrameWriter(&buf)
	r := acp.NewFrameReader(&buf)

	payload := []byte(`{"jsonrpc":"2.0","method":"ping"}`)
	if err := w.WriteFrame(payload); err != nil {
		t.Fatal(err)
	}

	got, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("want %s, got %s", payload, got)
	}
}

func TestTransportMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	w := acp.NewFrameWriter(&buf)
	r := acp.NewFrameReader(&buf)

	messages := []string{`{"id":1}`, `{"id":2}`, `{"id":3}`}
	for _, m := range messages {
		if err := w.WriteFrame([]byte(m)); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range messages {
		got, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("want %q got %q", want, got)
		}
	}
}

func TestTransportEOF(t *testing.T) {
	r := acp.NewFrameReader(strings.NewReader(""))
	_, err := r.Read()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

// TestTransportNoSpaceHeader verifies that "Content-Length:42" (no space after
// colon, valid per HTTP/1.1 field syntax) is accepted and that a truncated body
// returns an error rather than silently succeeding.
func TestTransportNoSpaceHeader(t *testing.T) {
	t.Run("no_space_valid_body", func(t *testing.T) {
		body := `{"id":99}`
		frame := fmt.Sprintf("Content-Length:%d\r\n\r\n%s", len(body), body)
		r := acp.NewFrameReader(strings.NewReader(frame))
		got, err := r.Read()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != body {
			t.Fatalf("want %q, got %q", body, got)
		}
	})

	t.Run("truncated_body", func(t *testing.T) {
		// Announce 10 bytes but supply only 3 — io.ReadFull must return an error.
		frame := "Content-Length:10\r\n\r\nabc"
		r := acp.NewFrameReader(strings.NewReader(frame))
		_, err := r.Read()
		if err == nil {
			t.Fatal("expected error for truncated body, got nil")
		}
	})
}
