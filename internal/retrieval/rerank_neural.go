package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mdemg/internal/circuitbreaker"
)

// NeuralRerankConfig holds config for the neural sidecar re-ranker.
type NeuralRerankConfig struct {
	URL       string // e.g. "http://localhost:8100"
	TimeoutMs int
	Fallback  string // fallback provider name if sidecar unavailable
}

type neuralRerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type neuralRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type neuralRerankResponse struct {
	Results   []neuralRerankResult `json:"results"`
	Model     string               `json:"model"`
	LatencyMs float64              `json:"latency_ms"`
}

// neuralRerank calls the Python sidecar /rerank endpoint.
// Returns scores aligned with input candidates (by original index).
func neuralRerank(ctx context.Context, cfg NeuralRerankConfig, query string, documents []string, topN int, cbReg *circuitbreaker.Registry) ([]float64, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("neural rerank URL not configured")
	}

	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 1000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	reqBody := neuralRerankRequest{
		Query:     query,
		Documents: documents,
		TopN:      topN,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal neural rerank request: %w", err)
	}

	doCall := func(ctx context.Context) ([]float64, error) {
		req, err := http.NewRequestWithContext(ctx, "POST", cfg.URL+"/rerank", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("http request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("neural sidecar error %d: %s", resp.StatusCode, string(body))
		}

		var result neuralRerankResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		// Map results back to original document order
		scores := make([]float64, len(documents))
		for _, r := range result.Results {
			if r.Index >= 0 && r.Index < len(scores) {
				scores[r.Index] = r.RelevanceScore
			}
		}
		return scores, nil
	}

	// Use circuit breaker if available
	if cbReg != nil {
		cb := cbReg.Get("neural-rerank")
		var scores []float64
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			scores, innerErr = doCall(ctx)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("neural rerank circuit breaker open")
		}
		return scores, err
	}

	return doCall(ctx)
}
