package tools

import (
	"context"
	"encoding/json"
	"strings"
)

// Option is a single selectable answer for a Question.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Question is one question the model wants to ask the user.
type Question struct {
	Question string   `json:"question"`
	Header   string   `json:"header"` // ≤30 char short label
	Options  []Option `json:"options"`
	Multiple bool     `json:"multiple"` // allow multi-select
}

// QuestionRequest groups one or more questions to present together.
type QuestionRequest struct {
	Questions []Question
}

// QuestionResponse carries the user's answers, aligned with
// QuestionRequest.Questions. Each inner slice holds the selected
// label strings for that question.
type QuestionResponse struct {
	Answers [][]string
}

// Questioner is implemented by the agent layer. AskQuestion blocks
// until the user answers or ctx is cancelled.
type Questioner interface {
	AskQuestion(ctx context.Context, req QuestionRequest) (QuestionResponse, error)
}

// QuestionTool presents questions to the user and returns their answers.
type QuestionTool struct{ q Questioner }

// NewQuestionTool creates a QuestionTool backed by the given Questioner.
func NewQuestionTool(q Questioner) *QuestionTool { return &QuestionTool{q: q} }

func (*QuestionTool) Name() string { return "question" }

func (*QuestionTool) Description() string {
	return "Ask the user one or more questions with selectable options. " +
		"Blocks until the user answers. Use for clarification or preference gathering."
}

func (*QuestionTool) Parameters() json.RawMessage {
	return MustParams(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{"type": "string"},
						"header":   map[string]any{"type": "string"},
						"options": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label":       map[string]any{"type": "string"},
									"description": map[string]any{"type": "string"},
								},
								"required": []string{"label"},
							},
						},
						"multiple": map[string]any{"type": "boolean"},
					},
					"required": []string{"question", "options"},
				},
			},
		},
		"required": []string{"questions"},
	})
}

func (t *QuestionTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var p struct {
		Questions []Question `json:"questions"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return Errf("question: invalid args: %v", err), nil
	}
	if len(p.Questions) == 0 {
		return Errf("question: no questions provided"), nil
	}
	resp, err := t.q.AskQuestion(ctx, QuestionRequest{Questions: p.Questions})
	if err != nil {
		return Errf("question cancelled: %v", err), nil
	}
	var b strings.Builder
	b.WriteString("User answered:")
	for i, q := range p.Questions {
		b.WriteString("\n- ")
		if q.Header != "" {
			b.WriteString(q.Header)
			b.WriteString(": ")
		}
		if i < len(resp.Answers) && len(resp.Answers[i]) > 0 {
			b.WriteString(strings.Join(resp.Answers[i], ", "))
		} else {
			b.WriteString("(no selection)")
		}
	}
	return Result{Content: b.String()}, nil
}

func (*QuestionTool) IsReadOnly() bool { return true }
