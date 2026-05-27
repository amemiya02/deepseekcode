//go:build !windows

package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
	"github.com/creack/pty"
)

var errPTYUnsupported = errors.New("bash_pty is not supported on this platform")

// isPTYClose reports whether err marks the normal end of a PTY master
// read. On macOS the master read returns io.EOF when the slave closes;
// on Linux the same event surfaces as syscall.EIO (read /dev/ptmx:
// input/output error). Both mean "child closed the PTY", not failure.
func isPTYClose(err error) bool {
	return err == nil || err == io.EOF || errors.Is(err, syscall.EIO)
}

// runPTY runs cmd via $SHELL -c inside a PTY, copies stdout/stderr into
// a single buffer, and returns the trimmed/truncated output. ctx cancels
// the child; timeout caps the total duration.
func runPTY(ctx context.Context, command string, timeout time.Duration) (output string, exitCode int, err error) {
	return runPTYWithSandbox(ctx, command, timeout, nil, sandbox.Profile{})
}

func runPTYWithSandbox(ctx context.Context, command string, timeout time.Duration, sb sandbox.Sandbox, profile sandbox.Profile) (output string, exitCode int, err error) {
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
	notice := wrapWithSandbox(ctx, sb, profile, cmd)

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
	output = notice + string(buf)
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

	if !isPTYClose(copyErrFinal) {
		return truncateOutput(output, 10_000), -1, fmt.Errorf("pty: %w", copyErrFinal)
	}

	return truncateOutput(output, 10_000), 0, nil
}

// RunPTYForJob runs a command via PTY and appends output to the provided OutputAppender.
// This is used by the agent for background_bash with pty=true.
// The appender's AppendOutput method is called with output chunks as they are produced.
func RunPTYForJob(ctx context.Context, command string, timeout time.Duration, appender OutputAppender) (string, int, error) {
	return RunPTYForJobWithSandbox(ctx, command, timeout, appender, nil, sandbox.Profile{})
}

func RunPTYForJobWithSandbox(ctx context.Context, command string, timeout time.Duration, appender OutputAppender, sb sandbox.Sandbox, profile sandbox.Profile) (string, int, error) {
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
	notice := wrapWithSandbox(ctx, sb, profile, cmd)
	if notice != "" && appender != nil {
		appender.AppendOutput([]byte(notice))
	}

	// Start PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", -1, fmt.Errorf("pty: %w", err)
	}
	defer ptmx.Close()

	// Read output incrementally, streaming to appender as we go
	var buf bytes.Buffer
	var bufMu sync.Mutex
	var copyErr error
	var copyWg sync.WaitGroup

	copyWg.Add(1)
	go func() {
		defer copyWg.Done()
		chunk := make([]byte, 4096)
		for {
			n, err := ptmx.Read(chunk)
			if n > 0 {
				bufMu.Lock()
				buf.Write(chunk[:n])
				bufMu.Unlock()

				// Stream to job buffer incrementally
				if appender != nil {
					appender.AppendOutput(chunk[:n])
				}
			}
			if err != nil {
				copyErr = err
				break
			}
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
	output := notice + buf.String()
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

	if !isPTYClose(copyErrFinal) {
		return truncateOutput(output, 10_000), -1, fmt.Errorf("pty: %w", copyErrFinal)
	}

	return truncateOutput(output, 10_000), 0, nil
}
