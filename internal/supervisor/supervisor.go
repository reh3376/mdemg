package supervisor

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

// AlertFunc is called when a supervised worker restarts or fails permanently.
type AlertFunc func(service, title, message string, severity string)

// workerState tracks a single supervised goroutine.
type workerState struct {
	name     string
	fn       func(ctx context.Context) error
	restarts int
	running  bool
}

// Supervisor monitors and restarts critical background goroutines.
type Supervisor struct {
	mu       sync.Mutex
	workers  []*workerState
	alertFn  AlertFunc
	maxRetry int
	backoff  time.Duration // base delay, doubles each retry
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a supervisor with default settings.
func New(alertFn AlertFunc) *Supervisor {
	return &Supervisor{
		alertFn:  alertFn,
		maxRetry: 3,
		backoff:  5 * time.Second,
	}
}

// Register adds a named goroutine to supervision.
// The function should block until ctx is cancelled or an error occurs.
func (s *Supervisor) Register(name string, fn func(ctx context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = append(s.workers, &workerState{name: name, fn: fn})
}

// Start launches all registered workers and monitors them.
// This method blocks until Stop is called.
func (s *Supervisor) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	slog.Info("supervisor: started", "workers", len(s.workers))

	s.mu.Lock()
	workers := make([]*workerState, len(s.workers))
	copy(workers, s.workers)
	s.mu.Unlock()

	for _, w := range workers {
		s.wg.Add(1)
		go s.runWorker(w)
	}

	s.wg.Wait()
	slog.Info("supervisor: all workers stopped")
}

// Stop cancels all workers and waits for them to exit.
func (s *Supervisor) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// runWorker runs a single worker with panic recovery and restart logic.
func (s *Supervisor) runWorker(w *workerState) {
	defer s.wg.Done()

	for {
		err := s.runWithRecovery(w)

		// Check if context was cancelled (graceful shutdown)
		if s.ctx.Err() != nil {
			slog.Info("supervisor: worker stopped (shutdown)", "worker", w.name)
			return
		}

		s.mu.Lock()
		w.restarts++
		restarts := w.restarts
		maxRetry := s.maxRetry
		backoff := s.backoff
		s.mu.Unlock()

		if restarts > maxRetry {
			msg := fmt.Sprintf("%s failed permanently after %d restarts", w.name, maxRetry)
			slog.Error("supervisor: "+msg, "worker", w.name, "last_error", err)
			if s.alertFn != nil {
				s.alertFn("supervisor", "Worker failed permanently", msg, "critical")
			}
			return
		}

		delay := backoff * time.Duration(1<<(restarts-1)) // exponential backoff
		msg := fmt.Sprintf("%s restarted (attempt %d/%d)", w.name, restarts, maxRetry)
		slog.Warn("supervisor: "+msg, "worker", w.name, "error", err, "backoff", delay)
		if s.alertFn != nil {
			s.alertFn("supervisor", "Worker restarted", msg, "medium")
		}

		select {
		case <-time.After(delay):
			// continue to restart
		case <-s.ctx.Done():
			return
		}
	}
}

// runWithRecovery runs the worker function and recovers from panics.
func (s *Supervisor) runWithRecovery(w *workerState) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()

	s.mu.Lock()
	w.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		w.running = false
		s.mu.Unlock()
	}()

	return w.fn(s.ctx)
}

// WorkerCount returns the number of registered workers.
func (s *Supervisor) WorkerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.workers)
}
