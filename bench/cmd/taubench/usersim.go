// Package taubench — LLM-backed user simulator.
// Ported verbatim from benchmarks/tau-bench/user-sim.ts (Reasonix source-of-truth).
// Model: deepseek-chat, temperature: 0.1, maxTokens: 200.
// ##STOP## (substring) terminates the conversation; never terminates on the opening message.
package taubench

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Turn is one message in the conversation transcript.
// Role is one of "user", "agent", or "tool".
type Turn struct {
	Role     string
	Content  string
	ToolName string // non-empty only when Role == "tool"
}

// simMessage is a single chat message sent to the LLM.
type simMessage struct {
	role    string
	content string
}

// simChatRequest is the request sent to chatClient.Chat.
type simChatRequest struct {
	model       string
	temperature float64
	maxTokens   int
	messages    []simMessage
}

// chatClient abstracts the LLM call so tests can inject a mock.
type chatClient interface {
	Chat(ctx context.Context, req simChatRequest) (string, error)
}

// UserSimulator drives the user side of a tau-bench conversation.
// Ported from the UserSimulator class in user-sim.ts.
type UserSimulator struct {
	client  chatClient
	persona Persona
}

// NewUserSimulator constructs a UserSimulator bound to the given client and persona.
func NewUserSimulator(client chatClient, persona Persona) *UserSimulator {
	return &UserSimulator{client: client, persona: persona}
}

// Next returns the next user utterance, or nil if the conversation is over.
// transcript is the turns so far (may be nil/empty for the opening message).
// ##STOP## anywhere in the response signals end-of-conversation, EXCEPT on
// the opening message (empty transcript) — per the TS spec.
func (s *UserSimulator) Next(ctx context.Context, transcript []Turn) (*string, error) {
	knowns, err := json.MarshalIndent(s.persona.Knowns, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("usersim: marshal knowns: %w", err)
	}

	sysContent := fmt.Sprintf(
		"%s\n\nCharacter: %s\nGoal: %s\nFacts you may share when asked (don't volunteer):\n%s",
		userSimSys,
		s.persona.Style,
		s.persona.Goal,
		string(knowns),
	)

	msgs := []simMessage{
		{role: "system", content: sysContent},
	}

	isOpening := len(transcript) == 0

	if isOpening {
		msgs = append(msgs, simMessage{
			role:    "user",
			content: "Write your opening message to the support agent. One or two sentences. Do not dump all the facts.",
		})
	} else {
		msgs = append(msgs, simMessage{
			role:    "user",
			content: fmt.Sprintf("Here is the conversation so far (you are the USER).\n\n%s\n\nWrite ONLY your next user reply, or output ##STOP## if the goal is clearly met or clearly refused.", transcriptToString(transcript)),
		})
	}

	text, err := s.client.Chat(ctx, simChatRequest{
		model:       "deepseek-chat",
		temperature: 0.1,
		maxTokens:   200,
		messages:    msgs,
	})
	if err != nil {
		return nil, err
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// ##STOP## substring terminates — but never on the opening message.
	if !isOpening && strings.Contains(text, "##STOP##") {
		return nil, nil
	}

	return &text, nil
}

// transcriptToString formats transcript turns into the string the TS
// helper produces (verbatim line format from user-sim.ts).
func transcriptToString(turns []Turn) string {
	lines := make([]string, 0, len(turns))
	for _, t := range turns {
		switch t.Role {
		case "user":
			lines = append(lines, "USER: "+t.Content)
		case "agent":
			lines = append(lines, "AGENT: "+t.Content)
		case "tool":
			lines = append(lines, fmt.Sprintf("(tool %s returned: %s)", t.ToolName, truncate(t.Content, 200)))
		}
	}
	return strings.Join(lines, "\n")
}

// truncate caps s to n runes, appending "…" (U+2026) when trimmed.
// Matches the TS helper: `s.length > n ? s.slice(0,n)+"…" : s`.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n]) + "…"
	}
	return s
}
