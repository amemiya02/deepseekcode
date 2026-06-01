# MCP

`deepseekcode` supports the [Model Context Protocol](https://modelcontextprotocol.io/)
for extending its tool surface with user-provided servers.

## Configuration

Servers are declared in `~/.deepseek/config.toml`. None are enabled by
default — the built-in tool surface ships without MCP.

```toml
[mcp_servers.example]
command = "node"
args = ["/path/to/server.js"]
env = { FOO = "bar" }

[mcp_servers.git]
command = "uvx"
args = ["mcp-server-git", "--repository", "."]
```

## Shipped behavior

The following MCP features are implemented and available today:

### Stdio transport

MCP servers are spawned as subprocesses and communicate over
stdin/stdout using JSON-RPC 2.0 with Content-Length framing. This is
the only transport currently supported.

### Startup connection

Configured MCP servers are connected during `dsc` startup. DeepSeekCode
spawns the server process, runs the MCP `initialize` handshake, calls
`tools/list`, and bridges the resulting tools into the session registry
before the first model turn.

### Tool bridge

Tools provided by an MCP server appear in the registry under their
declared names, prefixed as `mcp__<server>__<tool>`. The model calls
them like any built-in tool. Conflicts with built-ins are resolved in
favor of MCP (the user explicitly enabled it).

### Lifecycle management

- Server processes inherit `deepseekcode`'s working directory and a
  reduced environment derived from `[mcp_servers.X.env]`.
- If a server process dies, the registry detects it, marks the server
  as **degraded**, and attempts one automatic reconnect with a 10-second
  timeout. On failure, a 30-second backoff is applied before further
  attempts are allowed.
- `dsc` monitors for MCP tool-list drift between reconnections (tools
  added, removed, or schema changed) and surfaces changes to the agent
  loop so the model's tool list stays current.

### Permissions

MCP tools go through the same permission tier as built-ins. Each MCP
tool is treated as a non-read-only tool unless it explicitly declares
its safety in the MCP `tools/list` response. The Duet validator
considers MCP tool calls the same way it considers built-ins — paths
inside `affected_paths` and matching destructive patterns trigger Pro.

### Per-call timeout

Each MCP tool call is subject to a 60-second default timeout, which
can be overridden per server via the registry's `SetTimeout` API.

### `/mcp` status overlay

Type `/mcp` in the TUI to open a fullscreen overlay showing all
configured MCP servers: name, lifecycle state (connected/degraded/
failed), tool count, backoff timer, and last error. The list is
filterable — type to narrow by server name.

## Troubleshooting

- **Server fails to start:** Check that the `command` binary is on
  your PATH and that `args` point to a valid server script. The TUI
  status bar shows the total count of connected MCP tools.
- **Tools not appearing:** The server must complete the MCP initialize
  handshake and respond to `tools/list`. Check the server's stderr
  output for errors.
- **Degraded state:** If a server process crashes, `dsc` will attempt
  one reconnect. If that also fails, the server enters a backoff
  period. Restart `dsc` to reset.

## Roadmap

The following MCP features are planned but not yet shipped:

- **HTTP/SSE transport** — connect to remote MCP servers over HTTP
  with Server-Sent Events for streaming.
- **Per-tool enable/disable** — selectively enable or disable
  individual MCP tools without removing the server config.
