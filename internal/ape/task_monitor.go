package ape

import (
	"time"
)

// Monitor provides read access to running and completed task state.
type Monitor struct {
	dispatcher *Dispatcher
}

// NewMonitor creates a Monitor backed by the given Dispatcher.
func NewMonitor(dispatcher *Dispatcher) *Monitor {
	return &Monitor{dispatcher: dispatcher}
}

// GetTaskStatus returns the current status of a task by ID.
func (m *Monitor) GetTaskStatus(taskID string) (string, bool) {
	m.dispatcher.mu.RLock()
	defer m.dispatcher.mu.RUnlock()
	if at, ok := m.dispatcher.activeTasks[taskID]; ok {
		return at.Status, true
	}
	return "", false
}

// GetTaskReports returns all progress reports for a task.
func (m *Monitor) GetTaskReports(taskID string) []RSICProgressReport {
	m.dispatcher.mu.RLock()
	defer m.dispatcher.mu.RUnlock()
	reports := m.dispatcher.reports[taskID]
	out := make([]RSICProgressReport, len(reports))
	copy(out, reports)
	return out
}

// GetAllActiveTasks returns a snapshot of all active task states.
func (m *Monitor) GetAllActiveTasks() map[string]string {
	m.dispatcher.mu.RLock()
	defer m.dispatcher.mu.RUnlock()
	result := make(map[string]string, len(m.dispatcher.activeTasks))
	for id, at := range m.dispatcher.activeTasks {
		result[id] = at.Status
	}
	return result
}

// WaitForCycle blocks until all tasks in the given cycle are complete or the timeout elapses.
// Returns true if all tasks completed, false if timed out.
func (m *Monitor) WaitForCycle(cycleID string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastVersion uint64
	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			done, ver := m.allTasksDone(cycleID)
			if done {
				return true
			}
			// Detect stale reads: if version hasn't changed, tasks may have been
			// cleaned up between polls. Continue waiting only if version advances.
			if ver > 0 && ver == lastVersion {
				continue
			}
			lastVersion = ver
		}
	}
}

func (m *Monitor) allTasksDone(cycleID string) (done bool, version uint64) {
	m.dispatcher.mu.RLock()
	defer m.dispatcher.mu.RUnlock()

	found := false
	for _, at := range m.dispatcher.activeTasks {
		if at.Spec.CycleID == cycleID {
			found = true
			if at.Status == "running" {
				return false, m.dispatcher.stateVersion
			}
		}
	}
	return found, m.dispatcher.stateVersion
}

// CollectReportsForCycle returns all progress reports for tasks in a cycle.
func (m *Monitor) CollectReportsForCycle(cycleID string) []RSICProgressReport {
	m.dispatcher.mu.RLock()
	defer m.dispatcher.mu.RUnlock()

	var result []RSICProgressReport
	for _, at := range m.dispatcher.activeTasks {
		if at.Spec.CycleID == cycleID {
			if reports, ok := m.dispatcher.reports[at.Spec.TaskID]; ok {
				result = append(result, reports...)
			}
		}
	}
	return result
}

// CollectTasksForCycle returns all task specs dispatched for a cycle.
func (m *Monitor) CollectTasksForCycle(cycleID string) []RSICTaskSpec {
	m.dispatcher.mu.RLock()
	defer m.dispatcher.mu.RUnlock()

	var result []RSICTaskSpec
	for _, at := range m.dispatcher.activeTasks {
		if at.Spec.CycleID == cycleID {
			result = append(result, at.Spec)
		}
	}
	return result
}
