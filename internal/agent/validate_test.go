package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

type fakeTool struct {
	name   string
	schema string
}

func (f fakeTool) Name() string                { return f.name }
func (f fakeTool) Description() string         { return "" }
func (f fakeTool) Parameters() json.RawMessage { return json.RawMessage(f.schema) }
func (f fakeTool) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}

func TestValidateToolArgs(t *testing.T) {
	tool := fakeTool{
		name:   "edit_file",
		schema: `{"type":"object","properties":{"path":{"type":"string"},"new":{"type":"string"}},"required":["path","new"]}`,
	}

	cases := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{"all required present", `{"path":"a.go","new":"x"}`, false},
		{"missing required field", `{"path":"a.go"}`, true},
		{"extra optional fields ok", `{"path":"a.go","new":"x","extra":"y"}`, false},
		{"non-object", `"a string"`, true},
		{"malformed json", `{not json`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateToolArgs(tool, json.RawMessage(c.args))
			if (err != nil) != c.wantErr {
				t.Errorf("validateToolArgs(%s) err=%v, wantErr=%v", c.args, err, c.wantErr)
			}
		})
	}
}

func TestValidateToolArgsNoSchema(t *testing.T) {
	tool := fakeTool{name: "noop", schema: ""}
	if err := validateToolArgs(tool, json.RawMessage(`{}`)); err != nil {
		t.Errorf("empty schema should accept anything, got %v", err)
	}
}

func TestValidateToolArgsSchemaWithoutRequired(t *testing.T) {
	tool := fakeTool{
		name:   "ls",
		schema: `{"type":"object","properties":{"path":{"type":"string"}}}`,
	}
	if err := validateToolArgs(tool, json.RawMessage(`{}`)); err != nil {
		t.Errorf("schema without required should accept {}, got %v", err)
	}
}
