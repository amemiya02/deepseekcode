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

## Permissions

MCP tools go through the same permission tier as built-ins. Each MCP
tool is treated as a non-read-only tool unless it explicitly declares
its safety in the MCP `tools/list` response. The Duet validator
considers MCP tool calls the same way it considers built-ins — paths
inside `affected_paths` and matching destructive patterns trigger Pro.
