package mcp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/amemiya02/deepseekcode/internal/version"
)

// ServerCapabilities is the subset of MCP server capabilities we care
// about. Only tools matter today; resources and prompts may come later.
type ServerCapabilities struct {
	Tools     bool
	Resources bool
	Prompts   bool
}

// clientInfo is sent during initialize.
type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeParams is the initialize request params.
type initializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	ClientInfo      clientInfo `json:"clientInfo"`
	Capabilities    clientCaps `json:"capabilities"`
}

type clientCaps struct{}

// initializeResult is the shape of the initialize response.
type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapsResponse `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type serverCapsResponse struct {
	Tools     *struct{} `json:"tools,omitempty"`
	Resources *struct{} `json:"resources,omitempty"`
	Prompts   *struct{} `json:"prompts,omitempty"`
}

// initialize runs the MCP initialize handshake: send initialize, parse
// capabilities, then send the initialized notification.
func initialize(ctx context.Context, t Transport) (ServerCapabilities, error) {
	params := initializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo: clientInfo{
			Name:    "deepseekcode",
			Version: version.Version,
		},
		Capabilities: clientCaps{},
	}

	raw, err := t.Send(ctx, "initialize", params)
	if err != nil {
		return ServerCapabilities{}, err
	}

	var result initializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ServerCapabilities{}, err
	}

	if result.ProtocolVersion != ProtocolVersion {
		slog.Warn("mcp protocol version mismatch",
			"server", result.ProtocolVersion,
			"client", ProtocolVersion,
		)
	}

	caps := ServerCapabilities{
		Tools:     result.Capabilities.Tools != nil,
		Resources: result.Capabilities.Resources != nil,
		Prompts:   result.Capabilities.Prompts != nil,
	}

	if err := t.Notify(ctx, "notifications/initialized", nil); err != nil {
		return caps, err
	}

	return caps, nil
}
