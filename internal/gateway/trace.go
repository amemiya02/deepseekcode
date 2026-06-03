package gateway

import "os"

// defaultTracePath returns the JSONL trace path the gateway reads for
// /v1/cache. It honours the same DEEPSEEKCODE_TRACE_JSONL environment variable
// the dsc CLI and benchmark harness use, so a gateway started in-process by the
// desktop wrapper reports on the same trace the agent is writing. When the env
// var is unset the path is empty, which buildCacheReport treats as "no trace
// yet" and answers with a zero-valued report.
func defaultTracePath() string {
	return os.Getenv("DEEPSEEKCODE_TRACE_JSONL")
}
