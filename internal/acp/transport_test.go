package acp_test

import (
	"bytes"
	"errors"
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
	if err := w.Write(payload); err != nil {
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
		if err := w.Write([]byte(m)); err != nil {
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
