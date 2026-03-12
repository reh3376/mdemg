//go:build !windows

package cli

import (
	osExec "os/exec"
	"syscall"
)

// checkProcessAlive uses kill -0 to test if a PID exists.
func checkProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// setDaemonSysProcAttr configures the command for background daemon execution.
func setDaemonSysProcAttr(cmd *osExec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// terminateProcess sends SIGTERM to gracefully stop a process.
func terminateProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// forceKillProcess sends SIGKILL to forcefully stop a process.
func forceKillProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
