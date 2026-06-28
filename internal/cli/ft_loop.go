package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"mdemg/internal/alert"
	"mdemg/internal/config"
	"mdemg/internal/jobhealth"
	"mdemg/internal/tsdb"
)

// ftLoopStages is the ordered set of recursive-retraining-loop stages a manual
// run walks through (FT-RECURSIVE-001 / SPEC §2). report-stage records each so
// a by-hand run is fully observable in scheduled_job_events.
var ftLoopStages = []string{"capture", "curate", "train", "benchmark", "gate", "promote"}

func isFtLoopStage(s string) bool {
	for _, st := range ftLoopStages {
		if st == s {
			return true
		}
	}
	return false
}

// newFtLoopCmd is the parent `ft-loop` command (FT-RECURSIVE-001 Phase 6a:
// observability only — no actuator, no autonomous operation).
func newFtLoopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ft-loop",
		Short: "FT recursive-retraining loop observability (FT-RECURSIVE-001)",
		Long: `Observability surface for the recursive-retraining loop's manual path.

Phase 6a ships report-stage only: it makes a by-hand retrain run fully
observable (every stage lands in scheduled_job_events; a failed stage fires a
high-severity alert under the distinct "ft-loop" service). The autonomous
actuator that calls these is a later phase (FT-RECURSIVE-002).`,
	}
	cmd.AddCommand(newFtLoopReportStageCmd())
	return cmd
}

func newFtLoopReportStageCmd() *cobra.Command {
	var (
		stage, status, cycleID, detail, spaceID string
		latencyMs                               int64
	)
	cmd := &cobra.Command{
		Use:   "report-stage",
		Short: "Record a manual retrain-stage outcome to scheduled_job_events (jobhealth)",
		Long: `Record the outcome of one manual retrain stage.

Stages: ` + strings.Join(ftLoopStages, " | ") + `

On --status failure a high-severity alert fires under the "ft-loop" service
(distinct cooldown key, so a stage failure never suppresses or is suppressed by
a backup/maintenance failure). Best-effort + nil-safe: a TSDB/alert problem
never changes this command's exit status.

Example:
  mdemg ft-loop report-stage --stage train --status success \
    --cycle-id ftc-2026-06-27 --latency-ms 783000 --detail "90 iters, val_loss 0.268"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stage = strings.ToLower(strings.TrimSpace(stage))
			if !isFtLoopStage(stage) {
				return fmt.Errorf("invalid --stage %q (one of: %s)", stage, strings.Join(ftLoopStages, ", "))
			}
			var runErr error
			switch strings.ToLower(strings.TrimSpace(status)) {
			case "success", "ok":
			case "failure", "fail", "error":
				m := strings.TrimSpace(detail)
				if m == "" {
					m = "stage reported failure (no detail)"
				}
				runErr = errors.New(m)
			default:
				return fmt.Errorf("invalid --status %q (success|failure)", status)
			}
			if spaceID == "" {
				spaceID = os.Getenv("MDEMG_SPACE_ID")
			}
			if spaceID == "" {
				spaceID = "mdemg-dev"
			}
			reportFtLoopStage(stage, spaceID, cycleID, detail, latencyMs, runErr)
			outcome := "success"
			if runErr != nil {
				outcome = "failure"
			}
			fmt.Printf("ft-loop:%s recorded (status=%s, cycle=%s, space=%s)\n", stage, outcome, cycleID, spaceID)
			return nil
		},
	}
	cmd.Flags().StringVar(&stage, "stage", "", "stage: "+strings.Join(ftLoopStages, "|"))
	cmd.Flags().StringVar(&status, "status", "", "success|failure")
	cmd.Flags().StringVar(&cycleID, "cycle-id", "", "optional cycle id grouping a run's stages")
	cmd.Flags().StringVar(&detail, "detail", "", "optional human/error detail (becomes the alert message on failure)")
	cmd.Flags().Int64Var(&latencyMs, "latency-ms", 0, "optional stage wall-clock in ms")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "space id (default $MDEMG_SPACE_ID or mdemg-dev)")
	_ = cmd.MarkFlagRequired("stage")
	_ = cmd.MarkFlagRequired("status")
	return cmd
}

// reportFtLoopStage records the stage outcome to scheduled_job_events
// (job_name=ft-loop:<stage>) and, on failure, alerts under the distinct
// "ft-loop" service — reusing the NOSILENT-001 short-lived-pool +
// file-backed-dispatcher CLI-job pattern (this is a separate process from the
// server, so the alert reaches the same ~/.mdemg/alerts/current.json the hooks
// surface).
func reportFtLoopStage(stage, spaceID, cycleID, detail string, latencyMs int64, runErr error) {
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

	meta := map[string]any{"stage": stage}
	if cycleID != "" {
		meta["cycle_id"] = cycleID
	}
	if detail != "" {
		meta["detail"] = detail
	}
	ev := tsdb.JobEventRow{
		JobName:    "ft-loop:" + stage,
		SpaceID:    spaceID,
		InstanceID: instanceID,
		Success:    runErr == nil,
		LatencyMS:  latencyMs,
		Metadata:   meta,
	}
	if runErr != nil {
		ev.ErrorMessage = runErr.Error()
	}
	jobhealth.ReportWithService(ctx, pool, disp, ev, "ft-loop")

	// Short-lived process: let the file backend flush the failure alert.
	if disp != nil && runErr != nil {
		time.Sleep(750 * time.Millisecond)
	}
}
