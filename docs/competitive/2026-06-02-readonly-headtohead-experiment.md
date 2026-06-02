# Experiment Record — dsc vs Reasonix, Read-Only Multi-Turn Analysis (2026-06-02)

> **摘要（中文）：** 在同一个仓库、同一段只读分析任务、同一个模型（`deepseek-v4-flash`，也是 Reasonix 官方宣传的默认模型）下做 dsc 与 Reasonix 的头对头对比。dsc 干净完成 25 轮真实读取、产出完整交付物，缓存命中 93.77%、花费 ¥0.0634。Reasonix 在 headless `run` 模式下**没有执行任何工具**——它把"Read file: …"当作普通文本叙述输出，1 轮即停，未产出任何分析。因此 **headless `run` 这条线判定为 INCONCLUSIVE**：不能拿"完成的运行"和"未执行的运行"比数字。**但通过 Lever D（用 pty 驱动 Reasonix 的交互式 `code` 模式——它确实会派发工具，§6），我们拿到了一次两边都完成的同模型对比**：dsc（headless）93.77% 缓存 / ¥0.063，Reasonix（`code`，干净跑）90.02% 缓存 / ¥0.131 —— **dsc 缓存更高且成本约便宜 2×，方向性支持本命题**。注意这是**被迫的跨模式**对比（Reasonix headless `run` 的派发坏了，`code` 是它唯一能跑的模式），且 ¥ 口径不同、N 很小——见 §8 警告。全文不对 Reasonix 数字做"造假"指控。

This is a factual record. The headless-`run` head-to-head (§4) is **inconclusive**
because one arm never dispatched a tool. A follow-up via Reasonix's interactive `code`
mode (§6) did complete and yields a directional, heavily-caveated result. The record
makes **no statistically-grounded capability claim** (N is tiny, modes differ); it
exists so the result is reproducible and so future runs do not silently repeat the
same setup.

---

## 1. Objective & hypothesis

**Win condition under test (user-defined):** "如果能证明我们的缓存命中率比 Reasonix
高、开销更少不也可以吗" — i.e. on the *same* DeepSeek V4 model, demonstrate that the
dsc **harness** achieves a higher cache-hit rate and/or lower token cost than the
Reasonix harness on an identical task. A same-model head-to-head is the right design
for a *harness* claim, because the model is held constant and any delta is
attributable to harness behavior (prompt prefix stability, loop structure, tool
plumbing), not to model capability.

**Why read-only.** A prior code-edit head-to-head was blocked on the Reasonix side
by its macOS Seatbelt sandbox (shell-heredoc writes did not persist to the working
tree). To get *some* clean comparison we switched to a strictly read-only static
analysis task, which neither sandbox nor write-permissions can block.

---

## 2. Subjects under test (with versions)

| Subject | Identity | Version / build | Invocation |
|---|---|---|---|
| **dsc** (deepseekcode) | `github.com/amemiya02/deepseekcode` | `dsc dev`, commit `2f7cf78` on branch `feat/reasonix-competitive-proof` | `dsc -yolo -read-only -model deepseek-v4-flash -p "<prompt>"` |
| **Reasonix** | `reasonix` (npm, global) | **v0.53.2** | `reasonix run -m deepseek-v4-flash --budget 0.30 --transcript <path> "<prompt>"` |

**Model (both arms):** `deepseek-v4-flash`.
This is **Reasonix's own advertised baseline**, not a handicap: `reasonix code --help`
describes itself as "coding system prompt, **v4-flash baseline**", and
`reasonix run --help` gives `deepseek-v4-flash` as the `-m` example. So both harnesses
were run on the model Reasonix itself defaults to.

**Environment:** macOS (Darwin 25.5.0); DeepSeek cache is account-level and keyed on
the exact token prefix. The two harnesses emit *different* prompt prefixes, so neither
warms the other's cache — the arms are independent on the cache dimension (no
cross-contamination).

---

## 3. Method

1. **Shared base checkout** — `grpc/grpc-go` shallow-fetched at pinned commit
   `192c8a2a3506bb69336f5c135733a3435a04fb30` into `/tmp/swebench-go/analysis_base`;
   `go mod download` (exit 0); `go build ./balancer/...` (exit 0, clean).
2. **Per-arm isolation** — each arm got a fresh `cp -a` of the base
   (`an_dsc`, `an_rx`) so neither could see the other's tree or state.
3. **Identical prompt** — a single read-only prompt written to
   `/tmp/swebench-go/analysis_prompt.txt` (full text in §8). It forces *iterative,
   dependent* reads: trace the grpc-go RLS key-building call path end-to-end across
   ≥5 interlinked `.go` files (GOAL 1) and explain `BuilderMapEqual` /
   `cmp.AllowUnexported` semantics (GOAL 2), every claim cited `file:line`. The
   prompt explicitly forbids any file modification.
4. **Measurement** — for each arm: did it complete the deliverable? number of turns;
   aggregate cache-hit %; total cost in CNY (Reasonix reports USD → ×7.2). Read-only
   was verified with `git status --short` before/after (empty = no mutation).

---

## 4. Results

| arm | completed | turns | cache % | cost (¥) |
|---|---|---|---|---|
| **dsc** | ✅ yes | 25 | **93.77** | 0.0634 |
| **reasonix** | ❌ **no** | 1 | 81.5 | 0.0012 |

> The two numeric columns are **not comparable** (see §7). They are recorded for
> completeness, not as a score. The *completed* head-to-head is in §6.

### 4.1 dsc arm — completed

- Exit 0; final line `[stop: model_done]`; full deliverable for GOAL 1 + GOAL 2.
- **Read-only verified:** `git status --short` in `an_dsc` empty before and after.
- Cache trajectory: cold first turn **73%**, then steady-state **90–100%** across 25
  turns; aggregate **93.77%** (Σ hit / Σ in = 595,889 / 635,494).
- Total cost **¥0.0634** over 25 real turns.
- Note: dsc's `read_file` tool was blocked by its own symlink guard (macOS
  `/tmp → /private/tmp`, cwd was `an_dsc` while the prompt pointed at the sibling
  `analysis_base`); the agent fell back to non-mutating `cat -n` / `ls` / `grep` via
  bash, so the reads still happened and the tree stayed clean. This is a dsc-side
  quirk worth filing, but it did **not** prevent completion.

### 4.2 Reasonix arm — did NOT complete

- Process exited 0 and printed a summary line
  (`— turns:1 cache:81.5% cost:$0.000163 save-vs-claude:98.9%`), but **performed no
  analysis.**
- Transcript (`an_rx.transcript.jsonl`, 4 lines): a single `assistant_final` turn
  immediately followed by `done`, whose content is the **literal prose string**
  `Read file: /tmp/swebench-go/analysis_base/balancer/rls/internal/keys/builder.go`.
  That is markdown narration, **not an executed tool call**. There is no
  tool-result / tool-role entry; **no file content ever entered context**;
  `completion_tokens = 156`.
- The prompt required multiple dependent reads across ≥5 files; Reasonix produced
  **zero reads and zero analysis**. The deliverable was never generated.
- Its `cost = $0.000163 (¥0.0012)` and `cache = 81.5%` (3,456/4,241 prompt-token
  hit ratio) are **real per-run usage for the single aborted turn** — genuine
  numbers, but they describe a run that did no work, so they are not representative
  of a completed analysis.

---

## 5. Failure analysis — four Reasonix headless attempts, one confirmed root cause

Across this session, four distinct Reasonix headless `run` invocations all failed,
and they triangulate to a single mechanism:

| # | Task | Config | Symptom | Tool dispatched? |
|---|---|---|---|---|
| 1 | code-edit (RLS cmp fix) | default | emitted a full markdown `<details>` narration of the bash commands + the fix | **no** → `git diff` empty, F2P failed |
| 2 | code-edit (retry) | `--no-config` | emitted **raw** `<｜｜DSML｜｜tool_calls> … </｜｜DSML｜｜…>` markup as text | **no** → `NO_EDIT` |
| 3 | read-only analysis | default | emitted plain prose `Read file: …` | **no** → 1 turn, no reads |
| 4 | read-only analysis | **`--effort max`** | emitted a **well-formed** `<｜｜DSML｜｜tool_calls><｜｜DSML｜｜invoke name="Read">…` block | **no** → `assistant_final` → `done`, turns:1, cache 99.6%, cost $0.000063 |

**Root cause (CONFIRMED by attempt #4, with transcript evidence):** the bottleneck is
**Reasonix's headless `run` tool-call dispatch, not the model and not reasoning effort.**
Attempt #4 is the smoking gun: with `--effort max`, `deepseek-v4-flash` emitted a
**syntactically valid** DSML tool call —

```
<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="Read">
<｜｜DSML｜｜parameter name="filePath" string="true">/tmp/swebench-go/analysis_base/…
```

— yet `reasonix run` recorded the message as `role:assistant_final` and went straight
to `role:done` (`an_rx_eff1.transcript.jsonl`, 4 lines, no tool-role entry). The model
produced the correct tool-call channel; the headless loop **did not parse/execute it**
and treated it as final assistant text. `--effort max` only changed *what the model
emitted* (raw DSML instead of the prose of attempts 1/3) — it did not change the
dispatch outcome. So all four attempts share one cause: **`reasonix run` (v0.53.2, this
env) does not dispatch the tool calls the model emits.** Higher effort cannot fix a
dispatch-layer problem.

**What this is NOT:** still not a claim that Reasonix is incapable or that its numbers
are fake — the model format is fine and the API billed real tokens. The defect is
localized to the headless `run` dispatch path in this version/environment. Reasonix's
interactive `code`/`chat` TUI modes are presumed to wire the executor differently and
remain **untested by `run`** — confirming whether they dispatch where `run` does not
is Lever D (§6, now done).

---

## 6. Lever D — interactive `code` mode DOES dispatch; the real head-to-head

Lever D drove Reasonix's **interactive** `reasonix code` mode (not `run`) through a
pty (`pexpect`), with its **own** filesystem tools rooted at the checkout copy — i.e.
Reasonix operating in the mode it is actually built for. Same model
(`deepseek-v4-flash`), same read-only RLS prompt (single-line variant,
`code_prompt_oneline.txt`), fresh `cp -a` copy.

**Result: `code` mode dispatches tools and completes the task.** The clean run
(`an_rx_d2`) executed **16 tool calls** (`read_file` + `search_content`), produced a
**10,277-char deliverable with 29 `file:line` citations**, and left the tree
**clean** (`git status --short` empty — read-only preserved). This is the smoking-gun
contrast to §5: **the exact same model and prompt that `reasonix run` refused to
dispatch, `reasonix code` dispatches end-to-end.** The defect is `run`-specific.

### 6.1 The head-to-head (both arms completed, same model, read-only-verified)

| arm | mode | tools / turns | cache % | cost (¥) |
|---|---|---|---|---|
| **dsc** | headless `-p` | 25 turns | **93.77** | **0.063** |
| **reasonix** | interactive `code` (pty) | 16 tools | 90.02 | 0.131 |

**Directional reading:** on this single same-model, same-task, both-completed run, dsc
had the **higher cache-hit rate (93.77% vs 90.02%)** *and* was **~2× cheaper (¥0.063 vs
¥0.131)**. That direction supports the win-condition hypothesis — but it is N=1 and
mode-crossed; see the caveats in §8 before treating it as anything more than
directional.

### 6.2 Methods note — an earlier run had a driver artifact (now fixed)

A first `code` run (`an_rx_d`) reported an inflated **96.72% cache / ¥0.178 over 34
turns**. That was **my pty-driver's bug**, not Reasonix's behavior: the driver's
auto-responder matched the substring `"allow"` in the streamed deliverable text (which
discusses `cmp.AllowUnexported`) and sent ~26 spurious `y⏎` keystrokes *after* the task
was done, each adding a near-fully-cached idle "task complete?" turn. Truncating to the
real work (segments 1–8) salvaged **85.20% cache / ¥0.128 / 13 tools** — consistent with
the clean run-2, which is why run-2 is the figure of record. The driver was then fixed
(auto-responder removed; after the deliverable the model idles, the transcript stops
growing, and an idle timer quits cleanly) and re-run to produce §6.1. The contaminated
run is recorded here only to explain why run-2 is trusted.

---

## 7. Conclusion

Two separate findings, at two different confidence levels:

1. **Reliability (high confidence, reproducible):** in headless mode `reasonix run`
   (v0.53.2) **does not dispatch** the tool calls `deepseek-v4-flash` emits — even a
   syntactically valid DSML `Read` invoke at `--effort max` (§5) is recorded as final
   text. The **same model + prompt runs end-to-end in `reasonix code`** (§6), so the
   defect is `run`-specific, not a model or capability problem. dsc drives the same
   task headlessly without issue. This is a dispatch-reliability observation, **not** a
   claim Reasonix is incapable.

2. **Cache/cost thesis (directional only, N=1, mode-crossed):** in the one comparison
   where **both** arms completed (dsc headless vs Reasonix `code`, §6.1), dsc had both
   the **higher cache rate (93.77% vs 90.02%)** and **~2× lower cost (¥0.063 vs
   ¥0.131)** — the direction the win-condition predicts. But the two arms ran in
   **different modes** (forced: Reasonix headless can't dispatch), with different
   turn/tool counts, and the ¥ figures use different accounting (§8). So this is a
   **directional data point, not a proven win.** The `run`-mode head-to-head (§4)
   remains INCONCLUSIVE on its own (Reasonix did nothing there).

**Honest one-liner:** *dsc completes this task headlessly and, in the only completed
comparison, did so with higher cache and ~2× lower cost than Reasonix's `code` mode —
directionally supporting the thesis, on N=1, across different harness modes, with no
"fake-number" claim against Reasonix.*

---

## 8. Threats to validity / honesty caveats

- **(a) N = 1.** One completed comparison (§6.1), one task. Directional at most; not a
  benchmark. Do not generalize.
- **(b) Mode-crossed comparison.** dsc ran headless (`-p`); Reasonix ran interactive
  (`code` via pty). This is **forced**, not chosen: Reasonix's headless `run` does not
  dispatch tools (§5), so `code` is the only mode in which it completes. Each harness
  therefore ran in a *working* configuration, but **not the same** configuration — a
  real confound on both cache and cost.
- **(c) ¥ figures use different accounting.** dsc reports cost natively in CNY;
  Reasonix reports USD, converted here at a flat ×7.2. DeepSeek's USD and CNY price
  lists are set independently, so the FX conversion carries error. The
  accounting-independent comparison is on **tokens** (miss + output), which are
  model-intrinsic: dsc ≈ 39.6K miss; Reasonix run-2 ≈ 30.5K miss + 4.6K output. Treat
  the ~2× ¥ gap as directional, not exact.
- **(d) Cache % is turn/mode-confounded.** dsc's 93.77% is a 25-turn aggregate (cold
  73% first turn amortized); Reasonix's 90.02% is a 9-segment/16-tool aggregate. They
  do not count turns the same way. Also, run-2 executed **after** run-1 on the same
  account, so DeepSeek's account-level cache for Reasonix's prefix may have been
  partially pre-warmed — run-2's 90% could be *flattering* to Reasonix, yet it still
  sits below dsc's 93.77%.
- **(e) Read-only scope.** Chosen specifically to dodge Reasonix's write-sandbox; it is
  not representative of a full code-editing workload.
- **(f) Reliability finding is well-supported.** Effort and prompt-config are excluded
  as causes (§5 attempt #4 + §6 contrast). What remains untested: whether this is
  specific to **v0.53.2** and whether Reasonix's hosted product behaves differently.
- **(g) No "fake" claim.** Every Reasonix number here is real per-run usage. The
  `run`-mode critique is "it didn't dispatch the task," not "the numbers are
  fabricated."
- **(h) dsc is not flawless either** — its symlink guard blocked the native
  `read_file` tool and forced a bash fallback (§4.1). File and fix.

---

## 9. Reproduction

**Prompt** (`/tmp/swebench-go/analysis_prompt.txt`, abridged here; forces ≥5 dependent
reads, read-only):

```
READ-ONLY ANALYSIS TASK (DO NOT MODIFY ANY FILE).
Strictly read-only static analysis of grpc-go @192c8a2a... Work iteratively: read ONE
file, follow the next dependency it reveals, read THAT file, ... across ≥5 .go files.
GOAL 1 — trace (BuilderMap).RLSKey -> builder/matcher.keys(md) -> mapToString, then
  upstream to picker.go, then construction via MakeBuilderMap / config.go / builder.go;
  cite every hop file:line.
GOAL 2 — explain BuilderMapEqual + cmp.AllowUnexported(builder{}, matcher{}); cite the
  call site. DELIVERABLE: written explanation, every claim cited file:line. READ-ONLY.
```

**Commands:**

```bash
# base (once)
mkdir -p /tmp/swebench-go/analysis_base && cd /tmp/swebench-go/analysis_base
git init -q && git remote add origin https://github.com/grpc/grpc-go
git fetch -q --depth 1 origin 192c8a2a3506bb69336f5c135733a3435a04fb30
git checkout -q -f FETCH_HEAD && go mod download

# dsc arm
cp -a /tmp/swebench-go/analysis_base /tmp/swebench-go/an_dsc && cd /tmp/swebench-go/an_dsc
dsc -yolo -read-only -model deepseek-v4-flash -p "$(cat /tmp/swebench-go/analysis_prompt.txt)"

# reasonix arm
cp -a /tmp/swebench-go/analysis_base/. /tmp/swebench-go/an_rx/ && cd /tmp/swebench-go/an_rx
reasonix run -m deepseek-v4-flash --budget 0.30 \
  --transcript /tmp/swebench-go/an_rx.transcript.jsonl \
  "$(cat /tmp/swebench-go/analysis_prompt.txt)"
```

**Lever D — interactive `code` via pty** (the run of record, §6.1). Reasonix `code` is
a TUI requiring a real TTY, so it is driven with `pexpect`; the driver sends the
single-line prompt, waits for the transcript to stop growing, then quits. The
auto-responder must **not** key on substrings like `"allow"` (it appears in
`cmp.AllowUnexported` in the deliverable — see §6.2):

```bash
cp -a /tmp/swebench-go/analysis_base/. /tmp/swebench-go/an_rx_d2/
cd /tmp/swebench-go/an_rx_d2
# pexpect spawns: reasonix code <dir> --no-dashboard --no-mouse -n \
#   --transcript <path> --budget 0.30 ; send prompt; idle-detect on transcript; /exit
python3 /tmp/swebench-go/drive_code2.py
```

**Artifacts (scratch, not committed):** `/tmp/swebench-go/an_dsc.log` (dsc),
`/tmp/swebench-go/an_rx.transcript.jsonl` (run-mode fail),
`/tmp/swebench-go/an_rx_eff1.transcript.jsonl` (`--effort max` fail),
`/tmp/swebench-go/an_rx_d2.transcript.jsonl` (`code`-mode pass, run of record),
`/tmp/swebench-go/drive_code2.py` (pty driver).

---

## 10. What would make this conclusive

Lever D is **done** — Reasonix completes the task in `code` mode (§6), so a head-to-head
is now possible. To upgrade from "directional, N=1" to a real claim:

1. **Close the mode gap.** Either (a) fix/escalate `reasonix run` so both arms are
   headless, or (b) run dsc through *its* interactive path too, so both arms share a
   mode. Without this, §6.1's gap stays mode-confounded.
2. **Anchor on tokens, not ¥.** Report miss-tokens + output-tokens per completed
   deliverable (model-intrinsic, FX-free); show ¥ only as a secondary, clearly-labeled
   conversion (§8c).
3. **Control cache pre-warming.** Run arms in randomized/alternating order, or flush
   between, so account-level cache warmth (§8d) does not favor whichever ran second.
4. **Raise N** to several distinct tasks (and ≥2 runs each) before any directional
   statement becomes a claim.
