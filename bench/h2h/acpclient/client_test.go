package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("ACP_FAKE_AGENT") == "1" {
		runFakeAgent()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeAgent reads ndjson JSON-RPC from stdin and scripts the agent
// side: initialize -> ack; session/new -> s1; session/prompt -> two
// session/update usage notifications then the response.
func runFakeAgent() {
	sc := bufio.NewScanner(os.Stdin)
	if os.Getenv("ACP_FAKE_AGENT_CRASH") == "1" {
		sc.Scan() // swallow one request, then die without replying
		return
	}
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	emit := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(out, "%s\n", b)
		out.Flush()
	}
	for sc.Scan() {
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		switch req.Method {
		case "initialize":
			emit(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"protocolVersion": 1}})
		case "session/new":
			emit(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"sessionId": "s1"}})
		case "session/prompt":
			for i, miss := range []int{0, 500} {
				emit(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": "s1",
					"update": map[string]any{"usage": map[string]any{
						"prompt_cache_hit_tokens": 100 + i, "prompt_cache_miss_tokens": miss, "completion_tokens": 10}},
				}})
			}
			emit(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": map[string]any{"stopReason": "end_turn"}})
		}
	}
}

func TestClientHandshakeAndPrompt(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Start(context.Background(), exe, []string{"-test.run=NONE"}, []string{"ACP_FAKE_AGENT=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sid, err := c.NewSession("/tmp")
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	usage, err := c.Prompt(sid, "fix the bug")
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if len(usage) != 2 || usage[1].MissTokens != 500 {
		t.Fatalf("usage not captured from session/update stream: %+v", usage)
	}
}

// TestCallErrorsWhenAgentCrashes is the regression test for the ACP
// deadlock: a call in flight when the agent dies must return an error
// promptly instead of blocking forever.
func TestCallErrorsWhenAgentCrashes(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c, err := Start(context.Background(), exe, []string{"-test.run=NONE"},
		[]string{"ACP_FAKE_AGENT=1", "ACP_FAKE_AGENT_CRASH=1"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() // reap the exited child; error irrelevant here
	done := make(chan error, 1)
	go func() { done <- c.Initialize() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from call after agent crash, got nil")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("call deadlocked after agent crash")
	}
}

// scriptedStdin makes call() deterministic for the drain test: by the
// time call() reaches its select, a response is already buffered AND
// the client is already closed, so whichever select arm wins must
// still return the response.
type scriptedStdin struct{ c *Client }

func (s *scriptedStdin) Write(p []byte) (int, error) {
	s.c.pending[1] <- rpcMsg{Result: json.RawMessage(`{"ok":true}`)} // call() registered id 1 before writing
	close(s.c.closed)
	return len(p), nil
}
func (s *scriptedStdin) Close() error { return nil }

// TestCallDrainsBufferedResponseAfterClose pins the closed-path drain:
// a response delivered just before the process exit must not be
// reported as a crash. Iterated because the select between the two
// ready arms is randomized.
func TestCallDrainsBufferedResponseAfterClose(t *testing.T) {
	for i := 0; i < 50; i++ {
		c := &Client{pending: map[int64]chan rpcMsg{}, closed: make(chan struct{})}
		c.stdin = &scriptedStdin{c: c}
		res, err := c.call("x", nil)
		if err != nil {
			t.Fatalf("iteration %d: buffered response lost on closed path: %v", i, err)
		}
		if string(res) != `{"ok":true}` {
			t.Fatalf("iteration %d: wrong result: %s", i, res)
		}
	}
}
