package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/acp"
)

// pipeConn connects two io.Pipes so we can drive the server from the test.
type pipeConn struct {
	serverIn  *io.PipeWriter // test writes → server reads
	serverOut *io.PipeReader // server writes → test reads
	testW     *acp.FrameWriter
	testR     *acp.FrameReader
}

func newPipeConn() (*pipeConn, io.Reader, io.Writer) {
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	return &pipeConn{
		serverIn:  pw1,
		serverOut: pr2,
		testW:     acp.NewFrameWriter(pw1),
		testR:     acp.NewFrameReader(pr2),
	}, pr1, pw2
}

func sendRequest(w *acp.FrameWriter, id int64, method string, params interface{}) error {
	p, _ := json.Marshal(params)
	req := acp.Request{JSONRPC: "2.0", ID: acp.NewID(id), Method: method, Params: p}
	b, _ := json.Marshal(req)
	return w.WriteFrame(b)
}

func collectFrames(r *acp.FrameReader, count int, timeout time.Duration) ([][]byte, error) {
	ch := make(chan []byte, count)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < count; i++ {
			b, err := r.Read()
			if err != nil {
				return
			}
			ch <- b
		}
	}()
	// Gate close(ch) on the WaitGroup so the goroutine cannot send after close.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
	// Wait for the reader goroutine to finish before closing the channel,
	// preventing a data race between ch <- b and close(ch).
	wg.Wait()
	close(ch)
	var out [][]byte
	for b := range ch {
		out = append(out, b)
	}
	return out, nil
}

func TestACPServerSessionNew(t *testing.T) {
	conn, serverIn, serverOut := newPipeConn()
	sm := acp.NewSessionManager(stubAgentFactory)
	srv := acp.NewACPServer(sm, serverIn, serverOut)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go srv.Serve(ctx)

	sendRequest(conn.testW, 1, "session/new", acp.SessionNewParams{WorkingDir: "/tmp"})
	frames, _ := collectFrames(conn.testR, 1, time.Second)
	if len(frames) == 0 {
		t.Fatal("expected response frame")
	}
	var resp acp.Response
	json.Unmarshal(frames[0], &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if string(resp.ID) != "1" {
		t.Errorf("expected response ID=1, got %s", string(resp.ID))
	}
	var result acp.SessionNewResult
	json.Unmarshal(resp.Result, &result)
	if result.SessionID == "" {
		t.Fatal("expected non-empty sessionId")
	}
}

func TestACPServerSessionPromptStreams(t *testing.T) {
	conn, serverIn, serverOut := newPipeConn()
	sm := acp.NewSessionManager(stubAgentFactory)
	srv := acp.NewACPServer(sm, serverIn, serverOut)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go srv.Serve(ctx)

	// 1. Create session.
	sendRequest(conn.testW, 1, "session/new", acp.SessionNewParams{WorkingDir: "/tmp"})
	frames, _ := collectFrames(conn.testR, 1, time.Second)
	var resp1 acp.Response
	json.Unmarshal(frames[0], &resp1)
	var newRes acp.SessionNewResult
	json.Unmarshal(resp1.Result, &newRes)

	// 2. Send prompt — expect: textDelta notification + done notification + response.
	sendRequest(conn.testW, 2, "session/prompt", acp.SessionPromptParams{
		SessionID: newRes.SessionID, Prompt: "world",
	})
	frames2, _ := collectFrames(conn.testR, 3, 2*time.Second)
	if len(frames2) < 2 {
		t.Fatalf("expected at least 2 frames (textDelta + done), got %d", len(frames2))
	}
	// At least one frame should be a notification with method session/textDelta.
	var sawDelta, sawDone bool
	for _, f := range frames2 {
		var m map[string]json.RawMessage
		json.Unmarshal(f, &m)
		if method, ok := m["method"]; ok {
			var s string
			json.Unmarshal(method, &s)
			if s == "session/textDelta" {
				sawDelta = true
			}
			if s == "session/done" {
				sawDone = true
			}
		}
	}
	if !sawDelta {
		t.Error("expected session/textDelta notification")
	}
	if !sawDone {
		t.Error("expected session/done notification")
	}
	// Verify that exactly one frame is a response (no "method" field) with no error,
	// and that its ID echoes the request ID (2).
	var responseFrames int
	for _, f := range frames2 {
		var m map[string]json.RawMessage
		json.Unmarshal(f, &m)
		if _, hasMethod := m["method"]; !hasMethod {
			responseFrames++
			var resp acp.Response
			json.Unmarshal(f, &resp)
			if resp.Error != nil {
				t.Errorf("unexpected error in response frame: %v", resp.Error)
			}
			if string(resp.ID) != "2" {
				t.Errorf("expected response ID=2, got %s", string(resp.ID))
			}
		}
	}
	if responseFrames != 1 {
		t.Errorf("expected exactly 1 response frame, got %d", responseFrames)
	}
}

func TestACPServerMethodNotFound(t *testing.T) {
	conn, serverIn, serverOut := newPipeConn()
	sm := acp.NewSessionManager(stubAgentFactory)
	srv := acp.NewACPServer(sm, serverIn, serverOut)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go srv.Serve(ctx)

	sendRequest(conn.testW, 99, "unknown/method", nil)
	frames, _ := collectFrames(conn.testR, 1, time.Second)
	if len(frames) == 0 {
		t.Fatal("expected error response")
	}
	var resp acp.Response
	json.Unmarshal(frames[0], &resp)
	if resp.Error == nil || resp.Error.Code != acp.CodeMethodNotFound {
		t.Fatalf("expected MethodNotFound error, got %+v", resp.Error)
	}
	if string(resp.ID) != "99" {
		t.Errorf("expected response ID=99, got %s", string(resp.ID))
	}
}

func TestACPServerSessionCancel(t *testing.T) {
	conn, serverIn, serverOut := newPipeConn()
	sm := acp.NewSessionManager(stubAgentFactory)
	srv := acp.NewACPServer(sm, serverIn, serverOut)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go srv.Serve(ctx)

	// 1. Create a session.
	sendRequest(conn.testW, 1, "session/new", acp.SessionNewParams{WorkingDir: "/tmp"})
	frames, _ := collectFrames(conn.testR, 1, time.Second)
	if len(frames) == 0 {
		t.Fatal("expected response frame for session/new")
	}
	var resp1 acp.Response
	json.Unmarshal(frames[0], &resp1)
	if resp1.Error != nil {
		t.Fatalf("unexpected error from session/new: %v", resp1.Error)
	}
	var newRes acp.SessionNewResult
	json.Unmarshal(resp1.Result, &newRes)
	if newRes.SessionID == "" {
		t.Fatal("expected non-empty sessionId")
	}

	// 2. Cancel the session — expect a non-error response.
	sendRequest(conn.testW, 2, "session/cancel", acp.SessionCancelParams{SessionID: newRes.SessionID})
	frames2, _ := collectFrames(conn.testR, 1, time.Second)
	if len(frames2) == 0 {
		t.Fatal("expected response frame for session/cancel")
	}
	var resp2 acp.Response
	json.Unmarshal(frames2[0], &resp2)
	if resp2.Error != nil {
		t.Fatalf("unexpected error from session/cancel: %v", resp2.Error)
	}
	if string(resp2.ID) != "2" {
		t.Errorf("expected response ID=2, got %s", string(resp2.ID))
	}

	// 3. Assert the session is gone: a subsequent cancel on the same ID
	//    should still return a non-error response (Cancel is idempotent),
	//    but the session must no longer exist in the manager.
	if sm.Has(newRes.SessionID) {
		t.Errorf("session %s should have been removed after cancel", newRes.SessionID)
	}
}
