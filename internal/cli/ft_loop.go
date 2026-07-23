package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

			// FT-RECURSIVE-003 E3: --confirm performs the REAL promotion —
			// fail-closed serving swap to the cycle's candidate, then the
			// ledger + ft_model_versions records. A failed swap auto-restores
			// serving (SwapServing) and records the cycle rolled_back.
			candidate, gateScore, err := tsdb.CycleCandidatePath(ctx, pool, cycleID)
			if err != nil {
				return fmt.Errorf("read candidate path: %w", err)
			}
			if candidate == "" {
				return fmt.Errorf("cycle %s has no candidate_gguf recorded — pre-E3 cycle; promote manually via 'mdemg model swap'", cycleID)
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			repoDir, _ := os.Getwd()
			sc := servingConfigFromEnv(cfg, repoDir)

			// FT-RECURSIVE-003 E4: pre-swap canary — held-call replay against
			// the candidate on the gate side-port. Divergence blocks promotion
			// WITHOUT touching production serving (strictly better than the
			// swap-then-revert path: zero production restarts on a bad
			// candidate that fails structurally).
			if cfg.FtLoopCanaryEnabled {
				canCtx, cancelCan := context.WithTimeout(context.Background(), 20*time.Minute)
				stop, serr := ftloop.StartCandidateServer(canCtx, repoDir, candidate, cfg.FtLoopGatePort)
				if serr != nil {
					cancelCan()
					recFail := func(reason string) {
						rctx, rcancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer rcancel()
						_ = tsdb.RecordCycleEvent(rctx, pool, tsdb.FtCycleEvent{
							CycleID: cycleID, ModelVersion: modelVersion,
							Status: tsdb.FtCycleRolledBack, Stage: "canary_failed", Error: reason,
							EvalResults: map[string]any{"operator_decision": "confirm"},
						})
					}
					recFail("candidate would not serve: " + serr.Error())
					return fmt.Errorf("canary: candidate would not serve (production untouched): %w", serr)
				}
				probes := cfg.FtLoopCanaryProbes
				if !filepath.IsAbs(probes) {
					probes = filepath.Join(repoDir, probes)
				}
				canRes, cerr := ftloop.RunCanary(canCtx, ftloop.CanaryConfig{
					ProbesPath:  probes,
					ProbeCount:  cfg.FtLoopCanaryProbeCount,
					ProdBaseURL: cfg.FtLoopCanaryProdURL,
					CandBaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", cfg.FtLoopGatePort),
				})
				stop()
				cancelCan()
				if cerr != nil {
					return fmt.Errorf("canary replay failed (promotion aborted, production untouched): %w", cerr)
				}
				if !canRes.Pass() {
					rctx, rcancel := context.WithTimeout(context.Background(), 30*time.Second)
					_ = tsdb.RecordCycleEvent(rctx, pool, tsdb.FtCycleEvent{
						CycleID: cycleID, ModelVersion: modelVersion,
						Status: tsdb.FtCycleRolledBack, Stage: "canary_failed",
						Error: strings.Join(canRes.Divergences, "; "),
						EvalResults: map[string]any{"operator_decision": "confirm", "canary_probes": canRes.Probes},
					})
					rcancel()
					return fmt.Errorf("canary DIVERGED (%d/%d probes; production untouched): %s",
						len(canRes.Divergences), canRes.Probes, strings.Join(canRes.Divergences, "; "))
				}
				fmt.Printf("canary passed: %d probes, 0 divergences\n", canRes.Probes)
			}

			swapCtx, cancelSwap := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancelSwap()
			res, swapErr := ftloop.SwapServing(swapCtx, sc, candidate)
			// Drill-caught (FT-RECURSIVE-003 E3): the outer command ctx (15s)
			// is long dead after a multi-minute swap — post-swap ledger and
			// version writes get their own fresh context.
			recCtx, cancelRec := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelRec()
			if swapErr != nil {
				_ = tsdb.RecordCycleEvent(recCtx, pool, tsdb.FtCycleEvent{
					CycleID: cycleID, ModelVersion: modelVersion,
					Status: tsdb.FtCycleRolledBack, Stage: "promote_failed", Error: swapErr.Error(),
					EvalResults: map[string]any{"operator_decision": "confirm", "swap_reverted": res.Reverted},
				})
				return fmt.Errorf("promotion swap failed (serving restored=%v): %w", res.Reverted, swapErr)
			}

			ev := tsdb.FtCycleEvent{
				CycleID: cycleID, ModelVersion: modelVersion,
				Status: tsdb.FtCyclePromoted, Stage: "operator_confirm",
				EvalResults: map[string]any{
					"operator_decision": "confirm", "reason": reason,
					"swap_previous": res.Previous, "swap_target": res.Target,
				},
			}
			if err := tsdb.RecordCycleEvent(recCtx, pool, ev); err != nil {
				return fmt.Errorf("record decision (swap ALREADY performed): %w", err)
			}

			if active, aerr := tsdb.ActiveModelVersion(recCtx, pool); aerr == nil && active != nil && active.ModelPath != res.Target {
				_ = tsdb.MarkModelVersionStatus(recCtx, pool, active.Version, tsdb.ModelVersionSuperseded)
			}
			shortCycle := cycleID
			if len(shortCycle) > 8 {
				shortCycle = shortCycle[:8]
			}
			if err := tsdb.RecordModelVersion(recCtx, pool, tsdb.ModelVersionRow{
				Version: modelVersion + "-" + shortCycle, ModelPath: res.Target,
				BaseModel: cfg.FtLoopBaseModel, TrainingCycle: cycleID,
				OverallScore: gateScore, Status: tsdb.ModelVersionActive,
				Notes: "promoted via ft-loop promote --confirm",
			}); err != nil {
				fmt.Printf("WARN: ft_model_versions record failed: %v\n", err)
			}
			fmt.Printf("cycle %s PROMOTED and serving: %s (gate %.4f; previous %s)\n",
				cycleID, res.Target, gateScore, res.Previous)
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
