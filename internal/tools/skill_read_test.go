package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeSkillSource struct {
	bodies map[string]string
}

func (f fakeSkillSource) Body(name string) (string, bool) {
	b, ok := f.bodies[name]
	return b, ok
}

func (f fakeSkillSource) Names() []string {
	out := make([]string, 0, len(f.bodies))
	for n := range f.bodies {
		out = append(out, n)
	}
	return out
}

func TestSkillRead_ReturnsBody(t *testing.T) {
	src := fakeSkillSource{bodies: map[string]string{"review": "do a careful review"}}
	tool := NewSkillReadTool(src)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"review"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	if res.Content != "do a careful review" {
		t.Errorf("Content = %q", res.Content)
	}
}

func TestSkillRead_UnknownListsAvailable(t *testing.T) {
	src := fakeSkillSource{bodies: map[string]string{"review": "x", "plan": "y"}}
	tool := NewSkillReadTool(src)

	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"name":"nope"}`))
	if !res.IsError {
		t.Fatal("expected error result for unknown skill")
	}
	if !strings.Contains(res.Content, "plan") || !strings.Contains(res.Content, "review") {
		t.Errorf("error should list available skills, got %q", res.Content)
	}
}

func TestSkillRead_MissingName(t *testing.T) {
	tool := NewSkillReadTool(fakeSkillSource{})
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestSkillRead_ReadOnly(t *testing.T) {
	var tool ReadOnlyHint = NewSkillReadTool(fakeSkillSource{})
	if !tool.IsReadOnly() {
		t.Error("skill_read must be read-only")
	}
}
