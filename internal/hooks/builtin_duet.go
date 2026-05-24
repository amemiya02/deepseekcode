package hooks

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/amemiya02/deepseekcode/internal/permissions"
)

// DuetClient is the LLM-call dependency for the Duet builtin hook.
// Satisfied by llm.Client via its ValidatePro method.
type DuetClient interface {
	ValidatePro(ctx context.Context, prompt string) (approve bool, reasoning string, err error)
}

// NewDuetHook returns a BuiltinHook implementing destructive-call
// validation via the pro model. Safe to register under name "duet".
//
// modelFn supplies the current main-loop model so the hook can skip
// self-validation when the user has switched to pro via /models.
// transcriptFn supplies recent conversation context for pro to judge.
func NewDuetHook(client DuetClient, extraDestructive []string, cwd string, secretPatterns []string, modelFn func() string, transcriptFn func() []byte) BuiltinHook {
	return func(ctx context.Context, in HookInput) (HookOutput, error) {
		// When main model is pro, skip self-validation.
		if modelFn() == "deepseek-v4-pro" {
			return HookOutput{Decision: "continue"}, nil
		}

		if !isDestructiveCall(in.ToolName, in.ToolInput, cwd, extraDestructive, secretPatterns) {
			return HookOutput{Decision: "allow"}, nil
		}

		prompt := buildDuetPrompt(in.ToolName, in.ToolInput, transcriptFn())
		approve, reasoning, err := client.ValidatePro(ctx, prompt)
		if err != nil {
			slog.Warn("duet pro validation failed, fail-open", "err", err)
			return HookOutput{Decision: "continue", Reason: "pro validation skipped: " + err.Error()}, nil
		}

		if approve {
			return HookOutput{Decision: "allow", Reason: reasoning}, nil
		}
		return HookOutput{Decision: "deny", Reason: reasoning}, nil
	}
}

// isDestructiveCall mirrors agent.isDestructive without coupling to Agent.
func isDestructiveCall(toolName string, args json.RawMessage, cwd string, extraDestructive []string, secretPatterns []string) bool {
	if toolName == "bash" {
		var ba struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(args, &ba)
		return permissions.IsDestructiveBash(ba.Command, extraDestructive)
	}
	return permissions.IsDestructiveToolCall(toolName, args, cwd, secretPatterns)
}

// buildDuetPrompt constructs the adjudication prompt pro sees.
func buildDuetPrompt(toolName string, args json.RawMessage, transcript []byte) string {
	ctx := clipTail(transcript, 4000)
	prompt := "Recent conversation (newest last):\n" + ctx + "\n\n"
	prompt += "Proposed destructive operation:\n"
	prompt += "  tool: " + toolName + "\n"
	prompt += "  args: " + string(args) + "\n\n"
	prompt += "Decide:\n"
	prompt += "- approve: the operation is consistent with the user's stated intent and\n"
	prompt += "  the recent context, and any data it might destroy is recoverable or\n"
	prompt += "  was explicitly requested.\n"
	prompt += "- block: the operation could destroy unintended state, exceeds the\n"
	prompt += "  user's stated scope, or is being attempted with insufficient context.\n\n"
	prompt += "Respond ONLY with a JSON object of the form:\n"
	prompt += "  {\"approve\": true|false, \"reasoning\": \"<one paragraph>\"}"
	return prompt
}

func clipTail(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return "...(transcript truncated)...\n" + string(b[len(b)-max:])
}
