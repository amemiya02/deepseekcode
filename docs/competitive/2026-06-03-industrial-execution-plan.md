# DeepSeekCode — Industrial-Grade Execution Plan

*Generated: 2026-06-03 | Source: /grill-me interview (9 decisions locked) + live codebase grounding + competitor source (coding-agent-ref) + GitHub agent-infra research | Supersedes the "gaps" framing in 2026-06-03-coding-agent-ref-gap-analysis.md where they conflict.*

## North Star & the strategic through-line

dsc already **matches or exceeds** industrial agents (Claude Code / opencode / crush / CodeWhale / Reasonix) on the agent loop: plan mode, markdown sub-agents, async fan-out, snapshot-`/undo`, compaction, CC-style hooks, routing+escalation, `todo_write`, `background_bash`, web tools, OS sandbox, mature truecolor TUI. The **one axis we lose** is the DeepSeek cache head-to-head (~2–4× Reasonix's billable cost, entirely from Layer-2 full-body evictions).

**Through-line that orders everything:** fix the cache loss first, productize the *measurement* as a moat no competitor has, then expand distribution — and choose every breadth feature so it **also shrinks the request body** (codegraph + memory feed exact symbols instead of whole files → less cache pressure → fewer evictions). Breadth in service of the moat.

**Hard invariants (never violate):**
- **Frozen DeepSeek wire.** `internal/llm.MarshalCacheStable` byte-output is locked by a golden-bytes CI test; any change that moves DeepSeek bytes fails the build.
- **Cache-safe injection.** Memory/codegraph context is injected as **latent** (tail) content, **never** in the frozen static prefix.
- **No "cheaper than Reasonix" claim** until the Phase-1 A/B proves it on one account.
- **README bilingual** (README.md + README.zh-CN.md together); docs/ exempt.

## Decisions locked (the interview)

| # | Branch | Decision |
|---|---|---|
| 1 | Cache strategy | **Diagnose-first, then port** — body-diff instrument + 3-arm live A/B, then adopt the winner |
| 2 | Cache moat | **Cache-doctor** (live TUI panel + `dsc cache explain` + CI gate) + **adaptive last-N reasoning retention** |
| 3 | Multi-provider | **Frozen DeepSeek path + sidecar marshalers** (golden-bytes lock); native Anthropic `cache_control` + own test |
| 4 | Agent refinements | checkpoint/branch/fork UX + batch parallel fan-out + in-loop verification |
| 5 | Headless protocol | **ACP-first** (`internal/acp`, Zed JSON-RPC/stdio) + thin HTTP/SSE gateway |
| 6 | Desktop | **Web SPA → Wails** (Go-native, no Rust); SPA also hosts cache-doctor dashboard + timeline |
| 7 | TUI | New functional surfaces + targeted aesthetic polish + **first-run onboarding wizard** |
| 8 | Copy targets | in-binary **codegraph**, **doctor + onboarding**, long-term **memory**, **i18n + China networking** |
| 9 | Sequencing | **Cache-first**, then distribution, then breadth |

---

## PHASE 1 — Cache: fix the loss + build the moat  (the crown; ~1–2 weeks)

> Cheapest, highest-credibility work. Restores the one axis we lose and turns our measurement infra into a differentiator. No new product surface — pure depth.

**1.1 Body-diff instrument** — extend `internal/traceinspect` with `dsc trace diff-body <trace> <turnN> <turnN+1>`: reconstruct each turn's wire body via `MarshalCacheStable`, byte-diff the historical-message region, report the **first byte that drifts inside the supposed-stable region**. (Our traces hash the prefix via `static_prefix_hash`; they don't diff each historical *body message* across turns — that's the missing instrument.) *No API spend for the diff itself.*

**1.2 Three-arm live A/B (one account, controlled)** — resolve the dsc↔Reasonix contradiction (we *prepend* a `(reasoning omitted)` placeholder via `SanitizeForDeepSeek`; Reasonix *drops* `reasoning_content` entirely; their source comments make opposite rejection claims). Arms, same account/task/N:
- `arm1` = current (replay reasoning + placeholder)
- `arm2` = drop `reasoning_content` field entirely (match Reasonix; needs `omitempty`→always-`content:""` too)
- `arm3` = keep last-turn reasoning only, drop older
Measure **400-rejection rate, full-body evictions, cache%, turn-count.** (Turn count matters: our data shows turns drive cost more than per-turn size.) Reuse `/tmp/swebench-go` harness shape (`bench_run_reqmode.py` pattern).

**1.3 Adopt the winner + lock it** — change `flattenForWire`/`wireMessage` to the winning serialization; extend `e2e_cache_stable_test.go` + `cache_stable_property_test.go` with the golden-bytes assertion so the wire can never silently drift. This golden lock is also the guardrail for Phase-3 multi-provider.

**1.4 Cache-doctor (the moat)** — productize the diagnostic nobody else has:
- TUI panel: live per-turn `hit / miss / evict` with the drift pointer (`✗ EVICT — body drift @ msg[4]`). New `internal/tui` surface, themed to existing `theme.go`.
- `dsc cache explain <trace>`: per-turn ledger + *why* each eviction happened.
- **CI cache-regression gate:** a recorded-trace test that fails if evictions/cache% regress past threshold.

**1.5 Adaptive reasoning retention** — keep last-N turns' `reasoning_content`, drop older (beats Reasonix's all-or-nothing: better multi-step coherence at the same cache%). N is a tuned knob from 1.2; ship as `DEEPSEEKCODE_REASONING_RETAIN=N`.

**Acceptance:** A/B verdict committed; winning serialization golden-locked; cache-doctor renders live; CI gate green; **only then** may we make any head-to-head cache claim, and only what the data earns.

---

## PHASE 2 — Distribution: headless surface + GUI + onboarding

> Unlocks editors, web, desktop, remote exec, and a far better first impression. All of it sits *on top of* the frozen DeepSeek path — zero cache risk by construction.

**2.1 `internal/acp`** — Agent Client Protocol (Zed standard, JSON-RPC/stdio): `session/new`, `prompt`, `cancel`, tool-call + permission round-trips, streamed fs edits. Wire to the existing `Agent` loop (it already exposes events via the bus + `EventDone`). Free Zed/Neovim integration; neutralizes Reasonix's `acp` edge. `dsc serve --acp`.

**2.2 Thin HTTP/SSE gateway** — wrap the ACP session in HTTP + SSE for browser/remote clients; reuse `internal/llm` event types. `dsc serve --http :PORT`. This is also the remote/cloud-exec surface (defers branch-③'s "remote agent" item here).

**2.3 Web SPA** (React or Svelte) on the gateway — browser app, zero install. Home for: **cache-doctor dashboard** (Phase 1), **session timeline + checkpoint browser** (branch ③), provider/cost HUD. One frontend, reused by desktop.

**2.4 Wails desktop wrapper** — Go backend + native webview (the Go-ecosystem Tauri, **no Rust**) wrapping the same SPA: native window/menus/file-dialogs/tray. Single cohesive Go stack.

**2.5 Onboarding wizard + `dsc doctor`** —
- First-run wizard (Reasonix-style, as requested): welcome → guided DeepSeek API-key entry → model pick → live test call ✓ → write `internal/config` + `secrets.go`. TUI + web variants.
- `dsc doctor`: env/key/network/proxy/cache-fields/sandbox health check (copy Reasonix `internal/doctor`; CodeGraph's `doctor` is the same idea). Low effort, high first-impression payoff; ties into the wizard.

---

## PHASE 3 — Breadth & moat-depth (copy-and-adapt, cache-aware)

**3.1 Multi-provider (frozen-path isolation)** — keep `MarshalCacheStable` exactly as-is (golden-locked). Add per-provider sidecar marshalers behind the existing `Provider` interface (`NewProvider` already dispatches `deepseek`/`openai-compat`): native **Anthropic** (`cache_control` prompt caching + its own cache-stability test), **OpenAI**, broad **openai-compat** (Groq/Cerebras/OpenRouter/vLLM/Ollama). **No shared wire layer** → DeepSeek bytes provably immutable. `Capabilities.PrefixCache` gates cache-specific behavior per provider.

**3.2 Agent refinements** —
- **Checkpoint/branch/fork UX** on existing `snapshots`+`worktree`+`/undo` primitives: `/checkpoint <name>`, `/branch <turn>`, resume `--at`, visual timeline (the one real product gap vs Reasonix/CodeWhale — we have the engine, not the dashboard).
- **Batch parallel fan-out**: `spawn_batch([req,...])` → concurrent children (cap N) + join/aggregate, on top of the existing async `LoopSpawner` (today: one-at-a-time-async + `task_status` poll).
- **In-loop verification**: after mutating steps, auto-run configured build/test/lint; on fail feed back to the model and keep looping; add `StopVerifiedDone`. Makes "done" mean "verified."

**3.3 In-binary codegraph (Go-native, cache-aware)** — adopt the GitCortex model (single static Go binary, embedded graph, git-hook incremental <500ms, branch-namespaced): tree-sitter → nodes (`File/Func/Type/Interface`) + typed edges (`CALLS/IMPORTS/DEFINES/IMPLEMENTS`) → embedded store + optional vectors. Beyond our `struct_search` (AST query): callers/callees/**blast-radius/impact**. **Cache through-line:** lets the agent fetch *exact symbols* instead of whole files → smaller body → fewer evictions. Pre-load a centrality-ranked symbol table into the system context (GitCortex `--claude-md` trick) for zero-tool-call orientation. Syntactic call resolution only (fast, precise; skip full type inference). Mirrors Reasonix's `internal/codegraph`.

**3.4 Long-term memory (privacy-first, cache-safe)** — local SQLite + in-memory vectors (no external DB, lean-binary ethos). API: `remember` / `recall` / `forget`. Hybrid **BM25 + vector** retrieval (RRF). Auto-capture via the lifecycle hooks dsc **already has** (SessionStart/Pre/PostToolUse/PreCompact/Stop). **Privacy filter: strip secrets/API-keys/`<private>` before storage** (aligns with our no-leak rule). **Cache-safe injection: recalled memories go in the latent tail, never the frozen prefix.** **Defer the temporal/knowledge graph** — Mem0 removed theirs (lost on recall, 3× slower, 2× tokens); revisit only if recall quality demands it. Sources: `rohitg00/agentmemory` (4-tier consolidation, hooks, privacy), Mem0 (update-in-place reconciliation), Letta (tiered core/archival) for later.

**3.5 i18n + China networking** — `internal/i18n` zh-CN TUI/message strings (DeepSeek/China-first identity; CodeWhale leans here). `sysproxy`/`netclient`: honor system/corporate proxy, retry, mirror endpoints. Copy Reasonix `internal/{i18n,sysproxy,netclient}`.

**3.6 Backlog (lower tier)** — `outputstyle` (customizable response formatting, Claude Code parity, low effort), native `plugin` system (Reasonix `internal/plugin` + example), first-class `billing` ledger (folds into cache-doctor cost HUD), multimodal/image input (only if DeepSeek V4 vision is available), MCP transport breadth (we have stdio/sse/http; 6 is overkill).

---

## Cross-cutting workstreams & risks

**Parallelization (if fanning out agents):** Phase 1 (cache) must lead; Phases 2 and 3.3–3.5 can run as parallel lanes *once the golden-bytes lock from 1.3 exists* to keep the frozen path safe. 3.1 (multi-provider) depends on 1.3.

**Top risks / open questions:**
1. **A/B may show eviction is provider-internal TTL**, not our wire (memory's standing hypothesis). If 1.1's diff finds *no* in-prefix byte drift, eviction is likely DeepSeek-side → cache-doctor still ships (honest observability), but we stop chasing a wire fix and lean on 3.3/3.4 (body-shrinking) instead.
2. **Dropping `reasoning_content` may 400** on tool-call turns (our own `SanitizeForDeepSeek` comment warns of it) — that's exactly why arm2 is *tested*, not assumed.
3. **ACP spec churn** — pin a version; keep the gateway thin so a spec bump is localized.
4. **Memory recall polluting cache** — enforced by the latent-injection invariant + a test that memory never appears before the prefix boundary.

## Appendix — external references we copy from (with the specific design taken)

- **rohitg00/agentmemory** (Apache-2.0): privacy filter, lifecycle-hook auto-capture, hybrid BM25+vector RRF, local-first SQLite+vectors, 4-tier consolidation → **3.4**.
- **Mem0 / Letta / Zep**: Mem0 update-in-place reconciliation + "graph rarely pays for a coding agent" (defer-graph rationale); Letta tiered core/archival (later) → **3.4**.
- **GitCortex (`gcx`, MIT)**: single static binary, KuzuDB embedded, git-hook incremental, branch-namespaced, `--claude-md` pre-load, blast-radius PR comment → **3.3** (closest model; we're Go too).
- **spy-code / ckg / CodeGraph(Cirilcetra)**: stable node IDs, edge confidence, `nl_query` read-only guard, `doctor` health check, token-efficiency framing → **3.3 / 2.5**.
- **DeepSeek-Reasonix Go (`main-v2`)**: `internal/{acp,codegraph,doctor,memory,i18n,sysproxy,netclient,outputstyle,plugin,billing,checkpoint}` + drop-reasoning_content wire discipline → ② ③ ④ and copy targets.
- **crush**: `proto`/`server` headless shape, Bubble Tea v2 polish → ④ ⑥.

*Methodology: live `internal/` source grounding via codegraph (cache wire, agent loop, tools, TUI) + competitor source reads + 6 GitHub agent-infra projects (memory + code-graph) via web research. Cache claims gated on Phase-1 A/B; no head-to-head cost claim asserted here.*
