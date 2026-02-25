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
	// Uses the same emergence config as Phase 103 dynamic emergence.
	// If emergence is disabled or namer creation fails, L5 nodes
	// will fall back to mechanical intersection names.
	var namer *EmergenceNamer
	if s.svc.cfg.EmergenceEnabled {
		namerCfg := EmergenceNamerConfig{
			Enabled:   s.svc.cfg.EmergenceEnabled,
			Provider:  s.svc.cfg.EmergenceProvider,
			Model:     s.svc.cfg.EmergenceModel,
			MaxTokens: s.svc.cfg.EmergenceMaxTokens,
			TimeoutMs: s.svc.cfg.EmergenceTimeoutMs,
			OpenAIKey: s.svc.cfg.OpenAIAPIKey,
			OpenAIURL: s.svc.cfg.EffectiveLLMEndpoint(),
			OllamaURL: s.svc.cfg.OllamaEndpoint,
		}
		namer = NewEmergenceNamer(namerCfg, s.svc.cbRegistry)
	}

	created, err := s.svc.CreateL5EmergentNodes(ctx, spaceID, namer)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		NodesCreated: created,
	}, nil
}
