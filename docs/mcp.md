# MCP

`deepseekcode` supports the [Model Context Protocol](https://modelcontextprotocol.io/)
for extending its tool surface with user-provided servers.

> v0.1 status: configuration is parsed; the runtime bridge is on the
> v0.2 roadmap. Until then, this document records the planned shape so
> you can stage `[mcp_servers]` entries in your config in advance.

## Configuration

Servers are declared in `~/.deepseek/config.toml`. None are enabled by
default — the built-in 12-tool surface is what ships.

```toml
[mcp_servers.example]
command = "node"
args = ["/path/to/server.js"]
env = { FOO = "bar" }

[mcp_servers.git]
command = "uvx"
args = ["mcp-server-git", "--repository", "."]
```

## Lifecycle (v0.2)

- Servers spawn **lazily** on first tool-call referencing them (not at
  startup). This keeps `dsc`'s cold-start latency under 100ms.
- Stdio transport only. SSE/HTTP transports come later.
- Tools provided by an MCP server appear in the registry under their
  declared names. Conflicts with built-ins are resolved in favor of MCP
  (the user explicitly enabled it; presumably they meant it to win).
- Server processes inherit `deepseekcode`'s working directory and a
  reduced environment derived from `[mcp_servers.X.env]`.

## HTTP transport

`HTTPTransport` speaks JSON-RPC 2.0 over HTTP POST to remote MCP servers.
It replaces the stdio process spawn when the server URL is an `http://` or
`https://` endpoint rather than a local command.

- **Framing**: each `Send` posts a single JSON-RPC request envelope
  (`jsonrpc: "2.0"`, monotonic `id`, `method`, `params`) and expects a
  single JSON-RPC response.
- **Notifications**: `Notify` posts a JSON-RPC notification (no `id`,
  fire-and-forget).
- **Timeout**: 30 seconds per request (configurable on the underlying
  `http.Client`).
- **Concurrency**: request IDs are assigned via `atomic.Int64`, so
  concurrent `Send` calls are safe.
- **Errors**: non-2xx HTTP status returns an error with status code and
  body; a JSON-RPC `error` object returns an error with `code` and
  `message`.

## Permissions

MCP tools go through the same permission tier as built-ins. Each MCP
tool is treated as a non-read-only tool unless it explicitly declares
its safety in the MCP `tools/list` response. The Duet validator
considers MCP tool calls the same way it considers built-ins — paths
inside `affected_paths` and matching destructive patterns trigger Pro.
