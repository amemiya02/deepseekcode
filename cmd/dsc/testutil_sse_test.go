package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// --- SSE wire types shared by e2e tests ---

type sseChunkOut struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []sseChoiceOut `json:"choices"`
	Usage   *usageOut      `json:"usage,omitempty"`
}

type sseChoiceOut struct {
	Index        int         `json:"index"`
	Delta        sseDeltaOut `json:"delta"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type sseDeltaOut struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []sseToolCallOut `json:"tool_calls,omitempty"`
}

type sseToolCallOut struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function sseFunctionOut `json:"function"`
}

type sseFunctionOut struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type usageOut struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
}

// --- fake response model ---

type fakeResponse struct {
	toolCalls []fakeToolCall
	text      string
}

type fakeToolCall struct {
	ID   string
	Name string
	Args string
}

// --- SSE helpers ---

func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}
