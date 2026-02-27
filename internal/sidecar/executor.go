package sidecar

import "time"

// Executor abstracts runtime operations (Docker, daemon, port) so that
// commands work identically for local and studio-remote profiles.
type Executor interface {
	// RunDocker executes a docker command, returns stdout.
	RunDocker(args ...string) (string, error)
	// DockerAvailable returns true if Docker is accessible.
	DockerAvailable() bool
	// StartDaemon starts MDEMG server in background. Returns PID.
	StartDaemon(serveArgs []string) (int, error)
	// StopDaemon stops the MDEMG server process.
	StopDaemon(pid int) error
	// DaemonRunning checks if PID is alive.
	DaemonRunning(pid int) bool
	// WaitForPort waits until host:port is reachable.
	WaitForPort(host string, port int, timeout time.Duration) error
	// Host returns the runtime host name ("localhost" for local, remote IP for remote).
	Host() string
	// Close releases resources (SSH connections, etc.).
	Close() error
}
