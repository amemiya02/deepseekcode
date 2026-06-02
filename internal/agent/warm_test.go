// internal/agent/warm_test.go
package agent

import (
	"strings"
	"testing"
	"time"
)

func TestIsLikelyWarm(t *testing.T) {
	fp := "abc123"
	if !IsLikelyWarm(fp, fp, 30*time.Minute, time.Hour) {
		t.Fatal("same fp within ttl should be warm")
	}
	if IsLikelyWarm("old", fp, 1*time.Minute, time.Hour) {
		t.Fatal("changed fp should be cold")
	}
	if IsLikelyWarm(fp, fp, 5*time.Hour, time.Hour) {
		t.Fatal("beyond ttl should be cold")
	}
}

func TestWarmthNotice(t *testing.T) {
	tests := []struct {
		name         string
		warm         bool
		sinceLastUse time.Duration
		wantEmpty    bool
		wantContains string
	}{
		{
			name:         "not warm returns empty",
			warm:         false,
			sinceLastUse: 30 * time.Minute,
			wantEmpty:    true,
		},
		{
			name:         "warm 30m mentions minutes",
			warm:         true,
			sinceLastUse: 30 * time.Minute,
			wantContains: "minute",
		},
		{
			name:         "warm 2h mentions hours",
			warm:         true,
			sinceLastUse: 2 * time.Hour,
			wantContains: "hour",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WarmthNotice(tt.warm, tt.sinceLastUse)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty string, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected non-empty notice, got empty string")
			}
			if !strings.Contains(got, tt.wantContains) {
				t.Fatalf("expected notice to contain %q, got %q", tt.wantContains, got)
			}
		})
	}
}
