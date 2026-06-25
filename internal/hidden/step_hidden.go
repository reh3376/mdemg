package hidden

import "context"

// fullReclusterKeyT marks a consolidation request that explicitly asks for a
// full re-cluster (the CHURN-002 path) instead of the default incremental step —
// the operator escape hatch for periodic quality maintenance (HIDDEN-CHURN-003).
type fullReclusterKeyT struct{}

// WithFullRecluster flags ctx so the hidden step runs the full re-cluster even
// when HIDDEN_INCREMENTAL_ENABLED is true.
func WithFullRecluster(ctx context.Context) context.Context {
	return context.WithValue(ctx, fullReclusterKeyT{}, true)
}

func fullReclusterRequested(ctx context.Context) bool {
	v, _ := ctx.Value(fullReclusterKeyT{}).(bool)
	return v
}

// hiddenStep adapts CreateHiddenNodes to the NodeCreator interface.
type hiddenStep struct{ svc *Service }

func (s *hiddenStep) Name() string   { return "hidden" }
func (s *hiddenStep) Phase() int     { return 10 }
func (s *hiddenStep) Required() bool { return true }

func (s *hiddenStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	// HIDDEN-CHURN-003: incremental is the default — assign only orphan L0 nodes
	// to existing patterns (no full re-cluster → ~0% churn + lower CPU). Operator
	// falls back to the CHURN-002 full re-cluster via HIDDEN_INCREMENTAL_ENABLED=false.
	if s.svc.cfg.HiddenIncrementalEnabled && !fullReclusterRequested(ctx) {
		created, assigned, err := s.svc.IncrementalHiddenNodes(ctx, spaceID)
		if err != nil {
			return nil, err
		}
		return &StepResult{NodesCreated: created, NodesUpdated: assigned}, nil
	}
	created, err := s.svc.CreateHiddenNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{NodesCreated: created}, nil
}
