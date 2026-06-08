//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestGuidanceSynth_WarmPathSynthesisSucceeds is the Tier 2 integration test for
// GUIDANCE-SYNTH-001. Before the fix, the warm-path background Guide() ran with a
// hardcoded 30s timeout and the per-node constraint classifier ran serially, so
// guidance synthesis deadline-exceeded on every warm call (`synthesis_error`).
// After: parallel classifier + a 90s config-driven warm-compute budget let
// synthesis complete.
//
// Asserts that, against a populated stack, the warm→latest cycle produces
// guidance WITHOUT a synthesis_error (and ideally synthesis_used=true). Skips on
// an empty/LLM-unavailable environment (CI) per the prior sprints' CI lesson —
// synthesis requires the LLM + retrievable data.
func TestGuidanceSynth_WarmPathSynthesisSucceeds(t *testing.T) {
	cfg := GetTestConfig()
	requireServiceUp(t, cfg)

	// Trigger the production warm path.
	warmBody, _ := json.Marshal(map[string]any{
		"space_id":     "mdemg-dev",
		"context_hint": "never commit directly to the main branch, always use a dev branch and a pull request workflow",
		"session_id":   "gs-integration",
	})
	warmReq, _ := http.NewRequest(http.MethodPost, cfg.MDEMGEndpoint+"/v1/jiminy/warm", bytes.NewReader(warmBody))
	warmReq.Header.Set("Content-Type", "application/json")
	wc := &http.Client{Timeout: 15 * time.Second}
	wr, err := wc.Do(warmReq)
	if err != nil {
		t.Skipf("warm endpoint unreachable: %v", err)
	}
	_ = wr.Body.Close()

	// Poll /latest until guidance is computed (background Guide can take up to the
	// configured warm-compute timeout). Skip if it never populates (LLM path
	// unavailable / empty env).
	deadline := time.Now().Add(110 * time.Second)
	var data map[string]any
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		latest := getLatest(t, cfg.MDEMGEndpoint)
		d, _ := latest["data"].(map[string]any)
		if d == nil {
			continue
		}
		debug, _ := d["debug"].(map[string]any)
		// Only meaningful once guidance items exist (synthesis runs on items).
		if g, _ := d["guidance"].([]any); len(g) > 0 {
			data = d
			_ = debug
			break
		}
	}
	if data == nil {
		t.Skip("warm path did not produce guidance items in this environment (LLM unavailable/empty) — synthesis not exercisable")
	}

	debug, _ := data["debug"].(map[string]any)
	if se, ok := debug["synthesis_error"]; ok {
		t.Fatalf("GUIDANCE-SYNTH-001: synthesis still failing on the warm path: %v", se)
	}
	t.Logf("GUIDANCE-SYNTH-001 PASS: warm-path guidance produced without synthesis_error (synthesis_used=%v)", debug["synthesis_used"])
}

func getLatest(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, endpoint+"/v1/jiminy/latest?space_id=mdemg-dev", nil)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil
	}
	return out
}
