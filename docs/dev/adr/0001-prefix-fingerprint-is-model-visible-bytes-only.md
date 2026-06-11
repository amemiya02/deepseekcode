# Prefix Fingerprint covers model-visible bytes only

**Status:** accepted (2026-05-29)

The **Prefix Fingerprint** (the per-epoch static-prefix hash that gates DeepSeek
prompt-cache reuse) is computed from *only the model-visible bytes* — the system
prompt (which already contains the rendered skill directory), the actually-sent
tool specs, and any leading few-shot turns — using the same canonicalization
`llm.MarshalCacheStable` uses for the wire. Latent capability state (agent
profile, connected-but-inactive MCP servers, the full skill catalog) is **not**
folded into the fingerprint; it is tracked separately as the **Capability Set**
that `EpochManager` watches to decide when to mint a new epoch. See
`/CONTEXT.md` for the vocabulary.

## Considered options

- **6-component combined hash** (the original design): hash
  `system : tools : skill_dir : mcp_schema : agent_profile :
  few_shots`. **Rejected** — it double-counts (`skill_dir` is rendered into
  `system`; MCP tools are inside `tools` when their tier is active), its
  `mcp_schema` component is the only key-order-*sensitive* hash (raw
  `SchemaHash`) and so causes **phantom drift** on a reordered-keys MCP
  reconnect, and it drifts on bytes the model never sees (inactive MCP, profile
  name) — forcing wasteful cache misses.
- **Disjoint-but-canonical components (P2):** keep the components, make them
  non-overlapping and all key-sorted. **Rejected** — more machinery, and the
  combined hash is still stricter than the cache key, so it keeps drifting on
  latent state.
- **Model-visible bytes only (P1, chosen):** the fingerprint equals the cache
  key by construction; latent state becomes the Capability Set.

## Consequences

- The fingerprint provably equals the DeepSeek cache key (same canonical bytes
  as the wire), so the benchmark's "static prefix stable within an epoch" gate
  is now tautologically the cache key.
- Latent capability changes (e.g. a profile rename, an inactive-tier MCP
  reconnect) no longer force a cache miss; they record a **Pending Change** for
  receipts and let policy decide whether to mint a new epoch.
- `mcp.Registry.SchemaHash` and the 6-component `ComputeEpochHash` /
  `EpochComponentHashes` / `computeComponentHashes` are removed.
- Trade-off accepted: the epoch no longer carries a single hash that also
  encodes latent identity, so "did the Capability Set change?" is answered by a
  separate `CapabilityDiff` rather than by comparing one combined hash. That
  separation — cache key vs. policy identity — is the point of this decision.
