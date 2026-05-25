package agent

import (
	"context"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestAgentAskQuestion(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	go func() {
		ev := <-a.Events()
		qa, ok := ev.(EventQuestionAsk)
		if !ok {
			t.Errorf("expected EventQuestionAsk, got %T", ev)
			return
		}
		qa.Reply <- tools.QuestionResponse{Answers: [][]string{{"pg"}}}
	}()
	req := tools.QuestionRequest{
		Questions: []tools.Question{{Question: "DB?", Options: []tools.Option{{Label: "pg"}}}},
	}
	resp, err := a.AskQuestion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Answers) != 1 || resp.Answers[0][0] != "pg" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestAgentAskQuestionCancel(t *testing.T) {
	a := New(nil, tools.New(), nil, "")
	// Drain the event in background so send doesn't block.
	go func() { <-a.Events() }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req := tools.QuestionRequest{
		Questions: []tools.Question{{Question: "Q?", Options: []tools.Option{{Label: "a"}}}},
	}
	_, err := a.AskQuestion(ctx, req)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
