package hooks

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/amemiya02/deepseekcode/internal/permissions"
)

// DuetClient is the LLM-call dependency for the Duet builtin hook.
// Satisfied by llm.Client via its ValidatePro method.
type DuetClient interface {
	ValidatePro(ctx context.Context, prompt string) (approve bool, reasoning string, err error)
}

// DuetOptions configures the pro-validation call. The zero value keeps
// the historical behavior: no per-call timeout beyond the caller's ctx,
// no retry before failing open.
type DuetOptions struct {
	// ValidatorTimeout caps each ValidatePro call. <= 0 means no extra
	// timeout (the caller's ctx still applies). Wired from config
	// duet.validator_timeout_ms.
	ValidatorTimeout time.Duration
	// RetryOnFailure retries the validation exactly once after a
	// transient error before failing open. Wired from config
	// duet.retry_on_failure.
	RetryOnFailure bool
}

// NewDuetHook returns a BuiltinHook implementing destructive-call
// validation via the pro model with zero-value options. Safe to
// register under name "duet".
func NewDuetHook(client DuetClient, extraDestructive []string, cwd string, secretPatterns []string, modelFn func() string, transcriptFn func() []byte) BuiltinHook {
	return NewDuetHookWithOptions(client, extraDestructive, cwd, secretPatterns, modelFn, transcriptFn, DuetOptions{})
}

// NewDuetHookWithOptions is NewDuetHook with an explicit timeout/retry
// policy for the ValidatePro call.
//
// modelFn supplies the current main-loop model so the hook can skip
// self-validation when the user has switched to pro via /models.
// transcriptFn supplies recent conversation context for pro to judge.
func NewDuetHookWithOptions(client DuetClient, extraDestructive []string, cwd string, secretPatterns []string, modelFn func() string, transcriptFn func() []byte, opts DuetOptions) BuiltinHook {
	return func(ctx context.Context, in HookInput) (HookOutput, error) {
		// When main model is pro, skip self-validation.
		if modelFn() == "deepseek-v4-pro" {
			return HookOutput{Decision: "continue"}, nil
		}

		if !isDestructiveCall(in.ToolName, in.ToolInput, cwd, extraDestructive, secretPatterns) {
			return HookOutput{Decision: "allow"}, nil
		}

		prompt := buildDuetPrompt(in.ToolName, in.ToolInput, transcriptFn())
		validate := func() (bool, string, error) {
			vctx, cancel := ctx, context.CancelFunc(func() {})
			if opts.ValidatorTimeout > 0 {
				vctx, cancel = context.WithTimeout(ctx, opts.ValidatorTimeout)
			}
			defer cancel()
			return client.ValidatePro(vctx, prompt)
		}
		approve, reasoning, err := validate()
		if err != nil && opts.RetryOnFailure && ctx.Err() == nil {
			slog.Warn("duet pro validation failed, retrying once", "err", err)
			approve, reasoning, err = validate()
		}
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
