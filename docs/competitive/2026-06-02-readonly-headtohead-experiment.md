# Experiment Record — dsc vs Reasonix, Read-Only Multi-Turn Analysis (2026-06-02)

> **摘要（中文）：** 在同一个仓库、同一段只读分析任务、同一个模型（`deepseek-v4-flash`，也是 Reasonix 官方宣传的默认模型）下做 dsc 与 Reasonix 的头对头对比。dsc 干净完成 25 轮真实读取、产出完整交付物，缓存命中 93.77%、花费 ¥0.0634。Reasonix 在 headless `run` 模式下**没有执行任何工具**——它把"Read file: …"当作普通文本叙述输出，1 轮即停，未产出任何分析。因此本次"缓存更高、成本更低"的命题判定为 **INCONCLUSIVE（无法判定）**：不能拿"完成的运行"和"中途未执行的运行"比数字。唯一可靠的观察是：**在本环境的 headless 模式下，dsc 能可靠驱动多轮 agent 任务，Reasonix-flash 只叙述不执行**——这是一个集成/可靠性观察，**不是**对 Reasonix 能力的断言。

This is a factual record of one experiment. It deliberately makes **no competitive
capability claim**: one arm did not complete, so the run is inconclusive (see §6).
It exists so the result is reproducible and so future runs do not silently repeat
the same setup.

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

> The two numeric columns are **not comparable** (see §6). They are recorded for
> completeness, not as a score.

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
remain **untested** — confirming whether they dispatch where `run` does not is the
designated next step (Lever D).

**What this is NOT:** this is not evidence that Reasonix is incapable, nor that its
numbers are fake. Reasonix connected to the API, billed real tokens, and returned
real usage. The observation is specifically about **`reasonix run` headless tool
dispatch with `deepseek-v4-flash`, version 0.53.2, in this environment.** Its
interactive `code`/`chat` TUI modes and its hosted product are out of scope here and
may behave entirely differently.

---

## 6. Conclusion

**On the cache/cost thesis: INCONCLUSIVE. No win is claimed for either side.**

- You cannot compare a completed 25-turn run to an aborted 1-turn run. On raw numbers
  dsc had the higher cache % and Reasonix the lower ¥, but the lower ¥ is an artifact
  of Reasonix doing nothing, not of efficiency. Reporting it as a dsc win would be
  dishonest; reporting it as a Reasonix win would be absurd.

- **The one reliable observation:** in this environment's headless mode, **dsc
  completes a multi-turn dependent-read agent task cleanly (25 turns, 93.77% cache,
  ¥0.0634, read-only preserved), while `reasonix run` (v0.53.2) does not dispatch the
  tool calls `deepseek-v4-flash` emits** — even when, at `--effort max`, the model
  produces a syntactically valid DSML `Read` invoke (§5, attempt #4), the headless loop
  records it as final text and stops. This is a **headless-`run` dispatch reliability**
  data point, explicitly *not* a capability or quality claim about Reasonix (its
  interactive modes are untested).

---

## 7. Threats to validity / honesty caveats

- **(a) N = 1.** One task, one run per arm. Directional at most; not a benchmark.
- **(b) Cache % is confounded by turn count.** dsc's 93.77% is a 25-turn aggregate
  amortizing a cold 73% first turn; Reasonix's 81.5% is a single-turn ratio. They do
  not measure the same thing. Any real comparison must anchor on **cost (¥)** — and
  even ¥ is unusable here because one arm didn't complete.
- **(c) Read-only scope.** Chosen specifically to dodge Reasonix's write-sandbox; it
  is not representative of a full code-editing workload.
- **(d) Confound now largely ruled out.** We used the documented headless entry
  (`reasonix run`), the API connected and billed, the default config kept the system
  prompt, and `--effort` was swept to `max` (attempt #4) — the model then emitted a
  valid DSML tool call and `run` *still* did not dispatch it. So effort and
  prompt-config are excluded; the defect sits in the headless `run` dispatch path. What
  remains untested is whether Reasonix's **interactive** `code`/`chat` modes dispatch
  correctly (Lever D) and whether this is specific to **v0.53.2**.
- **(e) No "fake" claim.** Reasonix's emitted numbers are real per-run usage. The
  critique is "the run didn't do the task," not "the numbers are fabricated."
- **(f) dsc is not flawless either** — its symlink guard blocked the native
  `read_file` tool and forced a bash fallback (§4.1). File and fix.

---

## 8. Reproduction

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

**Artifacts (scratch, not committed):** `/tmp/swebench-go/an_dsc.log`,
`/tmp/swebench-go/an_rx.log`, `/tmp/swebench-go/an_rx.transcript.jsonl`.

---

## 9. What would make a future run conclusive

1. **Get Reasonix to actually execute.** Establish at least one configuration in which
   `reasonix run` (or `code` driven via a pty) dispatches tools on a DeepSeek V4 model
   and completes the deliverable. Until then, no head-to-head number means anything.
2. **Then hold the model constant** and compare cost (¥) on the completed deliverable,
   with cache % reported as secondary/contextual (it is turn-count-confounded).
3. **Raise N** to several tasks before any directional statement becomes a claim.

See the discussion that follows this record for the open question of *which* Reasonix
model to use once dispatch works (flash vs. a stronger DeepSeek V4 / "pro" tier) and
the fairness implications of each.
