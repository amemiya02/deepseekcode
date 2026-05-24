package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// ProValidator is the production Duet validator. It calls the pro model
// in JSON-mode with a constrained prompt and parses the returned
// approve/block decision. See docs/design.md §11.3.
//
// We deliberately do not pass the full tool registry — pro is in
// adjudicator mode, not actor mode, and should not be making tool calls.
type ProValidator struct {
	Client      *llm.Client
	Model       string        // typically "deepseek-v4-pro"
	Timeout     time.Duration // hard timeout per validation, default 10s
	ExtraSystem string        // optional extra system rules
}

// NewProValidator returns a validator with v0.1 defaults.
func NewProValidator(client *llm.Client, model string) *ProValidator {
	return &ProValidator{
		Client:  client,
		Model:   model,
		Timeout: 10 * time.Second,
	}
}

// Validate calls pro with the proposed tool call + recent transcript
// and returns its structured decision.
func (v *ProValidator) Validate(ctx context.Context, toolName string, args json.RawMessage, transcript []byte) (DuetDecision, error) {
	if v.Client == nil || v.Model == "" {
		return DuetDecision{Approve: true, Reasoning: "validator misconfigured (no client/model); allowing"}, nil
	}
	timeout := v.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	system := proValidatorSystem
	if v.ExtraSystem != "" {
		system += "\n\nAdditional project-specific guidance:\n" + v.ExtraSystem
	}

	userMsg := fmt.Sprintf(`Recent conversation (newest last):
%s

Proposed destructive operation:
  tool: %s
  args: %s

Decide:
- approve: the operation is consistent with the user's stated intent and
  the recent context, and any data it might destroy is recoverable or
  was explicitly requested.
- block: the operation could destroy unintended state, exceeds the
  user's stated scope, or is being attempted with insufficient context.

Respond ONLY with a JSON object of the form:
  {"approve": true|false, "reasoning": "<one paragraph>"}`,
		clipTranscript(transcript, 4000),
		toolName,
		string(args),
	)

	req := llm.Request{
		Model: v.Model,
		Messages: []llm.Message{
			{Role: "system", Blocks: []llm.ContentBlock{llm.TextBlock{Text: system}}},
			{Role: "user", Blocks: []llm.ContentBlock{llm.TextBlock{Text: userMsg}}},
		},
		Thinking:       llm.ThinkingEnabled(true),
		ResponseFormat: &llm.ResponseFmt{Type: "json_object"},
	}

	events, err := v.Client.Stream(tctx, req)
	if err != nil {
		return DuetDecision{}, err
	}
	var content strings.Builder
	for ev := range events {
		switch ev.Type {
		case llm.EventTextDelta:
			content.WriteString(ev.Text)
		case llm.EventError:
			return DuetDecision{}, ev.Err
		}
	}

	raw := strings.TrimSpace(content.String())
	if raw == "" {
		return DuetDecision{Approve: false, Reasoning: "validator returned empty response"}, nil
	}

	var dec DuetDecision
	if err := json.Unmarshal([]byte(raw), &dec); err != nil {
		// Try to recover by scanning for the first {...} block.
		if start := strings.IndexByte(raw, '{'); start >= 0 {
			if end := strings.LastIndexByte(raw, '}'); end > start {
				if err2 := json.Unmarshal([]byte(raw[start:end+1]), &dec); err2 == nil {
					return dec, nil
				}
			}
		}
		return DuetDecision{
			Approve:   false,
			Reasoning: "validator response was unparseable; defaulting to block",
		}, nil
	}
	return dec, nil
}

// clipTranscript caps the transcript bytes we hand pro. The validator
// doesn't need the whole history; bounded context keeps cost predictable
// and reasoning crisp.
func clipTranscript(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	// Keep the tail; older context matters less for a "should we do this
	// next step?" decision.
	return "...(transcript truncated to last " + itoaSimple(max) + " bytes)...\n" + string(b[len(b)-max:])
}

func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	return string(s)
}

const proValidatorSystem = `You are the Pro Validator for deepseekcode, a terminal coding agent.
A junior agent (the main loop, running on a cheaper, faster model) is
about to perform a potentially destructive operation. Your job is to
decide whether it should proceed.

You are NOT writing code. You are NOT continuing the task. You are
adjudicating a single proposed tool call.

Approve when:
- The operation is consistent with the user's explicit request.
- Any data the operation might destroy was created by the agent itself
  in this session, or was explicitly identified as expendable by the user.
- The blast radius is small and reversible (one file inside the project,
  not many files, not the working tree).

Block when:
- The operation could destroy uncommitted work the user did not approve.
- The operation goes beyond the user's stated scope (e.g. user asked
  about pkg/auth but operation touches pkg/payments).
- The user has not provided enough context for you to judge.
- The operation pushes/publishes/deploys, unless the user explicitly
  approved that scope.

Be terse. One paragraph of reasoning is enough.`
