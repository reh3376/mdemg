package jiminy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
)

// TierPredictor calls the neural sidecar for ML-powered tier selection.
// Falls back to rule-based tier selection when sidecar is unavailable.
type TierPredictor struct {
	sidecarURL    string
	timeoutMs     int
	enabled       bool
	minConfidence float64
	client        *http.Client
	breaker       *circuitbreaker.Breaker
}

// TierPredictRequest is sent to the sidecar for tier prediction.
type TierPredictRequest struct {
	ConstraintText string  `json:"constraint_text"`
	AgentContext   string  `json:"agent_context"`
	TrustScore     float64 `json:"trust_score"`
}

// TierPredictResponse is returned from the sidecar.
type TierPredictResponse struct {
	PredictedTier int     `json:"predicted_tier"`
	Confidence    float64 `json:"confidence"`
	Model         string  `json:"model"`
	LatencyMs     float64 `json:"latency_ms"`
}

// NewTierPredictor creates a new tier predictor.
func NewTierPredictor(sidecarURL string, timeoutMs int, enabled bool) *TierPredictor {
	return &TierPredictor{
		sidecarURL:    sidecarURL,
		timeoutMs:     timeoutMs,
		enabled:       enabled,
		minConfidence: 0.6,
		client: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
	}
}

// SetCircuitBreaker sets a circuit breaker to wrap sidecar HTTP calls.
func (p *TierPredictor) SetCircuitBreaker(b *circuitbreaker.Breaker) {
	p.breaker = b
}

// TierPredictResult holds the full result of a tier prediction call, including
// telemetry data that callers need for metrics recording.
type TierPredictResult struct {
	Tier      int
	Conf      float64
	Err       error
	LatencyMs float64
	TimedOut  bool
}

// PredictTier returns the ML-predicted optimal tier for a constraint.
// Returns Tier=0 if prediction is unavailable or low-confidence (caller should use rule-based fallback).
// The returned TierPredictResult always includes latency and error info for metrics recording.
func (p *TierPredictor) PredictTier(ctx context.Context, constraintText, agentContext string, trustScore float64) TierPredictResult {
	if !p.enabled || p.sidecarURL == "" {
		return TierPredictResult{}
	}

	start := time.Now()
	var tier int
	var conf float64

	callFn := func(ctx context.Context) error {
		reqBody, err := json.Marshal(TierPredictRequest{
			ConstraintText: constraintText,
			AgentContext:   agentContext,
			TrustScore:     trustScore,
		})
		if err != nil {
			return err
		}

		url := strings.TrimRight(p.sidecarURL, "/") + "/protocol/predict-tier"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("sidecar returned status %d", resp.StatusCode)
		}

		var result TierPredictResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return err
		}

		tier = result.PredictedTier
		conf = result.Confidence
		return nil
	}

	var callErr error
	if p.breaker != nil {
		callErr = p.breaker.Execute(ctx, callFn)
	} else {
		callErr = callFn(ctx)
	}

	latencyMs := float64(time.Since(start).Microseconds()) / 1000.0

	if callErr != nil {
		timedOut := ctx.Err() == context.DeadlineExceeded
		if timedOut {
			slog.Warn("j17: tier predictor timed out", "latency_ms", latencyMs, "error", callErr)
		}
		return TierPredictResult{Tier: 0, Conf: 0, Err: callErr, LatencyMs: latencyMs, TimedOut: timedOut}
	}

	// Only use prediction if confidence is above threshold
	if conf < p.minConfidence {
		return TierPredictResult{Tier: 0, Conf: conf, LatencyMs: latencyMs}
	}

	if tier < 1 || tier > 3 {
		return TierPredictResult{Tier: 0, Conf: 0, LatencyMs: latencyMs}
	}

	return TierPredictResult{Tier: tier, Conf: conf, LatencyMs: latencyMs}
}

// IsAvailable returns whether the tier predictor is configured and enabled.
func (p *TierPredictor) IsAvailable() bool {
	return p.enabled && p.sidecarURL != ""
}

// String returns a human-readable description.
func (p *TierPredictor) String() string {
	if !p.enabled {
		return "TierPredictor(disabled)"
	}
	return fmt.Sprintf("TierPredictor(%s, timeout=%dms, minConf=%.2f)", p.sidecarURL, p.timeoutMs, p.minConfidence)
}
