package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ResultClient stores drift check results as observations via MDEMG API.
type ResultClient struct {
	endpoint string
	spaceID  string
	client   *http.Client
}

func NewResultClient(endpoint, spaceID string) *ResultClient {
	return &ResultClient{
		endpoint: endpoint,
		spaceID:  spaceID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type observeRequest struct {
	SpaceID   string `json:"space_id"`
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	ObsType   string `json:"obs_type"`
}

// StoreResult posts a test/drift result as an observation.
func (rc *ResultClient) StoreResult(content, obsType string) error {
	body := observeRequest{
		SpaceID:   rc.spaceID,
		SessionID: "uxts-module",
		Content:   content,
		ObsType:   obsType,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal observe request: %w", err)
	}

	url := rc.endpoint + "/v1/conversation/observe"
	resp, err := rc.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("POST observe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("observe returned status %d", resp.StatusCode)
	}

	return nil
}

// StoreDriftResults stores drift detection results for a set of specs.
func (rc *ResultClient) StoreDriftResults(framework string, drifted []string, clean int) {
	if len(drifted) == 0 {
		return
	}

	content := fmt.Sprintf("UxTS drift detected in %s framework: %d specs drifted (%s), %d clean",
		framework, len(drifted), joinMax(drifted, 5), clean)

	if err := rc.StoreResult(content, "error"); err != nil {
		slog.Error("failed to store drift result", "module", moduleID, "error", err)
	}
}

func joinMax(items []string, max int) string {
	if len(items) <= max {
		return fmt.Sprintf("%v", items)
	}
	shown := items[:max]
	return fmt.Sprintf("%v and %d more", shown, len(items)-max)
}
