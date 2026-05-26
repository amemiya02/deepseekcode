//go:build windows

package tools

import (
	"context"
	"errors"
	"time"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

var errPTYUnsupported = errors.New("bash_pty is not supported on this platform")

// runPTY on Windows returns errPTYUnsupported.
func runPTY(ctx context.Context, command string, timeout time.Duration) (output string, exitCode int, err error) {
	return "", -1, errPTYUnsupported
}

func runPTYWithSandbox(ctx context.Context, command string, timeout time.Duration, sb sandbox.Sandbox, profile sandbox.Profile) (output string, exitCode int, err error) {
	return "", -1, errPTYUnsupported
}

// RunPTYForJob on Windows returns errPTYUnsupported.
func RunPTYForJob(ctx context.Context, command string, timeout time.Duration, appender OutputAppender) (output string, exitCode int, err error) {
	return "", -1, errPTYUnsupported
}

func RunPTYForJobWithSandbox(ctx context.Context, command string, timeout time.Duration, appender OutputAppender, sb sandbox.Sandbox, profile sandbox.Profile) (output string, exitCode int, err error) {
	return "", -1, errPTYUnsupported
}
