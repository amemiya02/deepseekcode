package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// StdioTransport is a Transport that speaks JSON-RPC 2.0 over a
// subprocess's stdin/stdout. Each JSON message is a single newline-
// terminated line. Stderr is not parsed; the subprocess inherits
// the parent's stderr for diagnostics.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	nextID atomic.Int64

	// pending maps request IDs to response channels. The reader
	// goroutine delivers responses by ID.
	pending   map[int64]chan jsonRPCResponse
	pendingMu sync.Mutex

	readerDone chan struct{} // closed when the reader goroutine exits
	closeOnce  sync.Once
	closeErr   error
}

// NewStdioTransport spawns command with args and optional env, then
// starts a reader goroutine to consume lines from its stdout.
func NewStdioTransport(ctx context.Context, command string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// Stderr inherits from the parent so diagnostics are visible.

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	t := &StdioTransport{
		cmd:        cmd,
		stdin:      stdin,
		pending:    make(map[int64]chan jsonRPCResponse),
		readerDone: make(chan struct{}),
	}
	t.nextID.Store(0)

	go t.readLoop(stdout)
	return t, nil
}

// Send marshals a request, writes it to stdin, and blocks until the
// matching response arrives or ctx is cancelled.
func (t *StdioTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
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

	t.mu.Lock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", body)
	t.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-t.readerDone:
		return nil, errors.New("mcp server exited unexpectedly")
	}
}

// Notify sends a notification (id-less request, no response).
func (t *StdioTransport) Notify(ctx context.Context, method string, params any) error {
	req := jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	t.mu.Lock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", body)
	t.mu.Unlock()
	if err != nil {
		return fmt.Errorf("write notification: %w", err)
	}
	return nil
}

// Close kills the subprocess and waits for it to exit.
func (t *StdioTransport) Close() error {
	t.closeOnce.Do(func() {
		// Close stdin so the subprocess knows to shut down.
		_ = t.stdin.Close()
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		t.closeErr = t.cmd.Wait()
	})
	return t.closeErr
}

// newPipeTransport creates a StdioTransport backed by io.Pipe instead
// of a subprocess. Used by tests; callers must arrange for the other
// end of the pipe to speak JSON-RPC.
func newPipeTransport(r io.ReadCloser, w io.WriteCloser) *StdioTransport {
	t := &StdioTransport{
		stdin:      w,
		pending:    make(map[int64]chan jsonRPCResponse),
		readerDone: make(chan struct{}),
	}
	t.nextID.Store(0)
	t.closeOnce.Do(func() {}) // Close is a no-op; caller owns pipe lifecycle
	go t.readLoop(r)
	return t
}

// readLoop reads lines from stdout and routes responses by ID.
func (t *StdioTransport) readLoop(stdout io.ReadCloser) {
	defer close(t.readerDone)
	defer stdout.Close()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // skip malformed lines
		}

		// If ID is 0 or response has no matching pending request,
		// treat as a server-initiated notification (ignore for now).
		if resp.ID == 0 {
			continue
		}

		t.pendingMu.Lock()
		ch, ok := t.pending[resp.ID]
		t.pendingMu.Unlock()
		if ok {
			ch <- resp
		}
	}

	// Scanner errored or EOF: drain pending channels so no Send
	// blocks forever.
	t.pendingMu.Lock()
	for id, ch := range t.pending {
		ch <- jsonRPCResponse{
			ID: id,
			Error: &jsonRPCError{
				Code:    -32000,
				Message: "mcp server exited",
			},
		}
	}
	t.pendingMu.Unlock()
}
