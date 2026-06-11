package backup

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// JobResultFunc is an optional post-run hook invoked after each scheduled
// backup completes (BACKUP-RESTORE-VERIFY-001 — jobhealth reporting without
// coupling internal/backup to internal/alert or internal/tsdb).
type JobResultFunc func(success bool, latencyMS int64, err error)

// Scheduler triggers periodic backups using simple tickers.
type Scheduler struct {
	svc       *Service
	stopCh    chan struct{}
	supervise func(name string, fn func(ctx context.Context) error) // SUPERVISOR-002
	hookMu    sync.Mutex
	onResult  JobResultFunc
}

// NewScheduler creates a new backup scheduler.
func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{svc: svc, stopCh: make(chan struct{})}
}

// SetResultHook wires the scheduled-run outcome hook (jobhealth).
func (s *Scheduler) SetResultHook(fn JobResultFunc) {
	s.hookMu.Lock()
	s.onResult = fn
	s.hookMu.Unlock()
}

func (s *Scheduler) resultHook() JobResultFunc {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	return s.onResult
}

// runScheduled triggers one backup, waits for the async job to finish, and
// reports the outcome. The Neo4j backup Trigger is queue-async (unlike the
// TSDB scheduler's synchronous Trigger), so success can only be observed by
// waiting on the job — a fire-and-forget report would always claim success.
func (s *Scheduler) runScheduled(backupType, label string) {
	start := time.Now()
	backupID, err := s.svc.Trigger(context.Background(), TriggerRequest{
		Type:  backupType,
		Label: label,
	})
	if err == nil {
		err = s.svc.waitForBackupJob(context.Background(), backupID)
	}
	if err != nil {
		slog.Error("backup: scheduler: scheduled backup failed", "type", backupType, "error", err)
	} else {
		slog.Info("backup: scheduler: scheduled backup completed", "type", backupType, "backup_id", backupID)
	}
	if hook := s.resultHook(); hook != nil {
		hook(err == nil, time.Since(start).Milliseconds(), err)
	}
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
		// BACKUP-RESTORE-VERIFY-001: an initial backup shortly after start.
		// Without it, a fresh install has no backup (and an honest
		// neo4j_backup_no_recent_success alert) until the first 24h tick.
		if delay := time.Duration(s.svc.cfg.InitialBackupDelayMin) * time.Minute; delay > 0 {
			select {
			case <-runCtx.Done():
				return nil
			case <-s.stopCh:
				slog.Info("backup: scheduler stopped")
				return nil
			case <-time.After(delay):
				slog.Info("backup: scheduler: running initial backup on start")
				s.runScheduled(string(BackupTypePartial), "initial-on-start")
			}
		}

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
				s.runScheduled(string(BackupTypeFull), "scheduled-full")

			case <-partialTicker.C:
				slog.Info("backup: scheduler: triggering partial backup")
				s.runScheduled(string(BackupTypePartial), "scheduled-partial")

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
