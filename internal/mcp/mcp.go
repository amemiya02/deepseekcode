// Package mcp implements the Model Context Protocol (MCP) client for
// stdio transport. It spawns MCP-compliant subprocesses, runs the
// JSON-RPC 2.0 initialize handshake, discovers tools, and bridges them
// into deepseekcode's tools.Tool interface so the agent can invoke MCP
// tools alongside built-in ones.
//
// Per D5 (ARCHITECTURE.md §4.5), only stdio transport is supported.
// Remote transports (HTTP/SSE) and OAuth are deferred to v0.4.
package mcp
