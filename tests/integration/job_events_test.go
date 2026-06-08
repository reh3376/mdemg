//go:build integration

package integration

import (
	"context"
	"testing"

	"mdemg/internal/tsdb"
)

// TestJobEvents_RoundTrip (NOSILENT-001 Tier 2): RecordJobEvent writes a row to
// scheduled_job_events; the staleness + failure query shapes read it back.
func TestJobEvents_RoundTrip(t *testing.T) {
	client := newTestClient(t)
	pool := client.Pool()
	ctx := context.Background()

	jobName := "tsdb-backup-itest"
	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM scheduled_job_events WHERE job_name = $1`, jobName)
	}
	cleanup()
	t.Cleanup(cleanup)

	// Record a success then a failure.
	if err := tsdb.RecordJobEvent(ctx, pool, tsdb.JobEventRow{
		JobName: jobName, SpaceID: "mdemg-dev", InstanceID: "itest", Success: true,
		LatencyMS: 4200, Metadata: map[string]any{"size_bytes": 12345},
	}); err != nil {
		t.Fatalf("record success: %v", err)
	}
	if err := tsdb.RecordJobEvent(ctx, pool, tsdb.JobEventRow{
		JobName: jobName, Success: false, ErrorMessage: "docker not found",
	}); err != nil {
		t.Fatalf("record failure: %v", err)
	}

	// Read back: total rows for the job.
	var total int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_job_events WHERE job_name = $1`, jobName).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 rows, got %d", total)
	}

	// Staleness query shape: successful runs in the last hour (≥1 → fresh).
	var successes int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_job_events WHERE job_name = $1 AND success = true AND recorded_at > now() - interval '1 hour'`,
		jobName).Scan(&successes); err != nil {
		t.Fatalf("staleness query: %v", err)
	}
	if successes != 1 {
		t.Errorf("expected 1 recent success, got %d", successes)
	}

	// Failure query shape: failures in the last hour (>0 → alert).
	var failures int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_job_events WHERE job_name = $1 AND success = false AND recorded_at > now() - interval '1 hour'`,
		jobName).Scan(&failures); err != nil {
		t.Fatalf("failure query: %v", err)
	}
	if failures != 1 {
		t.Errorf("expected 1 recent failure, got %d", failures)
	}

	// Metadata + error round-tripped.
	var errMsg string
	if err := pool.QueryRow(ctx,
		`SELECT error_message FROM scheduled_job_events WHERE job_name = $1 AND success = false LIMIT 1`,
		jobName).Scan(&errMsg); err != nil {
		t.Fatalf("error_message read: %v", err)
	}
	if errMsg != "docker not found" {
		t.Errorf("error_message = %q", errMsg)
	}
}
