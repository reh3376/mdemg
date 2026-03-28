package ape

import (
	"testing"
	"time"
)

func TestBackgroundCleanup_RemovesStaleTasks(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	// Add a task that's already old
	d.mu.Lock()
	d.activeTasks["old-task"] = &activeTask{
		Spec:      RSICTaskSpec{TaskID: "old-task"},
		StartedAt: time.Now().Add(-20 * time.Minute),
		Status:    "completed",
	}
	d.mu.Unlock()

	// Start cleanup with short interval for testing
	d.StartBackgroundCleanup(50*time.Millisecond, 10*time.Minute)
	defer d.StopBackgroundCleanup()

	// Wait for at least one cleanup cycle
	time.Sleep(200 * time.Millisecond)

	d.mu.RLock()
	_, exists := d.activeTasks["old-task"]
	d.mu.RUnlock()

	if exists {
		t.Error("expected stale task to be removed by background cleanup")
	}
}

func TestBackgroundCleanup_PreservesRecentTasks(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	// Add a recent task
	d.mu.Lock()
	d.activeTasks["recent-task"] = &activeTask{
		Spec:      RSICTaskSpec{TaskID: "recent-task"},
		StartedAt: time.Now(),
		Status:    "running",
	}
	d.mu.Unlock()

	// Start cleanup
	d.StartBackgroundCleanup(50*time.Millisecond, 10*time.Minute)
	defer d.StopBackgroundCleanup()

	time.Sleep(200 * time.Millisecond)

	d.mu.RLock()
	_, exists := d.activeTasks["recent-task"]
	d.mu.RUnlock()

	if !exists {
		t.Error("expected recent task to be preserved")
	}
}

func TestStopBackgroundCleanup_GracefulShutdown(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	d.StartBackgroundCleanup(50*time.Millisecond, 10*time.Minute)

	// Should not hang
	done := make(chan struct{})
	go func() {
		d.StopBackgroundCleanup()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("StopBackgroundCleanup timed out")
	}
}
