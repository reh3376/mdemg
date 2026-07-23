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
	"mdemg/internal/ftloop"
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
	cmd.AddCommand(newFtLoopPromoteCmd())
	return cmd
}

func newFtLoopPromoteCmd() *cobra.Command {
	var (
		cycleID string
		reject  bool
		reason  string
	)
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Operator-confirm (or reject) a promote_pending retrain cycle",
		Long: `Confirm or reject a cycle the controller left at promote_pending.

The controller halts every cycle at promote_pending — promotion is operator-
gated in Phase 6b (auto-promote + canary are Phase 7). --confirm records the
cycle promoted; --reject records it rolled_back (with --reason). The actual
GGUF symlink swap + llama-server restart is performed in the live promotion
flow; this records the operator decision in the ft_training_cycles ledger.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cycleID == "" {
				return fmt.Errorf("--cycle-id is required")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			client, err := tsdb.NewClient(ctx, tsdbConfigFromEnv())
			if err != nil {
				return fmt.Errorf("connect TSDB: %w", err)
			}
			defer client.Close()
			pool := client.Pool()

			status, found, err := tsdb.CycleStatus(ctx, pool, cycleID)
			if err != nil {
				return fmt.Errorf("query cycle: %w", err)
			}
			if !found {
				return fmt.Errorf("cycle %q not found in the ledger", cycleID)
			}
			if status != tsdb.FtCyclePromotePending {
				return fmt.Errorf("cycle %q is %q, not promote_pending — nothing to confirm/reject", cycleID, status)
			}

			modelVersion := strings.TrimSpace(os.Getenv("FT_LOOP_MODEL_VERSION"))
			if modelVersion == "" {
				modelVersion = "mdemg-llm-v1"
			}
			if reject {
				ev := tsdb.FtCycleEvent{
					CycleID: cycleID, ModelVersion: modelVersion,
					Status: tsdb.FtCycleRolledBack, Stage: "operator_reject", Error: reason,
					EvalResults: map[string]any{"operator_decision": "reject", "reason": reason},
				}
				if err := tsdb.RecordCycleEvent(ctx, pool, ev); err != nil {
					return fmt.Errorf("record decision: %w", err)
				}
				fmt.Printf("cycle %s rejected (rolled_back). reason: %s\n", cycleID, reason)
				return nil
			}

			// FT-RECURSIVE-003 E3/E6: --confirm runs the ONE promotion flow
			// (canary → fail-closed swap → ledger + version records), shared
			// with the controller's auto-promote path.
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			repoDir, _ := os.Getwd()
			pc := promotionConfigFromEnv(cfg, repoDir)
			res, perr := ftloop.PromoteCycle(context.Background(), pool, pc, cycleID, reason, "operator")
			if perr != nil {
				return perr
			}
			fmt.Printf("cycle %s PROMOTED and serving: %s (previous %s — 'mdemg model rollback --yes' to revert)\n",
				cycleID, res.Target, res.Previous)
			return nil
		},
	}
	cmd.Flags().StringVar(&cycleID, "cycle-id", "", "the promote_pending cycle to act on")
	cmd.Flags().BoolVar(&reject, "reject", false, "reject the cycle (rolled_back) instead of confirming")
	cmd.Flags().StringVar(&reason, "reason", "", "operator reasoning (recorded; required-ish for --reject)")
	_ = cmd.MarkFlagRequired("cycle-id")
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
