package backup

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler triggers periodic backups using simple tickers.
type Scheduler struct {
	svc       *Service
	stopCh    chan struct{}
	supervise func(name string, fn func(ctx context.Context) error) // SUPERVISOR-002
}

// NewScheduler creates a new backup scheduler.
func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{svc: svc, stopCh: make(chan struct{})}
}

// SetSupervise injects a supervised-goroutine launcher (SUPERVISOR-002).
// Must be called before Start; nil keeps legacy bare-goroutine behavior.
func (s *Scheduler) SetSupervise(fn func(name string, fn func(ctx context.Context) error)) {
	s.supervise = fn
}

// Start launches the scheduler goroutine.
func (s *Scheduler) Start() {
	fullInterval := time.Duration(s.svc.cfg.FullIntervalHours) * time.Hour
	partialInterval := time.Duration(s.svc.cfg.PartialIntervalHours) * time.Hour

	// Guard against zero intervals.
	if fullInterval <= 0 {
		fullInterval = 168 * time.Hour // weekly
	}
	if partialInterval <= 0 {
		partialInterval = 24 * time.Hour // daily
	}

	run := func(runCtx context.Context) error {
		fullTicker := time.NewTicker(fullInterval)
		partialTicker := time.NewTicker(partialInterval)
		defer fullTicker.Stop()
		defer partialTicker.Stop()

		for {
			select {
			case <-runCtx.Done():
				return nil
			case <-fullTicker.C:
				slog.Info("backup: scheduler: triggering full backup")
				if _, err := s.svc.Trigger(context.Background(), TriggerRequest{
					Type:  string(BackupTypeFull),
					Label: "scheduled-full",
				}); err != nil {
					slog.Error("backup: scheduler: full backup failed", "error", err)
				}

			case <-partialTicker.C:
				slog.Info("backup: scheduler: triggering partial backup")
				if _, err := s.svc.Trigger(context.Background(), TriggerRequest{
					Type:  string(BackupTypePartial),
					Label: "scheduled-partial",
				}); err != nil {
					slog.Error("backup: scheduler: partial backup failed", "error", err)
				}

			case <-s.stopCh:
				slog.Info("backup: scheduler stopped")
				return nil
			}
		}
	}
	if s.supervise != nil {
		s.supervise("neo4j-backup-scheduler", run)
		return
	}
	go func() {
		_ = run(context.Background())
	}()
}

// Stop signals the scheduler to stop.
func (s *Scheduler) Stop() {
	select {
	case <-s.stopCh:
		// already stopped
	default:
		close(s.stopCh)
	}
}
