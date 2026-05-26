# Prefix Cache Drift Detection

DeepSeek's prompt cache gives a **50× discount** on cached tokens. The
cache matches on the **prefix** of the request — everything before the
first differing byte. If the prefix changes between turns, the cache
misses and costs spike.

## What must stay stable

The cache-stable prefix consists of:

1. **System prompt** — the static part before `DynamicContextBoundary`.
2. **Tool definitions** — the full tool list (names, descriptions,
   parameter schemas).

Both are serialized deterministically (`MarshalCacheStable` sorts tools
by name and canonicalizes JSON-Schema keys). Any change to either
component invalidates the cache.

## Fingerprint algorithm

`ComputeFingerprint` (in `internal/llm/prefix_drift.go`) hashes:

- `SystemSHA256` = SHA-256 of the static system prompt
- `ToolsSHA256` = SHA-256 of canonical serialized tool specs (sorted by name, with canonicalized JSON-Schema)
- `CombinedSHA256` = SHA-256 of `SystemSHA256:ToolsSHA256`

Tool specs are **fully serialized** (name, description, parameters) and
canonicalized before hashing:

1. Tools are sorted by function name
2. Parameter schemas are canonicalized by recursive key-sort

This means tool description changes and schema changes (but not key order
differences) will trigger drift detection.

## How drift detection works

`PrefixMonitor` runs inside `runStep`, after the request is assembled
but before `Stream` is called:

1. **First turn**: pins the baseline fingerprint. No alert.
2. **Subsequent turns**: recomputes the fingerprint and compares to the
   pinned baseline.
3. **On drift**: re-pins and emits `EventInfo` with which component
   changed (`sys`, `tools`, or `sys+tools`). The TUI status line shows
   a `⚠ cache:<which>` badge.

The monitor is lightweight — three SHA-256 hashes per turn.

## Common drift sources

| Source | Component | Example |
|--------|-----------|---------|
| Hook/skill injection at session start | sys | First turn pins after injection; expected |
| MCP server reconnect | tools | Tool set changes mid-session |
| Hot-reload of hooks/skills | sys | System prompt rebuilt with new content |
| `dsc init` rewrites config | tools | New MCP servers added |

## Troubleshooting

1. **Status shows `⚠ cache:sys`**: Check if hooks or skills modified
   the system prompt after the first turn. Usually caused by a
   `SessionStart` hook that runs on every `Run` call rather than once.
2. **Status shows `⚠ cache:tools`**: An MCP server reconnected or a
   tool was added/removed mid-session. This is expected after MCP
   reconnects.
3. **Frequent drift**: If every turn drifts, the "static" prefix isn't
   actually static. Audit `PromptBuilder.Build()` and hook output for
   non-deterministic content (timestamps, random IDs).
