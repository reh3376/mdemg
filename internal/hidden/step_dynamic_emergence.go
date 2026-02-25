package hidden

import "context"

// dynamicEmergenceStep adapts CreateDynamicEmergentNodes to the NodeCreator interface.
// Phase 22: after hardcoded patterns (20), before dynamic edges (25).
type dynamicEmergenceStep struct{ svc *Service }

func (s *dynamicEmergenceStep) Name() string   { return "dynamic_emergence" }
func (s *dynamicEmergenceStep) Phase() int     { return 22 }
func (s *dynamicEmergenceStep) Required() bool { return false }

func (s *dynamicEmergenceStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	if !s.svc.cfg.EmergenceEnabled {
		return &StepResult{NodesCreated: 0}, nil
	}

	namer := s.svc.newEmergenceNamer()

	created, err := s.svc.CreateDynamicEmergentNodes(ctx, spaceID, namer)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		NodesCreated: created,
	}, nil
}
