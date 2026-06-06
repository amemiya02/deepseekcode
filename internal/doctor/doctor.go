// Package doctor implements the `dsc doctor` health-check command.
package doctor

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/config"
)

// CheckResult is the outcome of a single doctor check.
type CheckResult struct {
	Name   string
	OK     bool
	Detail string
}

// Checker is a function that performs one health check.
type Checker func(ctx context.Context, cfg config.Config, hc *http.Client) CheckResult

// RunChecks executes all checkers sequentially and returns their results.
// hc is injected for testability (nil = http.DefaultClient inside each check).
func RunChecks(ctx context.Context, cfg config.Config, hc *http.Client, checkers []Checker) []CheckResult {
	results := make([]CheckResult, 0, len(checkers))
	for _, c := range checkers {
		results = append(results, c(ctx, cfg, hc))
	}
	return results
}

// Run is the entry point for `dsc doctor`. It prints a pass/fail report to out.
// It returns a non-nil error when any check fails, so callers can gate on exit
// code (e.g. in CI: `dsc doctor || exit 1`).
// loadErr, if non-nil, is surfaced as an additional failing check so a corrupt
// or unreadable config is visible in the report rather than silently producing
// misleading zero-value results.
func Run(ctx context.Context, cfg config.Config, out io.Writer, loadErr error) error {
	hc := &http.Client{}
	checkers := []Checker{
		CheckKeyPresent,
		CheckKeyValid,
		CheckBaseURLReachable,
		CheckProxyConfigured,
		CheckCacheFieldsInProbe,
		CheckSandboxAvailable,
	}
	results := RunChecks(ctx, cfg, hc, checkers)

	// Prepend a synthetic config-load result when the caller had an error so
	// the operator sees it in context with the other checks.
	if loadErr != nil {
		results = append([]CheckResult{{
			Name:   "config load",
			OK:     false,
			Detail: loadErr.Error(),
		}}, results...)
	}

	allOK := true
	fmt.Fprintln(out, "dsc doctor")
	fmt.Fprintln(out, "----------")
	for _, r := range results {
		status := "PASS"
		if !r.OK {
			status = "FAIL"
			allOK = false
		}
		fmt.Fprintf(out, "  [%s] %s: %s\n", status, r.Name, r.Detail)
	}
	fmt.Fprintln(out, "")
	if !allOK {
		fmt.Fprintln(out, "Some checks failed. See details above.")
		return fmt.Errorf("doctor: %d check(s) failed", countFailed(results))
	}
	fmt.Fprintln(out, "All checks passed.")
	return nil
}

// countFailed returns the number of failed results.
func countFailed(results []CheckResult) int {
	n := 0
	for _, r := range results {
		if !r.OK {
			n++
		}
	}
	return n
}
