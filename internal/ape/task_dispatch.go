package ape

import (
	"context"
	"fmt"
	"log"
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

	learner       LearningStatsProvider
	convSvc       ConversationStatsProvider
	hiddenSvc     HiddenLayerProvider
	driver        neo4j.DriverWithContext
	protoEvolver       ProtocolEvolverProvider       // J17: protocol mutation executor
	guidanceCalibrator GuidanceCalibrationProvider   // RSIC-SK1: guidance self-calibration

	// Phase 88: Safety enforcement
	safetyValidator *SafetyValidator
	snapshotStore   *SnapshotStore
	dryRun          bool
	safetySummary   *SafetySummary
	deltas          []ActionDelta
	safetyMu        sync.Mutex
}

type activeTask struct {
	Spec      RSICTaskSpec
	StartedAt time.Time
	Status    string // "running" | "completed" | "failed"
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

// SetDryRun puts the dispatcher in dry-run mode (estimate only, no mutations).
func (d *Dispatcher) SetDryRun(dryRun bool) {
	d.dryRun = dryRun
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
func (d *Dispatcher) Dispatch(ctx context.Context, tasks []RSICTaskSpec) error {
	for i := range tasks {
		task := tasks[i]
		taskCtx, cancel := context.WithTimeout(ctx, task.Timeout) //nolint:gosec // G118: cancel stored in activeTask.cancel, called on completion

		at := &activeTask{
			Spec:      task,
			StartedAt: time.Now(),
			Status:    "running",
			cancel:    cancel,
		}

		d.mu.Lock()
		d.activeTasks[task.TaskID] = at
		d.reports[task.TaskID] = nil
		d.mu.Unlock()

		metrics.Metrics().RSICActionTotal(task.ActionType, "dispatched").Inc()
		go d.executeTask(taskCtx, at)
	}
	return nil
}

func (d *Dispatcher) executeTask(ctx context.Context, at *activeTask) {
	defer at.cancel()

	taskID := at.Spec.TaskID
	actionType := at.Spec.ActionType
	taskStart := time.Now()

	// Phase 88: Dry-run mode — build delta, skip execution
	if d.dryRun && d.safetyValidator != nil {
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
			log.Printf("RSIC safety: %s rejected — %s", actionType, decision.Reason)
			d.postReport(taskID, "failed", 100, "safety_rejected",
				fmt.Sprintf("Rejected by safety validator: %s", decision.Reason), nil, decision.Reason)
			d.mu.Lock()
			at.Status = "failed"
			d.mu.Unlock()
			return
		}
	}

	// Milestone: snapshot_taken
	d.postReport(taskID, "running", 10, "snapshot_taken", "Baseline metrics captured", nil, "")

	// Phase 88: Capture pre-mutation snapshot
	if d.snapshotStore != nil {
		snap, err := d.snapshotStore.CaptureSnapshot(ctx, at.Spec.CycleID, actionType, at.Spec.TargetSpace)
		if err != nil {
			log.Printf("RSIC snapshot: capture failed for %s: %v (continuing)", actionType, err)
		} else {
			metrics.Metrics().RSICSnapshotCreated(actionType).Inc()
			log.Printf("RSIC snapshot: captured %s (%d items, expires %s)", snap.SnapshotID, snap.AffectedCount, snap.ExpiresAt.Format("15:04:05"))
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
		deliverables, execErr = d.executeCodifyConstraint(ctx, at.Spec.TargetSpace, at.Spec.Rationale)
	case "retire_code":
		deliverables, execErr = d.executeRetireCode(ctx, at.Spec.TargetSpace, at.Spec.Rationale)
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
	default:
		execErr = fmt.Errorf("unknown action type: %s", actionType)
	}

	if execErr != nil {
		metrics.Metrics().RSICActionTotal(actionType, "failed").Inc()
		metrics.Metrics().RSICActionDuration(actionType).ObserveDuration(taskStart)
		d.mu.Lock()
		at.Status = "failed"
		d.mu.Unlock()
		d.postReport(taskID, "failed", 100, "execution_complete", "", nil, execErr.Error())
		log.Printf("RSIC task %s failed: %v", taskID, execErr)
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
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Tombstone observations that have been superseded by corrections
		cypher := `
			MATCH (correction:MemoryNode {space_id: $spaceId, obs_type: 'correction'})
			WHERE correction.created_at > datetime() - duration('P7D')
			WITH correction
			MATCH (stale:MemoryNode {space_id: $spaceId})
			WHERE stale.role_type = 'conversation_observation'
			  AND stale.obs_type <> 'correction'
			  AND stale.created_at < correction.created_at
			  AND NOT coalesce(stale.is_archived, false)
			WITH stale LIMIT 50
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
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Refresh stale edges by bumping their last_activated timestamp
		cypher := `
			MATCH ()-[e:LEARNING_EDGE {space_id: $spaceId}]->()
			WHERE e.last_activated < datetime() - duration('P30D')
			WITH e LIMIT 100
			SET e.last_activated = datetime()
			RETURN count(e) AS refreshed
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("refreshed"); ok {
				return v, nil
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"refreshed": result}, nil
}

// ─── J17 Protocol action executors ───

func (d *Dispatcher) executeCodifyConstraint(ctx context.Context, spaceID, rationale string) (map[string]any, error) {
	if d.protoEvolver == nil {
		return nil, fmt.Errorf("protocol evolver not available")
	}
	// rationale contains the constraint node ID from the reflection insight
	return d.protoEvolver.CodifyConstraint(ctx, spaceID, rationale)
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
				log.Printf("RSIC-SK1: boost %s failed: %v", item.NodeID, err)
			} else {
				boosted++
			}
		} else if item.EffectivenessRate < 0.1 && item.TotalSurfaced >= 5 {
			if err := d.guidanceCalibrator.UpdateNodeConfidence(ctx, item.NodeID, "ignored"); err != nil {
				log.Printf("RSIC-SK1: decay %s failed: %v", item.NodeID, err)
			} else {
				decayed++
			}
		}
	}
	archived, archErr := d.guidanceCalibrator.ArchiveStaleConstraints(ctx, spaceID)
	if archErr != nil {
		log.Printf("RSIC-SK1: archive stale constraints failed: %v", archErr)
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
	// Find cycleID from active task
	d.mu.RLock()
	if at, ok := d.activeTasks[taskID]; ok {
		report.CycleID = at.Spec.CycleID
	}
	d.mu.RUnlock()

	d.mu.Lock()
	d.reports[taskID] = append(d.reports[taskID], report)
	d.mu.Unlock()
}
