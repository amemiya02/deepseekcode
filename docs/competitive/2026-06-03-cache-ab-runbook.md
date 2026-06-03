# Cache A/B Serialization — 3-Arm Benchmark Runbook

> Wire-dump prerequisite: Plan 1 (2026-06-03-cache-wire-body-diagnostic.md) must be merged.
> Bench harness: /tmp/swebench-go (SWE-bench Go subset, ≥20 tasks).

## Arms

| Label | Env | Expected effect |
|---|---|---|
| arm-current | (unset) | Baseline: full reasoning_content in every turn |
| arm-drop-all | `DEEPSEEKCODE_REASONING_DROP=1` | Reasoning stripped; body smaller; may see 400s on reasoning model |
| arm-retain-last | `DEEPSEEKCODE_REASONING_RETAIN=3` | Last 3 turns keep reasoning; older turns get placeholder |

## Metrics

- **400-rate**: fraction of API calls returning HTTP 400 (reasoning-format errors).
- **full-body evictions**: count of turns where cache_hit_tokens=0 AND total_tokens > threshold (detect via wire-dump diff-body).
- **cache%**: `cache_hit_tokens / (cache_hit_tokens + cache_miss_tokens)` per session.
- **turns**: total assistant turns to task completion.
- **cost-USD**: total billed tokens × rate (input + cache-write + cache-hit).

## Run Protocol

1. Check out the branch under test. Confirm `go build ./cmd/dsc` is clean.
2. Set `DEEPSEEKCODE_WIRE_DUMP=/tmp/wire-dumps/<arm>` before each arm run.
3. Launch the bench harness for each arm sequentially (not concurrently — same DeepSeek account):

```bash
# arm-current
DEEPSEEKCODE_WIRE_DUMP=/tmp/wire-dumps/current \
  /tmp/swebench-go --tasks 20 --output /tmp/results/current.json

# arm-drop-all
DEEPSEEKCODE_REASONING_DROP=1 \
DEEPSEEKCODE_WIRE_DUMP=/tmp/wire-dumps/drop-all \
  /tmp/swebench-go --tasks 20 --output /tmp/results/drop-all.json

# arm-retain-last (N=3)
DEEPSEEKCODE_REASONING_RETAIN=3 \
DEEPSEEKCODE_WIRE_DUMP=/tmp/wire-dumps/retain-last \
  /tmp/swebench-go --tasks 20 --output /tmp/results/retain-last.json
```

4. For each arm pair run diff-body:

```bash
dsc trace diff-body /tmp/wire-dumps/current /tmp/wire-dumps/drop-all
dsc trace diff-body /tmp/wire-dumps/current /tmp/wire-dumps/retain-last
```

5. Record results in the table below.

## Result Table Template

| Metric | arm-current | arm-drop-all | arm-retain-last |
|---|---|---|---|
| Tasks completed / 20 | | | |
| 400-rate | | | |
| Full-body evictions (median/session) | | | |
| Cache% (mean across sessions) | | | |
| Turns (mean to completion) | | | |
| Cost-USD (total) | | | |

## Decision Criteria

- If arm-drop-all 400-rate > 5%: Reasonix policy is unsafe for reasoning model; do not ship.
- If arm-retain-last cache% ≥ arm-current cache% − 5pp AND cost-USD ≤ arm-current × 0.85: ship retain-last as default N=3.
- If no arm beats baseline on cost: file issue, do not change default.
