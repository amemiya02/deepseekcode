# Model Compatibility

`deepseekcode` is built **specifically for DeepSeek V4 models**. The agent
loop, wire format, and caching strategy encode several DeepSeek-specific
facts that a generic OpenAI-compatible client gets wrong. This document is
the single reference for those facts; the authoritative behavior lives in
`internal/llm` and is pinned by the tests cited below.

## Supported models

| Model ID | Role | Notes |
|---|---|---|
| `deepseek-v4-flash` | default working model | 1M context, 384K max output |
| `deepseek-v4-pro` | escalation / Duet validator | invoked selectively, not every turn |
| `deepseek-chat` | alias → flash semantics | **legacy until 2026-07-24** |
| `deepseek-reasoner` | alias → flash with thinking | **legacy until 2026-07-24** |

Pricing (¥ per 1M tokens, locked at design time — see `docs/pricing.md` and
`internal/llm/cache_metrics.go`):

| Model | Cache hit (input) | Cache miss (input) | Output |
|---|---|---|---|
| `deepseek-v4-flash` | 0.02 | 1.0 | 2.0 |
| `deepseek-v4-pro` | 0.025 | 3.0 | 6.0 |

The **50× gap** between cache-hit and cache-miss input pricing is the entire
reason the cache-stability machinery below exists.

## Wire-format facts

### `thinking` is a struct, not a bool

DeepSeek V4 rejects `"thinking": true` with `expected struct ThinkingOptions`.
Always construct it via `llm.ThinkingEnabled(bool)`, which returns
`*ThinkingOptions{Type: "enabled"}` or `nil`. Pinned by
`internal/llm/thinking_shape_test.go`.

### `reasoning_effort` controls reasoning depth

When thinking is enabled, `reasoning_effort` caps the model's reasoning
depth. Allowed values: `low`, `medium`, `high`, `max`. The default is
`max` (complex coding-agent tasks benefit from full reasoning). Set via
config (`defaults.reasoning_effort`), CLI (`-effort`), or TUI (`/effort`).

When thinking is disabled, `reasoning_effort` is omitted from the wire
request. Pinned by `TestReasoningEffortMaxSerializes` and
`TestLoopReasoningEffortAppearsWhenThinkingEnabled`.

### Cache-stable request serialization

`Request.MarshalCacheStable()` produces the exact bytes DeepSeek keys its
prompt cache on. It **sorts tools by function name** and **recursively
key-sorts every JSON-Schema** (via the shared `canonicalizeTools` helper in
`static_prefix.go`). Any non-deterministic field ordering anywhere upstream
of this silently invalidates the cache and forfeits the 50× discount.
Implementation narrative, package map, and contributor playbooks:
[prefix-cache.md](prefix-cache.md).

Guardrails that must never move:

- `TestCacheStableGolden` pins the wire bytes.
- `TestFingerprintTracksWireStaticHead` pins that the **Prefix Fingerprint**
  (`StaticPrefix.Fingerprint()`, a hash of the model-visible System + Tools
  bytes only) equals the cache key by construction.

Consequences for contributors: never change a tool's `Description()` or
`Parameters()` bytes, or the `DefaultSystemPrompt`, to alter runtime behavior
— enforce new behavior via runtime errors instead. The system prompt must
stay byte-stable across turns within an epoch.

### Finish-reason override

Even when DeepSeek returns `finish_reason=stop`, the loop continues if
`tool_calls` is non-empty (`hasTools := len(step.ToolCalls) > 0` is
authoritative). A turn that emits only tool calls therefore produces no
assistant text and no reasoning deltas.

### Message-shape requirements (`SanitizeForDeepSeek`)

A bare assistant turn or an unpaired `tool_call` 400s on V4. Every
persisted, synthesized, or replayed assistant message passes
`SanitizeForDeepSeek`, and `Replay` repairs dangling `tool_calls` on load.

## Streaming and timeouts

The client (`internal/llm/client.go`) enforces a **two-tier** stream timeout
(defaults; overridable via provider config `FirstTokenTimeoutMs` /
`ChunkStallTimeoutMs`):

- `FirstTokenTimeout` — **45s** — caps the wait from request send to the
  first token. Sized for **reasoner cold start**, which can legitimately take
  30–45s before the first reasoning token.
- `ChunkStallTimeout` — **20s** — caps the gap *between* SSE events once
  streaming has started.

These surface as the typed sentinels `llm.ErrFirstTokenTimeout` and
`llm.ErrChunkStall`; a mid-stream stall re-issues the identical request once
before salvaging the partial turn.

## Context window and compaction

DeepSeek V4 models have a **1M-token context window**. The defaults reflect
this (`internal/agent/compact.go`):

- `MaxContextTokens = 1_000_000`
- `AutoCompactInputTokens = 800_000` (override via
  `DEEPSEEKCODE_AUTO_COMPACT_INPUT_TOKENS`)

Token estimation is intentionally tokenizer-free (a calibrated chars/token
ratio, char/4 cold start) to preserve the single-binary, no-CGO build.

## Build constraints (why these facts are hand-maintained)

- **No external LLM SDK** — `internal/llm` is ~400 LOC of hand-rolled
  HTTP + SSE + typed events. The wire facts above are encoded directly, not
  delegated to a vendor SDK.
- **No CGO** — SQLite is `modernc.org/sqlite` (pure Go), so the binary
  cross-compiles as a single static artifact.
- Pricing and model metadata are hardcoded so the cost HUD is correct without
  an extra round trip; they update at our cadence, not silently from the API.
