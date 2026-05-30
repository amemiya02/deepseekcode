package llm

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limit", &APIError{Status: 429}, true},
		{"server error 500", &APIError{Status: 500}, true},
		{"server error 503", &APIError{Status: 503}, true},
		{"bad request", &APIError{Status: 400}, false},
		{"unauthorized", &APIError{Status: 401}, false},
		{"forbidden", &APIError{Status: 403}, false},
		{"not found", &APIError{Status: 404}, false},
		{"plain error", errors.New("network blew up"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsTransient(c.err); got != c.want {
				t.Errorf("IsTransient(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestIsContextOverflow(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"deepseek max context body", &APIError{Status: 400, Body: "This model's maximum context length is 1000000 tokens. However, you requested 1200000 tokens."}, true},
		{"openai-style code", &APIError{Status: 400, Body: `{"error":{"code":"context_length_exceeded"}}`}, true},
		{"too many tokens phrasing", &APIError{Status: 400, Body: "Bad request: too many tokens in prompt"}, true},
		{"reduce the length phrasing", &APIError{Status: 400, Body: "Please reduce the length of the messages."}, true},
		{"unrelated 400", &APIError{Status: 400, Body: "invalid 'thinking' field"}, false},
		{"overflow-ish text but wrong status", &APIError{Status: 500, Body: "maximum context length exceeded"}, false},
		{"rate limit", &APIError{Status: 429, Body: "slow down"}, false},
		{"plain error", errors.New("context length"), false}, // not an APIError
		{"wrapped api error", wrap(&APIError{Status: 400, Body: "maximum context length is 1M"}), true},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsContextOverflow(c.err); got != c.want {
				t.Errorf("IsContextOverflow(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// wrap mimics a caller wrapping the connect error with %w so the test pins that
// IsContextOverflow unwraps via errors.As, not a bare type assertion.
func wrap(err error) error { return errCtx{err} }

type errCtx struct{ err error }

func (e errCtx) Error() string { return "stream: " + e.err.Error() }
func (e errCtx) Unwrap() error { return e.err }

func TestIsInsufficientBalance(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"402", &APIError{Status: 402}, true},
		{"429", &APIError{Status: 429}, false},
		{"500", &APIError{Status: 500}, false},
		{"plain error", errors.New("no money"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsInsufficientBalance(c.err); got != c.want {
				t.Errorf("IsInsufficientBalance(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestLocalizeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		lang string
		want string // substring to match
	}{
		{"nil", nil, "en", ""},
		{"429 zh", &APIError{Status: 429}, "zh", "请求频率超限"},
		{"429 en", &APIError{Status: 429}, "en", "Rate limit"},
		{"402 zh", &APIError{Status: 402}, "zh", "余额不足"},
		{"402 en", &APIError{Status: 402}, "en", "balance"},
		{"500 zh", &APIError{Status: 500}, "zh", "服务端错误"},
		{"500 en", &APIError{Status: 500}, "en", "server error"},
		{"timeout zh", ErrFirstTokenTimeout, "zh", "请求超时"},
		{"timeout en", ErrFirstTokenTimeout, "en", "timed out"},
		{"chunk stall zh", ErrChunkStall, "zh", "请求超时"},
		{"context overflow zh", &APIError{Status: 400, Body: "maximum context length is 1000000"}, "zh", "上下文超出"},
		{"context overflow en", &APIError{Status: 400, Body: "maximum context length is 1000000"}, "en", "Context exceeded"},
		{"auth zh", &APIError{Status: 401}, "zh", "鉴权失败"},
		{"auth en", &APIError{Status: 401}, "en", "Authentication"},
		{"unknown en", errors.New("boom"), "en", "boom"},
		{"unknown zh", errors.New("boom"), "zh", "boom"},
		{"bad lang treated as en", &APIError{Status: 429}, "fr", "Rate limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LocalizeError(c.err, c.lang)
			if c.want == "" {
				if got != "" {
					t.Errorf("LocalizeError(%v, %q) = %q, want empty", c.err, c.lang, got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("LocalizeError(%v, %q) = %q, want containing %q", c.err, c.lang, got, c.want)
			}
		})
	}
}

func TestResolveLang(t *testing.T) {
	cases := []struct {
		name     string
		cfgLang  string
		envLCALL string
		envLANG  string
		want     string
	}{
		{"explicit en", "en", "", "", "en"},
		{"explicit zh", "zh", "", "", "zh"},
		{"empty no zh env", "", "", "", "en"},
		{"empty LANG zh", "", "", "zh_CN.UTF-8", "zh"},
		{"empty LANG en", "", "", "en_US.UTF-8", "en"},
		{"auto same as empty", "auto", "", "", "en"},
		{"LC_ALL overrides LANG", "", "zh_CN.UTF-8", "en_US.UTF-8", "zh"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LC_ALL", c.envLCALL)
			t.Setenv("LANG", c.envLANG)
			if got := ResolveLang(c.cfgLang); got != c.want {
				t.Errorf("ResolveLang(%q) = %q, want %q", c.cfgLang, got, c.want)
			}
		})
	}
}

func TestStreamTimeoutSentinels(t *testing.T) {
	// readSSE emits these wrapped exactly as below. Pin that the wrapped error
	// both matches the sentinel via errors.Is (so the agent can classify a
	// re-issuable stall) AND keeps a greppable message (so logs and the older
	// string-matching tests still read naturally).
	cases := []struct {
		sentinel error
		wrapped  error
		substr   string
	}{
		{ErrFirstTokenTimeout, fmt.Errorf("%w after %s", ErrFirstTokenTimeout, 45*time.Second), "first-token timeout"},
		{ErrChunkStall, fmt.Errorf("%w after %s", ErrChunkStall, 20*time.Second), "chunk stall"},
	}
	for _, c := range cases {
		if !errors.Is(c.wrapped, c.sentinel) {
			t.Errorf("errors.Is(%q, sentinel) = false, want true", c.wrapped)
		}
		if !strings.Contains(c.wrapped.Error(), c.substr) {
			t.Errorf("message %q does not contain %q", c.wrapped.Error(), c.substr)
		}
		// The two sentinels must be distinct so a chunk-stall is never mistaken
		// for a first-token timeout.
		if errors.Is(c.wrapped, ErrFirstTokenTimeout) && errors.Is(c.wrapped, ErrChunkStall) {
			t.Errorf("%q matches both timeout sentinels", c.wrapped)
		}
	}
}
