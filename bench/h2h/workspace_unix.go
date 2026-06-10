//go:build !windows

package h2h

import "syscall"

func setSysProcAttr(cmd *syscall.SysProcAttr) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL)
}
