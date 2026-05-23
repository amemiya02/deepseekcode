package tools

import "fmt"

// fmtSprintf wraps fmt.Sprintf so result.go can avoid an unused-import
// when fmt is referenced via a single helper. Keeping the indirection
// out of the hot Result path also makes microbenchmarking trivial.
func fmtSprintf(format string, args []any) string {
	return fmt.Sprintf(format, args...)
}
