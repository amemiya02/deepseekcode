//go:build !windows

package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

var errPTYUnsupported = errors.New("bash_pty is not supported on this platform")

// runPTY runs cmd via $SHELL -c inside a PTY, copies stdout/stderr into
// a single buffer, and returns the trimmed/truncated output. ctx cancels
// the child; timeout caps the total duration.
func runPTY(ctx context.Context, command string, timeout time.Duration) (output string, exitCode int, err error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	// Apply timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Env = append(os.Environ(), "TERM=dumb")

	// Start PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", -1, fmt.Errorf("pty: %w", err)
	}
	defer ptmx.Close()

	// Buffer for output
	var buf []byte
	var bufMu sync.Mutex
	var copyErr error
	var copyWg sync.WaitGroup

	copyWg.Add(1)
	go func() {
		defer copyWg.Done()
		data, err := io.ReadAll(ptmx)
		bufMu.Lock()
		buf = data
		copyErr = err
		bufMu.Unlock()
	}()

	// Wait for command to finish
	waitErr := cmd.Wait()

	// Give io.Copy a moment to finish reading residual output
	done := make(chan struct{})
	go func() {
		copyWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Copy finished
	case <-time.After(100 * time.Millisecond):
		// Timeout waiting for copy, proceed anyway
	}

	bufMu.Lock()
	output = string(buf)
	copyErrFinal := copyErr
	bufMu.Unlock()

	// Handle errors
	if ctx.Err() == context.DeadlineExceeded {
		truncated := truncateOutput(output, 10_000)
		return truncated, -1, context.DeadlineExceeded
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			truncated := truncateOutput(output, 10_000)
			return truncated, exitErr.ExitCode(), nil
		}
		return truncateOutput(output, 10_000), -1, fmt.Errorf("pty: %w", waitErr)
	}

	if copyErrFinal != nil && copyErrFinal != io.EOF {
		return truncateOutput(output, 10_000), -1, fmt.Errorf("pty: %w", copyErrFinal)
	}

	return truncateOutput(output, 10_000), 0, nil
}
