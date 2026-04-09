package ape

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"mdemg/internal/metrics"
)

// ActionOutcome records whether a single action succeeded or failed.
type ActionOutcome struct {
	ActionType string    `json:"action_type"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
}

// Calibrator tracks per-action-type success rates and validates cycle outcomes.
type Calibrator struct {
	mu              sync.RWMutex
	actionHistory   map[string][]ActionOutcome
	cycleHistory    []CycleOutcome
	convSvc         ConversationStatsProvider
	store           *RSICStore
	maxHistoryItems int
}

// NewCalibrator creates a Calibrator. maxHistory sets the per-type cap on
// history entries (0 or negative uses the default of 1000).
func NewCalibrator(convSvc ConversationStatsProvider, maxHistory int) *Calibrator {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &Calibrator{
		actionHistory:   make(map[string][]ActionOutcome),
		convSvc:         convSvc,
		maxHistoryItems: maxHistory,
	}
}

// SetStore attaches a persistence store to the calibrator.
func (c *Calibrator) SetStore(s *RSICStore) {
	c.store = s
}

// Hydrate loads persisted calibration state from the store.
// The spaceID parameter is ignored — calibration is stored globally.
func (c *Calibrator) Hydrate(_ string) error {
	if c.store == nil {
		return nil
	}
	history, actions, err := c.store.LoadCalibration("global")
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if history != nil {
		c.cycleHistory = history
	}
	if actions != nil {
		c.actionHistory = actions
	}
	return nil
}

// Validate checks success criteria for dispatched tasks and returns a CycleOutcome.
func (c *Calibrator) Validate(_ context.Context, cycleID string, tier CycleTier, spaceID string, tasks []RSICTaskSpec, reports []RSICProgressReport, metricsBefore map[string]float64, postReport *SelfAssessmentReport) *CycleOutcome {
	outcome := &CycleOutcome{
		CycleID:       cycleID,
		Tier:          tier,
		SpaceID:       spaceID,
		StartedAt:     time.Now(), // will be overridden by orchestrator
		CompletedAt:   time.Now(),
		MetricsBefore: metricsBefore,
		MetricsAfter:  make(map[string]float64),
	}

	// Count successes and failures from final reports
	taskFinal := make(map[string]RSICProgressReport)
	for _, r := range reports {
		if existing, ok := taskFinal[r.TaskID]; !ok || r.Timestamp.After(existing.Timestamp) {
			taskFinal[r.TaskID] = r
		}
	}

	outcome.ActionsExecuted = len(tasks)
	for _, r := range taskFinal {
		if r.Status == "completed" {
			outcome.SuccessCount++
		} else {
			outcome.FailedCount++
		}
	}

	// Phase AR-1: Populate MetricsAfter from post-cycle re-assessment
	if postReport != nil {
		outcome.MetricsAfter = map[string]float64{
			"overall_health":      postReport.OverallHealth,
			"retrieval_quality":   postReport.RetrievalQuality,
			"memory_health":       postReport.MemoryHealth,
			"edge_health":         postReport.EdgeHealth,
			"orphan_ratio":        postReport.OrphanRatio,
			"correction_rate":     postReport.CorrectionRate,
			"edge_weight_entropy": postReport.EdgeWeightEntropy,
		}
	}

	// Phase AR-1: Evaluate success criteria per task
	outcome.CriteriaMet = true // assume success unless proven otherwise
	outcome.CriteriaDetail = make(map[string]string)

	for _, task := range tasks {
		for _, criterion := range task.SuccessCriteria {
			// Resolve metric key: criteria use names like "correction_rate_delta"
			// but MetricsBefore/MetricsAfter use base names like "correction_rate".
			metricKey, _ := strings.CutSuffix(criterion.Metric, "_delta")
			beforeVal, hasBefore := metricsBefore[metricKey]
			afterVal, hasAfter := outcome.MetricsAfter[metricKey]
			if !hasBefore || !hasAfter {
				outcome.CriteriaDetail[criterion.Metric] = "missing_data"
				continue
			}
			delta := afterVal - beforeVal
			met := evaluateCriterion(criterion, delta, afterVal)
			if !met {
				outcome.CriteriaMet = false
				outcome.CriteriaDetail[criterion.Metric] = fmt.Sprintf("not_met: delta=%.4f, threshold=%.4f, op=%s", delta, criterion.Threshold, criterion.Operator)
			} else {
				outcome.CriteriaDetail[criterion.Metric] = "met"
			}
		}
	}

	return outcome
}

// evaluateCriterion checks whether a single criterion is satisfied.
// For delta-based criteria (operators "gte", "gt", "lte", "lt"), it compares the delta.
// For absolute criteria (operator "eq"), it compares the afterVal.
func evaluateCriterion(c Criterion, delta, afterVal float64) bool {
	switch c.Operator {
	case "gte":
		return delta >= c.Threshold
	case "gt":
		return delta > c.Threshold
	case "lte":
		return delta <= c.Threshold
	case "lt":
		return delta < c.Threshold
	case "eq":
		return afterVal == c.Threshold
	default:
		return true // unknown operator — don't fail
	}
}

// UpdateCalibration records per-action-type outcomes for future confidence calculations.
func (c *Calibrator) UpdateCalibration(outcome *CycleOutcome, tasks []RSICTaskSpec, reports []RSICProgressReport) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build task status map
	taskFinal := make(map[string]string)
	for _, r := range reports {
		taskFinal[r.TaskID] = r.Status
	}

	for _, t := range tasks {
		success := taskFinal[t.TaskID] == "completed" && outcome.CriteriaMet
		c.actionHistory[t.ActionType] = append(c.actionHistory[t.ActionType], ActionOutcome{
			ActionType: t.ActionType,
			Success:    success,
			Timestamp:  time.Now(),
		})
	}

	c.cycleHistory = append(c.cycleHistory, *outcome)

	// Emit calibration confidence gauges
	m := metrics.Metrics()
	if len(c.actionHistory) == 0 {
		m.RSICCalibrationConfidence("overall").Set(0.5)
	}
	for actionType, outcomes := range c.actionHistory {
		if len(outcomes) == 0 {
			continue
		}
		successes := 0
		for _, o := range outcomes {
			if o.Success {
				successes++
			}
		}
		m.RSICCalibrationConfidence(actionType).Set(float64(successes) / float64(len(outcomes)))
	}

	// Trim history to configured max entries per action type
	cap := c.maxHistoryItems
	for k, v := range c.actionHistory {
		if len(v) > cap {
			c.actionHistory[k] = v[len(v)-cap:]
		}
	}
	if len(c.cycleHistory) > cap {
		c.cycleHistory = c.cycleHistory[len(c.cycleHistory)-cap:]
	}

	// Phase 89: Persist calibration state (global — calibrator tracks all spaces)
	if c.store != nil {
		c.store.SaveCalibration("global", c.cycleHistory, c.actionHistory)
	}
}

// GetCalibration returns the current confidence (success rate) per action type.
func (c *Calibrator) GetCalibration() map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]float64)
	for actionType, outcomes := range c.actionHistory {
		if len(outcomes) == 0 {
			result[actionType] = 0.5 // default confidence
			continue
		}
		successes := 0
		for _, o := range outcomes {
			if o.Success {
				successes++
			}
		}
		result[actionType] = float64(successes) / float64(len(outcomes))
	}
	return result
}

// GetHistory returns the most recent cycle outcomes.
func (c *Calibrator) GetHistory(limit int) []CycleOutcome {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getHistoryLocked(limit)
}

// GetHistoryFiltered returns cycle outcomes matching the given filter.
func (c *Calibrator) GetHistoryFiltered(limit int, filter *HistoryFilter) []CycleOutcome {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if filter == nil {
		return c.getHistoryLocked(limit)
	}

	if limit <= 0 {
		limit = len(c.cycleHistory)
	}

	var out []CycleOutcome
	for i := len(c.cycleHistory) - 1; i >= 0 && len(out) < limit; i-- {
		h := c.cycleHistory[i]
		if filter.TriggerSource != "" && h.TriggerSource != filter.TriggerSource {
			continue
		}
		if filter.Tier != "" && h.Tier != filter.Tier {
			continue
		}
		if filter.SpaceID != "" && h.SpaceID != filter.SpaceID {
			continue
		}
		out = append(out, h)
	}
	return out
}

func (c *Calibrator) getHistoryLocked(limit int) []CycleOutcome {
	if limit <= 0 || limit > len(c.cycleHistory) {
		limit = len(c.cycleHistory)
	}
	start := len(c.cycleHistory) - limit
	out := make([]CycleOutcome, limit)
	copy(out, c.cycleHistory[start:])
	return out
}
