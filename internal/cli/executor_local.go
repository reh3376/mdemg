package cli

import (
	"fmt"
	"os"
	osExec "os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// LocalExecutor implements sidecar.Executor by wrapping existing local helpers.
type LocalExecutor struct{}

func newLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

// RunDocker delegates to the existing RunDockerCommand helper.
func (e *LocalExecutor) RunDocker(args ...string) (string, error) {
	return RunDockerCommand(args...)
}

// DockerAvailable checks if docker CLI is in PATH.
func (e *LocalExecutor) DockerAvailable() bool {
	return DockerAvailable()
}

// StartDaemon starts the MDEMG server as a detached local process.
// Returns the PID of the started process.
func (e *LocalExecutor) StartDaemon(serveArgs []string) (int, error) {
	execPath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve executable: %w", err)
	}

	logPath := logFilePath()
	if mkErr := os.MkdirAll(filepath.Dir(logPath), 0755); mkErr != nil {
		return 0, fmt.Errorf("create log directory: %w", mkErr)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}

	daemon := osExec.Command(execPath, serveArgs...)
	daemon.Stdout = logFile
	daemon.Stderr = logFile
	daemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	daemon.Env = os.Environ()

	if startErr := daemon.Start(); startErr != nil {
		logFile.Close()
		return 0, fmt.Errorf("start daemon: %w", startErr)
	}

	pid := daemon.Process.Pid
	_ = daemon.Process.Release()
	logFile.Close()

	return pid, nil
}

// StopDaemon sends SIGTERM, polls for exit (30s), then SIGKILL.
func (e *LocalExecutor) StopDaemon(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !e.DaemonRunning(pid) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill after timeout
	_ = syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(1 * time.Second)
	return nil
}

// DaemonRunning checks if a local PID is alive via kill -0.
func (e *LocalExecutor) DaemonRunning(pid int) bool {
	return isProcessAlive(pid)
}

// WaitForPort delegates to the existing WaitForPort helper.
func (e *LocalExecutor) WaitForPort(host string, port int, timeout time.Duration) error {
	return WaitForPort(host, port, timeout)
}

// Host returns "localhost" for local profile.
func (e *LocalExecutor) Host() string {
	return "localhost"
}

// Close is a no-op for local executor.
func (e *LocalExecutor) Close() error {
	return nil
}
