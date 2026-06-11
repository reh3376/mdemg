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
	restarts []time.Time // restart timestamps inside the sliding window
	running  bool
	launched bool
}

// Supervisor monitors and restarts critical background goroutines.
//
// Restart budget is a sliding window: a worker fails permanently only when
// it restarts more than maxRetry times within window. Restarts older than
// the window are forgotten, so an occasional transient never accumulates
// into permanent death (SUPERVISOR-002).
type Supervisor struct {
	mu       sync.Mutex
	workers  []*workerState
	alertFn  AlertFunc
	maxRetry int
	window   time.Duration
	backoff  time.Duration // base delay, doubles per in-window restart
	now      func() time.Time
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

const (
	defaultMaxRetry = 3
	defaultWindow   = time.Hour
	defaultBackoff  = 5 * time.Second
	maxBackoffDelay = 2 * time.Minute
)

// New creates a supervisor with default settings.
func New(alertFn AlertFunc) *Supervisor {
	return &Supervisor{
		alertFn:  alertFn,
		maxRetry: defaultMaxRetry,
		window:   defaultWindow,
		backoff:  defaultBackoff,
		now:      time.Now,
	}
}

// Configure overrides the restart-budget settings. Zero values keep the
// current setting; negative values are rejected with a warning.
func (s *Supervisor) Configure(maxRetry int, window, backoff time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxRetry > 0 {
		s.maxRetry = maxRetry
	} else if maxRetry < 0 {
		slog.Warn("supervisor: ignoring negative max restarts", "value", maxRetry)
	}
	if window > 0 {
		s.window = window
	} else if window < 0 {
		slog.Warn("supervisor: ignoring negative restart window", "value", window)
	}
	if backoff > 0 {
		s.backoff = backoff
	} else if backoff < 0 {
		slog.Warn("supervisor: ignoring negative backoff", "value", backoff)
	}
}

// Register adds a named goroutine to supervision. Workers registered before
// Start are launched by Start; use Go to register and launch after Start.
// The function should block until ctx is cancelled or an error occurs.
// A nil return without ctx cancellation means intentional completion —
// the worker is NOT restarted.
func (s *Supervisor) Register(name string, fn func(ctx context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = append(s.workers, &workerState{name: name, fn: fn})
}

// Go registers a worker and, if the supervisor is already running, launches
// it immediately under supervision. Safe to call before Start (equivalent to
// Register) and after Stop (the worker is recorded but never launched).
func (s *Supervisor) Go(name string, fn func(ctx context.Context) error) {
	s.mu.Lock()
	w := &workerState{name: name, fn: fn}
	s.workers = append(s.workers, w)
	launch := s.ctx != nil && s.ctx.Err() == nil
	if launch {
		w.launched = true
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if launch {
		slog.Info("supervisor: worker registered (late)", "worker", name)
		go s.runWorker(w)
	}
}

// Start launches all registered workers and monitors them. It blocks until
// Stop is called (or ctx is cancelled), then waits for workers to drain.
// The supervisor stays alive even if individual workers fail permanently,
// so late workers added via Go remain supervised.
func (s *Supervisor) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	var launch []*workerState
	for _, w := range s.workers {
		if !w.launched {
			w.launched = true
			launch = append(launch, w)
		}
	}
	s.wg.Add(len(launch))
	s.mu.Unlock()

	slog.Info("supervisor: started", "workers", len(launch))

	for _, w := range launch {
		go s.runWorker(w)
	}

	<-s.ctx.Done()
	s.wg.Wait()
	slog.Info("supervisor: all workers stopped")
}

// Stop cancels all workers and waits for them to exit.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
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

		// Nil return without shutdown = intentional completion (e.g. the
		// worker's own stop channel closed). Do not restart.
		if err == nil {
			slog.Info("supervisor: worker completed", "worker", w.name)
			return
		}

		s.mu.Lock()
		now := s.now()
		cutoff := now.Add(-s.window)
		kept := w.restarts[:0]
		for _, ts := range w.restarts {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		w.restarts = append(kept, now)
		inWindow := len(w.restarts)
		maxRetry := s.maxRetry
		backoff := s.backoff
		window := s.window
		s.mu.Unlock()

		if inWindow > maxRetry {
			msg := fmt.Sprintf("%s failed permanently: %d restarts within %s", w.name, inWindow, window)
			slog.Error("supervisor: "+msg, "worker", w.name, "last_error", err)
			if s.alertFn != nil {
				s.alertFn("supervisor", "Worker failed permanently", msg, "critical")
			}
			return
		}

		// exponential backoff, capped
		delay := min(backoff*time.Duration(1<<(inWindow-1)), maxBackoffDelay)
		msg := fmt.Sprintf("%s restarted (%d/%d in window)", w.name, inWindow, maxRetry)
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
