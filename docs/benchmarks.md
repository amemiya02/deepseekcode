# Performance Baseline

Reproduce with:
```
go test -bench=. -benchmem -run="^$" -benchtime=2s ./internal/tools/
```

Date: 2026-05-24
Platform: Apple M4 Pro, macOS, arm64

## internal/tools benchmarks

```
BenchmarkClassifyBash-14         1,620,954    726 ns/op    1882 B/op    13 allocs/op
BenchmarkLevenshteinLines-14       187,072   6430 ns/op     832 B/op     2 allocs/op
BenchmarkClosestMatch-14           100,884  11813 ns/op    9600 B/op   200 allocs/op
BenchmarkTokenize-14              4,773,925    250 ns/op     352 B/op    14 allocs/op
BenchmarkHasRedirect-14         136,747,584      9 ns/op       0 B/op     0 allocs/op
BenchmarkEditFileExecute-14         36,709  34460 ns/op   11427 B/op   107 allocs/op
```

## Notes

- **ClassifyBash** (~726 ns): Fast enough for per-command permission gating. The tokenized
  first-verb heuristic is ~3× faster than regex-based destructive detection.

- **LevenshteinLines** (~6.4 µs): Line-level DP on 50-line inputs. Two-row optimization
  keeps memory at O(min(n,m)).

- **ClosestMatch** (~11.8 µs): Scans 100 windows of 5-line candidates. Dominated by
  LevenshteinLines per window — linear in haystack size.

- **Tokenize** (~250 ns): Simple shell-like splitter. No regex — just byte scanning.

- **HasRedirect** (~9 ns): Character-level scan with quote tracking. Essentially free
  compared to the surrounding classification work.

- **EditFileExecute** (~34.5 µs): Full file edit cycle (read + string replace + atomic write)
  on a 100-line file. Dominated by I/O, not string operations.
