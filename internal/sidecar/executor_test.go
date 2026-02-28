package sidecar

import (
	"testing"
	"time"
)

// mockExecutor verifies the Executor interface can be satisfied.
type mockExecutor struct{}

func (m *mockExecutor) RunDocker(args ...string) (string, error) { return "", nil }
func (m *mockExecutor) DockerAvailable() bool                    { return true }
func (m *mockExecutor) StartDaemon(_ []string, _ ...string) (int, error) { return 42, nil }
func (m *mockExecutor) StopDaemon(_ int) error                   { return nil }
func (m *mockExecutor) DaemonRunning(_ int) bool                 { return true }
func (m *mockExecutor) WaitForPort(_ string, _ int, _ time.Duration) error {
	return nil
}
func (m *mockExecutor) Host() string  { return "localhost" }
func (m *mockExecutor) Close() error  { return nil }

// TestExecutorInterface verifies that the mock satisfies the Executor interface.
func TestExecutorInterface(t *testing.T) {
	var _ Executor = &mockExecutor{}
}
