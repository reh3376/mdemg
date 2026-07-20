package hidden

import "context"

// correctionStep adapts CreateCorrectionNodes to the NodeCreator interface.
// JIMINY-CORRECTION-PRODUCER-001: parallels constraintStep at phase 20.
type correctionStep struct{ svc *Service }

func (s *correctionStep) Name() string   { return "correction" }
func (s *correctionStep) Phase() int     { return 20 }
func (s *correctionStep) Required() bool { return false }

func (s *correctionStep) Run(ctx context.Context, spaceID string) (*StepResult, error) {
	r, err := s.svc.CreateCorrectionNodes(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		NodesCreated: r.Created,
		NodesUpdated: r.Updated,
		EdgesCreated: r.Linked,
	}, nil
}
