package ape

import (
	"context"
	"log/slog"
	"sort"

	"mdemg/internal/config"
)

// Planner maps reflection insights to concrete task specs.
type Planner struct {
	cfg        config.Config
	calibrator *Calibrator
}

// NewPlanner creates a Planner.
func NewPlanner(cfg config.Config) *Planner {
	return &Planner{cfg: cfg}
}

// SetCalibrator wires calibration feedback into the planner.
func (p *Planner) SetCalibrator(c *Calibrator) {
	p.calibrator = c
}

// Plan converts insights into prioritised RSICTaskSpecs, filtering by confidence.
func (p *Planner) Plan(_ context.Context, insights []ReflectionInsight, spaceID string, baseline map[string]float64) ([]RSICTaskSpec, error) {
	if len(insights) == 0 {
		return nil, nil
	}

	// Deduplicate actions by type (keep highest-severity version)
	actionMap := make(map[string]ImprovementAction)
	for _, insight := range insights {
		action := ImprovementAction{
			ActionType:  insight.RecommendedAction,
			TargetSpace: spaceID,
			Scope:       "space",
			Priority:    severityRank(insight.Severity),
			Rationale:   insight.Description,
		}
		if existing, ok := actionMap[action.ActionType]; !ok || action.Priority > existing.Priority {
			actionMap[action.ActionType] = action
		}
	}

	// Calibration-aware filtering: suppress actions with low historical success rate
	var calibration map[string]float64
	if p.calibrator != nil {
		calibration = p.calibrator.GetCalibration()
	}
	if calibration != nil && p.cfg.RSICMinActionConfidence > 0 {
		for actionType, action := range actionMap {
			if conf, ok := calibration[actionType]; ok && conf < p.cfg.RSICMinActionConfidence {
				// Allow critical-severity actions even with low confidence
				if action.Priority < severityRank(SeverityCritical) {
					slog.Info("RSIC planner: suppressing low-confidence action",
						"action", actionType, "confidence", conf, "threshold", p.cfg.RSICMinActionConfidence)
					delete(actionMap, actionType)
				}
			}
		}
	}

	// Build task specs — graph size from baseline drives blast radius calculation
	graphSize := int(baseline["total_nodes"])
	if graphSize == 0 {
		graphSize = int(baseline["edge_count"])
	}
	var specs []RSICTaskSpec
	cycleID := "" // will be set by CycleOrchestrator before dispatch
	for _, action := range actionMap {
		spec := BuildTaskSpec(p.cfg, action, cycleID, baseline, graphSize)
		specs = append(specs, *spec)
	}

	// Sort by priority DESC
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Priority > specs[j].Priority
	})

	return specs, nil
}
