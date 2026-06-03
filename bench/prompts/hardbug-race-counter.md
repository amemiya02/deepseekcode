The test `TestConcurrentAddIsRaceFree` fails under the race detector because
`Tally.Add` mutates a map concurrently without synchronization. Fix
`counter.go` so the test passes under `go test -race ./...` without changing the
public API (`New`, `Add`, `Get`). Make the minimal change.
