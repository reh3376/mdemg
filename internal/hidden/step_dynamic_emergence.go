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
	namer := NewEmergenceNamer(namerCfg, s.svc.cbRegistry)

	created, err := s.svc.CreateDynamicEmergentNodes(ctx, spaceID, namer)
	if err != nil {
		return nil, err
	}

	return &StepResult{
		NodesCreated: created,
	}, nil
}
