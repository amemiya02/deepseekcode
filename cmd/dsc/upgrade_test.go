package main

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/version"
)

func TestDecideUpgrade(t *testing.T) {
	tests := []struct {
		current, latest string
		method          version.Method
		wantMsg         string
		wantCmd         string
	}{
		{
			"v0.1.0", "v0.2.0", version.MethodBrew,
			"update available v0.1.0 → v0.2.0",
			"brew upgrade deepseekcode",
		},
		{
			"v0.1.0", "v0.2.0", version.MethodCurl,
			"update available v0.1.0 → v0.2.0",
			"curl -fsSL https://raw.githubusercontent.com/amemiya02/deepseekcode/main/install.sh | sh",
		},
		{
			"v0.1.0", "v0.2.0", version.MethodGoInstall,
			"update available v0.1.0 → v0.2.0",
			"go install github.com/amemiya02/deepseekcode/cmd/dsc@latest",
		},
		{
			"v0.1.0", "v0.2.0", version.MethodManual,
			"update available v0.1.0 → v0.2.0",
			"download from https://github.com/amemiya02/deepseekcode/releases/tag/v0.2.0",
		},
		{
			"v0.2.0", "v0.2.0", version.MethodBrew,
			"up to date (v0.2.0)",
			"",
		},
		{
			"dev", "v0.1.0", version.MethodManual,
			"update available dev → v0.1.0",
			"download from https://github.com/amemiya02/deepseekcode/releases/tag/v0.1.0",
		},
	}
	for _, tt := range tests {
		msg, cmd := decideUpgrade(tt.current, tt.latest, tt.method)
		if msg != tt.wantMsg {
			t.Errorf("decideUpgrade(%q,%q,%q) msg = %q, want %q",
				tt.current, tt.latest, tt.method, msg, tt.wantMsg)
		}
		if cmd != tt.wantCmd {
			t.Errorf("decideUpgrade(%q,%q,%q) cmd = %q, want %q",
				tt.current, tt.latest, tt.method, cmd, tt.wantCmd)
		}
	}
}
