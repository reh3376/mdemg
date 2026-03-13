package hidden

import "context"

// emergentL5Step runs L5 emergent concept detection as a pipeline step.
type emergentL5Step struct {
	svc *Service
}

func (s *emergentL5Step) Name() string   { return "emergent_l5" }
func (s *emergentL5Step) Phase() int     { return 30 } // post-processing, after main steps
func (s *emergentL5Step) Required() bool { return false }

func (s *emergentL5Step) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	if !s.svc.cfg.L5EmergentEnabled {
		return &StepResult{NodesCreated: 0}, nil
	}

	// Create EmergenceNamer for LLM-driven L5 concept naming.
	// If emergence is disabled, namer is nil and L5 nodes fall back to mechanical names.
	namer := s.svc.newEmergenceNamer()

	created, groundingEdges, err := s.svc.CreateL5EmergentNodes(ctx, spaceID, namer)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		NodesCreated: created,
		EdgesCreated: groundingEdges,
	}, nil
}
