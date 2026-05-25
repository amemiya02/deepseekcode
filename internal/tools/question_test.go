package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type mockQuestioner struct {
	resp QuestionResponse
	err  error
}

func (m *mockQuestioner) AskQuestion(_ context.Context, req QuestionRequest) (QuestionResponse, error) {
	return m.resp, m.err
}

func TestQuestionToolSingleSelect(t *testing.T) {
	mock := &mockQuestioner{resp: QuestionResponse{Answers: [][]string{{"pg"}}}}
	tool := NewQuestionTool(mock)
	if tool.Name() != "question" {
		t.Fatalf("name = %q", tool.Name())
	}
	if !tool.IsReadOnly() {
		t.Error("expected IsReadOnly")
	}
	args := json.RawMessage(`{"questions":[{"question":"DB?","header":"Database","options":[{"label":"pg","description":"Postgres"},{"label":"mysql","description":"MySQL"}],"multiple":false}]}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Database: pg") {
		t.Errorf("expected Database: pg, got: %s", res.Content)
	}
}

func TestQuestionToolMultiSelect(t *testing.T) {
	mock := &mockQuestioner{resp: QuestionResponse{Answers: [][]string{{"x", "z"}}}}
	tool := NewQuestionTool(mock)
	args := json.RawMessage(`{"questions":[{"question":"Pick","header":"Langs","options":[{"label":"x"},{"label":"y"},{"label":"z"}],"multiple":true}]}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "x, z") {
		t.Errorf("expected 'x, z', got: %s", res.Content)
	}
}

func TestQuestionToolEmptyQuestions(t *testing.T) {
	mock := &mockQuestioner{}
	tool := NewQuestionTool(mock)
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"questions":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for empty questions")
	}
}

func TestQuestionToolCancel(t *testing.T) {
	mock := &mockQuestioner{err: context.Canceled}
	tool := NewQuestionTool(mock)
	args := json.RawMessage(`{"questions":[{"question":"Q?","options":[{"label":"a"}]}]}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Error("expected error for cancel")
	}
	if !strings.Contains(res.Content, "cancelled") {
		t.Errorf("expected 'cancelled' in error, got: %s", res.Content)
	}
}

func TestQuestionToolNoSelection(t *testing.T) {
	mock := &mockQuestioner{resp: QuestionResponse{Answers: [][]string{{}}}}
	tool := NewQuestionTool(mock)
	args := json.RawMessage(`{"questions":[{"question":"Q?","header":"H","options":[{"label":"a"}]}]}`)
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "(no selection)") {
		t.Errorf("expected '(no selection)', got: %s", res.Content)
	}
}

func TestQuestionToolRegistry(t *testing.T) {
	mock := &mockQuestioner{}
	reg := New()
	reg.Register(NewQuestionTool(mock))
	if _, ok := reg.Get("question"); !ok {
		t.Error("question tool not found in registry")
	}
}
