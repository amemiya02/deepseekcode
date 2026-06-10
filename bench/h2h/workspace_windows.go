//go:build windows

package h2h

import "syscall"

func setSysProcAttr(cmd *syscall.SysProcAttr) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func killProcessGroup(pid int) {
	// Windows: no SIGKILL; best-effort process kill via os.Process.Kill
	// is handled in the caller. This is a no-op placeholder.
}
