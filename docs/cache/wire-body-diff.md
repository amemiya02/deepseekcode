# Wire-body diff — pinning Layer-2 cache eviction

Capture every turn's exact request body, then byte-diff two consecutive
turns to find the first byte that changed inside the region that should be
stable. A clean append (later body == earlier body + tail) is cache-stable;
any earlier-region change is the eviction cause.

## Capture a run
```bash
export DEEPSEEKCODE_WIRE_DUMP=/tmp/wire
dsc -yolo -p "…task…"        # writes /tmp/wire/turn_0001.json, turn_0002.json, …
unset DEEPSEEKCODE_WIRE_DUMP # diagnostic only — never leave on (bodies contain source)
```

## Diff two turns
```bash
dsc trace diff-body /tmp/wire/turn_0007.json /tmp/wire/turn_0008.json
```
Verdicts:
- `cache-stable` — earlier body is a clean prefix of the later (append-only). Good.
- `EVICTION CAUSE` — historical bytes changed at offset N; the printed A/B
  windows show what drifted. This is the byte that breaks the prefix cache.
- `truncated/compacted` — the later body is shorter (a compaction happened).
