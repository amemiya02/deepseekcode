// Package acpclient is a minimal Agent Client Protocol (JSON-RPC 2.0
// over stdio, newline-delimited) client — just enough to drive
// `reasonix acp --yolo` for the h2h benchmark. Method names verified
// against the live binary (Task 5 Step 1).
package acpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Usage represents token usage harvested from session/update notifications.
type Usage struct {
	HitTokens  int `json:"prompt_cache_hit_tokens"`
	MissTokens int `json:"prompt_cache_miss_tokens"`
	OutTokens  int `json:"completion_tokens"`
}

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client is a minimal ACP (JSON-RPC 2.0 over stdio) client.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan rpcMsg
	usage   []Usage
	usageMu sync.Mutex
	closed  chan struct{}
}

// Start launches the agent process and begins the read loop.
func Start(ctx context.Context, bin string, args, extraEnv []string) (*Client, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(cmd.Environ(), extraEnv...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{cmd: cmd, stdin: stdin, pending: map[int64]chan rpcMsg{}, closed: make(chan struct{})}
	go c.readLoop(stdout)
	return c, nil
}

func (c *Client) readLoop(r io.Reader) {
	defer close(c.closed)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var msg rpcMsg
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}
		switch {
		case msg.ID != nil && msg.Method == "": // response to one of our calls
			c.mu.Lock()
			if ch, ok := c.pending[*msg.ID]; ok {
				ch <- msg
				delete(c.pending, *msg.ID)
			}
			c.mu.Unlock()
		case msg.Method == "session/update": // notification: harvest usage
			var p struct {
				Update struct {
					Usage *Usage `json:"usage"`
				} `json:"update"`
			}
			if json.Unmarshal(msg.Params, &p) == nil && p.Update.Usage != nil {
				c.usageMu.Lock()
				c.usage = append(c.usage, *p.Update.Usage)
				c.usageMu.Unlock()
			}
		case msg.ID != nil && msg.Method == "session/request_permission":
			// --yolo should suppress these; if one arrives, select the
			// first offered option so the run never wedges.
			var p struct {
				Options []struct {
					OptionID string `json:"optionId"`
				} `json:"options"`
			}
			json.Unmarshal(msg.Params, &p)
			opt := "allow"
			if len(p.Options) > 0 {
				opt = p.Options[0].OptionID
			}
			resp, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": *msg.ID,
				"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": opt}},
			})
			c.write(resp)
		}
	}
}

func (c *Client) write(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.stdin.Write(append(b, '\n'))
	return err
}

func (c *Client) call(method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan rpcMsg, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err := c.write(req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	var msg rpcMsg
	select {
	case msg = <-ch:
	case <-c.closed:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		// readLoop has exited; any response it delivered before exiting
		// is already buffered in ch, so a non-blocking drain is reliable.
		select {
		case msg = <-ch:
		default:
			return nil, fmt.Errorf("%s: client closed (process may have crashed)", method)
		}
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("%s: rpc %d: %s", method, msg.Error.Code, msg.Error.Message)
	}
	return msg.Result, nil
}

// Initialize performs the ACP handshake.
func (c *Client) Initialize() error {
	_, err := c.call("initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{"fs": map[string]any{"readTextFile": false, "writeTextFile": false}},
	})
	return err
}

// NewSession creates a new agent session with the given working directory.
func (c *Client) NewSession(cwd string) (string, error) {
	res, err := c.call("session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}})
	if err != nil {
		return "", err
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return "", err
	}
	return out.SessionID, nil
}

// Prompt sends one user turn and blocks until the turn completes,
// returning the usage frames harvested from session/update during it.
func (c *Client) Prompt(sessionID, text string) ([]Usage, error) {
	c.usageMu.Lock()
	c.usage = nil
	c.usageMu.Unlock()
	_, err := c.call("session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return append([]Usage(nil), c.usage...), err
}

// Close terminates the agent process.
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}
