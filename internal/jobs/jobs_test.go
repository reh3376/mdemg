package jobs

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestJobStatus_Constants(t *testing.T) {
	tests := []struct {
		status JobStatus
		want   string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("status = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestJob_UpdateProgress(t *testing.T) {
	j := &Job{Progress: JobProgress{Total: 100}}
	j.UpdateProgress(50, "processing")

	if j.Progress.Current != 50 {
		t.Errorf("Current = %d, want 50", j.Progress.Current)
	}
	if j.Progress.Phase != "processing" {
		t.Errorf("Phase = %q, want %q", j.Progress.Phase, "processing")
	}
	if j.Progress.Percentage != 50.0 {
		t.Errorf("Percentage = %f, want 50.0", j.Progress.Percentage)
	}
	if j.Progress.LastUpdated == "" {
		t.Error("LastUpdated should be set")
	}
}

func TestJob_UpdateProgress_ZeroTotal(t *testing.T) {
	j := &Job{Progress: JobProgress{Total: 0}}
	j.UpdateProgress(10, "unknown")

	if j.Progress.Percentage != 0 {
		t.Errorf("Percentage with zero total = %f, want 0", j.Progress.Percentage)
	}
}

func TestJob_SetTotal(t *testing.T) {
	j := &Job{}
	j.SetTotal(200)

	if j.Progress.Total != 200 {
		t.Errorf("Total = %d, want 200", j.Progress.Total)
	}
}

func TestJob_SetRate(t *testing.T) {
	j := &Job{}
	j.SetRate("42 items/sec")

	if j.Progress.Rate != "42 items/sec" {
		t.Errorf("Rate = %q, want %q", j.Progress.Rate, "42 items/sec")
	}
}

func TestJob_Complete(t *testing.T) {
	j := &Job{Status: StatusRunning}
	result := map[string]any{"count": 42}
	j.Complete(result)

	if j.Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", j.Status, StatusCompleted)
	}
	if j.Result["count"] != 42 {
		t.Errorf("Result[count] = %v, want 42", j.Result["count"])
	}
	if j.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestJob_Fail(t *testing.T) {
	j := &Job{Status: StatusRunning}
	j.Fail(errors.New("disk full"))

	if j.Status != StatusFailed {
		t.Errorf("Status = %q, want %q", j.Status, StatusFailed)
	}
	if j.Error != "disk full" {
		t.Errorf("Error = %q, want %q", j.Error, "disk full")
	}
	if j.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestJob_Cancel(t *testing.T) {
	cancelled := false
	j := &Job{
		Status: StatusRunning,
		cancel: func() { cancelled = true },
	}
	j.Cancel()

	if j.Status != StatusCancelled {
		t.Errorf("Status = %q, want %q", j.Status, StatusCancelled)
	}
	if j.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if !cancelled {
		t.Error("cancel func should have been called")
	}
}

func TestJob_Cancel_NilCancelFunc(t *testing.T) {
	j := &Job{Status: StatusRunning}
	j.Cancel() // should not panic
	if j.Status != StatusCancelled {
		t.Errorf("Status = %q, want %q", j.Status, StatusCancelled)
	}
}

func TestJob_GetSnapshot(t *testing.T) {
	now := time.Now().UTC()
	j := &Job{
		ID:        "test-1",
		Type:      "backup",
		Status:    StatusRunning,
		StartedAt: &now,
		CreatedAt: now,
		Progress:  JobProgress{Total: 100, Current: 50},
	}

	snap := j.GetSnapshot()
	if snap.ID != "test-1" {
		t.Errorf("snapshot ID = %q, want %q", snap.ID, "test-1")
	}
	if snap.Status != StatusRunning {
		t.Errorf("snapshot Status = %q, want %q", snap.Status, StatusRunning)
	}
	if snap.Progress.Total != 100 {
		t.Errorf("snapshot Progress.Total = %d, want 100", snap.Progress.Total)
	}
}

func TestNewQueue_Defaults(t *testing.T) {
	q := NewQueue(0, 0)
	if q.maxJobs != 100 {
		t.Errorf("maxJobs = %d, want 100", q.maxJobs)
	}
	if q.ttlMinutes != 60 {
		t.Errorf("ttlMinutes = %d, want 60", q.ttlMinutes)
	}
}

func TestNewQueue_CustomValues(t *testing.T) {
	q := NewQueue(50, 30)
	if q.maxJobs != 50 {
		t.Errorf("maxJobs = %d, want 50", q.maxJobs)
	}
	if q.ttlMinutes != 30 {
		t.Errorf("ttlMinutes = %d, want 30", q.ttlMinutes)
	}
}

func TestQueue_CreateJob(t *testing.T) {
	q := NewQueue(10, 5)
	config := map[string]any{"path": "/tmp"}
	job, ctx := q.CreateJob("j1", "ingest", config)

	if job.ID != "j1" {
		t.Errorf("job ID = %q, want %q", job.ID, "j1")
	}
	if job.Type != "ingest" {
		t.Errorf("job Type = %q, want %q", job.Type, "ingest")
	}
	if job.Status != StatusPending {
		t.Errorf("job Status = %q, want %q", job.Status, StatusPending)
	}
	if ctx == nil {
		t.Error("context should not be nil")
	}

	// Verify job is in queue
	got, ok := q.GetJob("j1")
	if !ok {
		t.Fatal("job should be in queue")
	}
	if got.ID != "j1" {
		t.Errorf("GetJob ID = %q, want %q", got.ID, "j1")
	}
}

func TestQueue_StartJob(t *testing.T) {
	q := NewQueue(10, 5)
	q.CreateJob("j1", "backup", nil)
	q.StartJob("j1")

	job, _ := q.GetJob("j1")
	if job.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", job.Status, StatusRunning)
	}
	if job.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
}

func TestQueue_StartJob_NonExistent(t *testing.T) {
	q := NewQueue(10, 5)
	q.StartJob("nonexistent") // should not panic
}

func TestQueue_GetJob_NotFound(t *testing.T) {
	q := NewQueue(10, 5)
	_, ok := q.GetJob("nonexistent")
	if ok {
		t.Error("should return false for nonexistent job")
	}
}

func TestQueue_ListJobs_All(t *testing.T) {
	q := NewQueue(10, 5)
	q.CreateJob("j1", "backup", nil)
	q.CreateJob("j2", "ingest", nil)
	q.CreateJob("j3", "backup", nil)

	all := q.ListJobs("")
	if len(all) != 3 {
		t.Errorf("ListJobs(\"\") returned %d jobs, want 3", len(all))
	}
}

func TestQueue_ListJobs_ByType(t *testing.T) {
	q := NewQueue(10, 5)
	q.CreateJob("j1", "backup", nil)
	q.CreateJob("j2", "ingest", nil)
	q.CreateJob("j3", "backup", nil)

	backups := q.ListJobs("backup")
	if len(backups) != 2 {
		t.Errorf("ListJobs(\"backup\") returned %d jobs, want 2", len(backups))
	}
	for i := range backups {
		if backups[i].Type != "backup" {
			t.Errorf("expected type 'backup', got %q", backups[i].Type)
		}
	}
}

func TestQueue_CancelJob(t *testing.T) {
	q := NewQueue(10, 5)
	q.CreateJob("j1", "backup", nil)

	ok := q.CancelJob("j1")
	if !ok {
		t.Error("CancelJob should return true for pending job")
	}
	job, _ := q.GetJob("j1")
	if job.Status != StatusCancelled {
		t.Errorf("Status = %q, want %q", job.Status, StatusCancelled)
	}
}

func TestQueue_CancelJob_Running(t *testing.T) {
	q := NewQueue(10, 5)
	q.CreateJob("j1", "backup", nil)
	q.StartJob("j1")

	ok := q.CancelJob("j1")
	if !ok {
		t.Error("CancelJob should return true for running job")
	}
}

func TestQueue_CancelJob_AlreadyCompleted(t *testing.T) {
	q := NewQueue(10, 5)
	job, _ := q.CreateJob("j1", "backup", nil)
	job.Complete(nil)

	ok := q.CancelJob("j1")
	if ok {
		t.Error("CancelJob should return false for completed job")
	}
}

func TestQueue_CancelJob_NonExistent(t *testing.T) {
	q := NewQueue(10, 5)
	ok := q.CancelJob("nonexistent")
	if ok {
		t.Error("CancelJob should return false for nonexistent job")
	}
}

func TestQueue_Cleanup(t *testing.T) {
	q := NewQueue(10, 1) // 1 minute TTL
	job, _ := q.CreateJob("j1", "backup", nil)

	// Complete the job with a past timestamp
	job.mu.Lock()
	job.Status = StatusCompleted
	past := time.Now().Add(-2 * time.Minute)
	job.CompletedAt = &past
	job.mu.Unlock()

	q.cleanup()

	_, ok := q.GetJob("j1")
	if ok {
		t.Error("completed job older than TTL should be cleaned up")
	}
}

func TestQueue_Cleanup_RecentJobKept(t *testing.T) {
	q := NewQueue(10, 60) // 60 minute TTL
	job, _ := q.CreateJob("j1", "backup", nil)
	job.Complete(nil)

	q.cleanup()

	_, ok := q.GetJob("j1")
	if !ok {
		t.Error("recently completed job should not be cleaned up")
	}
}

func TestQueue_Cleanup_RunningJobKept(t *testing.T) {
	q := NewQueue(10, 1)
	q.CreateJob("j1", "backup", nil)
	q.StartJob("j1")

	q.cleanup()

	_, ok := q.GetJob("j1")
	if !ok {
		t.Error("running job should not be cleaned up regardless of TTL")
	}
}

func TestJob_ConcurrentAccess(t *testing.T) {
	j := &Job{Progress: JobProgress{Total: 1000}}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			j.UpdateProgress(n, "processing")
			j.SetRate("fast")
			_ = j.GetSnapshot()
		}(i)
	}
	wg.Wait()

	if j.Progress.Total != 1000 {
		t.Error("Total should remain 1000 after concurrent access")
	}
}

func TestQueue_ConcurrentCreateAndGet(t *testing.T) {
	q := NewQueue(1000, 60)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "job-" + time.Now().Format("150405.000000000") + "-" + string(rune('A'+n%26))
			q.CreateJob(id, "test", nil)
			q.ListJobs("")
			q.GetJob(id)
		}(i)
	}
	wg.Wait()
}
