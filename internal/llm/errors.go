package llm

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// APIError is returned when an LLM API replies with a non-2xx
// status. The Body field carries the raw response so callers can
// surface it to the user (often the most actionable signal).
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("llm api error: status=%d body=%s", e.Status, e.Body)
}

// IsRateLimit reports whether the error indicates a rate-limit response.
func IsRateLimit(err error) bool {
	a, ok := err.(*APIError)
	return ok && a.Status == 429
}

// IsAuth reports whether the error indicates an authentication failure.
func IsAuth(err error) bool {
	a, ok := err.(*APIError)
	return ok && (a.Status == 401 || a.Status == 403)
}

// IsTransient reports whether the error is retryable: rate-limit or
// server-side (5xx) errors. Auth errors (401/403) and client errors
// (400) are NOT transient — retrying them wastes time.
func IsTransient(err error) bool {
	a, ok := err.(*APIError)
	if !ok {
		return false
	}
	return a.Status == 429 || a.Status >= 500
}

// isNetworkError reports whether err is a low-level network failure (connection
// refused, DNS failure, TLS handshake error, etc.) as opposed to an HTTP-level
// error. These are always transient in a mirror-retry scenario: the mirror is
// simply unreachable, and the next mirror should be tried.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// A *url.Error from the http client already wraps net.OpError-level
	// failures (connection refused, DNS, TLS handshake), so matching it is
	// sufficient; a separate *net.OpError branch is dead in this path.
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// IsContextOverflow reports whether err is a provider rejection for exceeding
// the model's context window. DeepSeek signals this with HTTP 400 and a body
// naming the maximum context length; there is no dedicated error code, so the
// body is matched case-insensitively. Such a request can never succeed on a
// blind retry — the only recovery is to shrink the conversation (compaction),
// which is why the agent loop branches on this distinctly from IsTransient.
// IsInsufficientBalance reports a 402 Payment Required (DeepSeek: balance
// exhausted). Never retryable (IsTransient already excludes 402).
func IsInsufficientBalance(err error) bool {
	a, ok := err.(*APIError)
	return ok && a.Status == 402
}

// LocalizeError returns a concise user-facing message for err in language lang
// ("en" or "zh"; any other value treated as "en"). Recognized: 429 rate-limit,
// 402 insufficient-balance, 5xx server, ErrFirstTokenTimeout/ErrChunkStall,
// IsContextOverflow, 401/403 auth. For any UNRECOGNIZED error it returns
// err.Error() verbatim. nil err → "".
func LocalizeError(err error, lang string) string {
	if err == nil {
		return ""
	}
	if lang != "zh" {
		lang = "en"
	}

	// Timeout sentinels.
	if errors.Is(err, ErrFirstTokenTimeout) || errors.Is(err, ErrChunkStall) {
		if lang == "zh" {
			return "请求超时。DeepSeek 服务端排队等待超过上限后断开了连接，请稍后重试或降低 effort 等级"
		}
		return "Request timed out. DeepSeek server queue wait exceeded the limit; retry later or lower the effort level."
	}

	// Context overflow.
	if IsContextOverflow(err) {
		if lang == "zh" {
			return "上下文超出模型窗口上限，请压缩对话后重试"
		}
		return "Context exceeded the model window limit; compact the conversation and retry."
	}

	// Auth errors.
	if IsAuth(err) {
		if lang == "zh" {
			return "鉴权失败，请检查 API Key"
		}
		return "Authentication failed; check your API key."
	}

	// Rate limit.
	if IsRateLimit(err) {
		if lang == "zh" {
			return "请求频率超限，请稍后重试"
		}
		return "Rate limit exceeded; retry later."
	}

	// Insufficient balance (402).
	if IsInsufficientBalance(err) {
		if lang == "zh" {
			return "账户余额不足，请充值后重试"
		}
		return "Insufficient account balance; top up and retry."
	}

	// Server errors (5xx).
	if a, ok := err.(*APIError); ok && a.Status >= 500 {
		if lang == "zh" {
			return "DeepSeek 服务端错误，请稍后重试"
		}
		return "DeepSeek server error; retry later."
	}

	// Unrecognized: return raw error text.
	return err.Error()
}

// ResolveLang resolves the effective language. "en"/"zh" pass through. ""/"auto"
// inspect env in order LC_ALL, LC_MESSAGES, LANG: a value whose lower-case form
// has prefix "zh" → "zh"; otherwise "en".
func ResolveLang(cfgLang string) string {
	switch cfgLang {
	case "en", "zh":
		return cfgLang
	}
	// "" or "auto": inspect locale env vars.
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "zh") {
				return "zh"
			}
		}
	}
	return "en"
}

func IsContextOverflow(err error) bool {
	var a *APIError
	if !errors.As(err, &a) || a.Status != 400 {
		return false
	}
	b := strings.ToLower(a.Body)
	for _, marker := range []string{
		"context length",
		"maximum context",
		"context_length_exceeded",
		"too many tokens",
		"reduce the length",
		"maximum length",
	} {
		if strings.Contains(b, marker) {
			return true
		}
	}
	return false
}
