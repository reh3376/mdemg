package backup

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler triggers periodic backups using simple tickers.
type Scheduler struct {
	svc    *Service
	stopCh chan struct{}
}

// NewScheduler creates a new backup scheduler.
func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{svc: svc, stopCh: make(chan struct{})}
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

	fullTicker := time.NewTicker(fullInterval)
	partialTicker := time.NewTicker(partialInterval)

	go func() {
		defer fullTicker.Stop()
		defer partialTicker.Stop()

		for {
			select {
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
				return
			}
		}
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
