// Package jobs provides a lightweight background job queue for long-running operations.
package jobs

import (
	"context"
	"sync"
	"time"
)

// JobStatus represents the current state of a job.
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

// Job represents a background job with progress tracking.
type Job struct {
	ID          string            `json:"job_id"`
	Type        string            `json:"type"`
	Status      JobStatus         `json:"status"`
	Progress    JobProgress       `json:"progress"`
	Config      map[string]any    `json:"config,omitempty"`
	Result      map[string]any    `json:"result,omitempty"`
	Error       string            `json:"error,omitempty"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	mu          sync.RWMutex
	cancel      context.CancelFunc
}

// JobProgress tracks the progress of a job.
type JobProgress struct {
	Total       int     `json:"total"`
	Current     int     `json:"current"`
	Percentage  float64 `json:"percentage"`
	Phase       string  `json:"phase,omitempty"`
	Rate        string  `json:"rate,omitempty"`
	LastUpdated string  `json:"last_updated,omitempty"`
}

// UpdateProgress safely updates job progress.
func (j *Job) UpdateProgress(current int, phase string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress.Current = current
	j.Progress.Phase = phase
	if j.Progress.Total > 0 {
		j.Progress.Percentage = float64(current) / float64(j.Progress.Total) * 100
	}
	j.Progress.LastUpdated = time.Now().UTC().Format(time.RFC3339)
}

// SetTotal sets the total items to process.
func (j *Job) SetTotal(total int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress.Total = total
}

// SetRate sets the processing rate string.
func (j *Job) SetRate(rate string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress.Rate = rate
}

// Complete marks the job as completed with results.
func (j *Job) Complete(result map[string]any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = StatusCompleted
	j.Result = result
	now := time.Now().UTC()
	j.CompletedAt = &now
}

// Fail marks the job as failed with an error message.
func (j *Job) Fail(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = StatusFailed
	j.Error = err.Error()
	now := time.Now().UTC()
	j.CompletedAt = &now
}

// Snapshot returns a read-safe copy of the job's mutable fields.
// Use this when reading from a goroutine that doesn't hold the lock.
type JobSnapshot struct {
	Status   JobStatus
	Progress JobProgress
	Result   map[string]any
	Error    string
}

func (j *Job) Snapshot() JobSnapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	// Deep copy Result map to avoid concurrent map read/write
	var result map[string]any
	if j.Result != nil {
		result = make(map[string]any, len(j.Result))
		for k, v := range j.Result {
			result[k] = v
		}
	}
	return JobSnapshot{
		Status:   j.Status,
		Progress: j.Progress,
		Result:   result,
		Error:    j.Error,
	}
}

// Cancel cancels the job.
func (j *Job) Cancel() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = StatusCancelled
	now := time.Now().UTC()
	j.CompletedAt = &now
	if j.cancel != nil {
		j.cancel()
	}
}

// GetSnapshot returns a thread-safe copy of the job.
func (j *Job) GetSnapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:          j.ID,
		Type:        j.Type,
		Status:      j.Status,
		Progress:    j.Progress,
		Config:      j.Config,
		Result:      j.Result,
		Error:       j.Error,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
		CreatedAt:   j.CreatedAt,
	}
}

// Queue manages background jobs with FIFO processing and status tracking.
type Queue struct {
	jobs       map[string]*Job
	mu         sync.RWMutex
	maxJobs    int
	ttlMinutes int
}

// NewQueue creates a new job queue with configurable limits.
func NewQueue(maxJobs, ttlMinutes int) *Queue {
	if maxJobs <= 0 {
		maxJobs = 100
	}
	if ttlMinutes <= 0 {
		ttlMinutes = 60
	}
	q := &Queue{
		jobs:       make(map[string]*Job),
		maxJobs:    maxJobs,
		ttlMinutes: ttlMinutes,
	}
	// Start background cleanup
	go q.cleanupLoop()
	return q
}

// CreateJob creates a new job and adds it to the queue.
func (q *Queue) CreateJob(id, jobType string, config map[string]any) (*Job, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())

	now := time.Now().UTC()
	job := &Job{
		ID:        id,
		Type:      jobType,
		Status:    StatusPending,
		Config:    config,
		CreatedAt: now,
		cancel:    cancel,
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[id] = job
	return job, ctx
}

// StartJob marks a job as running.
func (q *Queue) StartJob(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, ok := q.jobs[id]; ok {
		job.mu.Lock()
		job.Status = StatusRunning
		now := time.Now().UTC()
		job.StartedAt = &now
		job.mu.Unlock()
	}
}

// GetJob returns a job by ID.
func (q *Queue) GetJob(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	job, ok := q.jobs[id]
	return job, ok
}

// ListJobs returns all jobs of a given type (or all if type is empty).
func (q *Queue) ListJobs(jobType string) []Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var result []Job
	for _, job := range q.jobs {
		if jobType == "" || job.Type == jobType {
			result = append(result, job.GetSnapshot())
		}
	}
	return result
}

// CancelJob cancels a running job.
func (q *Queue) CancelJob(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job, ok := q.jobs[id]; ok {
		if job.Status == StatusPending || job.Status == StatusRunning {
			job.Cancel()
			return true
		}
	}
	return false
}

// cleanupLoop removes completed jobs older than TTL.
func (q *Queue) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		q.cleanup()
	}
}

func (q *Queue) cleanup() {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(q.ttlMinutes) * time.Minute)
	for id, job := range q.jobs {
		job.mu.RLock()
		shouldDelete := (job.Status == StatusCompleted || job.Status == StatusFailed || job.Status == StatusCancelled) &&
			job.CompletedAt != nil && job.CompletedAt.Before(cutoff)
		job.mu.RUnlock()
		if shouldDelete {
			delete(q.jobs, id)
		}
	}
}

// Global job queue singleton
var globalQueue *Queue
var queueOnce sync.Once

// GetQueue returns the global job queue.
func GetQueue() *Queue {
	queueOnce.Do(func() {
		globalQueue = NewQueue(100, 60)
	})
	return globalQueue
}
