package ape

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"mdemg/internal/config"
	"mdemg/internal/metrics"
	"mdemg/internal/tsdb"
)

// RunCycleOpts carries optional parameters for RunCycle.
type RunCycleOpts struct {
	TriggerMeta    *TriggerMetadata
	IdempotencyKey string
	DryRun         bool
}

// HistoryFilter constrains which cycle outcomes are returned.
type HistoryFilter struct {
	TriggerSource TriggerSource
	Tier          CycleTier
	SpaceID       string
}

// CycleOrchestrator runs the full Assess → Reflect → Plan → Execute → Validate cycle.
type CycleOrchestrator struct {
	assessor   *Assessor
	reflector  *Reflector
	planner    *Planner
	dispatcher *Dispatcher
	monitor    *Monitor
	calibrator *Calibrator
	watchdog         *Watchdog
	snapshotStore    *SnapshotStore
	cfg              config.Config
	policy           *OrchestrationPolicy
	tierEffProvider  TierEffectivenessProvider // NLI feedback loop: tier effectiveness dataset builder
}

// NewCycleOrchestrator wires together all RSIC components.
func NewCycleOrchestrator(
	cfg config.Config,
	assessor *Assessor,
	reflector *Reflector,
	planner *Planner,
	dispatcher *Dispatcher,
	monitor *Monitor,
	calibrator *Calibrator,
	watchdog *Watchdog,
) *CycleOrchestrator {
	return &CycleOrchestrator{
		assessor:   assessor,
		reflector:  reflector,
		planner:    planner,
		dispatcher: dispatcher,
		monitor:    monitor,
		calibrator: calibrator,
		watchdog:   watchdog,
		cfg:        cfg,
	}
}

// RunCycle executes the full 5-stage RSIC cycle for the given space and tier.
// opts may be nil for backward compatibility (defaults to manual_api source).
func (c *CycleOrchestrator) RunCycle(ctx context.Context, spaceID string, tier CycleTier, opts *RunCycleOpts) (*CycleOutcome, error) {
	cycleID := fmt.Sprintf("rsic-%s-%s", tier, uuid.New().String()[:8])
	startedAt := time.Now()

	// Build trigger metadata (default to manual_api)
	var meta TriggerMetadata
	if opts != nil && opts.TriggerMeta != nil {
		meta = *opts.TriggerMeta
	} else {
		meta = TriggerMetadata{
			TriggerSource: TriggerManualAPI,
			TriggerID:     fmt.Sprintf("manual_api:%s:%s", spaceID, startedAt.Format("2006-01-02T15:04")),
			TriggeredAt:   startedAt,
			PolicyVersion: PolicyVersion,
		}
	}

	// Phase 88: Determine dry-run mode early (needed for all return paths)
	isDryRun := opts != nil && opts.DryRun

	// Phase 90: Extract idempotency key for all return paths
	var idempotencyKey string
	if opts != nil {
		idempotencyKey = opts.IdempotencyKey
	}

	slog.Info("RSIC cycle started", "cycle_id", cycleID, "tier", tier, "space_id", spaceID, "source", meta.TriggerSource)
	metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "started").Inc()

	// Stage 1: Assess
	report, err := c.assessor.Assess(ctx, spaceID, tier)
	if err != nil {
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "error").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		return nil, fmt.Errorf("assess failed: %w", err)
	}
	metrics.Metrics().RSICActionTotal("assess", "completed").Inc()
	slog.Info("RSIC assess complete", "cycle_id", cycleID, "health", report.OverallHealth, "confidence", report.Confidence)

	// Bail early if confidence is too low
	if report.Confidence < c.cfg.RSICMinConfidence {
		outcome := &CycleOutcome{
			CycleID:        cycleID,
			Tier:           tier,
			SpaceID:        spaceID,
			StartedAt:      startedAt,
			CompletedAt:    time.Now(),
			Error:          fmt.Sprintf("confidence %.2f below threshold %.2f", report.Confidence, c.cfg.RSICMinConfidence),
			TriggerSource:  meta.TriggerSource,
			TriggerID:      meta.TriggerID,
			TriggeredAt:    meta.TriggeredAt,
			PolicyVersion:  meta.PolicyVersion,
			IdempotencyKey: idempotencyKey,
			DryRun:         isDryRun,
			SafetyVersion:  SafetyVersion,
		}
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "low_confidence").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		c.calibrator.UpdateCalibration(outcome, nil, nil)
		return outcome, nil
	}

	// Stage 2: Reflect
	insights, err := c.reflector.Reflect(ctx, report)
	if err != nil {
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "error").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		return nil, fmt.Errorf("reflect failed: %w", err)
	}
	metrics.Metrics().RSICActionTotal("reflect", "completed").Inc()
	slog.Info("RSIC reflect complete", "cycle_id", cycleID, "insight_count", len(insights))

	if len(insights) == 0 {
		outcome := &CycleOutcome{
			CycleID:     cycleID,
			Tier:        tier,
			SpaceID:     spaceID,
			StartedAt:   startedAt,
			CompletedAt: time.Now(),
			MetricsBefore: map[string]float64{
				"overall_health": report.OverallHealth,
			},
			TriggerSource:  meta.TriggerSource,
			TriggerID:      meta.TriggerID,
			TriggeredAt:    meta.TriggeredAt,
			PolicyVersion:  meta.PolicyVersion,
			IdempotencyKey: idempotencyKey,
			DryRun:         isDryRun,
			SafetyVersion:  SafetyVersion,
		}
		slog.Info("RSIC no insights, system is healthy", "cycle_id", cycleID)
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "completed").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		c.calibrator.UpdateCalibration(outcome, nil, nil)
		if c.watchdog != nil {
			c.watchdog.RecordCycle()
		}
		return outcome, nil
	}

	// Stage 3: Plan
	baseline := map[string]float64{
		"overall_health":      report.OverallHealth,
		"retrieval_quality":   report.RetrievalQuality,
		"memory_health":       report.MemoryHealth,
		"edge_health":         report.EdgeHealth,
		"total_nodes":         float64(report.TotalNodes),
		"edge_count":          float64(report.EdgeCount),
		"orphan_ratio":        report.OrphanRatio,
		"volatile_count":      float64(report.VolatileCount),
		"correction_rate":     report.CorrectionRate,
		"edge_weight_entropy": report.EdgeWeightEntropy,
	}

	tasks, err := c.planner.Plan(ctx, insights, spaceID, baseline)
	if err != nil {
		return nil, fmt.Errorf("plan failed: %w", err)
	}
	slog.Info("RSIC plan complete", "cycle_id", cycleID, "task_count", len(tasks))

	// Stamp cycle ID into each task
	for i := range tasks {
		tasks[i].CycleID = cycleID
		tasks[i].TaskID = fmt.Sprintf("%s-task-%d", cycleID, i)
	}

	// Phase 88: Configure dispatcher safety mode
	c.dispatcher.SetDryRun(isDryRun)
	c.dispatcher.ResetSafetySummary()

	// Stage 4: Execute (dispatch + wait)
	if err := c.dispatcher.Dispatch(ctx, tasks); err != nil {
		return nil, fmt.Errorf("dispatch failed: %w", err)
	}

	// Wait for completion with tier-dependent timeout
	timeout := c.tierTimeout(tier)
	completed := c.monitor.WaitForCycle(cycleID, timeout)
	if !completed {
		slog.Warn("RSIC cycle timed out", "cycle_id", cycleID, "timeout", timeout)
	}

	// Reset dry-run after dispatch completes
	c.dispatcher.SetDryRun(false)

	// Phase 88: Dry-run early return with deltas
	if isDryRun {
		outcome := &CycleOutcome{
			CycleID:        cycleID,
			Tier:           tier,
			SpaceID:        spaceID,
			StartedAt:      startedAt,
			CompletedAt:    time.Now(),
			Insights:       insights,
			DryRun:         true,
			SafetyVersion:  SafetyVersion,
			SafetySummary:  c.dispatcher.GetSafetySummary(),
			Deltas:         c.dispatcher.GetDeltas(),
			TriggerSource:  meta.TriggerSource,
			TriggerID:      meta.TriggerID,
			TriggeredAt:    meta.TriggeredAt,
			PolicyVersion:  meta.PolicyVersion,
			IdempotencyKey: idempotencyKey,
			MetricsBefore:  baseline,
		}
		slog.Info("RSIC dry-run complete", "cycle_id", cycleID, "delta_count", len(outcome.Deltas))
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "dry_run").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		c.calibrator.UpdateCalibration(outcome, tasks, nil)
		return outcome, nil
	}

	// Phase AR-1: Post-cycle re-assessment
	var postReport *SelfAssessmentReport
	postReport, err = c.assessor.Assess(ctx, spaceID, tier)
	if err != nil {
		slog.Warn("RSIC post-cycle assessment failed, continuing without", "cycle_id", cycleID, "error", err)
		postReport = nil
	}

	// Stage 5: Validate + Calibrate
	reports := c.monitor.CollectReportsForCycle(cycleID)
	outcome := c.calibrator.Validate(ctx, cycleID, tier, spaceID, tasks, reports, baseline, postReport)
	outcome.StartedAt = startedAt
	outcome.Insights = insights
	outcome.TriggerSource = meta.TriggerSource
	outcome.TriggerID = meta.TriggerID
	outcome.TriggeredAt = meta.TriggeredAt
	outcome.PolicyVersion = meta.PolicyVersion
	outcome.IdempotencyKey = idempotencyKey

	// Phase 88: Attach safety metadata
	outcome.SafetyVersion = SafetyVersion
	outcome.SafetySummary = c.dispatcher.GetSafetySummary()

	c.calibrator.UpdateCalibration(outcome, tasks, reports)

	// Phase AR-1 R6: Auto-rollback for reversible actions that didn't improve metrics
	if !outcome.CriteriaMet && c.snapshotStore != nil {
		for _, task := range tasks {
			if isReversibleAction(task.ActionType) {
				snaps := c.snapshotStore.ListSnapshots()
				for _, snap := range snaps {
					if snap.CycleID == cycleID && snap.Action == task.ActionType {
						rbResult, rbErr := c.snapshotStore.Rollback(ctx, snap.SnapshotID)
						if rbErr != nil {
							slog.Error("RSIC rollback failed", "cycle_id", cycleID, "action", task.ActionType, "error", rbErr)
						} else if rbResult != nil && rbResult.RolledBack {
							slog.Info("RSIC rolled back action", "cycle_id", cycleID, "action", task.ActionType, "restored_count", rbResult.RestoredCount)
							metrics.Metrics().RSICActionTotal(task.ActionType, "rolled_back").Inc()
						}
						break
					}
				}
			}
		}
	}

	// Phase 89: Clean up stale dispatcher tasks
	c.dispatcher.CleanupStaleTasks(10 * time.Minute)

	// NLI feedback loop: generate tier effectiveness dataset at meso/macro cycle boundaries
	if c.tierEffProvider != nil && (tier == TierMeso || tier == TierMacro) {
		if ds := c.tierEffProvider.BuildDataset(); ds != nil {
			slog.Info("RSIC tier effectiveness dataset generated", "cycle_id", cycleID)
		}
	}

	metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "completed").Inc()
	metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)

	slog.Info("RSIC cycle complete", "cycle_id", cycleID, "executed", outcome.ActionsExecuted, "success", outcome.SuccessCount, "failed", outcome.FailedCount)

	// Reset watchdog
	if c.watchdog != nil {
		c.watchdog.RecordCycle()
	}

	return outcome, nil
}

// Assess exposes just the assessment stage for the API.
func (c *CycleOrchestrator) Assess(ctx context.Context, spaceID string, tier CycleTier) (*SelfAssessmentReport, error) {
	return c.assessor.Assess(ctx, spaceID, tier)
}

// GetCalibration returns current per-action confidence scores.
func (c *CycleOrchestrator) GetCalibration() map[string]float64 {
	return c.calibrator.GetCalibration()
}

// SetOrchestrationPolicy attaches an orchestration policy to the orchestrator.
func (c *CycleOrchestrator) SetOrchestrationPolicy(p *OrchestrationPolicy) {
	c.policy = p
}

// SetSnapshotStore attaches a snapshot store for auto-rollback support.
func (c *CycleOrchestrator) SetSnapshotStore(ss *SnapshotStore) {
	c.snapshotStore = ss
}

// SetTSDBClient passes a TimescaleDB client to the reflector for schema drift detection.
func (c *CycleOrchestrator) SetTSDBClient(client *tsdb.Client) {
	if c.reflector != nil {
		c.reflector.SetTSDBClient(client)
	}
}

// SetDatasetProvider attaches a TSDB curated dataset provider to both assessor and reflector.
func (c *CycleOrchestrator) SetDatasetProvider(p tsdb.DatasetProvider) {
	if c.assessor != nil {
		c.assessor.SetDatasetProvider(p)
	}
	if c.reflector != nil {
		c.reflector.SetDatasetProvider(p)
	}
}

// SetTierEffectivenessProvider attaches a tier effectiveness dataset builder for RSIC.
func (c *CycleOrchestrator) SetTierEffectivenessProvider(p TierEffectivenessProvider) {
	c.tierEffProvider = p
}

// isReversibleAction returns true if the action type can be rolled back.
func isReversibleAction(actionType string) bool {
	switch actionType {
	case "tombstone_stale", "graduate_volatile":
		return true
	default:
		return false
	}
}

// GetHistory returns recent cycle outcomes.
func (c *CycleOrchestrator) GetHistory(limit int) []CycleOutcome {
	return c.calibrator.GetHistory(limit)
}

// GetHistoryFiltered returns recent cycle outcomes matching the filter.
func (c *CycleOrchestrator) GetHistoryFiltered(limit int, filter *HistoryFilter) []CycleOutcome {
	return c.calibrator.GetHistoryFiltered(limit, filter)
}

// GetWatchdogState returns the current watchdog state.
func (c *CycleOrchestrator) GetWatchdogState() *WatchdogState {
	if c.watchdog == nil {
		return nil
	}
	st := c.watchdog.GetState()
	return &st
}

// GetActiveTasks returns currently active task statuses.
func (c *CycleOrchestrator) GetActiveTasks() map[string]string {
	return c.monitor.GetAllActiveTasks()
}

// GetTaskReports returns progress reports for a specific task.
func (c *CycleOrchestrator) GetTaskReports(taskID string) []RSICProgressReport {
	return c.monitor.GetTaskReports(taskID)
}

func (c *CycleOrchestrator) tierTimeout(tier CycleTier) time.Duration {
	switch tier {
	case TierMicro:
		return 30 * time.Second
	case TierMeso:
		return 10 * time.Minute
	case TierMacro:
		return 30 * time.Minute
	default:
		return 10 * time.Minute
	}
}
