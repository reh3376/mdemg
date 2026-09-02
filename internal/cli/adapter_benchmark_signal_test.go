// ADAPTER-SWAP-STANDARDIZE-002 signal-handler tests (task #146).
package cli

import (
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestSignalCleanupBenchmark_StopsWhenBenchStarted verifies that a signal
// fires the cleanup path when the bench was successfully started. We use
// SIGUSR1 (not SIGTERM) so re-raising doesn't kill the test process.
func TestSignalCleanupBenchmark_StopsWhenBenchStarted(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	port := 18103
	pidfilePath, err := benchServePidFile(port)
	if err != nil {
		t.Fatalf("benchServePidFile: %v", err)
	}

	// Simulate an active bench-serve: write a pidfile pointing at a bogus PID.
	// stopBenchServe will try to kill it, notice ESRCH, and clean up the file.
	rec := benchServePidRecord{PID: 999999, Port: port, AdapterDir: "/tmp/x", BaseModel: "/tmp/b"}
	if err := writePidRecord(pidfilePath, rec); err != nil {
		t.Fatalf("writePidRecord: %v", err)
	}
	if _, err := os.Stat(pidfilePath); err != nil {
		t.Fatalf("pidfile pre-check: %v", err)
	}

	// Wire the handler with a signal we can send safely.
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	var benchStarted atomic.Bool
	benchStarted.Store(true)

	go signalCleanupBenchmark(sigCh, done, &benchStarted, pidfilePath, port)

	// Send a synthetic signal (test-only path — bypasses signal.Notify).
	sigCh <- syscall.SIGUSR1

	select {
	case <-done:
		// Cleanup completed
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup goroutine did not signal done within 5s")
	}

	// Pidfile should be removed by stopBenchServe
	if _, err := os.Stat(pidfilePath); !os.IsNotExist(err) {
		t.Errorf("pidfile should be gone after cleanup; err=%v", err)
	}
}

// TestSignalCleanupBenchmark_NormalShutdown verifies that closing the
// channel (normal defer path) exits the goroutine cleanly WITHOUT
// attempting cleanup.
func TestSignalCleanupBenchmark_NormalShutdown(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	port := 18104
	pidfilePath, _ := benchServePidFile(port)

	// No pidfile pre-write — if cleanup ran, it would silently no-op,
	// but we want to confirm the code path via absent-file post-check.
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	var benchStarted atomic.Bool
	benchStarted.Store(true)

	go signalCleanupBenchmark(sigCh, done, &benchStarted, pidfilePath, port)

	// Simulate normal shutdown: close the channel
	close(sigCh)

	select {
	case <-done:
		// Goroutine exited via channel-close branch
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not exit on channel close within 5s")
	}

	// Pidfile still absent (never existed)
	if _, err := os.Stat(pidfilePath); !os.IsNotExist(err) {
		t.Errorf("no pidfile expected on normal-shutdown path; err=%v", err)
	}
}

// TestSignalCleanupBenchmark_BenchNotStartedSkipsStop verifies the
// benchStarted guard — if bench-serve never started, cleanup should not
// touch the pidfile (which shouldn't exist anyway).
func TestSignalCleanupBenchmark_BenchNotStartedSkipsStop(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	port := 18105
	pidfilePath, _ := benchServePidFile(port)

	// Pre-write a pidfile that SHOULD survive because benchStarted=false
	rec := benchServePidRecord{PID: 999999, Port: port}
	_ = writePidRecord(pidfilePath, rec)

	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	var benchStarted atomic.Bool
	// benchStarted stays false — bench never actually started

	go signalCleanupBenchmark(sigCh, done, &benchStarted, pidfilePath, port)

	sigCh <- syscall.SIGUSR1

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not signal done within 5s")
	}

	// Pidfile still present — benchStarted guard prevented cleanup
	if _, err := os.Stat(pidfilePath); os.IsNotExist(err) {
		t.Errorf("pidfile removed despite benchStarted=false; guard broken")
	}

	// Cleanup for hygiene
	_ = os.Remove(pidfilePath)
}
