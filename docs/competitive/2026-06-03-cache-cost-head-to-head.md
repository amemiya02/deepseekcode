# Cache-Cost Head-to-Head vs DeepSeek-Reasonix — Honest Findings

**Date:** 2026-06-03
**Status:** Closed. The cache/cost superiority hypothesis is **not supported**; this document records why, rigorously, and where deepseekcode (dsc) genuinely leads instead.

## 1. Objective

Test whether dsc, on the **same DeepSeek V4-flash model**, achieves a **higher cache-hit rate and lower per-task cost** than DeepSeek-Reasonix v0.53.2 on a real code-editing workload — and if not, find the controllable cause.

## 2. Subjects & versions

| | dsc | Reasonix |
|---|---|---|
| version | branch `feat/reasonix-competitive-proof` (post-revert) | v0.53.2 |
| model | `deepseek-v4-flash` | `deepseek-v4-flash` |
| backend | DeepSeek-direct (`api.deepseek.com`, per-token + prompt cache) | same |
| driver | `bin/dsc -yolo -p` (headless), `-trace-jsonl` | `reasonix acp` (headless JSON-RPC) |

## 3. Method

- **Workload:** real public bugs from `grpc/grpc-go` (Multi-SWE-bench-Go, ByteDance-Seed), 3 gold-validated instances: 3476, 2996, 2760. Score = canonical test files restored, then `go test -run <F2P regex> -vet=off`.
- **Metric:** primary = **¥ cost computed identically for both sides** from token counts via the published V4-flash price table (hit 0.02, miss 1.0, output 2.0 ¥/1M) — *not* either tool's self-reported cost. Secondary = billable tokens (miss+output); diagnostic = cache-hit %.
- **Cache isolation:** DeepSeek's prompt cache is per-account. dsc → Account A, Reasonix → Account B, so neither evicts the other's cache. Confirmed key-level isolation within one account is undocumented; two separate accounts used.
- **N:** N=3 per arm for the dsc-side A/Bs; per-turn cache distributions recorded (not just sums).

## 4. Findings — a three-layer cost model

### Layer 1 — self-inflicted prefix mutation via compaction (DOMINANT, **fixed**)
A cost-driven "body compaction" tier (added earlier this effort) rewrote the conversation body every ~3 turns. On a prefix-cached API, **any** rewrite of the cached prefix invalidates the block cache from that point, so the whole body re-sends as an 8–17K-token miss. Every compaction event was immediately followed by a miss spike. Clean A/B (grpc-go-2760, N=3):

| | median billable | median turns | compactions |
|---|---|---|---|
| compaction ON | ~105K | 21 | 1–3 |
| compaction OFF | ~64K | 11 | 0 |

→ **Removing it cut median billable ~40% and halved turns.** Reverted (commits reverting 9d5bd96/80c6b9a/0ce76d6) + a regression guard (`TestNoCompactionBelowOverflow`). **Lesson: compaction is a capacity tool of last resort near the 1M window, never a cost tool on a prefix-cached API.**

### Layer 2 — provider-side block eviction (structural, **unresolved**)
Even with 0 compactions, a byte-stable frozen prefix, and an append-only body, dsc's conversation cache is **evicted intermittently** (hit collapses to the ~7,936-token static-prefix floor; ~21K re-miss), in a roughly **alternating** pattern. Account-isolated diagnostic (3476):

| arm | turns | full evictions | cache% |
|---|---|---|---|
| dsc / Account A | 9 | 3 | 66.5 |
| dsc / Account B | 12 | 2 | 75.2 |
| **Reasonix / Account B** | 10 | **0** | **89.0** |

dsc evicts on **both** accounts; Reasonix on the **same** account B does not. So it is **structural to dsc's request pattern, not account state.** Ruled out as the cause, each with evidence:
- account / cache luck (evicts on A *and* B);
- inter-turn latency (evicted-turn pre-gap 3,340ms vs normal 2,728ms — weak);
- prefix mutation (frozen system + append-only body, stable hashes);
- auxiliary LLM calls (the duet hook fires only on destructive *paths*, not normal in-cwd edits);
- **reasoning_content replay** — implemented a fix that replaces verbose historical `reasoning_content` with the API-required placeholder, tested it, and it **did not reduce eviction** (4 vs 2, cache unchanged); reverted.

The eviction correlates with high-miss "write" turns vs high-hit "read" turns, which points to a **DeepSeek-internal cache TTL/refresh policy** that dsc's request rhythm triggers and Reasonix's does not. No controllable dsc-side fix was found. (A definitive raw-request byte diff against Reasonix remains the open probe.)

### Layer 3 — output / thinking verbosity (**not a lever**)
Hypothesis: thinking emits `reasoning_content` billed at 2× output, so disabling it should save cost. A/B (N=3 × 3 instances) refuted it:

| thinking mode | median ¥ | median billable | median cache | resolve |
|---|---|---|---|---|
| **on** (default) | **0.057** | 48,219 | 84% | 8/9 |
| adaptive | 0.067 | 58,201 | 84% | 9/9 |
| off | 0.071 | 64,851 | 78% | 9/9 |

Thinking **on** is cheapest — reasoning reduces turns/re-exploration more than it costs. Default kept `on`; a `DEEPSEEKCODE_THINKING_MODE` (on/off/adaptive) knob was added for future tuning (default unchanged).

## 5. Verdict on the cache/cost claim

Same model, same price table, same instances:

| instance | dsc-best ¥ | Reasonix ¥ | dsc cache | rx cache |
|---|---|---|---|---|
| 3476 | 0.057 | ~0.025 | 78% | 90–96% |
| 2996 | 0.013 | ~0.007 | 87% | 97% |
| 2760 | 0.107 | ~0.028 | 76% | 97% |

**dsc is ~2–4× more expensive and lower cache-hit than Reasonix on DeepSeek-direct, and the entire residual gap is the Layer-2 provider-side eviction.** We therefore **do not claim** dsc is cheaper or more cache-efficient than Reasonix in a same-backend head-to-head. This is now established at the mechanism level, not merely observed.

## 6. Where dsc genuinely leads (the honest narrative)

- **Prefix-cache A/B:** the frozen Static Prefix design measurably reduces miss on the cacheable prefix (README: 94.7% cache-hit / 4.5× savings in the controlled prefix A/B — a *different* axis from the head-to-head above, and unaffected by these findings).
- **Capability parity:** tau-bench shows parity with the competitor — no capability regression from the cost work.
- **Budget / routing rigor:** per-turn budget projection, auto-routing/escalation, and trace instrumentation (now incl. wall-clock `ts_unix_nano` on usage records).
- **A real engineering insight:** *compaction is a net-negative anti-pattern on a prefix-cached API* — found, quantified, fixed, and guarded here.

## 7. Caveats

- 3 instances, one repo (grpc-go), N=3 — narrow-but-deep; the *mechanism* is the evidence, not instance count.
- Reasonix baselines for 3476 are N=2 (prior transcripts, recomputed via the same price table); the account-B isolated diagnostic corroborates Reasonix's near-zero eviction.
- Layer-2 root cause is narrowed but not byte-pinned; it is consistent with a DeepSeek-internal cache policy outside dsc's control.

## 8. Backends note (the generic OpenAI interface)

dsc speaks OpenAI-compatible Chat Completions with a configurable `base_url`, so it can run on any compatible backend (DeepSeek-direct, Baidu Qianfan, …). Note that **Qianfan's Coding-Plan endpoint (`/v2/coding/chat/completions`) does not return `prompt_cache_hit/miss` tokens** and is flat-rate, so it cannot be used to measure the cache/cost axis above — but it *is* suitable for capability/resolve-rate benchmarking of harness+model (see follow-up). (dsc currently hardcodes the `/v1/chat/completions` path; pointing it at Qianfan requires making that path configurable.)

## 9. Reproduction

Harness in `/tmp/swebench-go/`: `bench_run.py` (atomic dsc run), `bench_run_rx.py` (Reasonix-via-ACP, ¥ via same price table), `diag_acct.py` (account-vs-structural eviction), `accounts.env` + `accounts_lib.py` (two-account isolation). Traces: `run_<iid>_<mode>_r<N>.trace.jsonl`, `results.jsonl`.
