package hidden

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

func TestDynamicEmergenceStep_Metadata(t *testing.T) {
	step := &dynamicEmergenceStep{svc: &Service{cfg: config.Config{}}}

	if step.Name() != "dynamic_emergence" {
		t.Errorf("Name() = %q, want %q", step.Name(), "dynamic_emergence")
	}
	if step.Phase() != 22 {
		t.Errorf("Phase() = %d, want %d", step.Phase(), 22)
	}
	if step.Required() {
		t.Error("Required() should be false")
	}
}

func TestDynamicEmergenceStep_DisabledReturnsZero(t *testing.T) {
	step := &dynamicEmergenceStep{svc: &Service{cfg: config.Config{
		EmergenceEnabled: false,
	}}}

	result, err := step.Run(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NodesCreated != 0 {
		t.Errorf("NodesCreated = %d, want 0", result.NodesCreated)
	}
}
