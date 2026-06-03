package main

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestVerifyCmdFlag confirms that the --verify-cmd wiring sets agent.Verify
// when the flag is non-empty, and leaves it nil when the flag is empty.
func TestVerifyCmdFlag(t *testing.T) {
	cases := []struct {
		name       string
		verifyCmd  string
		wantVerify bool
		wantCmd    string
	}{
		{"empty_leaves_nil", "", false, ""},
		{"non_empty_sets_verify", "go build ./...", true, "go build ./..."},
		{"shell_pipeline", "go test ./... && go vet ./...", true, "go test ./... && go vet ./..."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := agent.New(nil, tools.New(), nil, "test-model")
			applyVerifyCmd(a, tc.verifyCmd)
			if tc.wantVerify {
				if a.Verify == nil {
					t.Fatalf("expected agent.Verify to be set, got nil")
				}
				if a.Verify.Cmd != tc.wantCmd {
					t.Fatalf("Verify.Cmd: got %q, want %q", a.Verify.Cmd, tc.wantCmd)
				}
			} else {
				if a.Verify != nil {
					t.Fatalf("expected agent.Verify to be nil, got %+v", a.Verify)
				}
			}
		})
	}
}
