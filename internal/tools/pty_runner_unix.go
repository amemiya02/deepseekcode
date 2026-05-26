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

// OutputAppender is an interface for appending output to a buffer.
// Implemented by the agent's Job type.
type OutputAppender interface {
	AppendOutput(p []byte)
}

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

	// Buffer for output (io.ReadAll captures all PTY output)
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

// RunPTYForJob runs a command via PTY and appends output to the provided OutputAppender.
// This is used by the agent for background_bash with pty=true.
// The appender's AppendOutput method is called with output chunks as they are produced.
func RunPTYForJob(ctx context.Context, command string, timeout time.Duration, appender OutputAppender) (string, int, error) {
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

	// Read all output
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

		// Append to job if appender provided
		if appender != nil && len(data) > 0 {
			appender.AppendOutput(data)
		}
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
	output := string(buf)
	copyErrFinal := copyErr
	bufMu.Unlock()

	// Handle errors
	if ctx.Err() == context.DeadlineExceeded {
		return truncateOutput(output, 10_000), -1, context.DeadlineExceeded
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return truncateOutput(output, 10_000), exitErr.ExitCode(), nil
		}
		return truncateOutput(output, 10_000), -1, fmt.Errorf("pty: %w", waitErr)
	}

	if copyErrFinal != nil && copyErrFinal != io.EOF {
		return truncateOutput(output, 10_000), -1, fmt.Errorf("pty: %w", copyErrFinal)
	}

	return truncateOutput(output, 10_000), 0, nil
}
