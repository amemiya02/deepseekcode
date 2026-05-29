package llmtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amemiya02/deepseekcode/internal/llm"
)

// collect drains a stream channel into its terminal Event (EventFinish or
// EventError), returning the accumulated text/reasoning seen on the way.
func collect(t *testing.T, ch <-chan llm.Event) (final llm.Event, text, reasoning string) {
	t.Helper()
	for ev := range ch {
		switch ev.Type {
		case llm.EventTextDelta:
			text += ev.Text
		case llm.EventReasoningDelta:
			reasoning += ev.Text
		case llm.EventFinish, llm.EventError:
			final = ev
		}
	}
	return final, text, reasoning
}

func TestServer_StreamsTextReasoningAndUsage(t *testing.T) {
	srv := NewServer(Turn{
		Reasoning: "let me think",
		Text:      "hello",
		Usage:     &Usage{Prompt: 200, Completion: 10, CacheHit: 180, CacheMiss: 20},
	})
	defer srv.Close()

	ch, err := srv.Client().Stream(context.Background(), llm.Request{Model: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	final, text, reasoning := collect(t, ch)

	if final.Type != llm.EventFinish {
		t.Fatalf("want EventFinish, got %v (err=%v)", final.Type, final.Err)
	}
	if text != "hello" {
		t.Errorf("text = %q, want %q", text, "hello")
	}
	if reasoning != "let me think" {
		t.Errorf("reasoning = %q, want %q", reasoning, "let me think")
	}
	if final.FinishReason != "stop" {
		t.Errorf("finish = %q, want stop", final.FinishReason)
	}
	if final.Usage.PromptCacheHitTokens != 180 {
		t.Errorf("cache-hit tokens = %d, want 180", final.Usage.PromptCacheHitTokens)
	}
	if srv.Count() != 1 {
		t.Errorf("served %d requests, want 1", srv.Count())
	}
}

func TestServer_ToolCallAssembled(t *testing.T) {
	srv := NewServer(Turn{
		ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Args: `{"text":"hi"}`}},
	})
	defer srv.Close()

	ch, err := srv.Client().Stream(context.Background(), llm.Request{Model: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	final, _, _ := collect(t, ch)
	if final.Type != llm.EventFinish {
		t.Fatalf("want EventFinish, got %v", final.Type)
	}
	if final.FinishReason != "tool_calls" {
		t.Errorf("finish = %q, want tool_calls (inferred)", final.FinishReason)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "c1" || final.ToolCalls[0].Function.Name != "echo" {
		t.Fatalf("tool calls not assembled: %+v", final.ToolCalls)
	}
	if final.ToolCalls[0].Function.Arguments != `{"text":"hi"}` {
		t.Errorf("args = %q", final.ToolCalls[0].Function.Arguments)
	}
}

func TestServer_StatusFaultIsAPIError(t *testing.T) {
	srv := NewServer(Turn{Status: 500, Body: "boom"})
	defer srv.Close()

	_, err := srv.Client().Stream(context.Background(), llm.Request{Model: "deepseek-v4-flash"})
	if err == nil {
		t.Fatal("expected an error from a 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not mention status 500", err.Error())
	}
}

func TestServer_FirstTokenTimeout(t *testing.T) {
	srv := NewServer(Turn{DelayFirst: 200 * time.Millisecond, Text: "late"})
	defer srv.Close()

	c := srv.Client()
	c.FirstTokenTimeout = 30 * time.Millisecond
	c.ChunkStallTimeout = time.Second

	ch, err := c.Stream(context.Background(), llm.Request{Model: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("Stream connect: %v", err)
	}
	final, _, _ := collect(t, ch)
	if final.Type != llm.EventError {
		t.Fatalf("want EventError, got %v", final.Type)
	}
	if !strings.Contains(final.Err.Error(), "first-token timeout") {
		t.Errorf("error %q is not a first-token timeout", final.Err.Error())
	}
}

func TestServer_AutoTerminatesWhenUnderScripted(t *testing.T) {
	srv := NewServer() // no turns scripted
	defer srv.Close()

	ch, err := srv.Client().Stream(context.Background(), llm.Request{Model: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	final, _, _ := collect(t, ch)
	if final.Type != llm.EventFinish || final.FinishReason != "stop" {
		t.Fatalf("synthetic terminal turn should finish with stop, got %v/%q", final.Type, final.FinishReason)
	}
}
