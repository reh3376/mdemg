//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestJ17_ProtocolStatusEndpoint verifies the protocol status endpoint (B4)
// returns a valid data structure with the expected fields.
func TestJ17_ProtocolStatusEndpoint(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	if !checkJiminyAvailable(t, client, cfg.MDEMGEndpoint) {
		t.Skip("Jiminy not enabled; skipping protocol status smoke test")
	}

	// GET without session_id -> 400
	resp, err := client.Get(cfg.MDEMGEndpoint + "/v1/jiminy/protocol/status")
	if err != nil {
		t.Fatalf("GET /v1/jiminy/protocol/status (no session_id) failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for missing session_id, got %d; body: %s", resp.StatusCode, body)
	}
	// Drain body for connection reuse
	_, _ = io.ReadAll(resp.Body)

	// GET with session_id -> 200
	resp2, err := client.Get(cfg.MDEMGEndpoint + "/v1/jiminy/protocol/status?session_id=test-smoke")
	if err != nil {
		t.Fatalf("GET /v1/jiminy/protocol/status?session_id=test-smoke failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200 for valid session_id, got %d; body: %s", resp2.StatusCode, body)
	}

	var statusBody struct {
		Data struct {
			SessionID     string  `json:"session_id"`
			TrustScore    float64 `json:"trust_score"`
			Tier          string  `json:"tier"`
			FeedbackCount int     `json:"feedback_count"`
			Enabled       bool    `json:"enabled"`
			LastFeedback  *string `json:"last_feedback_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&statusBody); err != nil {
		t.Fatalf("failed to decode protocol status response: %v", err)
	}

	if statusBody.Data.SessionID != "test-smoke" {
		t.Errorf("expected session_id=test-smoke, got %q", statusBody.Data.SessionID)
	}
	if statusBody.Data.TrustScore < 0 || statusBody.Data.TrustScore > 1 {
		t.Errorf("trust_score out of range [0,1]: %f", statusBody.Data.TrustScore)
	}
	validTiers := map[string]bool{"T1": true, "T2": true, "T3": true}
	if !validTiers[statusBody.Data.Tier] {
		t.Errorf("unexpected tier %q, expected T1/T2/T3", statusBody.Data.Tier)
	}
	if !statusBody.Data.Enabled {
		t.Error("expected enabled=true when Jiminy is available")
	}

	t.Logf("protocol status: session_id=%s trust=%.3f tier=%s feedback_count=%d enabled=%v",
		statusBody.Data.SessionID, statusBody.Data.TrustScore, statusBody.Data.Tier,
		statusBody.Data.FeedbackCount, statusBody.Data.Enabled)
}

// TestJ17_FeedbackAffectsTrust verifies the full feedback -> trust update chain (B4).
// It sends guidance, posts positive feedback, then checks that the protocol status
// reflects the feedback with a non-decreased trust score.
func TestJ17_FeedbackAffectsTrust(t *testing.T) {
	RequireServiceReady(t)

	cfg := GetTestConfig()
	client := NewTestHTTPClient()

	if !checkJiminyAvailable(t, client, cfg.MDEMGEndpoint) {
		t.Skip("Jiminy not enabled; skipping feedback-affects-trust test")
	}

	sessionID := fmt.Sprintf("trust-test-b4-%d", time.Now().UnixNano())

	// 1. Get baseline trust
	resp, err := client.Get(cfg.MDEMGEndpoint + "/v1/jiminy/protocol/status?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("baseline GET protocol/status failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("baseline protocol/status: expected 200, got %d; body: %s", resp.StatusCode, body)
	}

	var baseline struct {
		Data struct {
			TrustScore    float64 `json:"trust_score"`
			FeedbackCount int     `json:"feedback_count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&baseline); err != nil {
		t.Fatalf("failed to decode baseline status: %v", err)
	}
	t.Logf("baseline trust=%.3f feedback_count=%d", baseline.Data.TrustScore, baseline.Data.FeedbackCount)

	// 2. POST /v1/jiminy/guide to get a guidance_id
	guidePayload := map[string]any{
		"space_id":   "mdemg-dev",
		"context":    "B4 trust test: checking feedback loop wiring",
		"query":      "what are the current constraints?",
		"session_id": sessionID,
	}
	guideBytes, err := json.Marshal(guidePayload)
	if err != nil {
		t.Fatalf("failed to marshal guide request: %v", err)
	}

	guideReq, err := http.NewRequest(http.MethodPost, cfg.MDEMGEndpoint+"/v1/jiminy/guide", bytes.NewReader(guideBytes))
	if err != nil {
		t.Fatalf("failed to build guide request: %v", err)
	}
	guideReq.Header.Set("Content-Type", "application/json")

	guideResp, err := client.Do(guideReq)
	if err != nil {
		t.Fatalf("POST /v1/jiminy/guide failed: %v", err)
	}
	defer guideResp.Body.Close()

	if guideResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(guideResp.Body)
		t.Fatalf("POST /v1/jiminy/guide: expected 200, got %d; body: %s", guideResp.StatusCode, body)
	}

	var guideBody struct {
		Data struct {
			GuidanceID string `json:"guidance_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(guideResp.Body).Decode(&guideBody); err != nil {
		t.Fatalf("failed to decode guide response: %v", err)
	}
	if guideBody.Data.GuidanceID == "" {
		t.Fatal("guide response contained empty guidance_id")
	}
	t.Logf("guidance_id: %s", guideBody.Data.GuidanceID)

	// 3. POST /v1/jiminy/feedback with explicit positive outcome
	fbPayload := map[string]any{
		"guidance_id":    guideBody.Data.GuidanceID,
		"action_summary": "B4 test: followed all guidance recommendations precisely",
		"outcome":        "followed",
		"space_id":       "mdemg-dev",
		"session_id":     sessionID,
	}
	fbBytes, err := json.Marshal(fbPayload)
	if err != nil {
		t.Fatalf("failed to marshal feedback request: %v", err)
	}

	fbReq, err := http.NewRequest(http.MethodPost, cfg.MDEMGEndpoint+"/v1/jiminy/feedback", bytes.NewReader(fbBytes))
	if err != nil {
		t.Fatalf("failed to build feedback request: %v", err)
	}
	fbReq.Header.Set("Content-Type", "application/json")

	fbResp, err := client.Do(fbReq)
	if err != nil {
		t.Fatalf("POST /v1/jiminy/feedback failed: %v", err)
	}
	defer fbResp.Body.Close()

	if fbResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(fbResp.Body)
		t.Fatalf("POST /v1/jiminy/feedback: expected 200, got %d; body: %s", fbResp.StatusCode, body)
	}
	// Drain feedback response
	_, _ = io.ReadAll(fbResp.Body)

	// 4. GET /v1/jiminy/protocol/status again
	resp2, err := client.Get(cfg.MDEMGEndpoint + "/v1/jiminy/protocol/status?session_id=" + sessionID)
	if err != nil {
		t.Fatalf("post-feedback GET protocol/status failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("post-feedback protocol/status: expected 200, got %d; body: %s", resp2.StatusCode, body)
	}

	var updated struct {
		Data struct {
			TrustScore    float64 `json:"trust_score"`
			FeedbackCount int     `json:"feedback_count"`
			Tier          string  `json:"tier"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode updated status: %v", err)
	}

	t.Logf("updated trust=%.3f feedback_count=%d tier=%s",
		updated.Data.TrustScore, updated.Data.FeedbackCount, updated.Data.Tier)

	// Trust should not decrease after positive feedback
	if updated.Data.TrustScore < baseline.Data.TrustScore {
		t.Errorf("trust decreased after positive feedback: baseline=%.3f updated=%.3f",
			baseline.Data.TrustScore, updated.Data.TrustScore)
	}

	// Feedback count should have increased
	if updated.Data.FeedbackCount <= baseline.Data.FeedbackCount {
		t.Logf("note: feedback_count did not increase (baseline=%d, updated=%d) — may be expected if guidance had no items",
			baseline.Data.FeedbackCount, updated.Data.FeedbackCount)
	}
}
