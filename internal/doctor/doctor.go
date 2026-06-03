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
func Run(ctx context.Context, cfg config.Config, out io.Writer) error {
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
	} else {
		fmt.Fprintln(out, "All checks passed.")
	}
	return nil
}
