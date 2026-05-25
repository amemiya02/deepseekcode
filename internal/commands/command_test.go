package commands

import (
	"testing"
)

func TestParseCommand_FullFrontmatter(t *testing.T) {
	input := `---
description: git commit and push
model: deepseek-v4-pro
subtask: true
---
commit and push
## DIFF
` + "`" + "!" + "`" + "git diff" + "`"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "git commit and push" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Model != "deepseek-v4-pro" {
		t.Errorf("Model = %q", got.Model)
	}
	if !got.Subtask {
		t.Error("Subtask should be true")
	}
	wantTmpl := "commit and push\n## DIFF\n`!`git diff`"
	if got.Template != wantTmpl {
		t.Errorf("Template =\n%q\nwant:\n%q", got.Template, wantTmpl)
	}
}

func TestParseCommand_NoFrontmatter(t *testing.T) {
	input := "just a template\nline 2"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "" || got.Model != "" || got.Subtask {
		t.Error("fields should be zero")
	}
	if got.Template != input {
		t.Errorf("Template = %q, want %q", got.Template, input)
	}
}

func TestParseCommand_UnclosedFrontmatter(t *testing.T) {
	input := `---
description: unclosed
model: deepseek-v4-flash
this whole thing is template`
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	// No closing "---" → entire content is template.
	if got.Description != "" {
		t.Errorf("Description should be empty, got %q", got.Description)
	}
	if got.Template != input {
		t.Errorf("Template should be entire input")
	}
}

func TestParseCommand_QuotedModel(t *testing.T) {
	input := "---\nmodel: \"deepseek-v4-pro\"\n---\ntemplate"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "deepseek-v4-pro" {
		t.Errorf("Model = %q, want unquoted", got.Model)
	}
}

func TestParseCommand_ColonInDescription(t *testing.T) {
	input := "---\ndescription: url: https://example.com\n---\ntmpl"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "url: https://example.com" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestParseCommand_EmptyFile(t *testing.T) {
	got, err := ParseCommand("")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Command{}) {
		t.Errorf("empty input should return zero Command, got %+v", got)
	}
}

func TestParseCommand_SubtaskBoolValues(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"True", true},
		{"garbage", false},
	} {
		input := "---\nsubtask: " + tc.val + "\n---\ntmpl"
		got, err := ParseCommand(input)
		if err != nil {
			t.Fatalf("subtask=%s: %v", tc.val, err)
		}
		if got.Subtask != tc.want {
			t.Errorf("subtask=%s → %v, want %v", tc.val, got.Subtask, tc.want)
		}
	}
}

func TestParseCommand_UnknownKeysIgnored(t *testing.T) {
	input := "---\nfoo: bar\ndescription: ok\n---\ntmpl"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "ok" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestParseCommand_AgentParsed(t *testing.T) {
	input := "---\nagent: planner\n---\ntmpl"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "planner" {
		t.Errorf("Agent = %q, want planner", got.Agent)
	}
}

func TestParseCommand_SingleQuotes(t *testing.T) {
	input := "---\ndescription: 'my desc'\nmodel: 'deepseek-v4-pro'\n---\ntmpl"
	got, err := ParseCommand(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "my desc" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Model != "deepseek-v4-pro" {
		t.Errorf("Model = %q", got.Model)
	}
}
