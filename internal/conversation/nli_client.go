package conversation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NLIResult holds the classification result from the sidecar.
type NLIResult struct {
	Label  string // "entailment", "contradiction", "neutral"
	Scores struct {
		Entailment    float64
		Contradiction float64
		Neutral       float64
	}
}

// classifyNLI calls the sidecar POST /nli endpoint.
func classifyNLI(ctx context.Context, sidecarURL, premise, hypothesis string, timeoutMs int) (*NLIResult, error) {
	if sidecarURL == "" {
		return nil, fmt.Errorf("NLI sidecar URL not configured")
	}

	if timeoutMs <= 0 {
		timeoutMs = 1000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	reqBody, err := json.Marshal(map[string]string{
		"premise":    premise,
		"hypothesis": hypothesis,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal NLI request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", sidecarURL+"/nli", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create NLI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NLI http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NLI sidecar error %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Label  string `json:"label"`
		Scores struct {
			Entailment    float64 `json:"entailment"`
			Contradiction float64 `json:"contradiction"`
			Neutral       float64 `json:"neutral"`
		} `json:"scores"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode NLI response: %w", err)
	}

	result := &NLIResult{Label: raw.Label}
	result.Scores.Entailment = raw.Scores.Entailment
	result.Scores.Contradiction = raw.Scores.Contradiction
	result.Scores.Neutral = raw.Scores.Neutral
	return result, nil
}
