package metrics

import "testing"

// CONSOLIDATE-PERF-001 Sprint A: the per-phase consolidation gauge is registered,
// label-scoped per (space_id, phase), and settable — the breakdown that targets
// the Sprint-B optimization.
func TestConsolidationPhaseDuration(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	m := NewStandardMetrics(r)

	g := m.ConsolidationPhaseDuration("mdemg-dev", "forward_initial")
	if g.Value() != 0 {
		t.Errorf("initial value = %.3f, want 0", g.Value())
	}
	g.Set(12.5)
	if g.Value() != 12.5 {
		t.Errorf("after Set value = %.3f, want 12.5", g.Value())
	}

	// Distinct phase → distinct gauge.
	other := m.ConsolidationPhaseDuration("mdemg-dev", "backward")
	if other.Value() != 0 {
		t.Errorf("backward phase should be a distinct gauge, got %.3f", other.Value())
	}
	// Same (space, phase) → same gauge.
	again := m.ConsolidationPhaseDuration("mdemg-dev", "forward_initial")
	if again.Value() != 12.5 {
		t.Errorf("same label set should resolve to the same gauge, got %.3f", again.Value())
	}
}
