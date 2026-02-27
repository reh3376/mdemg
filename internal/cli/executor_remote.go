package cli

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"mdemg/internal/sidecar"
)

const mdemgDockerContext = "mdemg-studio"

// RemoteExecutor implements sidecar.Executor for the studio-remote profile.
// It uses either docker-context or ssh-exec transport.
type RemoteExecutor struct {
	host      string
	transport sidecar.Transport
}

func newRemoteExecutor(host string, transport sidecar.Transport) (*RemoteExecutor, error) {
	if host == "" {
		return nil, fmt.Errorf("remote host is required for studio-remote profile")
	}
	if transport == "" {
		transport = sidecar.TransportDockerContext
	}
	return &RemoteExecutor{
		host:      host,
		transport: transport,
	}, nil
}

// runSSH executes a command on the remote host via SSH.
func (e *RemoteExecutor) runSSH(args ...string) (string, error) {
	sshArgs := []string{
		"-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes",
		e.host,
	}
	sshArgs = append(sshArgs, args...)

	cmd := exec.Command("ssh", sshArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// RunDocker executes a docker command routed to the remote host.
// Uses docker-context or ssh-exec transport depending on configuration.
func (e *RemoteExecutor) RunDocker(args ...string) (string, error) {
	switch e.transport {
	case sidecar.TransportDockerContext:
		dockerArgs := []string{"--context", mdemgDockerContext}
		dockerArgs = append(dockerArgs, args...)
		cmd := exec.Command("docker", dockerArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("docker --context %s %s: %s (%w)",
				mdemgDockerContext, strings.Join(args, " "),
				strings.TrimSpace(string(out)), err)
		}
		return string(out), nil

	case sidecar.TransportSSHExec:
		remoteArgs := append([]string{"docker"}, args...)
		return e.runSSH(remoteArgs...)

	default:
		return "", fmt.Errorf("unsupported transport: %s", e.transport)
	}
}

// DockerAvailable checks if Docker is accessible on the remote host.
func (e *RemoteExecutor) DockerAvailable() bool {
	_, err := e.RunDocker("info", "--format", "{{.ServerVersion}}")
	return err == nil
}

// StartDaemon starts the MDEMG server on the remote host via SSH.
// Returns the remote PID.
func (e *RemoteExecutor) StartDaemon(serveArgs []string) (int, error) {
	// Build the remote command
	args := strings.Join(serveArgs, " ")
	remoteCmd := fmt.Sprintf(
		"mkdir -p ~/.mdemg/logs && nohup mdemg %s > ~/.mdemg/logs/mdemg.log 2>&1 & echo $!",
		args,
	)

	out, err := e.runSSH("bash", "-c", fmt.Sprintf("'%s'", remoteCmd))
	if err != nil {
		return 0, fmt.Errorf("start remote daemon: %w", err)
	}

	pidStr := strings.TrimSpace(out)
	pid, parseErr := strconv.Atoi(pidStr)
	if parseErr != nil {
		return 0, fmt.Errorf("parse remote PID %q: %w", pidStr, parseErr)
	}

	return pid, nil
}

// StopDaemon stops the MDEMG server on the remote host via SSH.
func (e *RemoteExecutor) StopDaemon(pid int) error {
	_, err := e.runSSH("kill", strconv.Itoa(pid))
	if err != nil {
		// Process may already be dead
		if !e.DaemonRunning(pid) {
			return nil
		}
		return fmt.Errorf("remote kill %d: %w", pid, err)
	}

	// Poll for process exit (30s)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if !e.DaemonRunning(pid) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill
	_, _ = e.runSSH("kill", "-9", strconv.Itoa(pid))
	time.Sleep(1 * time.Second)
	return nil
}

// DaemonRunning checks if a remote PID is alive via SSH kill -0.
func (e *RemoteExecutor) DaemonRunning(pid int) bool {
	_, err := e.runSSH("kill", "-0", strconv.Itoa(pid))
	return err == nil
}

// WaitForPort waits until host:port is reachable via TCP from the control host.
func (e *RemoteExecutor) WaitForPort(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s after %v", addr, timeout)
}

// Host returns the configured remote host.
func (e *RemoteExecutor) Host() string {
	return e.host
}

// Close is a no-op (no persistent SSH connection).
func (e *RemoteExecutor) Close() error {
	return nil
}

// EnsureDockerContext checks if the mdemg-studio Docker context exists
// and creates it if not.
func (e *RemoteExecutor) EnsureDockerContext() error {
	// Check if context already exists
	out, err := exec.Command("docker", "context", "ls", "--format", "{{.Name}}").CombinedOutput()
	if err == nil {
		for line := range strings.SplitSeq(string(out), "\n") {
			if strings.TrimSpace(line) == mdemgDockerContext {
				return nil // already exists
			}
		}
	}

	// Create the context
	endpoint := fmt.Sprintf("ssh://%s", e.host)
	cmd := exec.Command("docker", "context", "create", mdemgDockerContext,
		"--docker", fmt.Sprintf("host=%s", endpoint))
	if createOut, createErr := cmd.CombinedOutput(); createErr != nil {
		return fmt.Errorf("create docker context %s: %s (%w)",
			mdemgDockerContext, strings.TrimSpace(string(createOut)), createErr)
	}

	return nil
}
