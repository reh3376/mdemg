package jiminy

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mdemg/internal/circuitbreaker"
)

func TestNLIComprehensionScorer_DisabledFallback(t *testing.T) {
	scorer := NewNLIComprehensionScorer("", 100, false)

	// Followed → 1.0
	score := scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", true)
	if score != 1.0 {
		t.Errorf("disabled scorer followed = %f, want 1.0", score)
	}

	// Not followed → 0.0
	score = scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", false)
	if score != 0.0 {
		t.Errorf("disabled scorer not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_NoSidecarFallback(t *testing.T) {
	scorer := NewNLIComprehensionScorer("http://localhost:99999", 100, true)

	// Sidecar unreachable → falls back to heuristic
	score := scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", true)
	if score != 1.0 {
		t.Errorf("unreachable sidecar followed = %f, want 1.0", score)
	}
}

func TestNLIComprehensionScorer_EmptyURL(t *testing.T) {
	scorer := NewNLIComprehensionScorer("", 100, true)

	// Empty URL → fallback
	score := scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", false)
	if score != 0.0 {
		t.Errorf("empty URL not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_HappyPathEntailment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nli" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req nliRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		resp := nliResponse{
			Label: "entailment",
		}
		resp.Scores.Entailment = 0.92
		resp.Scores.Contradiction = 0.03
		resp.Scores.Neutral = 0.05
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)

	score := scorer.ScoreComprehension(context.Background(), "constraint", "action summary", true)
	if math.Abs(score-0.92) > 0.001 {
		t.Errorf("entailment score = %f, want ~0.92", score)
	}
}

func TestNLIComprehensionScorer_Contradiction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := nliResponse{
			Label: "contradiction",
		}
		resp.Scores.Entailment = 0.05
		resp.Scores.Contradiction = 0.90
		resp.Scores.Neutral = 0.05
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)

	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", false)
	if score != 1.0 {
		t.Errorf("contradiction score = %f, want 1.0", score)
	}
}

func TestNLIComprehensionScorer_Neutral(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := nliResponse{
			Label: "neutral",
		}
		resp.Scores.Entailment = 0.20
		resp.Scores.Contradiction = 0.10
		resp.Scores.Neutral = 0.70
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)

	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", true)
	if score != 0.5 {
		t.Errorf("neutral score = %f, want 0.5", score)
	}
}

func TestNLIComprehensionScorer_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		resp := nliResponse{Label: "entailment"}
		resp.Scores.Entailment = 0.9
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 50, true) // 50ms timeout

	// followed=true → falls back to 1.0
	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", true)
	if score != 1.0 {
		t.Errorf("timeout followed = %f, want 1.0", score)
	}

	// followed=false → falls back to 0.0
	score = scorer.ScoreComprehension(context.Background(), "constraint", "action", false)
	if score != 0.0 {
		t.Errorf("timeout not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)

	// followed=true → falls back to 1.0
	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", true)
	if score != 1.0 {
		t.Errorf("HTTP 500 followed = %f, want 1.0", score)
	}

	// followed=false → falls back to 0.0
	score = scorer.ScoreComprehension(context.Background(), "constraint", "action", false)
	if score != 0.0 {
		t.Errorf("HTTP 500 not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_CircuitBreakerOpen(t *testing.T) {
	cbCfg := circuitbreaker.Config{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          30 * time.Second,
		MaxConcurrent:    1,
	}
	cb := circuitbreaker.New("test-nli-cb", cbCfg)

	// Force circuit open
	cb.RecordFailure()
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected circuit to be open, got %s", cb.State())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server when circuit is open")
		resp := nliResponse{Label: "entailment"}
		resp.Scores.Entailment = 0.9
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)
	scorer.SetCircuitBreaker(cb)

	// followed=true → falls back to 1.0
	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", true)
	if score != 1.0 {
		t.Errorf("circuit open followed = %f, want 1.0", score)
	}

	// followed=false → falls back to 0.0
	score = scorer.ScoreComprehension(context.Background(), "constraint", "action", false)
	if score != 0.0 {
		t.Errorf("circuit open not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_CircuitBreakerWrapsCall(t *testing.T) {
	// Verify that when breaker is set, happy path still works through it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := nliResponse{
			Label: "entailment",
		}
		resp.Scores.Entailment = 0.88
		resp.Scores.Contradiction = 0.06
		resp.Scores.Neutral = 0.06
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
	cb := circuitbreaker.New("test-nli-cb-happy", cbCfg)

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)
	scorer.SetCircuitBreaker(cb)

	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", true)
	if math.Abs(score-0.88) > 0.001 {
		t.Errorf("breaker happy path score = %f, want ~0.88", score)
	}
}

func TestNLIComprehensionScorer_SetCircuitBreaker(t *testing.T) {
	scorer := NewNLIComprehensionScorer("http://localhost:8000", 100, true)

	if scorer.breaker != nil {
		t.Error("breaker should be nil before SetCircuitBreaker")
	}

	cbCfg := circuitbreaker.Config{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Timeout:          15 * time.Second,
		MaxConcurrent:    1,
	}
	cb := circuitbreaker.New("test-set-nli-cb", cbCfg)
	scorer.SetCircuitBreaker(cb)

	if scorer.breaker == nil {
		t.Error("breaker should be set after SetCircuitBreaker")
	}
	if scorer.breaker.Name() != "test-set-nli-cb" {
		t.Errorf("breaker name = %q, want 'test-set-nli-cb'", scorer.breaker.Name())
	}
}

func TestNLIComprehensionScorer_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{bad json"))
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 5000, true)

	// Malformed JSON → falls back to heuristic
	score := scorer.ScoreComprehension(context.Background(), "constraint", "action", true)
	if score != 1.0 {
		t.Errorf("malformed JSON followed = %f, want 1.0", score)
	}

	score = scorer.ScoreComprehension(context.Background(), "constraint", "action", false)
	if score != 0.0 {
		t.Errorf("malformed JSON not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		resp := nliResponse{Label: "entailment"}
		resp.Scores.Entailment = 0.9
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	scorer := NewNLIComprehensionScorer(srv.URL, 10000, true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	score := scorer.ScoreComprehension(ctx, "constraint", "action", true)
	if score != 1.0 {
		t.Errorf("cancelled context followed = %f, want 1.0", score)
	}
}
