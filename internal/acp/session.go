package acp

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// EventKind identifies what kind of event came from an AgentRunner.
type EventKind int

const (
	EventKindTextDelta EventKind = iota
	EventKindInfo
	EventKindDone
)

// AgentEvent is the unified event from an AgentRunner.
type AgentEvent struct {
	Kind       EventKind
	Text       string // for TextDelta and Info
	StopReason string // for Done
	Err        error  // for Done
}

// AgentRunner is the interface the acp layer uses to drive an agent.
// It wraps internal/agent.Agent without importing it directly so the
// acp package stays decoupled and testable with stubs.
type AgentRunner interface {
	// Run executes the agent with the given prompt. It calls onEvent
	// synchronously for each event and returns when the agent is done.
	Run(ctx context.Context, userPrompt string, onEvent func(AgentEvent)) error
}

// AgentFactory creates a new AgentRunner for the given working directory.
type AgentFactory func(workingDir string) (AgentRunner, error)

// session holds the state for a single ACP session.
type session struct {
	id     string
	runner AgentRunner
	cancel context.CancelFunc
}

// SessionManager manages the lifecycle of ACP sessions.
type SessionManager struct {
	factory  AgentFactory
	mu       sync.Mutex
	sessions map[string]*session
	counter  atomic.Int64
}

// NewSessionManager creates a SessionManager backed by the given factory.
func NewSessionManager(factory AgentFactory) *SessionManager {
	return &SessionManager{
		factory:  factory,
		sessions: make(map[string]*session),
	}
}

// NewSession creates a new session and returns its id.
func (sm *SessionManager) NewSession(ctx context.Context, workingDir string) (string, error) {
	runner, err := sm.factory(workingDir)
	if err != nil {
		return "", fmt.Errorf("acp: create agent: %w", err)
	}
	n := sm.counter.Add(1)
	id := fmt.Sprintf("sess-%d", n)
	_, cancel := context.WithCancel(ctx)
	sm.mu.Lock()
	sm.sessions[id] = &session{id: id, runner: runner, cancel: cancel}
	sm.mu.Unlock()
	return id, nil
}

// Lookup returns the session for the given id, or false if not found.
func (sm *SessionManager) Lookup(id string) (*session, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[id]
	return s, ok
}

// Cancel cancels and removes the session.
func (sm *SessionManager) Cancel(id string) {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	if ok {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()
	if ok {
		s.cancel()
	}
}

// Prompt runs the agent for the session with the given prompt, calling
// onEvent for each event. It blocks until the agent is done.
func (sm *SessionManager) Prompt(ctx context.Context, id, prompt string, onEvent func(AgentEvent)) error {
	sm.mu.Lock()
	s, ok := sm.sessions[id]
	sm.mu.Unlock()
	if !ok {
		return fmt.Errorf("acp: session %q not found", id)
	}
	return s.runner.Run(ctx, prompt, onEvent)
}
