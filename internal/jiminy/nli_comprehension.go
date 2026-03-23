package jiminy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
)

// NLIComprehensionScorer scores constraint comprehension via the neural sidecar.
type NLIComprehensionScorer struct {
	sidecarURL string
	timeoutMs  int
	enabled    bool
	client     *http.Client
	breaker    *circuitbreaker.Breaker
}

// NewNLIComprehensionScorer creates a new NLI comprehension scorer.
func NewNLIComprehensionScorer(sidecarURL string, timeoutMs int, enabled bool) *NLIComprehensionScorer {
	return &NLIComprehensionScorer{
		sidecarURL: sidecarURL,
		timeoutMs:  timeoutMs,
		enabled:    enabled,
		client: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
	}
}

// SetCircuitBreaker sets a circuit breaker to wrap sidecar HTTP calls.
func (s *NLIComprehensionScorer) SetCircuitBreaker(b *circuitbreaker.Breaker) {
	s.breaker = b
}

type nliRequest struct {
	Premise    string `json:"premise"`
	Hypothesis string `json:"hypothesis"`
}

type nliResponse struct {
	Label  string `json:"label"` // "entailment", "contradiction", "neutral"
	Scores struct {
		Entailment    float64 `json:"entailment"`
		Contradiction float64 `json:"contradiction"`
		Neutral       float64 `json:"neutral"`
	} `json:"scores"`
}

// ScoreComprehension returns a comprehension score (0.0-1.0) for a constraint.
// Falls back to heuristic (followed=1.0, ignored=0.0) if sidecar unavailable.
func (s *NLIComprehensionScorer) ScoreComprehension(ctx context.Context,
	constraintText string, agentActionSummary string, followed bool) float64 {

	if !s.enabled || s.sidecarURL == "" {
		if followed {
			return 1.0
		}
		return 0.0
	}

	var result *nliResponse
	var callErr error

	callFn := func(ctx context.Context) error {
		r, err := s.classifyNLI(ctx, constraintText, agentActionSummary)
		if err != nil {
			return err
		}
		result = r
		return nil
	}

	if s.breaker != nil {
		callErr = s.breaker.Execute(ctx, callFn)
	} else {
		callErr = callFn(ctx)
	}

	if callErr != nil {
		if followed {
			return 1.0
		}
		return 0.0
	}

	switch result.Label {
	case "entailment":
		return result.Scores.Entailment
	case "contradiction":
		return 1.0 // understood but violated — comprehension is high
	default: // neutral
		return 0.5
	}
}

func (s *NLIComprehensionScorer) classifyNLI(ctx context.Context, premise, hypothesis string) (*nliResponse, error) {
	reqBody, err := json.Marshal(nliRequest{
		Premise:    premise,
		Hypothesis: hypothesis,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal NLI request: %w", err)
	}

	url := strings.TrimRight(s.sidecarURL, "/") + "/nli"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("create NLI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NLI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NLI returned status %d", resp.StatusCode)
	}

	var result nliResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode NLI response: %w", err)
	}
	return &result, nil
}
