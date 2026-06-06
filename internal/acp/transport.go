package acp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FrameWriter writes Content-Length framed JSON-RPC messages to w.
// Format: "Content-Length: N\r\n\r\n<N bytes>".
// Mirrors internal/lsp/transport.go framing exactly.
type FrameWriter struct {
	w io.Writer
}

// NewFrameWriter creates a FrameWriter.
func NewFrameWriter(w io.Writer) *FrameWriter {
	return &FrameWriter{w: w}
}

// WriteFrame frames and writes p.
func (fw *FrameWriter) WriteFrame(p []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(p))
	if _, err := io.WriteString(fw.w, header); err != nil {
		return err
	}
	_, err := fw.w.Write(p)
	return err
}

// FrameReader reads Content-Length framed messages from r.
type FrameReader struct {
	br *bufio.Reader
}

// NewFrameReader creates a FrameReader.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{br: bufio.NewReader(r)}
}

// Read reads one framed message, returning the raw body bytes.
func (fr *FrameReader) Read() ([]byte, error) {
	var contentLength int
	for {
		line, err := fr.br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("acp: bad Content-Length: %w", err)
			}
			contentLength = n
		}
		// ignore unknown headers
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("acp: missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(fr.br, body); err != nil {
		return nil, err
	}
	return body, nil
}
