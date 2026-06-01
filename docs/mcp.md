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

### SSE transport

Remote MCP servers using Server-Sent Events can be configured with
`transport = "sse"` and a `url` field:

```toml
[mcp_servers.remote-tools]
transport = "sse"
url = "https://example.com/mcp/sse"
```

The `url` should point to the server's SSE endpoint. The transport
automatically discovers the POST URL from the initial `endpoint` event.
If `transport` is omitted, it defaults to `"stdio"`.

### Per-tool enable/disable

Individual MCP tools can be selectively exposed or hidden using
`enabled_tools` (allowlist) or `disabled_tools` (blocklist). These
are mutually exclusive — use one or the other, not both.

```toml
[mcp_servers.fs]
command = "mcp-fs"
disabled_tools = ["delete_file"]

[mcp_servers.github]
command = "mcp-github"
enabled_tools = ["search_issues", "read_file"]
```

Tools listed in `disabled_tools` are not visible to the model even if
the server provides them. `enabled_tools` exposes only the listed
tools; all others from that server are hidden.

## Shipped behavior

The following MCP features are implemented and available today:

### Stdio transport

MCP servers are spawned as subprocesses and communicate over
stdin/stdout using JSON-RPC 2.0 with newline-delimited messages. This
is the default transport when `transport` is omitted.

### SSE transport

Remote MCP servers can be reached via Server-Sent Events. The transport
opens an SSE stream to discover the POST endpoint, then sends JSON-RPC
requests over HTTP POST and receives responses as SSE events. Reconnect
and backoff are handled by the same lifecycle manager as stdio.

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

No MCP features are currently on the roadmap. The shipped transport
options (stdio, SSE), lifecycle management, tool bridge, drift
detection, and per-tool enable/disable cover the core MCP workflow.
