package acp

import (
	"context"
	"encoding/json"
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
			s.sendError(reqID, CodeInternalError, err.Error())
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
	b, _ := json.Marshal(result)
	resp := Response{JSONRPC: JSONRPC20, ID: id, Result: b}
	s.writeFrame(resp)
}

func (s *ACPServer) sendError(id ID, code int, msg string) {
	resp := Response{
		JSONRPC: JSONRPC20,
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	}
	s.writeFrame(resp)
}

func (s *ACPServer) sendNotification(method string, params interface{}) {
	b, _ := json.Marshal(params)
	n := Notification{JSONRPC: JSONRPC20, Method: method, Params: b}
	s.writeFrame(n)
}

func (s *ACPServer) writeFrame(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.writer.WriteFrame(b)
}
