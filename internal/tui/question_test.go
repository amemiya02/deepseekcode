package tui

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestQuestionFlowSingleSelect(t *testing.T) {
	q := NewQuestionFlow()
	if q.Active() {
		t.Fatal("should not be active initially")
	}
	reply := make(chan tools.QuestionResponse, 1)
	q.Open(agent.EventQuestionAsk{
		Questions: []tools.Question{
			{Question: "DB?", Header: "Database", Options: []tools.Option{{Label: "pg"}, {Label: "mysql"}}},
		},
		Reply: reply,
	})
	if !q.Active() {
		t.Fatal("should be active after Open")
	}
	// Move down to mysql
	q.MoveDelta(1)
	if q.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", q.cursor)
	}
	// Select (single-select: enter confirms cursor item)
	done, resp, ch := q.Next()
	if !done {
		t.Fatal("expected done=true for single question")
	}
	if ch == nil {
		t.Fatal("expected non-nil reply channel")
	}
	if len(resp.Answers) != 1 || resp.Answers[0][0] != "mysql" {
		t.Errorf("answers = %v, want [[mysql]]", resp.Answers)
	}
}

func TestQuestionFlowMultiSelect(t *testing.T) {
	q := NewQuestionFlow()
	reply := make(chan tools.QuestionResponse, 1)
	q.Open(agent.EventQuestionAsk{
		Questions: []tools.Question{
			{Question: "Pick", Header: "Langs", Multiple: true, Options: []tools.Option{{Label: "x"}, {Label: "y"}, {Label: "z"}}},
		},
		Reply: reply,
	})
	// Toggle x
	q.Toggle()
	// Move to z
	q.MoveDelta(2)
	q.Toggle()
	done, resp, _ := q.Next()
	if !done {
		t.Fatal("expected done=true")
	}
	if len(resp.Answers[0]) != 2 || resp.Answers[0][0] != "x" || resp.Answers[0][1] != "z" {
		t.Errorf("answers = %v, want [x z]", resp.Answers[0])
	}
}

func TestQuestionFlowMultiQuestion(t *testing.T) {
	q := NewQuestionFlow()
	reply := make(chan tools.QuestionResponse, 1)
	q.Open(agent.EventQuestionAsk{
		Questions: []tools.Question{
			{Question: "Q1", Options: []tools.Option{{Label: "a"}, {Label: "b"}}},
			{Question: "Q2", Multiple: true, Options: []tools.Option{{Label: "x"}, {Label: "y"}}},
		},
		Reply: reply,
	})
	// Q1: select a (cursor=0, default)
	done, _, _ := q.Next()
	if done {
		t.Fatal("should not be done after first question")
	}
	// Q2: toggle y
	q.MoveDelta(1)
	q.Toggle()
	done, resp, ch := q.Next()
	if !done {
		t.Fatal("expected done after second question")
	}
	if ch == nil {
		t.Fatal("expected reply channel")
	}
	if len(resp.Answers) != 2 {
		t.Fatalf("got %d answers, want 2", len(resp.Answers))
	}
	if resp.Answers[0][0] != "a" {
		t.Errorf("Q1 = %v, want [a]", resp.Answers[0])
	}
	if resp.Answers[1][0] != "y" {
		t.Errorf("Q2 = %v, want [y]", resp.Answers[1])
	}
}

func TestQuestionFlowCancel(t *testing.T) {
	q := NewQuestionFlow()
	reply := make(chan tools.QuestionResponse, 1)
	q.Open(agent.EventQuestionAsk{
		Questions: []tools.Question{{Question: "Q?", Options: []tools.Option{{Label: "a"}}}},
		Reply:     reply,
	})
	ch := q.Cancel()
	if ch == nil {
		t.Fatal("expected non-nil reply channel from Cancel")
	}
	if q.Active() {
		t.Fatal("should not be active after Cancel")
	}
}

func TestQuestionFlowNoOptions(t *testing.T) {
	q := NewQuestionFlow()
	reply := make(chan tools.QuestionResponse, 1)
	q.Open(agent.EventQuestionAsk{
		Questions: []tools.Question{{Question: "Free?"}},
		Reply:     reply,
	})
	done, resp, _ := q.Next()
	if !done {
		t.Fatal("expected done=true")
	}
	if len(resp.Answers[0]) != 0 {
		t.Errorf("expected empty answer for no-options question, got %v", resp.Answers[0])
	}
}
