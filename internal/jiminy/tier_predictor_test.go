package jiminy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mdemg/internal/circuitbreaker"
)

func TestTierPredictor_DisabledReturnsZero(t *testing.T) {
	tp := NewTierPredictor("http://localhost:8000", 100, false)

	r := tp.PredictTier(context.Background(), "constraint text", "context", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("disabled predictor: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}

	if tp.IsAvailable() {
		t.Error("disabled predictor should not be available")
	}
}

func TestTierPredictor_EmptyURLReturnsZero(t *testing.T) {
	tp := NewTierPredictor("", 100, true)

	r := tp.PredictTier(context.Background(), "constraint text", "context", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("empty URL predictor: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}

	if tp.IsAvailable() {
		t.Error("empty URL predictor should not be available")
	}
}

func TestTierPredictor_UnreachableSidecarReturnsZero(t *testing.T) {
	tp := NewTierPredictor("http://localhost:99999", 100, true)

	r := tp.PredictTier(context.Background(), "constraint text", "context", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("unreachable sidecar: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
	if r.Err == nil {
		t.Error("unreachable sidecar: expected non-nil error")
	}
	if r.LatencyMs <= 0 {
		t.Errorf("unreachable sidecar: expected positive latency, got %f", r.LatencyMs)
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

func TestTierPredictor_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/protocol/predict-tier" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req TierPredictRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		resp := TierPredictResponse{
			PredictedTier: 2,
			Confidence:    0.85,
			Model:         "test-model",
			LatencyMs:     5.0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 5000, true)

	r := tp.PredictTier(context.Background(), "constraint text", "agent context", 0.7)
	if r.Tier != 2 {
		t.Errorf("tier = %d, want 2", r.Tier)
	}
	if r.Conf != 0.85 {
		t.Errorf("conf = %f, want 0.85", r.Conf)
	}
	if r.Err != nil {
		t.Errorf("expected no error, got %v", r.Err)
	}
	if r.LatencyMs <= 0 {
		t.Errorf("expected positive latency, got %f", r.LatencyMs)
	}
}

func TestTierPredictor_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the client timeout
		time.Sleep(500 * time.Millisecond)
		resp := TierPredictResponse{PredictedTier: 2, Confidence: 0.9}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 50, true) // 50ms timeout

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("timeout: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
	if r.Err == nil {
		t.Error("timeout: expected non-nil error")
	}
}

func TestTierPredictor_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{bad json"))
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 5000, true)

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("malformed JSON: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
	if r.Err == nil {
		t.Error("malformed JSON: expected non-nil error")
	}
}

func TestTierPredictor_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 5000, true)

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("HTTP 500: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
	if r.Err == nil {
		t.Error("HTTP 500: expected non-nil error")
	}
}

func TestTierPredictor_ConfidenceBelowFloor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TierPredictResponse{
			PredictedTier: 2,
			Confidence:    0.3,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 5000, true)
	tp.minConfidence = 0.6

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 0 {
		t.Errorf("low confidence: tier=%d, want 0", r.Tier)
	}
	if r.Conf != 0.3 {
		t.Errorf("low confidence: conf=%f, want 0.3", r.Conf)
	}
	if r.Err != nil {
		t.Errorf("low confidence: expected no error, got %v", r.Err)
	}
}

func TestTierPredictor_TierOutOfRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TierPredictResponse{
			PredictedTier: 4, // out of range (valid is 1-3)
			Confidence:    0.9,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 5000, true)

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("out of range tier: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
}

func TestTierPredictor_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for context to be cancelled
		time.Sleep(5 * time.Second)
		resp := TierPredictResponse{PredictedTier: 2, Confidence: 0.9}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 10000, true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := tp.PredictTier(ctx, "constraint", "ctx", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("cancelled context: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
	if r.Err == nil {
		t.Error("cancelled context: expected non-nil error")
	}
}

func TestTierPredictor_CircuitBreakerOpen(t *testing.T) {
	// Create a breaker and force it open by recording enough failures
	cbCfg := circuitbreaker.Config{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          30 * time.Second,
		MaxConcurrent:    1,
	}
	cb := circuitbreaker.New("test-tier-cb", cbCfg)

	// Record a failure to open the circuit
	cb.RecordFailure()

	// Verify it's open
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected circuit to be open, got %s", cb.State())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server when circuit is open")
		resp := TierPredictResponse{PredictedTier: 2, Confidence: 0.9}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := NewTierPredictor(srv.URL, 5000, true)
	tp.SetCircuitBreaker(cb)

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 0 || r.Conf != 0 {
		t.Errorf("circuit open: tier=%d, conf=%f, want 0/0", r.Tier, r.Conf)
	}
	if r.Err == nil {
		t.Error("circuit open: expected non-nil error")
	}
}

func TestTierPredictor_CircuitBreakerWrapsCall(t *testing.T) {
	// Verify that when breaker is set, it wraps the call (happy path still works)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TierPredictResponse{
			PredictedTier: 1,
			Confidence:    0.75,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cbCfg := circuitbreaker.Config{
		Enabled:          true,
		FailureThreshold: 5,
		SuccessThreshold: 1,
		Timeout:          30 * time.Second,
		MaxConcurrent:    1,
	}
	cb := circuitbreaker.New("test-tier-cb-happy", cbCfg)

	tp := NewTierPredictor(srv.URL, 5000, true)
	tp.SetCircuitBreaker(cb)

	r := tp.PredictTier(context.Background(), "constraint", "ctx", 0.5)
	if r.Tier != 1 {
		t.Errorf("breaker happy path: tier=%d, want 1", r.Tier)
	}
	if r.Conf != 0.75 {
		t.Errorf("breaker happy path: conf=%f, want 0.75", r.Conf)
	}
	if r.Err != nil {
		t.Errorf("breaker happy path: unexpected error %v", r.Err)
	}
}

func TestTierPredictor_SetCircuitBreaker(t *testing.T) {
	tp := NewTierPredictor("http://localhost:8000", 100, true)

	if tp.breaker != nil {
		t.Error("breaker should be nil before SetCircuitBreaker")
	}

	cbCfg := circuitbreaker.Config{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Timeout:          15 * time.Second,
		MaxConcurrent:    1,
	}
	cb := circuitbreaker.New("test-set-cb", cbCfg)
	tp.SetCircuitBreaker(cb)

	if tp.breaker == nil {
		t.Error("breaker should be set after SetCircuitBreaker")
	}
	if tp.breaker.Name() != "test-set-cb" {
		t.Errorf("breaker name = %q, want 'test-set-cb'", tp.breaker.Name())
	}
}
