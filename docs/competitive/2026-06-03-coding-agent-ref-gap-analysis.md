# Competitive Gap Analysis — coding-agent-ref sweep (breadth + DeepSeek-cache depth)

*Generated: 2026-06-03 | Scope: `/Users/zichuanxu/vscode/coding-agent-ref` (claude-code, opencode, CodeWhale, crush, DeepSeek-Reasonix) vs. dsc | Confidence: mixed (flagged per item)*

## 0. Read this first — provenance & a staleness correction

- The local `*-analysis.md` files in `coding-agent-ref/` are dated **2026-05-25** and were written at a **feature/marketing depth**, not source depth. They are a fine *breadth* index but are **not** authoritative.
- **The Reasonix analysis file is obsolete.** It describes the **TypeScript v0.50.1 / Ink** build. The checked-out `DeepSeek-Reasonix/` is now the **Go 1.0.0 rewrite** (`module reasonix`, `go 1.25.0`, Tauri `desktop/`, `internal/` packages, `REASONIX.md` spec). All Reasonix claims below come from my **direct source reads** of the Go tree (prior session: `internal/provider/openai/{openai.go,think.go}`, `internal/agent/session.go`, `internal/provider/openai/realcache_test.go`) and the live package tree — **not** the stale doc.
- **Hard constraint honored throughout:** our own controlled head-to-head (committed in `docs/competitive/2026-06-03-cache-cost-head-to-head.md`) shows **dsc currently LOSES on billable cost / cache efficiency vs Reasonix on the same price table**. Nothing here claims dsc is cheaper or more cache-efficient. The "99.82% cache" in the stale doc is *their* marketing figure; my measured head-to-head put Reasonix at ~89–97% and dsc materially behind, entirely from Layer-2 evictions.

Confidence legend: **[V]** verified from source/measurement · **[A]** from 2026-05-25 analysis doc (surface, needs verify) · **[I]** inference.

---

## 1. Where dsc actually stands (don't under-credit ourselves)

Live `internal/` package comparison (verified this session):

| Capability | dsc package | crush | Reasonix (Go) | Note |
|---|---|---|---|---|
| Agent loop | `agent` | `agent` | `internal/agent` | parity |
| Sub-agents | `agents` | (coordinator in `agent`) | spawn | parity |
| **Prefix-cache unit infra** | **`cacheunit`** | — | (in provider) | **dsc moat** |
| **Cache trace schema + inspector** | **`traceinspect`,`traceschema`** | — | `cachehit_e2e_test` only | **dsc moat (observability)** |
| Tool-call repair | `repair` | — | `provider.Sanitize*` (4-pass) | parity-ish; theirs is more battle-tested |
| Model routing | `routing` | provider list | "Fin"/flash-first | parity |
| OS sandbox | `sandbox` | — | — | **dsc ahead** (CodeWhale-class) |
| Git worktree isolation | `worktree` | (workspace) | branches/checkpoints | parity |
| Snapshots | `snapshots` | `history` | checkpoints | parity |
| Structural search | `structsearch` | — | semantic_search | different axis (AST vs embedding) |
| V3 tokenizer | `tokenizer` | — | built-in V3 tok | parity |
| LSP | `lsp` | `lsp` | — | dsc+crush ahead |
| MCP | `mcp` | (client) | 3-transport | parity |
| Skills | `skills` | `skills` | Claude-compatible | parity |
| **Headless server / API** | **— (none)** | **`server`** | **`acp` headless** | **GAP** |
| **ACP / editor protocol** | **— (none)** | **`proto`** | **`acp` (works headless+edits)** | **GAP** |
| **OAuth** | **— (none)** | **`oauth`** | ? | gap (enterprise/hosted) |
| **Event bus / pubsub** | **— (none)** | **`pubsub`** | ? | gap (UI streaming, plugins) |
| **SQLite persistence** | session JSONL | **`db`** | JSONL | design choice; see §3 |
| **Desktop / Web UI** | **— (TUI only)** | — | **Tauri `desktop/`** | gap (optional) |

**Takeaway:** dsc is **not** behind on substance. On the *DeepSeek-cache discipline that matters most* we have **more measurement infrastructure than anyone** (`cacheunit` + `traceinspect` + `traceschema` + `bench/cmd/cacheparity`). Our real deficits are (a) **distribution surfaces** (headless server / ACP / desktop) and (b) the **unsolved Layer-2 eviction** that makes our otherwise-rich pipeline cost more per task. Per prior memory, the 2026-05-25 competitive pass *systematically understated our codebase* — **verify "missing" claims against source before building.**

---

## 2. Breadth gaps — what to add, ranked by leverage

### Tier 1 — close the distribution gap (highest strategic value)
1. **Headless server + ACP/editor protocol [V gap].** crush ships `internal/server` + `internal/proto`; Reasonix ships `acp` that *works headless and performs edits* (our memory flags this as a capability dsc's run-mode does **not** dispatch). This is the single biggest breadth gap: it unlocks IDE/VS Code integration, a web dashboard, and CI/automation **without** rebuilding the agent. **Take-and-adapt directly from crush's `proto`/`server` shape**, wired to dsc's existing `agent`.
2. **Cache-forensics as a product surface [I, dsc moat → differentiation].** No competitor exposes live per-turn hit/miss/eviction. We already compute it (`traceinspect`). Surface it in the TUI as a "cache doctor" panel + a `dsc cache explain` command. This is a *uniquely ours* feature — see §5.

### Tier 2 — fill concrete tool/UX holes
3. **Web tools [I gap].** No `web` package in dsc `internal/`. Reasonix has `web_search` + `semantic_search`; claude-code has WebFetch/WebSearch. Add a `web_fetch` + optional `web_search` tool (MCP-backed is fine). *Verify dsc doesn't already expose this via `tools` before building.*
4. **Embedding/semantic search [A gap].** We have `structsearch` (AST/structural) — strong and differentiated — but no embedding index for "find code by meaning." Reasonix + CodeWhale both have it. Lower priority than server/ACP; structsearch already covers many cases.
5. **Checkpoint/branch UX parity [A].** Reasonix: checkpoints + branches; CodeWhale: save/resume/fork. We have `snapshots` + `worktree` primitives — likely need a *user-facing* branch/fork flow on top. Verify current UX before speccing.

### Tier 3 — optional / ecosystem
6. **Desktop or web dashboard** (Reasonix Tauri, opencode Electron, CodeWhale web) — only worth it *after* the headless server exists (it becomes a thin client). Don't build standalone.
7. **OAuth / enterprise auth** (crush `oauth`) — only if we target hosted/team deployments.
8. **China-IM channels** (Reasonix QQ, CodeWhale Feishu) — niche; skip unless a user asks.

### TUI (explicit user ask)
- **Gold standard = crush (Bubble Tea v2).** dsc already has an `internal/tui`; memory ("TUI polish phase") says the plan is *crush-level polish, single-column hybrid/phased*, and that the competitive analysis **understated** our TUI. **Action:** before speccing "missing" TUI work, diff dsc `tui` against crush `internal/ui` for concrete deltas (component set, Lip Gloss styling, viewport/scroll, key handling). Do **not** assume we're behind — verify. This deserves its own focused source pass (the `.pxr/` plan).

---

## 3. DeepSeek-cache depth — the crown axis (where we currently lose)

### 3.1 Honest current state [V]
- dsc-best (thinking `on`, ~¥0.057 / ~48K billable / ~84% cache on the editing bench) is still **~2–4× Reasonix** (¥0.007–0.028 / 90–97% cache) on the **same price table**. The gap is **entirely Layer-2 full-body evictions** (body cache drops to the ~7,936-tok static-prefix floor on roughly alternating turns).
- Established **structural to dsc's request pattern, not account state**: on the same Account B, Reasonix had **0 full evictions / 89% cache** while dsc evicted (A: 3/9, B: 2/12).

### 3.2 How Reasonix avoids it — verified from their Go source [V]
The mechanism is **byte-stable, append-only wire history**. Three concrete moves in `internal/provider/openai/openai.go` `buildRequest()` + `internal/agent/session.go`:
1. **`reasoning_content` is dropped entirely** — their wire `chatMessage` struct *has no reasoning field*. Comment: *"reasoning_content is deliberately NOT sent back… DeepSeek counts re-sent reasoning as billable prompt input (~500 extra tokens/turn)."* Their `realcache_test.go` shows they ran our exact experiment and even hit *"DeepSeek REJECTED a request carrying reasoning_content in history."*
2. **`content` is always serialized** (`json:"content"`, no `omitempty`) — a pure-tool-call assistant turn sends `content:""` every time, byte-identical across turns.
3. **Stable tool shape** — tool_calls in stable index order, synthesized stable IDs, `SanitizeToolPairing` repairs orphans. Every historical message re-serializes to the *same bytes* → DeepSeek's block cache matches the entire prior body → only the new tail misses.
- `Session.Add` is append-only; `Replace` is used **only** by compaction (the single prefix-reset point). Same stream shape as us (`stream:true` + `include_usage`) → **streaming is not their secret** (consistent with our request-shape experiment FAIL).

### 3.3 What dsc does differently — the suspect list [V/I]
Our architecture already matches theirs (frozen prefix, append-only body — after we removed cost-driven compaction, which was the Layer-1 self-inflicted ~40% loss). So the residual eviction must be a **per-turn wire-byte instability** theirs doesn't have. Ranked suspects:
1. **`reasoning_content` field *presence* [V — untested delta].** dsc's `flattenForWire` *always emits* a reasoning_content field (a placeholder). We tested shrinking the placeholder *size* (`DEEPSEEKCODE_MIN_HISTORY_REASONING`) → no change. **We never tested removing the field entirely**, which is exactly what Reasonix does. This is the #1 cheap experiment.
2. **`content` null-vs-`""` drift [I].** If dsc ever serializes `content:null` on one turn and `""` (or omits it) on another for the same historical message, the prefix breaks at that byte.
3. **Tool-result re-serialization on write/edit turns [I].** The eviction *alternates with write turns* — a historical tool result whose bytes change on a later turn (re-read file, timestamp, reordered keys) would invalidate everything after it. Matches the signature.

### 3.4 The decisive next step (cheap, no API spend for the diff) [V]
**Capture dsc's raw HTTP request body on consecutive turns and byte-diff turn N vs N+1**, looking for the first changed byte *inside* the region that should be stable. Our traces hash the *prefix* (`static_prefix_hash`) but don't diff each historical *body message* across turns — that's the missing instrument. One diff pins the cause. Build it as a `traceinspect` subcommand (`dsc trace diff-body <trace>`), then the field-presence experiment in §3.3.1 becomes a one-line confirm.

### 3.5 Take directly from Reasonix [V]
- **Port their cache contract as a test.** Reasonix has `cachehit_e2e_test.go` — an executable invariant that the wire body stays byte-stable across turns. We have the trace infra to do the same. **Adopt "byte-stable history" as a CI gate**, not a hope. This is the single highest-confidence borrow.
- **Drop reasoning_content from the wire** (gated behind the diff finding) — matching their field-omission, not just minimization.

---

## 4. Take-and-adapt table (directly portable, ranked by effort:impact)

| From | What | Effort | Impact | dsc adaptation |
|---|---|---|---|---|
| Reasonix [V] | Byte-stable wire **contract test** (`cachehit_e2e`) | **Low** | **High** | Reuse `traceinspect`; gate in CI |
| Reasonix [V] | **Drop** `reasoning_content` field from wire | Low | High* | Behind §3.4 diff; *if confirmed cause |
| crush [V] | `proto`/`server` **headless+ACP** shape | Med | **High** | Wire to existing `agent`; enables IDE/web/CI |
| crush [A] | Bubble Tea v2 TUI polish patterns | Med | Med | Feed the `.pxr/` TUI plan; diff first |
| CodeWhale [A] | SWE-bench harness wired in-repo | Low | Med | We already have `/tmp/swebench-go`; make it first-class |
| Reasonix/CodeWhale [A] | Embedding semantic search | Med | Med | Complement `structsearch`, don't replace |
| claude-code [A] | Richer slash-command surface / hooks scopes | Low | Low-Med | We have `commands`+`hooks`; audit coverage |

---

## 5. Our differentiation — how to actually surpass them

The winning wedge is **not** "match Reasonix's cache number" (necessary, not sufficient). It's:
1. **Cache forensics nobody else has.** We are the only agent with `cacheunit` + `traceinspect` + `traceschema` + a `cacheparity` bench. Productize it: live per-turn hit/miss/eviction in the TUI ("cache doctor"), `dsc cache explain`, and a **CI cache-regression gate**. Sell *provable* DeepSeek-cache correctness, not a marketing percentage.
2. **OS sandbox + worktree isolation** (CodeWhale-class safety) that Reasonix/crush lack — lean into "safe autonomous edits."
3. **Structural (AST) search** as a distinct, precise complement to everyone's fuzzy embedding search.
4. **The contract-test culture**: ship the byte-stable-history invariant as a public, runnable test — turn our deepest weakness (eviction) into our most credible claim once fixed.

---

## 6. Prioritized roadmap

**Now (cheap, high-confidence):**
- [ ] `traceinspect` per-turn **body byte-diff** subcommand (§3.4) — instruments the root cause.
- [ ] Experiment: **drop reasoning_content field** entirely (not minimize) and re-measure eviction (§3.3.1).
- [ ] Adopt **byte-stable-history contract test** in CI (port Reasonix `cachehit_e2e` idea via our trace infra).

**Next (medium):**
- [ ] **Headless server + ACP** (adapt crush `proto`/`server`) → unlocks IDE/web/CI distribution.
- [ ] Promote `/tmp/swebench-go` harness into the repo as a first-class resolve-rate + cache bench.
- [ ] **TUI source diff** vs crush `internal/ui` → feed the `.pxr/` polish plan (verify, don't assume gaps).

**Later (optional):**
- [ ] Web/desktop thin client on the headless server; embedding semantic search; OAuth.

---

## 7. Confidence & what still needs a deeper (more expensive) pass

- **[V] solid:** Reasonix Go cache mechanism; dsc-vs-competitor `internal/` package map; the head-to-head cost loss and its Layer-2 cause; the cheap experiment plan.
- **[A] needs source verification before building:** Reasonix Go *feature* breadth (tools list, checkpoints, semantic search, acp surface) — the stale doc is the only source and it described the TS build. crush TUI component deltas. dsc's current web-tool / branch-UX coverage (avoid building what we already have).
- **Deeper opt-in pass (costs more):** (1) full read of `DeepSeek-Reasonix/internal/` Go tree + `REASONIX.md` to replace the stale analysis doc; (2) crush `internal/ui` vs dsc `internal/tui` line-level TUI diff; (3) audit dsc `tools`/`commands` to confirm the §2 "gaps" are real. Say the word and I'll run a focused, scoped source pass (I'll keep it tight given session cost).

*Methodology: 6 prior analysis docs (2026-05-25, surface) + direct Go source reads of Reasonix's provider/agent/session/cache-test layer (prior session) + live `internal/` package tree comparison (this session) + committed head-to-head measurements + cache-cost-root-cause memory. No new API spend incurred for this report.*
