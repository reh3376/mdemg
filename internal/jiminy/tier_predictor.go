package jiminy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TierPredictor calls the neural sidecar for ML-powered tier selection.
// Falls back to rule-based tier selection when sidecar is unavailable.
type TierPredictor struct {
	sidecarURL    string
	timeoutMs     int
	enabled       bool
	minConfidence float64
	client        *http.Client
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

// PredictTier returns the ML-predicted optimal tier for a constraint.
// Returns 0 if prediction is unavailable or low-confidence (caller should use rule-based fallback).
func (p *TierPredictor) PredictTier(ctx context.Context, constraintText, agentContext string, trustScore float64) (int, float64) {
	if !p.enabled || p.sidecarURL == "" {
		return 0, 0
	}

	reqBody, err := json.Marshal(TierPredictRequest{
		ConstraintText: constraintText,
		AgentContext:   agentContext,
		TrustScore:     trustScore,
	})
	if err != nil {
		return 0, 0
	}

	url := strings.TrimRight(p.sidecarURL, "/") + "/protocol/predict-tier"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return 0, 0
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0
	}

	var result TierPredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0
	}

	// Only use prediction if confidence is above threshold
	if result.Confidence < p.minConfidence {
		return 0, result.Confidence
	}

	if result.PredictedTier < 1 || result.PredictedTier > 3 {
		return 0, 0
	}

	return result.PredictedTier, result.Confidence
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
