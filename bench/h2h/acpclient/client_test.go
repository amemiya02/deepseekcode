package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
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
