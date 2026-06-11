package ape

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"mdemg/internal/config"
	"mdemg/internal/conversation"
	"mdemg/internal/hidden"
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
	assessor        *Assessor
	reflector       *Reflector
	planner         *Planner
	dispatcher      *Dispatcher
	monitor         *Monitor
	calibrator      *Calibrator
	watchdog        *Watchdog
	snapshotStore   *SnapshotStore
	cfg             config.Config
	policy          *OrchestrationPolicy
	tierEffProvider TierEffectivenessProvider // NLI feedback loop: tier effectiveness dataset builder

	// Phase 11.6.x: bounded-buffer channel acts as a counting semaphore around
	// reflector.Reflect to cap concurrent LLM calls per orchestrator instance.
	// Capacity = cfg.RSICLLMConcurrencyLimit. Send to acquire, receive on defer
	// to release. nil disables throttling (used by tests that pre-construct an
	// orchestrator without going through NewCycleOrchestrator).
	llmSem chan struct{}

	// Phase 12 Epic 6: optional ConflictTracker for divergence-detection
	// hooks. Wired by server.go via SetConflictTracker; nil-receiver-safe
	// in conversation.ConflictTracker.Track so the hook is gated only by
	// non-nil + cfg.ContextFingerprintRefreshEnabled.
	conflictTracker *conversation.ConflictTracker

	// Phase 14.2 Epic 2: optional ContextCatalog Builder + Loader for the
	// stage-6 fingerprint refresh. Both nil → stage 6 is a no-op (default
	// for tests + cold-installs). Wired by serve.go after Builder/Loader
	// are constructed against the live Neo4j driver.
	contextCatalogBuilder hidden.Builder
	contextCatalogLoader  hidden.CatalogLoader
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
	limit := cfg.RSICLLMConcurrencyLimit
	if limit < 1 {
		limit = 1
	}
	return &CycleOrchestrator{
		assessor:   assessor,
		reflector:  reflector,
		planner:    planner,
		dispatcher: dispatcher,
		monitor:    monitor,
		calibrator: calibrator,
		watchdog:   watchdog,
		cfg:        cfg,
		llmSem:     make(chan struct{}, limit),
	}
}

// acquireLLMSlot blocks the calling goroutine until the per-orchestrator
// LLM-stage semaphore has free capacity. The first arrival path is a
// non-blocking send; only when that fails do we increment the blocked
// counter and wait for a slot to free. Returns ctx.Err() if cancelled
// while waiting. A nil llmSem returns immediately so test orchestrators
// constructed without NewCycleOrchestrator are not forced to set it up.
func (c *CycleOrchestrator) acquireLLMSlot(ctx context.Context) error {
	if c.llmSem == nil {
		return nil
	}
	select {
	case c.llmSem <- struct{}{}:
		return nil
	default:
	}
	metrics.Metrics().RSICLLMSemaphoreBlocked.Inc()
	select {
	case c.llmSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseLLMSlot frees one slot acquired via acquireLLMSlot. Safe to call
// when llmSem is nil; pairs with acquireLLMSlot via defer.
func (c *CycleOrchestrator) releaseLLMSlot() {
	if c.llmSem == nil {
		return
	}
	<-c.llmSem
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

	// Stage 2: Reflect — bounded by per-orchestrator LLM-stage semaphore so
	// a fan-out of concurrent RunCycle calls cannot saturate the local model
	// server. The blocked-counter only increments when a goroutine actually
	// has to wait, so a healthy run with sufficient capacity stays at zero.
	if err := c.acquireLLMSlot(ctx); err != nil {
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "error").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		return nil, fmt.Errorf("reflect cancelled while waiting for llm slot: %w", err)
	}
	defer c.releaseLLMSlot()

	insights, err := c.reflector.Reflect(ctx, report)
	if err != nil {
		metrics.Metrics().RSICCycleTotal(string(tier), string(meta.TriggerSource), "error").Inc()
		metrics.Metrics().RSICCycleDuration(string(tier)).ObserveDuration(startedAt)
		return nil, fmt.Errorf("reflect failed: %w", err)
	}
	metrics.Metrics().RSICActionTotal("reflect", "completed").Inc()
	slog.Info("RSIC reflect complete", "cycle_id", cycleID, "insight_count", len(insights))

	// Phase 12 Epic 6: divergence detection on the just-produced insight set.
	c.recordReflectDivergence(ctx, spaceID, insights)

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
	// RSIC-VALIDATE-001: single-source metric extraction — the old inline
	// map populated 10 keys while task criteria referenced ~15 others,
	// making validation vacuous.
	baseline := reportMetricsMap(report)

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
	c.dispatcher.ResetSafetySummary()

	// Stage 4: Execute (dispatch + wait, dryRun passed per-task)
	if err := c.dispatcher.Dispatch(ctx, tasks, isDryRun); err != nil {
		return nil, fmt.Errorf("dispatch failed: %w", err)
	}

	// Wait for completion with tier-dependent timeout
	timeout := c.tierTimeout(tier)
	completed := c.monitor.WaitForCycle(cycleID, timeout)
	timedOut := !completed
	if timedOut {
		slog.Warn("RSIC cycle timed out", "cycle_id", cycleID, "timeout", timeout)
	}

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
	outcome.TimedOut = timedOut

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

	// Stage 6: Context Catalog refresh (Phase 14.2 Epic 2).
	// Time-bounded; failures logged but do not fail the cycle.
	c.maybeRefreshContextCatalog(ctx, spaceID, cycleID)

	// Reset watchdog
	if c.watchdog != nil {
		c.watchdog.RecordCycle()
	}

	return outcome, nil
}

// maybeRefreshContextCatalog rebuilds the per-space ContextCatalog when the
// active version is older than ContextFingerprintRefreshIntervalHours (or no
// catalog exists yet — cold-start). Gated on cfg.ContextFingerprintRefreshEnabled
// and on both Builder + Loader being wired.
//
// Time-bounded by cfg.ContextFingerprintRefreshTimeoutMs; partial work is
// fine (next cycle picks up where this one left off).
func (c *CycleOrchestrator) maybeRefreshContextCatalog(parentCtx context.Context, spaceID, cycleID string) {
	if !c.cfg.ContextFingerprintRefreshEnabled {
		return
	}
	if c.contextCatalogBuilder == nil || c.contextCatalogLoader == nil {
		return
	}
	timeout := time.Duration(c.cfg.ContextFingerprintRefreshTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	age, cold, err := c.contextCatalogLoader.Freshness(ctx, spaceID)
	if err != nil {
		slog.Warn("RSIC stage6 freshness probe failed", "cycle_id", cycleID, "space_id", spaceID, "error", err)
		return
	}
	interval := time.Duration(c.cfg.ContextFingerprintRefreshIntervalHours) * time.Hour
	if !cold && age < interval {
		return
	}

	startedAt := time.Now()
	cat, err := c.contextCatalogBuilder.BuildForSpace(ctx, spaceID)
	if err != nil {
		slog.Warn("RSIC stage6 catalog build failed", "cycle_id", cycleID, "space_id", spaceID, "cold_start", cold, "error", err)
		return
	}
	slog.Info("RSIC stage6 catalog refreshed",
		"cycle_id", cycleID,
		"space_id", spaceID,
		"version", cat.Version,
		"total_bits", cat.TotalBits,
		"cold_start", cold,
		"age_before", age.String(),
		"duration", time.Since(startedAt).String(),
	)
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

// SetConflictTracker attaches the conflicting-guidance recorder so the
// orchestrator can emit divergence rows when LLM and rule-based reflectors
// disagree on the same dimension. Phase 12 Epic 6.
func (c *CycleOrchestrator) SetConflictTracker(t *conversation.ConflictTracker) {
	c.conflictTracker = t
}

// SetContextCatalog attaches a Builder + CatalogLoader pair so RunCycle's
// stage 6 can refresh the per-space context fingerprint catalog when stale.
// Either nil disables stage 6. Phase 14.2 Epic 2.
func (c *CycleOrchestrator) SetContextCatalog(b hidden.Builder, l hidden.CatalogLoader) {
	c.contextCatalogBuilder = b
	c.contextCatalogLoader = l
}

// recordReflectDivergence inspects insights produced by Reflector.Reflect and
// emits a guidance_conflicts row when two insights recommend opposing actions
// for the same target. Currently detects the documented pair
// (graduate_volatile vs tombstone_stale, both targeting volatile nodes), and
// the prune-vs-refresh edge pair (prune_decayed_edges vs refresh_stale_edges).
// Async (caller's critical path is unaffected) and gated by both
// cfg.ConflictTrackerEnabled and a non-nil tracker.
func (c *CycleOrchestrator) recordReflectDivergence(ctx context.Context, spaceID string, insights []ReflectionInsight) {
	if c.conflictTracker == nil || !c.cfg.ConflictTrackerEnabled || len(insights) < 2 {
		return
	}

	// Known opposing-pair sets. Add new entries as new dimensions surface.
	opposingPairs := map[string]string{
		"graduate_volatile":   "tombstone_stale",
		"tombstone_stale":     "graduate_volatile",
		"prune_decayed_edges": "refresh_stale_edges",
		"refresh_stale_edges": "prune_decayed_edges",
	}

	// Sort insight actions for deterministic context-hash inputs.
	actions := make([]string, 0, len(insights))
	for _, in := range insights {
		actions = append(actions, in.RecommendedAction)
	}
	sort.Strings(actions)

	// First-wins: stop at the first detected opposition. The rate limiter in
	// ConflictTracker.Track already prevents balloon-on-busy-cycle; we don't
	// need to enumerate every conflicting pair per cycle.
	for i, a := range actions {
		if other, ok := opposingPairs[a]; ok {
			for _, b := range actions[i+1:] {
				if b == other {
					rationale := fmt.Sprintf("RSIC reflect divergence: insights recommend both %q and %q for the same dimension; rule-based vs LLM reflectors disagree.", a, b)
					rec := conversation.ConflictRecord{
						SpaceID:            spaceID,
						RSICRecommendation: strings.Join(actions, ","),
						DivergenceKind:     conversation.DivergenceKindTextual,
						Rationale:          rationale,
						Source:             "ape.cycle.reflect",
						ContextHash:        conversation.HashContext(a, b, ""),
					}
					go func() {
						_ = c.conflictTracker.Track(ctx, rec)
					}()
					return
				}
			}
		}
	}
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
