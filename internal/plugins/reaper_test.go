package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// MCP-REVIVE-001: a process from a previous server generation holding our
// socket path must be reaped before the new instance starts (kickstart -k
// kills the server without Manager.Stop — 3 orphan generations were live).
func TestReapOrphanPluginProcesses(t *testing.T) {
	socketPath := fmt.Sprintf("/tmp/mdemg-reaper-test-%d.sock", os.Getpid())

	// A decoy "orphan" whose command line carries the socket path the way a
	// real plugin does (in its own argv — `sh -c` exec-optimizes away the
	// shell, losing decorative args, so use tail -f on the path itself).
	if err := os.WriteFile(socketPath, nil, 0o644); err != nil {
		t.Fatalf("create decoy socket file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	orphan := exec.Command("tail", "-f", socketPath)
	if err := orphan.Start(); err != nil {
		t.Fatalf("start decoy: %v", err)
	}
	defer func() { _ = orphan.Process.Kill() }()

	// Wait() in a goroutine: as our child, a killed decoy stays signalable
	// (zombie) until reaped by Wait — Signal(0) can't observe the kill.
	done := make(chan error, 1)
	go func() { done <- orphan.Wait() }()
	time.Sleep(150 * time.Millisecond) // let it register in the process table

	reapOrphanPluginProcesses(socketPath)

	select {
	case <-done:
		// process exited — reaped (well before its 30s sleep)
	case <-time.After(3 * time.Second):
		t.Fatal("orphan process still alive after reap")
	}
}

// No-match must be a silent no-op (pgrep exits 1).
func TestReapOrphanPluginProcesses_NoMatch(t *testing.T) {
	reapOrphanPluginProcesses("/tmp/mdemg-reaper-test-nonexistent.sock")
}
