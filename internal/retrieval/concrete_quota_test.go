package retrieval

import (
	"testing"

	"mdemg/internal/models"
)

func mkRes(id string, layer int, role string) models.RetrieveResult {
	return models.RetrieveResult{
		NodeID:   id,
		Layer:    layer,
		RoleType: role,
	}
}

func TestApplyConcreteQuota_DisabledPassthrough(t *testing.T) {
	in := []models.RetrieveResult{mkRes("a", 3, "emergent_concept"), mkRes("b", 0, "leaf")}
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{Enabled: false, MinSlots: 1})
	if len(out) != 2 || out[0].NodeID != "a" {
		t.Fatalf("disabled should not reorder, got %+v", out)
	}
}

func TestApplyConcreteQuota_QuotaAlreadySatisfied(t *testing.T) {
	in := []models.RetrieveResult{
		mkRes("l0a", 0, "leaf"),
		mkRes("emerg", 3, "emergent_concept"),
	}
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 1, LayerMax: 1,
		RoleTypes: []string{"leaf", "constraint"},
	})
	if out[0].NodeID != "l0a" {
		t.Errorf("natural top-K already has concrete first; got %+v", out)
	}
}

func TestApplyConcreteQuota_PromotesFromTail(t *testing.T) {
	in := []models.RetrieveResult{
		mkRes("e1", 3, "emergent_concept"),
		mkRes("e2", 4, "emergent_concept"),
		mkRes("e3", 5, "emergent_concept"),
		mkRes("e4", 3, "emergent_concept"),
		mkRes("e5", 4, "emergent_concept"),
		mkRes("l0", 0, "leaf"), // buried at rank 5
		mkRes("l1", 1, "constraint"),
	}
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 1, LayerMax: 1,
		RoleTypes: []string{"leaf", "constraint"},
	})
	if out[0].NodeID != "l0" {
		t.Errorf("expected buried L0 promoted to slot 0; got %s", out[0].NodeID)
	}
	// Rest of the pool preserves natural rank
	if out[1].NodeID != "e1" || out[2].NodeID != "e2" {
		t.Errorf("natural rank not preserved for non-promoted; got %+v", out[:3])
	}
	// l1 should still be in the pool
	found := false
	for _, r := range out {
		if r.NodeID == "l1" {
			found = true
		}
	}
	if !found {
		t.Errorf("non-promoted concrete l1 lost from pool")
	}
}

func TestApplyConcreteQuota_MinSlots2_PromotesWhenUnderQuota(t *testing.T) {
	// topK=5, natural has only 1 concrete (l0 at rank 5); MinSlots=2 → must
	// promote a SECOND concrete into top-K by pulling from deeper in the pool.
	in := []models.RetrieveResult{
		mkRes("e1", 3, "emergent_concept"),
		mkRes("e2", 4, "emergent_concept"),
		mkRes("e3", 5, "emergent_concept"),
		mkRes("e4", 3, "emergent_concept"),
		mkRes("l0a", 0, "leaf"), // natural top-5 has only this concrete
		mkRes("e5", 4, "emergent_concept"),
		mkRes("l1", 1, "constraint"), // buried at rank 7
	}
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 2, LayerMax: 1,
		RoleTypes: []string{"leaf", "constraint"},
	})
	// Both l0a AND l1 promoted to slots 0,1 (in natural rank order: l0a first)
	if out[0].NodeID != "l0a" || out[1].NodeID != "l1" {
		t.Errorf("expected concretes at 0,1; got %s,%s", out[0].NodeID, out[1].NodeID)
	}
}

func TestApplyConcreteQuota_NaturalHas2Concrete_NoReorder(t *testing.T) {
	// topK=5 with 2 concretes already in top-K, MinSlots=2 → quota satisfied,
	// no reordering (preserves natural rank).
	in := []models.RetrieveResult{
		mkRes("e1", 3, "emergent_concept"),
		mkRes("e2", 4, "emergent_concept"),
		mkRes("l0a", 0, "leaf"),
		mkRes("e3", 5, "emergent_concept"),
		mkRes("l1", 1, "constraint"),
	}
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 2, LayerMax: 1,
		RoleTypes: []string{"leaf", "constraint"},
	})
	// Natural order preserved (design intent: promote only when UNDER quota).
	if out[0].NodeID != "e1" {
		t.Errorf("quota already satisfied should be no-op; got %s at slot 0", out[0].NodeID)
	}
}

func TestApplyConcreteQuota_NoConcreteInPool(t *testing.T) {
	in := []models.RetrieveResult{
		mkRes("e1", 3, "emergent_concept"),
		mkRes("e2", 4, "emergent_concept"),
	}
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 1, LayerMax: 1,
		RoleTypes: []string{"leaf"},
	})
	// Nothing to promote → unchanged
	if len(out) != 2 || out[0].NodeID != "e1" {
		t.Errorf("no-concrete-pool should be a no-op; got %+v", out)
	}
}

func TestApplyConcreteQuota_EmptyRoleSet_AcceptsAllUnderLayerMax(t *testing.T) {
	// topK=2, natural has 1 concrete (`odd`) at rank 1 → quota satisfied for
	// MinSlots=1. To exercise the "empty role set accepts all" branch under a
	// reordering path, use MinSlots=1 with topK=1 (natural top-1 has 0
	// concrete → must promote from rank 1 into slot 0).
	in := []models.RetrieveResult{
		mkRes("e1", 3, "emergent_concept"),
		mkRes("odd", 0, "novel_role"),
	}
	out := ApplyConcreteQuota(in, 1, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 1, LayerMax: 1,
		RoleTypes: nil, // empty = accept any role_type under LayerMax
	})
	if out[0].NodeID != "odd" {
		t.Errorf("empty role set should accept any role at L<=LayerMax; got %s", out[0].NodeID)
	}
}

func TestApplyConcreteQuota_MinSlotsExceedsPool(t *testing.T) {
	in := []models.RetrieveResult{
		mkRes("l0", 0, "leaf"),
		mkRes("e1", 3, "emergent_concept"),
	}
	// MinSlots=5 but only 1 concrete exists — promote what we can
	out := ApplyConcreteQuota(in, 5, ConcreteQuotaCfg{
		Enabled: true, MinSlots: 5, LayerMax: 1,
		RoleTypes: []string{"leaf"},
	})
	if len(out) != 2 {
		t.Errorf("pool size unchanged; got %d", len(out))
	}
	if out[0].NodeID != "l0" {
		t.Errorf("single concrete should be at slot 0; got %s", out[0].NodeID)
	}
}
