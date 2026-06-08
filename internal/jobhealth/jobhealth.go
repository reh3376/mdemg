// Package jobhealth is the single policy point for "a scheduled job ran":
// record the outcome to scheduled_job_events (V0024) and, on failure, raise a
// high-severity alert. Both the TSDB pool and the alert dispatcher are
// nil-safe, so callers in degraded environments (TSDB down, alerts disabled)
// still behave sanely.
//
// Why a separate package: it imports both internal/tsdb (the writer) and
// internal/alert (the dispatcher). internal/tsdb must NOT import internal/alert
// (the backup scheduler stays decoupled via a plain callback hook), so the
// glue lives here, above both.
package jobhealth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/alert"
	"mdemg/internal/tsdb"
)

// Report records a scheduled-job run and fires a high-severity alert when it
// failed. pool nil → skip the TSDB record (still alerts). disp nil → skip the
// alert (still records). This is the only place the record+alert policy lives,
// so all three jobs (tsdb-backup, maintenance, export-auto) behave identically.
func Report(ctx context.Context, pool *pgxpool.Pool, disp *alert.Dispatcher, ev tsdb.JobEventRow) {
	if pool != nil {
		// Best-effort; RecordJobEvent already logs on failure.
		_ = tsdb.RecordJobEvent(ctx, pool, ev)
	}
	if ev.Success || disp == nil {
		return
	}
	msg := ev.ErrorMessage
	if msg == "" {
		msg = "job failed with no error detail (see logs)"
	}
	disp.SendAlert(ctx, "scheduled-job",
		fmt.Sprintf("Scheduled job failed: %s", ev.JobName),
		fmt.Sprintf("%s (space=%s): %s", ev.JobName, ev.SpaceID, msg),
		alert.SeverityHigh)
}
