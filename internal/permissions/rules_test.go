package permissions

import (
	"encoding/json"
	"testing"
)

func TestMatchTool(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "bash", true},
		{"*", "any_tool", true},
		{"bash", "bash", true},
		{"bash", "read_file", false},
		{"read_*", "read_file", true},
		{"read_*", "write_file", false},
		{"*.py", "foo.py", true}, // filepath.Match uses * as glob
		{"[invalid", "bash", false},
	}
	for _, tc := range tests {
		got := matchTool(tc.pattern, tc.name)
		if got != tc.want {
			t.Errorf("matchTool(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestMatchArgs(t *testing.T) {
	tests := []struct {
		pattern string
		args    json.RawMessage
		want    bool
	}{
		{"", json.RawMessage(`{"cmd":"ls"}`), true},
		{`"command":\s*"ls"`, json.RawMessage(`{"command":"ls"}`), true},
		{`rm\s+-rf`, json.RawMessage(`{"command":"rm -rf /"}`), true},
		{`rm\s+-rf`, json.RawMessage(`{"command":"ls"}`), false},
	}
	for _, tc := range tests {
		got := matchArgs(tc.pattern, tc.args)
		if got != tc.want {
			t.Errorf("matchArgs(%q, %s) = %v, want %v", tc.pattern, tc.args, got, tc.want)
		}
	}
}

func TestRuleEngineEvaluate(t *testing.T) {
	engine := &RuleEngine{
		Deny: []PermissionRule{
			{ToolPattern: "bash", ArgsPattern: `rm\s+-rf`, Decision: "deny"},
			{ToolPattern: "write_file", ArgsPattern: `/etc/`, Decision: "deny"},
		},
		Ask: []PermissionRule{
			{ToolPattern: "bash", ArgsPattern: "", Decision: "ask"},
		},
		Allow: []PermissionRule{
			{ToolPattern: "read_file", ArgsPattern: "", Decision: "allow"},
			{ToolPattern: "grep", ArgsPattern: "", Decision: "allow"},
		},
	}

	tests := []struct {
		name     string
		tool     string
		args     json.RawMessage
		wantDec  string
		wantReas string
	}{
		{
			name:     "deny rm",
			tool:     "bash",
			args:     json.RawMessage(`{"command":"rm -rf /"}`),
			wantDec:  "deny",
			wantReas: "matched deny rule: bash (args: rm\\s+-rf)",
		},
		{
			name:     "ask other bash",
			tool:     "bash",
			args:     json.RawMessage(`{"command":"git status"}`),
			wantDec:  "ask",
			wantReas: "matched ask rule: bash",
		},
		{
			name:     "allow read_file",
			tool:     "read_file",
			args:     json.RawMessage(`{"path":"foo.go"}`),
			wantDec:  "allow",
			wantReas: "matched allow rule: read_file",
		},
		{
			name:    "no match",
			tool:    "unknown_tool",
			args:    json.RawMessage(`{}`),
			wantDec: "",
		},
		{
			name:     "glob match",
			tool:     "read_file",
			args:     json.RawMessage(`{"path":"x"}`),
			wantDec:  "allow",
			wantReas: "matched allow rule: read_file",
		},
		{
			name:     "deny write to etc",
			tool:     "write_file",
			args:     json.RawMessage(`{"path":"/etc/hosts"}`),
			wantDec:  "deny",
			wantReas: "matched deny rule: write_file (args: /etc/)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotDec, gotReas := engine.Evaluate(tc.tool, tc.args)
			if gotDec != tc.wantDec {
				t.Errorf("decision = %q, want %q", gotDec, tc.wantDec)
			}
			if tc.wantReas != "" && gotReas != tc.wantReas {
				t.Errorf("reason = %q, want %q", gotReas, tc.wantReas)
			}
		})
	}
}

func TestRuleEngineEvaluateNil(t *testing.T) {
	var engine *RuleEngine
	dec, reason := engine.Evaluate("bash", json.RawMessage(`{}`))
	if dec != "" || reason != "" {
		t.Errorf("nil engine should return empty: got %q, %q", dec, reason)
	}
}

func TestRuleEngineEvaluateEmpty(t *testing.T) {
	engine := &RuleEngine{}
	dec, reason := engine.Evaluate("bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if dec != "" || reason != "" {
		t.Errorf("empty engine should return empty: got %q, %q", dec, reason)
	}
}

func TestRuleEngineDenyPriority(t *testing.T) {
	engine := &RuleEngine{
		Deny:  []PermissionRule{{ToolPattern: "bash", ArgsPattern: "", Decision: "deny"}},
		Allow: []PermissionRule{{ToolPattern: "bash", ArgsPattern: "", Decision: "allow"}},
	}
	dec, _ := engine.Evaluate("bash", json.RawMessage(`{}`))
	if dec != "deny" {
		t.Errorf("deny must take priority over allow: got %q", dec)
	}
}

func TestEvaluateHonorsCommandPrefixSpecifier(t *testing.T) {
	engine := &RuleEngine{
		Allow: []PermissionRule{
			{ToolPattern: "bash", Specifier: &Specifier{Field: "command", Prefix: "go test"}, Decision: "allow"},
			{ToolPattern: "bash_pty", Specifier: &Specifier{Field: "command", Prefix: "go test"}, Decision: "allow"},
		},
	}

	t.Run("matching prefix", func(t *testing.T) {
		dec, reason := engine.Evaluate("bash", json.RawMessage(`{"command":"go test ./..."}`))
		if dec != "allow" {
			t.Errorf("decision = %q, want allow", dec)
		}
		if reason != "matched allow rule: bash (cmd prefix: go test)" {
			t.Errorf("reason = %q", reason)
		}
	})

	t.Run("non-matching prefix", func(t *testing.T) {
		dec, _ := engine.Evaluate("bash", json.RawMessage(`{"command":"go build"}`))
		if dec != "" {
			t.Errorf("decision = %q, want empty (no match)", dec)
		}
	})

	t.Run("bash_pty also matches", func(t *testing.T) {
		dec, _ := engine.Evaluate("bash_pty", json.RawMessage(`{"command":"go test -v"}`))
		if dec != "allow" {
			t.Errorf("decision = %q, want allow", dec)
		}
	})
}

func TestEvaluateHonorsPathGlobSpecifier(t *testing.T) {
	engine := &RuleEngine{
		Allow: []PermissionRule{
			{ToolPattern: "read_file", Specifier: &Specifier{Field: "path", Glob: "src/**"}, Decision: "allow"},
		},
		Deny: []PermissionRule{
			{ToolPattern: "write_file", Specifier: &Specifier{Field: "path", Glob: "internal/**"}, Decision: "deny"},
		},
	}

	t.Run("allow read in src/**", func(t *testing.T) {
		dec, reason := engine.Evaluate("read_file", json.RawMessage(`{"path":"src/main.go"}`))
		if dec != "allow" {
			t.Errorf("decision = %q, want allow", dec)
		}
		if reason != "matched allow rule: read_file (path glob: src/**)" {
			t.Errorf("reason = %q", reason)
		}
	})

	t.Run("no allow outside src/**", func(t *testing.T) {
		dec, _ := engine.Evaluate("read_file", json.RawMessage(`{"path":"other/main.go"}`))
		if dec != "" {
			t.Errorf("decision = %q, want empty (no match)", dec)
		}
	})

	t.Run("deny write in internal/**", func(t *testing.T) {
		dec, _ := engine.Evaluate("write_file", json.RawMessage(`{"path":"internal/pkg/foo.go"}`))
		if dec != "deny" {
			t.Errorf("decision = %q, want deny", dec)
		}
	})
}
