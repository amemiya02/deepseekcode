package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Transport sends JSON-RPC 2.0 requests over stdio using LSP's
// Content-Length header framing. It is not safe for concurrent use;
// callers must serialise Send / Notify calls.
type Transport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	nextID atomic.Int64

	pending   map[int64]chan jsonRPCResponse
	pendingMu sync.Mutex

	// OnNotification is called for server→client notifications (e.g.
	// textDocument/publishDiagnostics). Must be set before the transport
	// is used.
	OnNotification func(method string, params json.RawMessage)

	readerDone chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

// NewTransport spawns command with args and starts the reader goroutine.
func NewTransport(ctx context.Context, command string, args []string) (*Transport, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	t := &Transport{
		cmd:        cmd,
		stdin:      stdin,
		pending:    make(map[int64]chan jsonRPCResponse),
		readerDone: make(chan struct{}),
	}
	t.nextID.Store(0)

	go t.readLoop(stdout)
	return t, nil
}

// Send writes a request and blocks until the matching response arrives.
func (t *Transport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ch := make(chan jsonRPCResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()

	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
	}()

	if err := t.write(body); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("lsp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-t.readerDone:
		return nil, errors.New("lsp server exited unexpectedly")
	}
}

// Notify sends a notification (no response expected).
func (t *Transport) Notify(ctx context.Context, method string, params any) error {
	req := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	if err := t.write(body); err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

// Close kills the subprocess and waits for it to exit.
func (t *Transport) Close() error {
	t.closeOnce.Do(func() {
		_ = t.stdin.Close()
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		t.closeErr = t.cmd.Wait()
	})
	return t.closeErr
}

func (t *Transport) write(body []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := io.WriteString(t.stdin, header); err != nil {
		return err
	}
	_, err := t.stdin.Write(body)
	return err
}

// readLoop reads Content-Length-framed messages from stdout.
func (t *Transport) readLoop(stdout io.ReadCloser) {
	defer close(t.readerDone)
	defer stdout.Close()

	rd := bufio.NewReader(stdout)
	for {
		body, err := readLSPMessage(rd)
		if err != nil {
			break
		}
		t.dispatch(body)
	}

	// Drain pending channels so no Send blocks forever.
	t.pendingMu.Lock()
	for id, ch := range t.pending {
		ch <- jsonRPCResponse{
			ID: id,
			Error: &jsonRPCError{
				Code:    -32000,
				Message: "lsp server exited",
			},
		}
	}
	t.pendingMu.Unlock()
}

// readLSPMessage reads a single Content-Length-framed message.
func readLSPMessage(rd *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %q", v)
			}
		}
	}
	if contentLength <= 0 {
		return nil, errors.New("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(rd, body); err != nil {
		return nil, err
	}
	return body, nil
}

// newPipeTransport creates a Transport backed by io pipes instead of
// a subprocess. Used by tests.
func newPipeTransport(r io.ReadCloser, w io.WriteCloser) *Transport {
	t := &Transport{
		stdin:      w,
		pending:    make(map[int64]chan jsonRPCResponse),
		readerDone: make(chan struct{}),
	}
	t.nextID.Store(0)
	t.closeOnce.Do(func() {})
	go t.readLoop(r)
	return t
}

// dispatch routes a received message to the appropriate pending channel
// or to the notification callback. Uses a probe-based approach to
// distinguish three JSON-RPC 2.0 shapes:
//   - response:   has "id", no "method"
//   - notification: no "id", has "method"
//   - server→client request: has both "id" and "method" → reply MethodNotFound
func (t *Transport) dispatch(body []byte) {
	var probe struct {
		ID     *int64 `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return
	}

	hasID := probe.ID != nil
	hasMethod := probe.Method != ""

	switch {
	case hasID && !hasMethod:
		// Response to one of our requests.
		var resp jsonRPCResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return
		}
		t.pendingMu.Lock()
		ch, ok := t.pending[resp.ID]
		t.pendingMu.Unlock()
		if ok {
			ch <- resp
		}

	case !hasID && hasMethod:
		// Server→client notification.
		if t.OnNotification != nil {
			var notif jsonRPCNotification
			if err := json.Unmarshal(body, &notif); err != nil {
				return
			}
			params, _ := json.Marshal(notif.Params)
			t.OnNotification(notif.Method, params)
		}

	case hasID && hasMethod:
		// Server→client request. Reply MethodNotFound.
		errResp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      *probe.ID,
			Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
		}
		errBody, _ := json.Marshal(errResp)
		_ = t.write(errBody)
	}
}
