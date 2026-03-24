//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// TestRSICCycle_IncludesSynergyHealth verifies that the RSIC self-assessment
// cycle includes the SynergyHealth dimension in its response. This ensures
// the Claude Code ↔ MDEMG token optimization health is surfaced by the assessor.
func TestRSICCycle_IncludesSynergyHealth(t *testing.T) {
	endpoint := os.Getenv("MDEMG_URL")
	if endpoint == "" {
		endpoint = "http://localhost:9999"
	}

	// Healthz check — skip if server is not running.
	client := NewTestHTTPClient()
	healthResp, err := client.Get(endpoint + "/healthz")
	if err != nil {
		t.Skip("MDEMG server not running")
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Skip("MDEMG server not running")
	}

	// Call POST /v1/self-improve/assess with space_id=mdemg-dev
	body := map[string]any{
		"space_id": "mdemg-dev",
		"tier":     "micro",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal assess request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/self-improve/assess", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create assess request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("assess request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assess returned status %d, expected 200", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode assess response: %v", err)
	}

	// Verify synergy_health exists and is a float64 in [0, 1]
	synergyRaw, ok := result["synergy_health"]
	if !ok {
		t.Fatal("synergy_health field missing from assess response")
	}
	synergyHealth, ok := synergyRaw.(float64)
	if !ok {
		t.Fatalf("synergy_health is not a float64, got %T", synergyRaw)
	}
	if synergyHealth < 0 || synergyHealth > 1 {
		t.Errorf("synergy_health out of range [0, 1]: %f", synergyHealth)
	}
	t.Logf("synergy_health: %f", synergyHealth)

	// Verify synergy_lines_claude exists
	if _, ok := result["synergy_lines_claude"]; !ok {
		t.Error("synergy_lines_claude field missing from assess response")
	} else {
		t.Logf("synergy_lines_claude: %v", result["synergy_lines_claude"])
	}

	// Verify synergy_lines_memory exists
	if _, ok := result["synergy_lines_memory"]; !ok {
		t.Error("synergy_lines_memory field missing from assess response")
	} else {
		t.Logf("synergy_lines_memory: %v", result["synergy_lines_memory"])
	}

	// Verify jiminy_healthy exists
	if _, ok := result["jiminy_healthy"]; !ok {
		t.Error("jiminy_healthy field missing from assess response")
	} else {
		t.Logf("jiminy_healthy: %v", result["jiminy_healthy"])
	}
}
