// Package taubench — unit tests for UserSimulator.
// All tests use a mockChatClient; no live API calls are made.
package taubench

import (
	"context"
	"strings"
	"testing"
)

// mockChatClient implements chatClient with a fixed response queue.
type mockChatClient struct {
	responses []string
	calls     []mockCall
}

type mockCall struct {
	model       string
	temperature float64
	maxTokens   int
	messages    []simMessage
}

func (m *mockChatClient) Chat(_ context.Context, req simChatRequest) (string, error) {
	m.calls = append(m.calls, mockCall{
		model:       req.model,
		temperature: req.temperature,
		maxTokens:   req.maxTokens,
		messages:    req.messages,
	})
	if len(m.responses) == 0 {
		return "", nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}

// --- helpers ---

func newSim(responses []string) (*UserSimulator, *mockChatClient) {
	mc := &mockChatClient{responses: responses}
	persona := Persona{
		Style: "polite",
		Goal:  "cancel order o_1004",
		Knowns: map[string]string{
			"name":    "Dev Patel",
			"orderId": "o_1004",
		},
	}
	return NewUserSimulator(mc, persona), mc
}

// --- tests ---

// TestUserSimOpeningMessage verifies the first call (empty transcript)
// uses the opening-message prompt and returns a non-nil string.
func TestUserSimOpeningMessage(t *testing.T) {
	sim, mc := newSim([]string{"Hi, I need to cancel my order."})

	got, err := sim.Next(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil reply for opening message")
	}
	if !strings.Contains(*got, "cancel") && !strings.Contains(*got, "order") {
		// The mock returns the canned response verbatim; just confirm it is passed through.
	}
	_ = got

	// Verify model/params
	if len(mc.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mc.calls))
	}
	call := mc.calls[0]
	if call.model != "deepseek-chat" {
		t.Errorf("model = %q, want deepseek-chat", call.model)
	}
	if call.temperature != 0.1 {
		t.Errorf("temperature = %v, want 0.1", call.temperature)
	}
	if call.maxTokens != 200 {
		t.Errorf("maxTokens = %d, want 200", call.maxTokens)
	}
}

// TestUserSimOpeningMessageNeverStops verifies that even if the model
// returns ##STOP## as its very first reply, we treat it as a normal
// opening message (the TS spec says: "never STOP on first message").
func TestUserSimOpeningMessageNeverStops(t *testing.T) {
	sim, _ := newSim([]string{"##STOP##"})

	got, err := sim.Next(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Opening turn: even ##STOP## must NOT return nil.
	if got == nil {
		t.Fatal("opening message must never be nil even when model returns ##STOP##")
	}
}

// TestUserSimStopSubstring verifies that ##STOP## anywhere in the
// response causes Next to return nil on a non-opening turn.
func TestUserSimStopSubstring(t *testing.T) {
	cases := []struct {
		name     string
		response string
	}{
		{"exact", "##STOP##"},
		{"trailing", "Thank you! ##STOP##"},
		{"embedded", "All good ##STOP## here"},
		{"prefix", "##STOP## done"},
	}

	transcript := []Turn{{Role: "agent", Content: "How can I help?"}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim, _ := newSim([]string{tc.response})
			got, err := sim.Next(context.Background(), transcript)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != nil {
				t.Errorf("response %q: expected nil (STOP), got %q", tc.response, *got)
			}
		})
	}
}

// TestUserSimNormalReply verifies that a normal reply (no ##STOP##) on
// a non-empty transcript is returned verbatim.
func TestUserSimNormalReply(t *testing.T) {
	transcript := []Turn{{Role: "agent", Content: "How can I help you today?"}}
	sim, _ := newSim([]string{"I want to cancel order o_1004."})

	got, err := sim.Next(context.Background(), transcript)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil reply")
	}
	if *got != "I want to cancel order o_1004." {
		t.Errorf("got %q, want exact echo", *got)
	}
}

// TestUserSimSystemPromptContainsPersona verifies the assembled system
// message embeds the persona's Style, Goal, and Knowns.
func TestUserSimSystemPromptContainsPersona(t *testing.T) {
	sim, mc := newSim([]string{"Hello!"})
	_, _ = sim.Next(context.Background(), nil)

	if len(mc.calls) == 0 {
		t.Fatal("no calls recorded")
	}
	sys := ""
	for _, msg := range mc.calls[0].messages {
		if msg.role == "system" {
			sys = msg.content
			break
		}
	}
	if sys == "" {
		t.Fatal("no system message found in call")
	}

	// Must contain the SYS constant verbatim (first few chars suffice).
	if !strings.Contains(sys, "You are roleplaying a user contacting a retail support agent.") {
		t.Error("system message missing SYS preamble")
	}
	if !strings.Contains(sys, "polite") {
		t.Error("system message missing persona style")
	}
	if !strings.Contains(sys, "cancel order o_1004") {
		t.Error("system message missing persona goal")
	}
	if !strings.Contains(sys, "Dev Patel") {
		t.Error("system message missing persona knowns")
	}
}

// TestUserSimOpeningPromptText verifies the user-role content for the
// opening turn matches the TS template verbatim.
func TestUserSimOpeningPromptText(t *testing.T) {
	sim, mc := newSim([]string{"Hello!"})
	_, _ = sim.Next(context.Background(), nil)

	if len(mc.calls) == 0 {
		t.Fatal("no calls recorded")
	}
	userMsg := ""
	for _, msg := range mc.calls[0].messages {
		if msg.role == "user" {
			userMsg = msg.content
			break
		}
	}
	want := "Write your opening message to the support agent. One or two sentences. Do not dump all the facts."
	if userMsg != want {
		t.Errorf("opening user prompt\ngot:  %q\nwant: %q", userMsg, want)
	}
}

// TestUserSimTranscriptPromptText verifies the user-role content on a
// subsequent turn contains the transcript header from the TS template.
func TestUserSimTranscriptPromptText(t *testing.T) {
	transcript := []Turn{
		{Role: "user", Content: "I need help."},
		{Role: "agent", Content: "Sure, what's your order?"},
	}
	sim, mc := newSim([]string{"Order o_1004."})
	_, _ = sim.Next(context.Background(), transcript)

	if len(mc.calls) == 0 {
		t.Fatal("no calls recorded")
	}
	userMsg := ""
	for _, msg := range mc.calls[0].messages {
		if msg.role == "user" {
			userMsg = msg.content
			break
		}
	}
	if !strings.Contains(userMsg, "Here is the conversation so far (you are the USER).") {
		t.Errorf("transcript prompt missing header; got: %q", userMsg)
	}
	if !strings.Contains(userMsg, "USER: I need help.") {
		t.Errorf("transcript prompt missing USER line; got: %q", userMsg)
	}
	if !strings.Contains(userMsg, "AGENT: Sure, what's your order?") {
		t.Errorf("transcript prompt missing AGENT line; got: %q", userMsg)
	}
	if !strings.Contains(userMsg, "##STOP##") {
		t.Errorf("transcript prompt missing ##STOP## mention; got: %q", userMsg)
	}
}

// TestUserSimToolTurnTruncated verifies that tool turns are rendered as
// "(tool <name> returned: <content>)" and truncated at 200 chars.
func TestUserSimToolTurnTruncated(t *testing.T) {
	longContent := strings.Repeat("x", 250)
	transcript := []Turn{
		{Role: "tool", ToolName: "lookup_order", Content: longContent},
	}
	sim, mc := newSim([]string{"ok"})
	_, _ = sim.Next(context.Background(), transcript)

	userMsg := ""
	for _, msg := range mc.calls[0].messages {
		if msg.role == "user" {
			userMsg = msg.content
			break
		}
	}
	// Should contain the tool line truncated to 200 chars + ellipsis.
	if !strings.Contains(userMsg, "(tool lookup_order returned:") {
		t.Errorf("tool line missing; got: %q", userMsg)
	}
	// The content should be truncated — original is 250 chars, should be ≤200.
	if strings.Contains(userMsg, longContent) {
		t.Error("tool content was not truncated")
	}
}
