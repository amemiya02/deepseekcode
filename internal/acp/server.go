package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ACPServer dispatches JSON-RPC requests over stdio.
type ACPServer struct {
	sm     *SessionManager
	reader *FrameReader
	writer *FrameWriter
	mu     sync.Mutex // guards writer
}

// NewACPServer creates an ACPServer reading from r and writing to w.
func NewACPServer(sm *SessionManager, r io.Reader, w io.Writer) *ACPServer {
	return &ACPServer{
		sm:     sm,
		reader: NewFrameReader(r),
		writer: NewFrameWriter(w),
	}
}

// Serve reads and dispatches requests until ctx is cancelled or r returns EOF.
func (s *ACPServer) Serve(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		body, err := s.reader.Read()
		if err != nil {
			return
		}
		go s.dispatch(ctx, body)
	}
}

// resolveID returns the request ID suitable for use in a response.
// Per JSON-RPC 2.0 §5, if the id cannot be determined (nil pointer),
// the response MUST include id: null.
func resolveID(p *ID) ID {
	if p == nil {
		return ID("null")
	}
	return *p
}

func (s *ACPServer) dispatch(ctx context.Context, body []byte) {
	// Decode enough to get id and method.
	var peek struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      *ID             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &peek); err != nil {
		return // malformed — drop
	}

	reqID := resolveID(peek.ID)

	switch peek.Method {
	case "session/new":
		var p SessionNewParams
		if err := json.Unmarshal(peek.Params, &p); err != nil {
			s.sendError(reqID, CodeInvalidParams, err.Error())
			return
		}
		id, err := s.sm.NewSession(ctx, p.WorkingDir)
		if err != nil {
			s.sendError(reqID, CodeInternalError, err.Error())
			return
		}
		s.sendResult(reqID, SessionNewResult{SessionID: id})

	case "session/prompt":
		var p SessionPromptParams
		if err := json.Unmarshal(peek.Params, &p); err != nil {
			s.sendError(reqID, CodeInvalidParams, err.Error())
			return
		}
		// dispatch is already running in a goroutine spawned by Serve;
		// run the blocking prompt call directly here (no extra goroutine needed).
		err := s.sm.Prompt(ctx, p.SessionID, p.Prompt, func(ev AgentEvent) {
			switch ev.Kind {
			case EventKindTextDelta:
				s.sendNotification("session/textDelta", TextDeltaParams{
					SessionID: p.SessionID, Delta: ev.Text,
				})
			case EventKindInfo:
				s.sendNotification("session/info", InfoParams{
					SessionID: p.SessionID, Text: ev.Text,
				})
			case EventKindDone:
				var errPtr *string
				if ev.Err != nil {
					msg := ev.Err.Error()
					errPtr = &msg
				}
				s.sendNotification("session/done", DoneParams{
					SessionID:  p.SessionID,
					StopReason: ev.StopReason,
					Error:      errPtr,
				})
			}
		})
		if err != nil {
			if errors.Is(err, ErrSessionNotFound) {
				s.sendError(reqID, CodeInvalidParams, err.Error())
			} else {
				s.sendError(reqID, CodeInternalError, err.Error())
			}
			return
		}
		s.sendResult(reqID, struct{}{})

	case "session/cancel":
		var p SessionCancelParams
		if err := json.Unmarshal(peek.Params, &p); err != nil {
			s.sendError(reqID, CodeInvalidParams, err.Error())
			return
		}
		s.sm.Cancel(p.SessionID)
		s.sendResult(reqID, struct{}{})

	default:
		s.sendError(reqID, CodeMethodNotFound, "method not found: "+peek.Method)
	}
}

func (s *ACPServer) sendResult(id ID, result interface{}) {
	b, err := json.Marshal(result)
	if err != nil {
		// A result struct we control failed to marshal; surface an internal error.
		s.sendError(id, CodeInternalError, "acp: marshal result: "+err.Error())
		return
	}
	resp := Response{JSONRPC: JSONRPC20, ID: id, Result: b}
	_ = s.writeFrame(resp)
}

func (s *ACPServer) sendError(id ID, code int, msg string) {
	resp := Response{
		JSONRPC: JSONRPC20,
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
	_ = s.writeFrame(resp)
}

func (s *ACPServer) sendNotification(method string, params interface{}) {
	b, err := json.Marshal(params)
	if err != nil {
		// Notification params we control failed to marshal; drop the
		// notification rather than emit a malformed frame.
		return
	}
	n := Notification{JSONRPC: JSONRPC20, Method: method, Params: b}
	_ = s.writeFrame(n)
}

// writeFrame marshals v and writes it as a single framed message. It returns
// the marshal or write error instead of silently discarding it so callers can
// react (and so a marshalling regression is observable, not swallowed).
func (s *ACPServer) writeFrame(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("acp: marshal frame: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writer.WriteFrame(b); err != nil {
		return fmt.Errorf("acp: write frame: %w", err)
	}
	return nil
}
