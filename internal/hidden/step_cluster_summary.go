package hidden

import (
	"context"
	"log/slog"
)

// clusterSummaryStep enhances mechanical summaries on L1-L4 nodes with LLM-generated summaries.
// Runs as phase 28 — after dynamic edges (25), before L5 emergent (30).
type clusterSummaryStep struct{ svc *Service }

func (s *clusterSummaryStep) Name() string   { return "cluster_summary" }
func (s *clusterSummaryStep) Phase() int     { return 28 }
func (s *clusterSummaryStep) Required() bool { return false }

func (s *clusterSummaryStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	if !s.svc.cfg.ClusterSummaryEnabled {
		return &StepResult{}, nil
	}

	enhanced, err := s.svc.EnhanceSummariesWithLLM(ctx, spaceID)
	if err != nil {
		slog.Warn("cluster_summary step failed", "error", err)
		return &StepResult{}, nil // fail-open: don't block pipeline
	}

	return &StepResult{NodesCreated: enhanced}, nil
}
