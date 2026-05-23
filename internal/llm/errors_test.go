package llm

import (
	"errors"
	"testing"
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
