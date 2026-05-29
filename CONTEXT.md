# deepseekcode — Cache & Prefix Context

The domain language for how `deepseekcode` keeps DeepSeek's prompt cache hot.
The 50× cache-hit discount is the product's cost story, so the terms below —
what the model sees, how we identify it, and what is allowed to change it —
are load-bearing.

## Language

**Static Prefix**:
The model-visible, cache-stable head of a request — the system prompt (which
already contains the rendered skill directory), the tool specs, and any leading
few-shot messages — that must stay byte-identical across turns to hit DeepSeek's
prompt cache.
_Avoid_: "system prompt" (too narrow), "context".

**Prefix Fingerprint**:
The canonical hash of a **Static Prefix**, computed from the same canonical
bytes the wire serializer emits, so that cache-irrelevant reordering
(JSON-Schema key order, tool order) never changes it. It is the DeepSeek cache
key.
_Avoid_: "static prefix hash" as a separate artifact (it *is* the fingerprint),
"checksum".

**Capability Set**:
The latent inputs that determine *which* **Static Prefix** is built but are not
themselves sent to the model — the active agent profile, the connected MCP
servers and their schemas, and the full skill catalog version. The
**EpochManager** watches the Capability Set to decide when to mint a new
**Prefix Epoch**.
_Avoid_: folding any of these into the **Prefix Fingerprint**.

**Prefix Epoch**:
A frozen **Static Prefix** plus its **Prefix Fingerprint**, immutable from the
first model request until policy explicitly mints a successor; the first turn of
a new epoch is the one expected cache miss.
_Avoid_: "session" (an epoch is finer-grained — a session spans one or more
epochs).

**Drift**:
A change in the **Prefix Fingerprint** *within* a single **Prefix Epoch** — a
cache-correctness violation, never expected mid-epoch. Distinct from a
**Capability Set** change, which is an expected policy trigger.
_Avoid_: calling a profile/MCP/skill change "drift".

**Pending Change**:
A **Capability Set** mutation detected after an epoch was frozen — recorded and
shown in receipts, but not made model-visible until an explicit epoch switch.

## Relationships

- A **Prefix Epoch** freezes exactly one **Static Prefix** and its **Prefix Fingerprint**.
- A **Static Prefix** is *built from* the live **Capability Set** but does not *contain* the Capability Set's latent identity.
- **Drift** is an *unexpected* change of the **Prefix Fingerprint** within an epoch; a **Capability Set** change is an *expected* trigger that produces a **Pending Change** until a new epoch is minted.
- The wire serializer and the **Prefix Fingerprint** share one canonicalization, so the fingerprint equals the DeepSeek cache key by construction.

## Example dialogue

> **Dev:** "The user switched from `coding-default` to `explore`. Is that Drift?"
> **Maintainer:** "No. The profile is part of the **Capability Set**, not the **Static Prefix**. The switch changes which tools get sent, so a new **Prefix Epoch** is minted and its first turn is the expected cache miss. **Drift** is only when the **Prefix Fingerprint** moves *within* an epoch — that's a bug."

## Flagged ambiguities

- "static prefix hash" was used for two different things: the cache key *and* a
  6-component epoch identity that also hashed latent state (profile name,
  connected-but-inactive MCP schema) and double-hashed the skill directory
  (already inside the system prompt) and MCP tools (already inside the tool
  specs). Resolved: the **Prefix Fingerprint** is model-visible bytes only;
  latent identity is the **Capability Set**, watched by policy, never folded
  into the fingerprint.
