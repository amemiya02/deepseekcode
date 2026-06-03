# Controlled Experiments Compendium — dsc Cache/Cost vs DeepSeek-Reasonix

**Date:** 2026-06-03
**Purpose:** The single, detailed, reproducible record of every persuasive controlled experiment run during the cache/cost investigation. Raw per-run data is included so each conclusion can be re-checked. Narrative summary lives in `2026-06-03-cache-cost-head-to-head.md`; this file is the evidence appendix.

## 0. Common setup (applies to all experiments unless noted)

- **Model:** `deepseek-v4-flash` on DeepSeek-direct (`api.deepseek.com`), per-token billing + prompt cache.
- **Workload:** real public bugs in `grpc/grpc-go` (Multi-SWE-bench-Go, ByteDance-Seed). Three gold-validated instances: **3476**, **2996** (small/easy), **2760** (large/high-variance).
- **dsc invocation:** `bin/dsc -yolo -p "<fix prompt>" -trace-jsonl <trace>`, post-revert branch `feat/reasonix-competitive-proof`.
- **Scoring:** after the agent finishes, canonical `*_test.go` files are restored (neutralizing any test tampering), the test_patch re-applied, then `go test <pkg> -run '<F2P regex>' -count=1 -vet=off`. `-vet=off` is required — modern `go vet` flags the 2018–2019 code. resolved = exit 0.
- **¥ metric:** computed identically for every arm from token counts via the published V4-flash price table — **cache-hit 0.02, cache-miss 1.0, output 2.0 ¥ / 1M tokens** — never a tool's self-reported cost. billable = miss + output. "eviction" = a turn whose cache-hit collapses to the ~7,936-token static-prefix floor (full conversation-body re-send).
- **Cache isolation:** DeepSeek's prompt cache is per-account. Account A = dsc, Account B = Reasonix, so the two never evict each other. Key-level isolation within one account is undocumented → two separate accounts.

---

## Experiment 1 — Compaction A/B (Layer 1): the dominant, self-inflicted cost

**Question:** does the cost-driven "body compaction" tier (rewrites history every ~3 turns to shrink the carried body) reduce or increase cost on a prefix-cached API?

**Design:** instance grpc-go-2760, N=3 each. OFF = `DEEPSEEKCODE_BODY_TOKEN_BUDGET=-1` (compaction disabled), ON = `=16000` (the shipped default at the time). Same binary, same account, sequential.

**Raw results:**

| arm | run | resolved | turns | compactions | miss-spikes(>3K) | cache% | MISS | OUT | billable |
|---|---|---|---|---|---|---|---|---|---|
| OFF | 1 | ✓ | 11 | 0 | 3 | 86.8 | 38,602 | 11,962 | 50,564 |
| OFF | 2 | ✓ | 12 | 0 | 4 | 80.7 | 56,601 | 7,830 | 64,431 |
| OFF | 3 | ✓ | 10 | 0 | 4 | 76.9 | 56,908 | 9,751 | 66,659 |
| ON | 1 | ✓ | 21 | 2 | 8 | 79.6 | 87,022 | 18,406 | 105,428 |
| ON | 2 | ✓ | 21 | 1 | 4 | 86.4 | 56,154 | 9,766 | 65,920 |
| ON | 3 | ✓ | 32 | 3 | 8 | 85.5 | 91,246 | 22,088 | 113,334 |

**Result:** median billable OFF **64,431** vs ON **105,428** (**−40%**); median turns OFF **11** vs ON **21** (halved); compactions 0 vs 1–3. resolve 3/3 both.

**Mechanism (per-turn proof, from a 33-turn ON trace):** every compaction event is *immediately* followed by a miss spike — compactions after turns 6/9/21/24/28 → next-turn miss 10,926 / 3,812 / 10,701 / 8,679 / 11,531, with cache-hit collapsing to the ~7,936 static-prefix floor each time. Rewriting the cached prefix invalidates DeepSeek's 128-token block cache from the fold point; the whole body re-sends as a miss.

**Conclusion:** compaction is **net-negative** on a prefix-cached API — it causes the very re-sends it was meant to avoid, and the lost context inflates turn count. **Reverted** (revert commits of 9d5bd96/80c6b9a/0ce76d6) + regression guard `TestNoCompactionBelowOverflow` + `internal/agent/` added to the `cover-cache` lane. Compaction retained only as a capacity tool near the 1M window.

### 1b — supporting reverted-experiments (read_file cap, body-discipline editing)
Earlier mitigations that did **not** close the gap (same 3 instances, N=2, billable):

| instance | read_file-cap recheck | body-discipline editing |
|---|---|---|
| 3476 | 68,879 / 69,775 | 49,931 / 55,154 |
| 2996 | 13,116 / 10,384 | 18,609 / 8,702 |
| 2760 | 75,086 / 92,224 | 45,464 / **131,438** |

read_file 500-line cap ≈ 9% effect; body-discipline (the ON arm above) produced the 131,438 spike. Both superseded by the revert.

---

## Experiment 2 — Thinking-mode A/B (Layer 3): is reasoning a cost sink?

**Question:** `reasoning_content` is billed at 2× output; does disabling thinking save cost?

**Design:** `DEEPSEEKCODE_THINKING_MODE` ∈ {on, off, adaptive}, N=3 × 3 instances = 27 runs, account A, sequential. on = current default; off = never think; adaptive = think turn-0 (plan) + repair turns only.

**Raw results (all 27 runs):**

| mode | inst | run | resolved | turns | cache% | billable | ¥ | evict | maxmiss |
|---|---|---|---|---|---|---|---|---|---|
| on | 3476 | 1 | ✓ | 9 | 66.5 | 63,668 | 0.0695 | 3 | 17,707 |
| on | 3476 | 2 | ✓ | 10 | 77.9 | 48,219 | 0.0551 | 2 | 18,677 |
| on | 3476 | 3 | ✓ | 13 | 84.0 | 48,047 | 0.0574 | 1 | 16,610 |
| on | 2996 | 1 | ✓ | 12 | 89.0 | 21,979 | 0.0278 | 2 | 7,078 |
| on | 2996 | 2 | ✓ | 5 | 85.5 | 10,618 | 0.0127 | 1 | 6,167 |
| on | 2996 | 3 | ✓ | 6 | 86.8 | 10,737 | 0.0130 | 1 | 6,221 |
| on | 2760 | 1 | ✓ | 13 | 75.8 | 90,279 | 0.1071 | 3 | 25,033 |
| on | 2760 | 2 | ✗ | 20 | 87.8 | 115,146 | 0.1555 | 2 | 38,331 |
| on | 2760 | 3 | ✓ | 9 | 72.1 | 75,658 | 0.0898 | 2 | 28,834 |
| off | 3476 | 1 | ✓ | 10 | 69.6 | 64,851 | 0.0713 | 2 | 20,230 |
| off | 3476 | 2 | ✓ | 12 | 77.7 | 62,769 | 0.0704 | 3 | 15,786 |
| off | 3476 | 3 | ✓ | 14 | 69.8 | 94,332 | 0.1030 | 4 | 18,547 |
| off | 2996 | 1 | ✓ | 5 | 81.8 | 13,162 | 0.0153 | 1 | 6,300 |
| off | 2996 | 2 | ✓ | 8 | 83.3 | 20,679 | 0.0244 | 2 | 7,631 |
| off | 2996 | 3 | ✓ | 4 | 81.3 | 10,510 | 0.0122 | 1 | 6,198 |
| off | 2760 | 1 | ✓ | 15 | 78.3 | 74,662 | 0.0875 | 3 | 17,388 |
| off | 2760 | 2 | ✓ | 11 | 78.9 | 67,194 | 0.0801 | 2 | 21,579 |
| off | 2760 | 3 | ✓ | 10 | 78.0 | 80,691 | 0.1039 | 2 | 27,501 |
| adaptive | 3476 | 1 | ✓ | 10 | 73.0 | 55,176 | 0.0606 | 2 | 16,058 |
| adaptive | 3476 | 2 | ✓ | 13 | 80.2 | 58,201 | 0.0667 | 2 | 17,165 |
| adaptive | 3476 | 3 | ✓ | 13 | 78.7 | 61,331 | 0.0689 | 2 | 17,693 |
| adaptive | 2996 | 1 | ✓ | 5 | 83.9 | 11,123 | 0.0132 | 1 | 6,427 |
| adaptive | 2996 | 2 | ✓ | 7 | 90.8 | 9,268 | 0.0120 | 0 | 4,256 |
| adaptive | 2996 | 3 | ✓ | 6 | 86.9 | 11,055 | 0.0134 | 1 | 6,286 |
| adaptive | 2760 | 1 | ✓ | 15 | 85.3 | 62,191 | 0.0765 | 2 | 20,749 |
| adaptive | 2760 | 2 | ✓ | 13 | 76.0 | 82,599 | 0.0945 | 2 | 21,267 |
| adaptive | 2760 | 3 | ✓ | 21 | 84.1 | 102,323 | 0.1268 | 3 | 24,129 |

**Medians by mode (across 9 runs):**

| mode | median ¥ | median billable | median cache | resolve |
|---|---|---|---|---|
| **on** | **0.0574** | 48,219 | 84% | 8/9 |
| adaptive | 0.0667 | 58,201 | 84% | 9/9 |
| off | 0.0713 | 64,851 | 78% | 9/9 |

**Result:** thinking **on is the cheapest**; off is +24%. Reasoning reduces turns/re-exploration more than its 2×-priced output costs. The single on-failure was `2760 r2` (a 20-turn spiral on the highest-variance instance; off/adaptive happened not to spiral in their 3 runs — within N=3 variance).

**Conclusion:** Layer 3 is **not a cost lever**. Default kept `on`. `DEEPSEEKCODE_THINKING_MODE` knob shipped (commit 0cafabf) for future tuning; default unchanged.

---

## Experiment 3 — Account-vs-structural eviction diagnostic (Layer 2): the key finding

**Question:** dsc's conversation cache evicts repeatedly even with 0 compactions + stable prefix; Reasonix's doesn't. Is it dsc's account state, or dsc's request structure?

**Design:** instance 3476. Run dsc on **Account B** and Reasonix on **Account B** (same account), capture per-turn cache, compare to the existing dsc/Account A trace.

**Raw results:**

| arm | turns | full evictions | maxmiss | cache% |
|---|---|---|---|---|
| dsc / Account A | 9 | 3 | 17,707 | 66.5 |
| dsc / Account B | 12 | 2 | 20,152 | 75.2 |
| **Reasonix / Account B** | 10 | **0** | 15,521 | **89.0** |

**Result:** dsc evicts on **both** accounts; Reasonix on the **same** Account B has **zero** full-body evictions. → eviction is **structural to dsc's request pattern, not account state.**

---

## Experiment 4 — reasoning_content-replay fix test (rules out the leading structural hypothesis)

**Hypothesis:** dsc replays full historical `reasoning_content` in every request (`flattenForWire` always emits it; `SanitizeForDeepSeek` even adds placeholders) — DeepSeek's contract treats reasoning as output-only, so this may break caching.

**Design:** implemented `DEEPSEEKCODE_MIN_HISTORY_REASONING` (replace verbose historical reasoning with the API-required tiny placeholder), ran 3476 on Account B, compared to the no-flag Account-B baseline.

**Raw results:**

| arm | turns | evictions | maxmiss | cache% | billable |
|---|---|---|---|---|---|
| baseline (no flag) | 12 | 2 | 20,152 | 75.2 | — |
| + MIN_HISTORY_REASONING | 15 | 4 | 17,791 | 76.4 | 79,320 |

**Result:** minimizing replayed reasoning did **not** reduce eviction (cache 76.4% ≈ 75.2%). **Hypothesis refuted; change reverted.** reasoning_content replay bloats the body but is not the eviction cause.

---

## Experiment 5 — dsc vs Reasonix per-turn decomposition (where the gap actually is)

**Design:** turn-by-turn token decomposition on the same instances, dsc (Account A) vs Reasonix (prior transcripts).

**grpc-go-2760:**

| arm | turns | body/turn (median prompt) | MISS/turn med (max) | OUT/turn med (max) | cache% | billable |
|---|---|---|---|---|---|---|
| dsc r1 | 18 | 19,260 | 258 (9,498) | 144 (4,213) | 88.3 | 45,464 |
| dsc r2 | 33 | 19,873 | 568 (17,271) | 126 (9,878) | 85.2 | 131,438 |
| RX r1 | 10 | 26,796 | 262 (3,675) | 216 (4,099) | 96.8 | 14,250 |
| RX r2 | 17 | 27,096 | 268 (3,847) | 169 (1,606) | 97.3 | 16,830 |

**grpc-go-2996:** dsc 8 turns, miss med 87 (max 8,577), out med 148, cache 87.7%, bill 18,609; RX 6 turns, miss med 232 (max 1,168), out med 220, cache 97.2%, bill 4,491.

**Result (counter-intuitive):**
- dsc's **body is *smaller*** than Reasonix's (~19K vs ~27K) — read_file cap already fixed bloat.
- On a **median** turn the two are nearly identical (dsc miss 258/out 144 vs RX miss 262/out 216) — dsc's steady state is healthy.
- The entire gap is in the **spikes**: dsc max miss 9.5K–17K vs RX never above ~3.7K; dsc max output 4K–10K vs RX ~150 median. dsc suffers full-body evictions; Reasonix only ever re-sends a small delta.

---

## Experiment 6 — dsc vs Reasonix ¥ head-to-head (same price table)

**Reasonix recomputed via the same flash price table (prior transcripts):**

| transcript | turns | cache% | billable | ¥ |
|---|---|---|---|---|
| 2760_rx_1 | 10 | 96.8 | 14,250 | 0.0254 |
| 2760_rx_2 | 17 | 97.3 | 16,830 | 0.0304 |
| 2996_rx_1 | 6 | 97.2 | 4,491 | 0.0081 |
| 2996_rx_2 | 6 | 97.6 | 3,612 | 0.0067 |
| 3476_rx_1 | 10 | 95.5 | 17,100 | 0.0273 |
| 3476_rx_2 | 6 | 90.7 | 16,874 | 0.0232 |

**Head-to-head (dsc-best mode = on):**

| instance | dsc-best ¥ | Reasonix ¥ | dsc cache | rx cache | cost gap |
|---|---|---|---|---|---|
| 3476 | 0.057 | ~0.025 | 78% | 90–96% | ~2.3× |
| 2996 | 0.013 | ~0.007 | 87% | 97% | ~1.8× |
| 2760 | 0.107 | ~0.028 | 76% | 97% | ~3.8× |

**Conclusion:** dsc is **~2–4× more expensive and lower cache-hit** on DeepSeek-direct; the residual gap is 100% the Layer-2 eviction. **We do not claim dsc is cheaper or more cache-efficient head-to-head.**

---

## Experiment 7 — eviction-vs-latency correlation (rules out a TTL/latency fix)

Aggregated over all 27 thinking-A/B runs (timestamped via the new `ts_unix_nano`):

| turn class | n | median inter-turn gap |
|---|---|---|
| evicted | 52 | 3,340 ms |
| normal | 210 | 2,728 ms |

**Result:** only a weak (+22%) latency association — not the strong correlation a "keep turns fast" fix would need. Eviction is not primarily inter-turn-latency-driven.

---

## Experiment 8 — the per-turn eviction *pattern* (the distinctive signature)

dsc 3476, thinking on, run 1 (gaps are inter-turn ms):

| turn | hit | miss | out | gap | |
|---|---|---|---|---|---|
| 1 | 7,808 | 2,371 | 191 | — | |
| 2 | 10,368 | 165 | 214 | 2,227 | |
| 3 | 10,624 | 8,754 | 412 | 3,684 | |
| 4 | 19,712 | 496 | 1,140 | 8,511 | |
| 5 | 7,808 | 13,613 | 487 | 4,345 | **EVICT** |
| 6 | 21,888 | 66 | 212 | 3,768 | |
| 7 | 7,936 | 16,908 | 429 | 3,865 | **EVICT** |
| 8 | 25,216 | 103 | 280 | 3,517 | |
| 9 | 7,936 | 17,707 | 120 | 2,403 | **EVICT** |

**Result:** eviction **alternates** (cached / evicted / cached / evicted), with hit collapsing to the static-prefix floor on evict turns and the body re-caching on the next. It tracks high-miss "write" turns vs high-hit "read" turns, not the inter-turn gap — the signature of a **DeepSeek-internal cache TTL/refresh policy** that dsc's request rhythm triggers and Reasonix's does not. No controllable dsc-side fix found; a raw-request byte-diff vs Reasonix is the open probe.

---

## Cross-referenced prior experiments (different axes, still valid)

- **Read-only & code-mode head-to-head** (`2026-06-02-readonly-headtohead-experiment.md`): `reasonix run` (headless) has a tool-dispatch bug (emits valid markup, records it as final) → run-mode inconclusive. Interactive `reasonix code` (pty-driven) does dispatch; clean run: dsc **93.77%** cache / ¥0.063 vs `reasonix code` 90.02% / ¥0.131 (N=1, mode-crossed — directional only).
- **tau-bench:** capability **parity** with the competitor — no regression from the cost work.
- **Prefix-cache A/B** (README): the frozen Static Prefix vs a cache-naive agent → **94.7% cache-hit / 4.5× cheaper** (self-comparison; a *different* axis from the same-model head-to-head, and unaffected by these findings).

---

## Synthesis — what each experiment proves

| # | Experiment | Proves | Status |
|---|---|---|---|
| 1 | Compaction A/B | Cost-driven compaction is net-negative on a prefix-cached API (−40% to remove) | ✅ fixed |
| 2 | Thinking A/B | Thinking is not a cost lever (on is cheapest) | ✅ default kept |
| 3 | Account diagnostic | dsc's eviction is structural, not account state | ✅ established |
| 4 | reasoning-replay fix | reasoning_content replay is not the eviction cause | ✅ refuted |
| 5 | Per-turn decomposition | Gap is spikes/evictions, not body size or steady state | ✅ established |
| 6 | ¥ head-to-head | dsc ~2–4× Reasonix on DeepSeek-direct | ✅ established |
| 7 | Eviction vs latency | Not a latency/TTL-fixable problem | ✅ established |
| 8 | Eviction pattern | Likely a DeepSeek-internal cache policy (alternating write/read) | ⚠️ narrowed, not byte-pinned |

**Honest bottom line:** on DeepSeek-direct (per-token + cache, Reasonix's basis), dsc **cannot** be shown cheaper or higher-cache than Reasonix — now established at the mechanism level. The one real, controllable win was Layer 1 (compaction removal, −40%). Genuine dsc leads are on other axes: prefix-cache A/B, tau-bench parity, budget/routing rigor, and the compaction-anti-pattern insight itself.

## Reproduction

`/tmp/swebench-go/`: `bench_run.py` (atomic dsc run, thinking mode arg), `bench_run_rx.py` (Reasonix-via-ACP, ¥ via same price table), `diag_acct.py` (Exp 3/4), `accounts.env` + `accounts_lib.py` (two-account isolation), `instances.json` + `gold_<iid>/` + `<iid>.test.patch`. Per-run records in `results.jsonl`; per-turn traces in `run_<iid>_<mode>_r<N>.trace.jsonl` (now carrying `ts_unix_nano`).
