package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// McpToolMeta is the metadata for a single MCP server tool.
type McpToolMeta struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// listTools calls tools/list and returns the server's tools.
func listTools(ctx context.Context, t Transport) ([]McpToolMeta, error) {
	raw, err := t.Send(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []McpToolMeta `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list response: %w", err)
	}
	return result.Tools, nil
}

// callToolContent is one content block in a tools/call response.
type callToolContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"` // base64 for images
}

// callTool calls tools/call and returns the concatenated text content.
// isError is true if the server indicated the tool call itself errored.
func callTool(ctx context.Context, t Transport, name string, args json.RawMessage) (content string, isError bool, err error) {
	params := struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}{
		Name:      name,
		Arguments: args,
	}
	raw, err := t.Send(ctx, "tools/call", params)
	if err != nil {
		return "", false, err
	}
	var result struct {
		Content []callToolContent `json:"content"`
		IsError bool              `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", false, fmt.Errorf("parse tools/call response: %w", err)
	}

	var out string
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			out += c.Text
		case "image":
			out += "[image: " + c.MIMEType + "]"
		default:
			out += fmt.Sprintf("[%s content]", c.Type)
		}
	}
	return out, result.IsError, nil
}
