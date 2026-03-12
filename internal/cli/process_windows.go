//go:build windows

package cli

import (
	"fmt"
	"os"
	osExec "os/exec"
	"strings"
	"syscall"
)

// checkProcessAlive queries the Windows task list to check if a PID exists.
func checkProcessAlive(pid int) bool {
	cmd := osExec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return !strings.Contains(string(out), "INFO: No tasks")
}

// setDaemonSysProcAttr configures the command for background execution on Windows.
func setDaemonSysProcAttr(cmd *osExec.Cmd) {
	const createNewProcessGroup = 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
	}
}

// terminateProcess kills a process on Windows (no SIGTERM equivalent).
func terminateProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	return p.Kill()
}

// forceKillProcess forcefully kills a process (same as terminate on Windows).
func forceKillProcess(pid int) error {
	return terminateProcess(pid)
}
