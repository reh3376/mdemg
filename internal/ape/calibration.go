package ape

import (
	"context"
	"sync"
	"time"
)

// ActionOutcome records whether a single action succeeded or failed.
type ActionOutcome struct {
	ActionType string    `json:"action_type"`
	Success    bool      `json:"success"`
	Timestamp  time.Time `json:"timestamp"`
}

// Calibrator tracks per-action-type success rates and validates cycle outcomes.
type Calibrator struct {
	mu            sync.RWMutex
	actionHistory map[string][]ActionOutcome
	cycleHistory  []CycleOutcome
	convSvc       ConversationStatsProvider
	store         *RSICStore
}

// NewCalibrator creates a Calibrator.
func NewCalibrator(convSvc ConversationStatsProvider) *Calibrator {
	return &Calibrator{
		actionHistory: make(map[string][]ActionOutcome),
		convSvc:       convSvc,
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
func (c *Calibrator) Validate(_ context.Context, cycleID string, tier CycleTier, spaceID string, tasks []RSICTaskSpec, reports []RSICProgressReport, metricsBefore map[string]float64) *CycleOutcome {
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

	return outcome
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
		success := taskFinal[t.TaskID] == "completed"
		c.actionHistory[t.ActionType] = append(c.actionHistory[t.ActionType], ActionOutcome{
			ActionType: t.ActionType,
			Success:    success,
			Timestamp:  time.Now(),
		})
	}

	c.cycleHistory = append(c.cycleHistory, *outcome)

	// Trim history to last 100 entries per action type
	for k, v := range c.actionHistory {
		if len(v) > 100 {
			c.actionHistory[k] = v[len(v)-100:]
		}
	}
	if len(c.cycleHistory) > 100 {
		c.cycleHistory = c.cycleHistory[len(c.cycleHistory)-100:]
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
