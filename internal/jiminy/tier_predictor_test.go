package jiminy

import (
	"context"
	"testing"
)

func TestTierPredictor_DisabledReturnsZero(t *testing.T) {
	tp := NewTierPredictor("http://localhost:8000", 100, false)

	tier, conf := tp.PredictTier(context.Background(), "constraint text", "context", 0.5)
	if tier != 0 || conf != 0 {
		t.Errorf("disabled predictor: tier=%d, conf=%f, want 0/0", tier, conf)
	}

	if tp.IsAvailable() {
		t.Error("disabled predictor should not be available")
	}
}

func TestTierPredictor_EmptyURLReturnsZero(t *testing.T) {
	tp := NewTierPredictor("", 100, true)

	tier, conf := tp.PredictTier(context.Background(), "constraint text", "context", 0.5)
	if tier != 0 || conf != 0 {
		t.Errorf("empty URL predictor: tier=%d, conf=%f, want 0/0", tier, conf)
	}

	if tp.IsAvailable() {
		t.Error("empty URL predictor should not be available")
	}
}

func TestTierPredictor_UnreachableSidecarReturnsZero(t *testing.T) {
	tp := NewTierPredictor("http://localhost:99999", 100, true)

	tier, conf := tp.PredictTier(context.Background(), "constraint text", "context", 0.5)
	if tier != 0 || conf != 0 {
		t.Errorf("unreachable sidecar: tier=%d, conf=%f, want 0/0", tier, conf)
	}
}

func TestTierPredictor_String(t *testing.T) {
	tp := NewTierPredictor("http://localhost:8000", 200, true)
	s := tp.String()
	if s == "" {
		t.Error("String() should return non-empty description")
	}

	tpOff := NewTierPredictor("", 0, false)
	if tpOff.String() != "TierPredictor(disabled)" {
		t.Errorf("disabled String() = %q, want 'TierPredictor(disabled)'", tpOff.String())
	}
}
