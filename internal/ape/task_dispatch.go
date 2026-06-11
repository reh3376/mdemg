package ape

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/metrics"
)

// Dispatcher launches and manages RSIC task goroutines.
type Dispatcher struct {
	mu          sync.RWMutex
	activeTasks map[string]*activeTask
	reports     map[string][]RSICProgressReport

	learner            LearningStatsProvider
	convSvc            ConversationStatsProvider
	hiddenSvc          HiddenLayerProvider
	driver             neo4j.DriverWithContext
	protoEvolver       ProtocolEvolverProvider     // J17: protocol mutation executor
	guidanceCalibrator GuidanceCalibrationProvider // RSIC-SK1: guidance self-calibration
	freshnessProvider  FreshnessProvider           // Phase 47.2: ingest staleness provider

	// Phase 88: Safety enforcement
	safetyValidator *SafetyValidator
	snapshotStore   *SnapshotStore
	safetySummary   *SafetySummary
	deltas          []ActionDelta
	safetyMu        sync.Mutex

	// B2: Background cleanup goroutine lifecycle
	stopCleanup chan struct{}
	cleanupDone chan struct{}

	// DD-P1P2: Version counter for stale-read detection in WaitForCycle
	stateVersion uint64

	// DD-P2-16: Goroutine semaphore to bound concurrent RSIC task execution
	sem chan struct{}

	// SR-001: Alert delivery
	alertDispatcher AlertDispatcher
}

type activeTask struct {
	Spec      RSICTaskSpec
	StartedAt time.Time
	Status    string // "running" | "completed" | "failed"
	DryRun    bool
	cancel    context.CancelFunc
}

// NewDispatcher creates a Dispatcher wired to the given service providers.
func NewDispatcher(driver neo4j.DriverWithContext, learner LearningStatsProvider, convSvc ConversationStatsProvider, hiddenSvc HiddenLayerProvider) *Dispatcher {
	return &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
		learner:     learner,
		convSvc:     convSvc,
		hiddenSvc:   hiddenSvc,
		driver:      driver,
		sem:         make(chan struct{}, 50), // DD-P2-16: bound concurrent goroutines
	}
}

// SetSafetyValidator attaches a safety validator to the dispatcher.
func (d *Dispatcher) SetSafetyValidator(sv *SafetyValidator) {
	d.safetyValidator = sv
}

// SetSnapshotStore attaches a snapshot store to the dispatcher.
func (d *Dispatcher) SetSnapshotStore(ss *SnapshotStore) {
	d.snapshotStore = ss
}

// SetProtocolEvolver attaches a J17 protocol evolver for protocol mutation tasks.
func (d *Dispatcher) SetProtocolEvolver(pe ProtocolEvolverProvider) {
	d.protoEvolver = pe
}

// SetGuidanceCalibrator attaches an RSIC-SK1 guidance calibration provider.
func (d *Dispatcher) SetGuidanceCalibrator(gc GuidanceCalibrationProvider) {
	d.guidanceCalibrator = gc
}

// SetFreshnessProvider attaches an ingest freshness provider for stale space re-ingestion (Phase 47.2).
func (d *Dispatcher) SetFreshnessProvider(p FreshnessProvider) {
	d.freshnessProvider = p
}

// SetAlertDispatcher attaches an alert dispatcher for delivering RSIC alerts to the user.
func (d *Dispatcher) SetAlertDispatcher(ad AlertDispatcher) {
	d.alertDispatcher = ad
}

// ResetSafetySummary initializes a fresh safety summary for a cycle.
func (d *Dispatcher) ResetSafetySummary() {
	d.safetyMu.Lock()
	defer d.safetyMu.Unlock()
	d.safetySummary = &SafetySummary{}
	d.deltas = nil
}

// GetSafetySummary returns the accumulated safety summary.
func (d *Dispatcher) GetSafetySummary() *SafetySummary {
	d.safetyMu.Lock()
	defer d.safetyMu.Unlock()
	if d.safetySummary == nil {
		return nil
	}
	cpy := *d.safetySummary
	return &cpy
}

// GetDeltas returns accumulated dry-run deltas.
func (d *Dispatcher) GetDeltas() []ActionDelta {
	d.safetyMu.Lock()
	defer d.safetyMu.Unlock()
	return d.deltas
}

// Dispatch launches all tasks as background goroutines and returns immediately.
// dryRun is stored per-task to avoid shared mutable state across goroutines.
func (d *Dispatcher) Dispatch(ctx context.Context, tasks []RSICTaskSpec, dryRun bool) error {
	for i := range tasks {
		task := tasks[i]
		taskCtx, cancel := context.WithTimeout(ctx, task.Timeout) //nolint:gosec // G118: cancel stored in activeTask.cancel, called on completion

		at := &activeTask{
			Spec:      task,
			StartedAt: time.Now(),
			Status:    "running",
			DryRun:    dryRun,
			cancel:    cancel,
		}

		d.mu.Lock()
		d.activeTasks[task.TaskID] = at
		d.reports[task.TaskID] = nil
		d.mu.Unlock()

		metrics.Metrics().RSICActionTotal(task.ActionType, "dispatched").Inc()
		d.sem <- struct{}{} // DD-P2-16: acquire semaphore
		go func() {
			defer func() { <-d.sem }() // release semaphore
			d.executeTask(taskCtx, at)
		}()
	}
	return nil
}

func (d *Dispatcher) executeTask(ctx context.Context, at *activeTask) {
	defer at.cancel()

	taskID := at.Spec.TaskID
	actionType := at.Spec.ActionType
	taskStart := time.Now()

	// Phase 88: Dry-run mode — build delta, skip execution
	if at.DryRun && d.safetyValidator != nil {
		delta := d.safetyValidator.BuildDelta(ctx, &at.Spec, actionType)
		d.safetyMu.Lock()
		d.deltas = append(d.deltas, delta)
		if d.safetySummary != nil {
			d.safetySummary.ActionsChecked++
			if delta.WouldExecute {
				d.safetySummary.ActionsAllowed++
			} else {
				d.safetySummary.ActionsRejected++
				d.safetySummary.Rejections = append(d.safetySummary.Rejections, SafetyRejection{
					Action:            actionType,
					Reason:            delta.RejectionReason,
					EstimatedAffected: delta.EstimatedAffected,
					Limit:             delta.SafetyLimit,
				})
			}
		}
		d.safetyMu.Unlock()

		d.postReport(taskID, "completed", 100, "dry_run_complete",
			fmt.Sprintf("Dry-run delta computed for %s", actionType), nil, "")
		d.mu.Lock()
		at.Status = "completed"
		d.stateVersion++
		d.mu.Unlock()
		return
	}

	// Phase 88: Safety validation before execution
	if d.safetyValidator != nil {
		decision := d.safetyValidator.ValidateAction(ctx, &at.Spec, actionType)
		d.safetyMu.Lock()
		if d.safetySummary != nil {
			d.safetySummary.ActionsChecked++
			if decision.Allowed {
				d.safetySummary.ActionsAllowed++
			} else {
				d.safetySummary.ActionsRejected++
				d.safetySummary.Rejections = append(d.safetySummary.Rejections, SafetyRejection{
					Action:            actionType,
					Reason:            decision.Reason,
					EstimatedAffected: decision.EstimatedAffected,
					Limit:             decision.Limit,
				})
			}
		}
		d.safetyMu.Unlock()

		if !decision.Allowed {
			slog.Warn("RSIC safety: action rejected", "action", actionType, "reason", decision.Reason)
			d.postReport(taskID, "failed", 100, "safety_rejected",
				fmt.Sprintf("Rejected by safety validator: %s", decision.Reason), nil, decision.Reason)
			d.mu.Lock()
			at.Status = "failed"
			d.stateVersion++
			d.mu.Unlock()
			return
		}
	}

	// Milestone: snapshot_taken
	d.postReport(taskID, "running", 10, "snapshot_taken", "Baseline metrics captured", nil, "")

	// Phase 88: Capture pre-mutation snapshot (only for reversible actions)
	if d.snapshotStore != nil && isReversibleAction(actionType) {
		snap, err := d.snapshotStore.CaptureSnapshot(ctx, at.Spec.CycleID, actionType, at.Spec.TargetSpace)
		if err != nil {
			slog.Warn("RSIC snapshot: capture failed, continuing", "action", actionType, "error", err)
		} else {
			metrics.Metrics().RSICSnapshotCreated(actionType).Inc()
			slog.Info("RSIC snapshot: captured", "snapshot_id", snap.SnapshotID, "affected_count", snap.AffectedCount, "expires_at", snap.ExpiresAt.Format("15:04:05"))
			d.safetyMu.Lock()
			if d.safetySummary != nil {
				d.safetySummary.SnapshotsCreated++
			}
			d.safetyMu.Unlock()
		}
	}

	// Execute based on action type
	var execErr error
	var deliverables map[string]any

	switch actionType {
	case "prune_decayed_edges":
		deliverables, execErr = d.executePruneDecayed(ctx, at.Spec.TargetSpace)
	case "prune_excess_edges":
		deliverables, execErr = d.executePruneExcess(ctx, at.Spec.TargetSpace)
	case "trigger_consolidation":
		deliverables, execErr = d.executeConsolidation(ctx, at.Spec.TargetSpace)
	case "graduate_volatile":
		deliverables, execErr = d.executeGraduateVolatile(ctx, at.Spec.TargetSpace)
	case "tombstone_stale":
		deliverables, execErr = d.executeTombstoneStale(ctx, at.Spec.TargetSpace)
	case "refresh_stale_edges":
		deliverables, execErr = d.executeRefreshStaleEdges(ctx, at.Spec.TargetSpace)
	case "codify_constraint":
		nodeID := at.Spec.TargetNodeID
		if nodeID == "" {
			nodeID = at.Spec.Rationale // backward compat
		}
		deliverables, execErr = d.executeCodifyConstraint(ctx, at.Spec.TargetSpace, nodeID)
	case "codify_all_constraints":
		deliverables, execErr = d.executeCodifyAllConstraints(ctx, at.Spec.TargetSpace)
	case "retire_code":
		code := at.Spec.TargetCode
		if code == "" {
			code = at.Spec.Rationale // backward compat
		}
		deliverables, execErr = d.executeRetireCode(ctx, at.Spec.TargetSpace, code)
	case "adjust_tier_threshold":
		deliverables, execErr = d.executeAdjustTierThreshold(ctx, at.Spec.TargetSpace)
	case "adjust_replay_buffer":
		deliverables, execErr = d.executeAdjustReplayBuffer(ctx, at.Spec.TargetSpace)
	case "review_guidance_effectiveness":
		deliverables, execErr = d.executeReviewGuidanceEffectiveness(ctx, at.Spec.TargetSpace)
	case "adjust_guidance_confidence":
		deliverables, execErr = d.executeAdjustGuidanceConfidence(ctx, at.Spec.TargetSpace)
	case "archive_ineffective_constraints":
		deliverables, execErr = d.executeArchiveIneffectiveConstraints(ctx, at.Spec.TargetSpace)
	case "ingest_stale_spaces":
		deliverables, execErr = d.executeIngestStaleSpaces(ctx, at.Spec.TargetSpace)
	case "alert_jiminy_critical":
		deliverables, execErr = d.executeAlertJiminyCritical(ctx, at.Spec.TargetSpace)
	case "alert_memory_bloat":
		deliverables, execErr = d.executeAlertMemoryBloat(ctx, at.Spec.TargetSpace)
	case "alert_synergy_overlap":
		deliverables, execErr = d.executeAlertSynergyOverlap(ctx, at.Spec.TargetSpace)
	case "flush_recovery_buffer":
		deliverables, execErr = d.executeFlushRecoveryBuffer(ctx, at.Spec.TargetSpace)
	case "review_nli_calibration":
		deliverables, execErr = d.executeReviewNLICalibration(ctx, at.Spec.TargetSpace)
	case "alert_sidecar_down":
		deliverables, execErr = d.executeAlertSidecarDown(ctx, at.Spec.TargetSpace)
	// RSIC-DATA: TSDB-aware action handlers
	case "review_llm_provider":
		deliverables, execErr = d.executeAlertLog(ctx, at.Spec, "LLM provider performance issue detected")
	case "alert_llm_health":
		deliverables, execErr = d.executeAlertLog(ctx, at.Spec, "LLM error rate spike")
	case "alert_embedding_regression":
		deliverables, execErr = d.executeAlertLog(ctx, at.Spec, "Embedding pipeline regression: empty call_sites")
	case "trigger_training_pipeline":
		deliverables, execErr = d.executeAlertLog(ctx, at.Spec, "Training data threshold reached")
	// Gap fixes: handlers referenced by existing reflection patterns
	case "alert_tsdb_health":
		deliverables, execErr = d.executeAlertLog(ctx, at.Spec, "TSDB health alert")
	case "alert_schema_drift":
		deliverables, execErr = d.executeAlertLog(ctx, at.Spec, "TSDB schema drift detected")
	default:
		execErr = fmt.Errorf("unknown action type: %s", actionType)
	}

	if execErr != nil {
		metrics.Metrics().RSICActionTotal(actionType, "failed").Inc()
		metrics.Metrics().RSICActionDuration(actionType).ObserveDuration(taskStart)
		d.mu.Lock()
		at.Status = "failed"
		d.stateVersion++
		d.mu.Unlock()
		d.postReport(taskID, "failed", 100, "execution_complete", "", nil, execErr.Error())
		slog.Error("RSIC task failed", "task_id", taskID, "error", execErr)
		return
	}

	// Milestone: execution_complete
	d.postReport(taskID, "running", 80, "execution_complete", "Action executed successfully", deliverables, "")

	// Milestone: validation_complete
	d.postReport(taskID, "completed", 100, "validation_complete", "Task completed", deliverables, "")

	metrics.Metrics().RSICActionTotal(actionType, "success").Inc()
	metrics.Metrics().RSICActionDuration(actionType).ObserveDuration(taskStart)

	d.mu.Lock()
	at.Status = "completed"
	d.stateVersion++
	d.mu.Unlock()
}

// ─── Action executors ───

func (d *Dispatcher) executePruneDecayed(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.learner == nil {
		return nil, fmt.Errorf("learning service not available")
	}
	pruned, err := d.learner.PruneDecayedEdges(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"decayed_pruned": pruned}, nil
}

func (d *Dispatcher) executePruneExcess(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.learner == nil {
		return nil, fmt.Errorf("learning service not available")
	}
	pruned, err := d.learner.PruneExcessEdgesPerNode(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"excess_pruned": pruned}, nil
}

func (d *Dispatcher) executeConsolidation(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.hiddenSvc == nil {
		return nil, fmt.Errorf("hidden layer service not available")
	}
	result, err := d.hiddenSvc.RunFullConversationConsolidation(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"consolidation_result": result}, nil
}

func (d *Dispatcher) executeGraduateVolatile(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}
	// Graduate volatile observations via Neo4j directly
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND n.volatile = true
			  AND coalesce(n.stability_score, 0.1) >= 0.7
			SET n.volatile = false
			RETURN count(n) AS graduated
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("graduated"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"graduated": result}, nil
}

func (d *Dispatcher) executeTombstoneStale(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// RSIC-VALIDATE-001: tombstone ONLY observations RELATED to a recent
		// correction — same session as the correction, or its 1-hop
		// CO_ACTIVATED_WITH neighborhood. The previous query archived 50
		// ARBITRARY older observations whenever ANY correction existed in
		// the window (no relationship at all) — silently eroding unrelated
		// memory on every dispatch.
		cypher := `
			MATCH (correction:MemoryNode {space_id: $spaceId, obs_type: 'correction'})
			WHERE correction.created_at > datetime() - duration('P7D')
			WITH collect(correction) AS corrections
			UNWIND corrections AS correction
			MATCH (stale:MemoryNode {space_id: $spaceId})
			WHERE stale.role_type = 'conversation_observation'
			  AND stale.obs_type <> 'correction'
			  AND stale.created_at < correction.created_at
			  AND NOT coalesce(stale.is_archived, false)
			  AND (
			        (stale.session_id IS NOT NULL AND stale.session_id = correction.session_id)
			     OR (stale)-[:CO_ACTIVATED_WITH]-(correction)
			  )
			WITH DISTINCT stale LIMIT 50
			SET stale.is_archived = true
			RETURN count(stale) AS tombstoned
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("tombstoned"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"tombstoned": result}, nil
}

func (d *Dispatcher) executeRefreshStaleEdges(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	type refreshResult struct {
		Refreshed       int64
		AvgWeightBefore float64
		AvgWeightAfter  float64
	}

	res, err := neo4j.ExecuteWrite(ctx, sess, func(tx neo4j.ManagedTransaction) (refreshResult, error) {
		// Capture avg weight before refresh
		beforeRes, err := tx.Run(ctx, `
			MATCH ()-[e:LEARNING_EDGE {space_id: $spaceId}]->()
			WHERE e.last_activated < datetime() - duration('P30D')
			WITH e LIMIT 100
			RETURN avg(e.weight) AS avg_weight, collect(id(e)) AS ids
		`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return refreshResult{}, err
		}
		var avgBefore float64
		if beforeRes.Next(ctx) {
			if v, ok := beforeRes.Record().Get("avg_weight"); ok && v != nil {
				if f, ok := v.(float64); ok {
					avgBefore = f
				}
			}
		}
		if err := beforeRes.Err(); err != nil {
			return refreshResult{}, err
		}

		// Refresh stale edges: bump last_activated and recompute weight via co-activation decay
		// RSIC-VALIDATE-001: capture staleness BEFORE bumping last_activated.
		// The previous single SET bumped last_activated first, so the weight
		// expression read duration.between(now, now) = 0 days — the decay
		// term vanished and every "refresh" was a pure +0.1·log(count+1)
		// boost. Weights could only inflate.
		refreshCypher := `
			MATCH ()-[e:LEARNING_EDGE {space_id: $spaceId}]->()
			WHERE e.last_activated < datetime() - duration('P30D')
			WITH e, duration.between(e.last_activated, datetime()).days AS staleDays
			LIMIT 100
			SET e.weight = CASE
			        WHEN e.co_activation_count > 0
			        THEN e.weight * (1.0 - 0.02 * toFloat(staleDays) / 30.0)
			             + 0.1 * log(toFloat(e.co_activation_count) + 1)
			        ELSE e.weight * 0.8
			    END,
			    e.last_activated = datetime()
			RETURN count(e) AS refreshed, avg(e.weight) AS avg_weight_after
		`
		res, err := tx.Run(ctx, refreshCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return refreshResult{AvgWeightBefore: avgBefore}, err
		}
		var refreshed int64
		var avgAfter float64
		if res.Next(ctx) {
			if v, ok := res.Record().Get("refreshed"); ok {
				if n, ok := v.(int64); ok {
					refreshed = n
				}
			}
			if v, ok := res.Record().Get("avg_weight_after"); ok && v != nil {
				if f, ok := v.(float64); ok {
					avgAfter = f
				}
			}
		}
		return refreshResult{
			Refreshed:       refreshed,
			AvgWeightBefore: avgBefore,
			AvgWeightAfter:  avgAfter,
		}, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"refreshed":         res.Refreshed,
		"avg_weight_before": res.AvgWeightBefore,
		"avg_weight_after":  res.AvgWeightAfter,
	}, nil
}

// ─── J17 Protocol action executors ───

func (d *Dispatcher) executeCodifyConstraint(ctx context.Context, spaceID, rationale string) (map[string]any, error) {
	if d.protoEvolver == nil {
		return nil, fmt.Errorf("protocol evolver not available")
	}
	// rationale contains the constraint node ID from the reflection insight
	return d.protoEvolver.CodifyConstraint(ctx, spaceID, rationale)
}

func (d *Dispatcher) executeCodifyAllConstraints(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.protoEvolver == nil {
		return nil, fmt.Errorf("protocol evolver not available")
	}
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}

	// Query Neo4j for all constraint nodes without constraint_code
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	nodeIDs, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) ([]string, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'constraint'
			  AND (n.constraint_code IS NULL OR n.constraint_code = '')
			RETURN n.node_id AS nodeID
		`
		result, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for result.Next(ctx) {
			if v, ok := result.Record().Get("nodeID"); ok {
				if id, ok := v.(string); ok {
					ids = append(ids, id)
				}
			}
		}
		return ids, result.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("query uncoded constraints: %w", err)
	}

	codified := 0
	for _, nodeID := range nodeIDs {
		if _, err := d.protoEvolver.CodifyConstraint(ctx, spaceID, nodeID); err != nil {
			slog.Warn("codify_all_constraints: failed to codify", "node_id", nodeID, "error", err)
			continue
		}
		codified++
	}

	return map[string]any{
		"total_uncoded": len(nodeIDs),
		"codified":      codified,
	}, nil
}

func (d *Dispatcher) executeRetireCode(ctx context.Context, spaceID, rationale string) (map[string]any, error) {
	if d.protoEvolver == nil {
		return nil, fmt.Errorf("protocol evolver not available")
	}
	// rationale contains the constraint code to retire
	return d.protoEvolver.RetireCode(ctx, spaceID, rationale)
}

func (d *Dispatcher) executeAdjustTierThreshold(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.protoEvolver == nil {
		return nil, fmt.Errorf("protocol evolver not available")
	}
	return d.protoEvolver.AdjustTierThresholds(ctx, spaceID)
}

func (d *Dispatcher) executeAdjustReplayBuffer(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.protoEvolver == nil {
		return nil, fmt.Errorf("protocol evolver not available")
	}
	return d.protoEvolver.AdjustReplayBuffer(ctx, spaceID)
}

// ─── RSIC-SK1: Guidance calibration executors ───

func (d *Dispatcher) executeReviewGuidanceEffectiveness(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.guidanceCalibrator == nil {
		return nil, fmt.Errorf("guidance calibrator not available")
	}
	items, err := d.guidanceCalibrator.GetConstraintEffectiveness(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	var lowCount, highCount, insufficientCount int
	for _, item := range items {
		if item.TotalSurfaced < 3 {
			insufficientCount++
		} else if item.EffectivenessRate >= 0.7 {
			highCount++
		} else {
			lowCount++
		}
	}
	return map[string]any{
		"total":        len(items),
		"low":          lowCount,
		"high":         highCount,
		"insufficient": insufficientCount,
	}, nil
}

func (d *Dispatcher) executeAdjustGuidanceConfidence(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.guidanceCalibrator == nil {
		return nil, fmt.Errorf("guidance calibrator not available")
	}
	items, err := d.guidanceCalibrator.GetConstraintEffectiveness(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	var boosted, decayed int
	for _, item := range items {
		if item.TotalSurfaced < 3 {
			continue
		}
		if item.EffectivenessRate >= 0.7 {
			if err := d.guidanceCalibrator.UpdateNodeConfidence(ctx, item.NodeID, "followed"); err != nil {
				slog.Error("RSIC-SK1: boost failed", "node_id", item.NodeID, "error", err)
			} else {
				boosted++
			}
		} else if item.EffectivenessRate < 0.1 && item.TotalSurfaced >= 5 {
			if err := d.guidanceCalibrator.UpdateNodeConfidence(ctx, item.NodeID, "ignored"); err != nil {
				slog.Error("RSIC-SK1: decay failed", "node_id", item.NodeID, "error", err)
			} else {
				decayed++
			}
		}
	}
	archived, archErr := d.guidanceCalibrator.ArchiveStaleConstraints(ctx, spaceID)
	if archErr != nil {
		slog.Error("RSIC-SK1: archive stale constraints failed", "error", archErr)
	}
	return map[string]any{
		"boosted":  boosted,
		"decayed":  decayed,
		"archived": archived,
	}, nil
}

func (d *Dispatcher) executeArchiveIneffectiveConstraints(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.guidanceCalibrator == nil {
		return nil, fmt.Errorf("guidance calibrator not available")
	}
	archived, err := d.guidanceCalibrator.ArchiveStaleConstraints(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"archived": archived}, nil
}

// executeIngestStaleSpaces triggers re-ingestion for spaces past the staleness threshold (Phase 47.2).
func (d *Dispatcher) executeIngestStaleSpaces(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.freshnessProvider == nil {
		return nil, fmt.Errorf("freshness provider not configured")
	}
	reIngested, err := d.freshnessProvider.TriggerIngestForStaleSpaces(ctx, 24)
	if err != nil {
		return nil, fmt.Errorf("ingest stale spaces: %w", err)
	}
	return map[string]any{
		"action":       "ingest_stale_spaces",
		"re_ingested":  reIngested,
		"target_space": spaceID,
	}, nil
}

// ─── RSIC alert + recovery executors ───

func (d *Dispatcher) executeAlertJiminyCritical(ctx context.Context, spaceID string) (map[string]any, error) {
	slog.Error("RSIC alert: Jiminy guidance pipeline critical", "space_id", spaceID)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal("alert_jiminy_critical", "success").Inc()
	}
	if d.alertDispatcher != nil {
		d.alertDispatcher.SendAlert(ctx, "jiminy", "Jiminy Pipeline Critical",
			"Jiminy guidance pipeline is unhealthy — guidance not reaching agent", SeverityCritical)
	}
	return map[string]any{
		"alert":    "jiminy_critical",
		"space_id": spaceID,
		"severity": "critical",
		"message":  "Jiminy guidance pipeline is unhealthy — guidance not reaching agent",
	}, nil
}

func (d *Dispatcher) executeAlertSidecarDown(ctx context.Context, spaceID string) (map[string]any, error) {
	slog.Error("RSIC alert: Neural sidecar is down — J17 protocol degraded", "space_id", spaceID)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal("alert_sidecar_down", "success").Inc()
	}
	if d.alertDispatcher != nil {
		d.alertDispatcher.SendAlert(ctx, "neural-sidecar", "Neural Sidecar Down",
			"Neural sidecar is down — J17 protocol running in T3 fallback mode, no ML tier prediction or NLI scoring", SeverityHigh)
	}
	return map[string]any{
		"alert":    "sidecar_down",
		"space_id": spaceID,
		"severity": "high",
		"message":  "Neural sidecar is down — J17 protocol running in T3 fallback mode, no ML tier prediction or NLI scoring",
	}, nil
}

func (d *Dispatcher) executeAlertMemoryBloat(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}
	// Query total node count to assess bloat
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	nodeCount, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) (int64, error) {
		result, err := tx.Run(ctx, `MATCH (n:MemoryNode {space_id: $spaceId}) RETURN count(n) AS cnt`,
			map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if result.Next(ctx) {
			if v, ok := result.Record().Get("cnt"); ok {
				if cnt, ok := v.(int64); ok {
					return cnt, nil
				}
			}
		}
		return 0, result.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("query node count: %w", err)
	}

	slog.Warn("RSIC alert: memory bloat assessment", "space_id", spaceID, "node_count", nodeCount)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal("alert_memory_bloat", "success").Inc()
	}
	if d.alertDispatcher != nil {
		d.alertDispatcher.SendAlert(ctx, "memory", "Memory Bloat Detected",
			fmt.Sprintf("Memory node count %d exceeds healthy threshold — consider pruning", nodeCount), SeverityMedium)
	}
	return map[string]any{
		"alert":      "memory_bloat",
		"space_id":   spaceID,
		"node_count": nodeCount,
	}, nil
}

func (d *Dispatcher) executeAlertSynergyOverlap(ctx context.Context, spaceID string) (map[string]any, error) {
	slog.Warn("RSIC alert: synergy overlap detected", "space_id", spaceID)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal("alert_synergy_overlap", "success").Inc()
	}
	if d.alertDispatcher != nil {
		d.alertDispatcher.SendAlert(ctx, "synergy", "Synergy Overlap Detected",
			"Synergy layer overlap or redundancy detected — review consolidation pipeline", SeverityLow)
	}
	return map[string]any{
		"alert":    "synergy_overlap",
		"space_id": spaceID,
		"message":  "Synergy layer overlap or redundancy detected — review consolidation pipeline",
	}, nil
}

func (d *Dispatcher) executeFlushRecoveryBuffer(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}
	// Flush recovery buffer entries: re-classify recovery_buffer nodes as regular observations
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	type flushResult struct {
		Flushed   int64
		Remaining int64
	}

	res, err := neo4j.ExecuteWrite(ctx, sess, func(tx neo4j.ManagedTransaction) (flushResult, error) {
		// Re-classify recovery buffer nodes back to conversation_observation
		flushed, err := tx.Run(ctx, `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.obs_type = 'recovery_buffer' OR n.source = 'synergy_recovery'
			WITH n LIMIT 100
			SET n.obs_type = 'conversation_observation', n.recovered_at = datetime()
			RETURN count(n) AS flushed
		`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return flushResult{}, err
		}
		var cnt int64
		if flushed.Next(ctx) {
			if v, ok := flushed.Record().Get("flushed"); ok {
				if n, ok := v.(int64); ok {
					cnt = n
				}
			}
		}
		if err := flushed.Err(); err != nil {
			return flushResult{}, err
		}

		// Count remaining recovery buffer entries
		remaining, err := tx.Run(ctx, `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.obs_type = 'recovery_buffer' OR n.source = 'synergy_recovery'
			RETURN count(n) AS cnt
		`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return flushResult{Flushed: cnt}, err
		}
		var rem int64
		if remaining.Next(ctx) {
			if v, ok := remaining.Record().Get("cnt"); ok {
				if c, ok := v.(int64); ok {
					rem = c
				}
			}
		}
		return flushResult{Flushed: cnt, Remaining: rem}, remaining.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("flush recovery buffer: %w", err)
	}

	slog.Info("RSIC: recovery buffer flushed", "space_id", spaceID, "flushed", res.Flushed, "remaining", res.Remaining)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal("flush_recovery_buffer", "success").Inc()
	}
	return map[string]any{
		"action":          "flush_recovery_buffer",
		"space_id":        spaceID,
		"entries_flushed": res.Flushed,
		"remaining":       res.Remaining,
	}, nil
}

func (d *Dispatcher) executeReviewNLICalibration(ctx context.Context, spaceID string) (map[string]any, error) {
	if d.driver == nil {
		return nil, fmt.Errorf("neo4j driver not available")
	}
	// Query NLI calibration metrics from the graph
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	type nliStats struct {
		TotalConstraints  int64
		ActiveConstraints int64
		AvgConfidence     float64
	}

	stats, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) (nliStats, error) {
		res, err := tx.Run(ctx, `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.obs_type = 'constraint' OR n.obs_type = 'correction'
			RETURN count(n) AS total,
			       count(CASE WHEN coalesce(n.archived, false) = false THEN 1 END) AS active,
			       avg(coalesce(n.confidence, 0.5)) AS avg_confidence
		`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nliStats{}, err
		}
		var s nliStats
		if res.Next(ctx) {
			if v, ok := res.Record().Get("total"); ok {
				if n, ok := v.(int64); ok {
					s.TotalConstraints = n
				}
			}
			if v, ok := res.Record().Get("active"); ok {
				if n, ok := v.(int64); ok {
					s.ActiveConstraints = n
				}
			}
			if v, ok := res.Record().Get("avg_confidence"); ok && v != nil {
				if f, ok := v.(float64); ok {
					s.AvgConfidence = f
				}
			}
		}
		return s, res.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("review NLI calibration: %w", err)
	}

	slog.Info("RSIC: NLI calibration review", "space_id", spaceID,
		"total_constraints", stats.TotalConstraints,
		"active_constraints", stats.ActiveConstraints,
		"avg_confidence", stats.AvgConfidence)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal("review_nli_calibration", "success").Inc()
	}
	return map[string]any{
		"action":             "review_nli_calibration",
		"space_id":           spaceID,
		"total_constraints":  stats.TotalConstraints,
		"active_constraints": stats.ActiveConstraints,
		"avg_confidence":     stats.AvgConfidence,
	}, nil
}

// CleanupStaleTasks removes completed/failed tasks older than maxAge and caps total entries at 1000.
func (d *Dispatcher) CleanupStaleTasks(maxAge time.Duration) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	removed := 0
	for id, at := range d.activeTasks {
		if (at.Status == "completed" || at.Status == "failed") && now.Sub(at.StartedAt) > maxAge {
			delete(d.activeTasks, id)
			delete(d.reports, id)
			removed++
		}
	}

	// Cap at 1000 entries — evict oldest if over limit
	for len(d.activeTasks) > 1000 {
		var oldestID string
		var oldestTime time.Time
		for id, at := range d.activeTasks {
			if oldestID == "" || at.StartedAt.Before(oldestTime) {
				oldestID = id
				oldestTime = at.StartedAt
			}
		}
		if oldestID != "" {
			delete(d.activeTasks, oldestID)
			delete(d.reports, oldestID)
			removed++
		}
	}
	return removed
}

// StartBackgroundCleanup launches a goroutine that periodically cleans up
// stale tasks from the activeTasks map. This prevents stale entry accumulation
// between RSIC cycles (which may be infrequent for macro cycles).
func (d *Dispatcher) StartBackgroundCleanup(interval, maxAge time.Duration) {
	d.stopCleanup = make(chan struct{})
	d.cleanupDone = make(chan struct{})
	go func() {
		defer close(d.cleanupDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				removed := d.CleanupStaleTasks(maxAge)
				if removed > 0 {
					slog.Info("rsic: background cleanup removed stale tasks", "removed", removed)
				}
			case <-d.stopCleanup:
				return
			}
		}
	}()
	slog.Info("rsic: background task cleanup started", "interval", interval, "maxAge", maxAge)
}

// StopBackgroundCleanup stops the background cleanup goroutine and waits for it to finish.
func (d *Dispatcher) StopBackgroundCleanup() {
	if d.stopCleanup != nil {
		close(d.stopCleanup)
		<-d.cleanupDone
		slog.Info("rsic: background task cleanup stopped")
	}
}

// ─── Report posting ───

func (d *Dispatcher) postReport(taskID, status string, pct float64, milestone, summary string, deliverables map[string]any, errMsg string) {
	report := RSICProgressReport{
		TaskID:       taskID,
		Status:       status,
		ProgressPct:  pct,
		Milestone:    milestone,
		Summary:      summary,
		Deliverables: deliverables,
		Timestamp:    time.Now(),
		Error:        errMsg,
	}
	// Single write lock for both read and append to prevent race between RUnlock and Lock.
	d.mu.Lock()
	if at, ok := d.activeTasks[taskID]; ok {
		report.CycleID = at.Spec.CycleID
		if IsDiagnosticAction(at.Spec.ActionType) {
			report.Diagnostic = true
		}
	}
	d.reports[taskID] = append(d.reports[taskID], report)
	d.mu.Unlock()
}

// executeAlertLog is a generic alert/log handler for RSIC-DATA and gap-fix actions.
func (d *Dispatcher) executeAlertLog(ctx context.Context, spec RSICTaskSpec, message string) (map[string]any, error) {
	slog.Warn("RSIC alert", "action", spec.ActionType, "space", spec.TargetSpace, "message", message, "rationale", spec.Rationale)
	if m := metrics.Metrics(); m != nil {
		m.RSICActionTotal(spec.ActionType, "success").Inc()
	}
	if d.alertDispatcher != nil {
		d.alertDispatcher.SendAlert(ctx, "rsic", "RSIC: "+spec.ActionType, message, SeverityMedium)
	}
	return map[string]any{
		"alerted":  true,
		"action":   spec.ActionType,
		"space_id": spec.TargetSpace,
		"message":  message,
	}, nil
}
