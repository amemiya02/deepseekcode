package permissions

import (
	"testing"
)

func TestParseSpecifierRule(t *testing.T) {
	t.Run("Bash expands to bash+bash_pty", func(t *testing.T) {
		rules, err := ParseSpecifierRule("Bash(go test:*)", "allow")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rules) != 2 {
			t.Fatalf("want 2 rules (bash+bash_pty), got %d", len(rules))
		}
		if rules[0].ToolPattern != "bash" || rules[1].ToolPattern != "bash_pty" {
			t.Errorf("tool patterns = %q, %q; want bash, bash_pty", rules[0].ToolPattern, rules[1].ToolPattern)
		}
		for _, r := range rules {
			if r.Decision != "allow" {
				t.Errorf("decision = %q, want allow", r.Decision)
			}
			if r.Specifier == nil {
				t.Fatal("specifier should not be nil")
			}
			if r.Specifier.Field != "command" {
				t.Errorf("field = %q, want command", r.Specifier.Field)
			}
			if r.Specifier.Prefix != "go test" {
				t.Errorf("prefix = %q, want 'go test'", r.Specifier.Prefix)
			}
		}
	})

	t.Run("Bash without star suffix", func(t *testing.T) {
		rules, err := ParseSpecifierRule("Bash(go test)", "deny")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rules[0].Specifier.Prefix != "go test" {
			t.Errorf("prefix = %q, want 'go test'", rules[0].Specifier.Prefix)
		}
	})

	t.Run("Read expands to read_file", func(t *testing.T) {
		rules, err := ParseSpecifierRule("Read(src/**)", "allow")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("want 1 rule, got %d", len(rules))
		}
		if rules[0].ToolPattern != "read_file" {
			t.Errorf("tool pattern = %q, want read_file", rules[0].ToolPattern)
		}
		if rules[0].Specifier.Field != "path" {
			t.Errorf("field = %q, want path", rules[0].Specifier.Field)
		}
		if rules[0].Specifier.Glob != "src/**" {
			t.Errorf("glob = %q, want src/**", rules[0].Specifier.Glob)
		}
	})

	t.Run("Edit expands to edit_file", func(t *testing.T) {
		rules, err := ParseSpecifierRule("Edit(internal/**)", "deny")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rules[0].ToolPattern != "edit_file" {
			t.Errorf("tool pattern = %q, want edit_file", rules[0].ToolPattern)
		}
	})

	t.Run("bare tool name without specifier", func(t *testing.T) {
		rules, err := ParseSpecifierRule("bash", "ask")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// bare "bash" is not in toolFamily, so treated as literal
		if len(rules) != 1 {
			t.Fatalf("want 1 rule, got %d", len(rules))
		}
		if rules[0].Specifier != nil {
			t.Error("specifier should be nil for bare tool name")
		}
	})

	t.Run("star specifier", func(t *testing.T) {
		rules, err := ParseSpecifierRule("Bash(*)", "allow")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rules[0].Specifier.Prefix != "" {
			t.Errorf("prefix = %q, want empty", rules[0].Specifier.Prefix)
		}
	})

	t.Run("empty rule", func(t *testing.T) {
		_, err := ParseSpecifierRule("", "allow")
		if err == nil {
			t.Error("expected error for empty rule")
		}
	})

	t.Run("unbalanced parens", func(t *testing.T) {
		_, err := ParseSpecifierRule("Bash(go test", "allow")
		if err == nil {
			t.Error("expected error for unbalanced parens")
		}
	})

	t.Run("missing tool name", func(t *testing.T) {
		_, err := ParseSpecifierRule("(foo)", "allow")
		if err == nil {
			t.Error("expected error for missing tool name")
		}
	})
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		glob, path string
		want       bool
	}{
		{"src/**", "src/foo/bar.go", true},
		{"src/**", "src/foo.go", true},
		{"src/**", "src/", true},
		{"src/**", "other/foo.go", false},
		{"*.go", "main.go", true},
		{"*.go", "dir/main.go", false},
		{"internal/**/x.go", "internal/pkg/x.go", true},
		{"internal/**/x.go", "internal/x.go", true},
		{"internal/**/x.go", "other/x.go", false},
		{"**", "any/path/here", true},
		{"*.go", "main.rs", false},
	}
	for _, tc := range tests {
		got := globMatch(tc.glob, tc.path)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.glob, tc.path, got, tc.want)
		}
	}
}
