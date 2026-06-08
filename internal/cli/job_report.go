package cli

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/alert"
	"mdemg/internal/config"
	"mdemg/internal/jobhealth"
	"mdemg/internal/tsdb"
)

// reportScheduledJob records a CLI scheduled-job outcome (maintenance,
// export-auto) to scheduled_job_events and fires a high-severity alert on
// failure — so a launchd/cron job that fails is no longer silent (NOSILENT-001).
//
// Best-effort + nil-safe: a TSDB or alert-config problem here never changes the
// job's own exit status. It opens its own short-lived TSDB pool (decoupled from
// the job's pool to avoid defer-ordering surprises) and builds a file-backed
// dispatcher from config — the same ~/.mdemg/alerts/current.json the
// session-start / prompt hooks surface, so the alert reaches the operator even
// though this is a separate process from the server.
func reportScheduledJob(jobName, spaceID string, startedAt time.Time, runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var pool *pgxpool.Pool
	if client, err := tsdb.NewClient(ctx, tsdbConfigFromEnv()); err == nil {
		pool = client.Pool()
		defer client.Close()
	}

	var disp *alert.Dispatcher
	instanceID := resolveInstanceID("", spaceID)
	if cfg, err := config.FromEnv(); err == nil {
		disp = alert.NewDispatcher(alert.Config{
			Enabled:           cfg.AlertEnabled,
			CooldownSec:       cfg.AlertCooldownSec,
			AlertFilePath:     cfg.AlertFilePath,
			MacOSNotify:       cfg.AlertMacOSNotify,
			MacOSNotifyMinSev: alert.Severity(cfg.AlertMacOSNotifyMinSev),
			MaxAlerts:         cfg.AlertMaxEntries,
		})
		if cfg.InstanceID != "" {
			instanceID = cfg.InstanceID
		}
	}

	ev := tsdb.JobEventRow{
		JobName:    jobName,
		SpaceID:    spaceID,
		InstanceID: instanceID,
		Success:    runErr == nil,
		LatencyMS:  time.Since(startedAt).Milliseconds(),
	}
	if runErr != nil {
		ev.ErrorMessage = runErr.Error()
	}
	jobhealth.Report(ctx, pool, disp, ev)

	// The dispatcher delivers alerts on a goroutine; this is a short-lived CLI
	// process, so give the file backend a moment to flush before we exit —
	// otherwise the alert never lands on disk. Only waits on the failure path.
	if disp != nil && runErr != nil {
		time.Sleep(750 * time.Millisecond)
	}
}
