//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestRRFScale_SuggestSurfacesGuidance is the Tier 2 integration test for
// RRF-SCALE-001. Before the fix, the consulting suggestion/constraint path
// gated on hardcoded legacy-scale score thresholds (r.Score < 0.55 etc.); RRF
// default-on dropped the score scale so every result was rejected and the path
// returned empty. This test asserts that against the live stack + real
// mdemg-dev data, a constraint-matching context now surfaces guidance
// (suggestions and/or constraints) — i.e. the path is no longer a no-op.
//
// It deliberately asserts on the gate-controlled outputs (suggestions/
// constraints/conflicts counts) rather than exact items, so it is robust to
// LLM-classifier warmth (the LLM constraint classifier needs a warm model;
// the suggestion path is gate-only and deterministic).
func TestRRFScale_SuggestSurfacesGuidance(t *testing.T) {
	cfg := GetTestConfig()
	requireServiceUp(t, cfg)

	const spaceID = "mdemg-dev" // production space with 111 constraint nodes

	body := map[string]any{
		"space_id":            spaceID,
		"context":             "never commit directly to the main branch, always use a dev branch and a pull request workflow",
		"max_suggestions":     10,
		"min_confidence":      0.2, // below the RRF strong-match band so the pre-filter doesn't mask the gate fix
		"include_constraints": true,
		"include_conflicts":   true,
	}
	resp := postSuggest(t, cfg.MDEMGEndpoint, body)

	suggestions, _ := resp["suggestions"].([]any)
	constraints, _ := resp["constraints"].([]any)
	debug, _ := resp["debug"].(map[string]any)

	total := len(suggestions) + len(constraints)
	if total == 0 {
		t.Fatalf("RRF-SCALE-001: consulting path returned 0 suggestions + 0 constraints for a constraint-matching context — "+
			"the score-gate fix should make this non-empty. debug=%v", debug)
	}
	t.Logf("RRF-SCALE-001 PASS: %d suggestions + %d constraints surfaced (was 0 before fix). debug=%v",
		len(suggestions), len(constraints), debug)
}

// TestRRFScale_SuggestRejectsNoise asserts the fix did not over-correct into a
// false-positive flood: a context with no plausible match should not surface a
// large pile of constraints. (Tolerant: we only guard against a flood, since
// retrieval may still return a few weakly-related concepts.)
func TestRRFScale_SuggestRejectsNoise(t *testing.T) {
	cfg := GetTestConfig()
	requireServiceUp(t, cfg)

	body := map[string]any{
		"space_id":            "mdemg-dev",
		"context":             "zxqwv plorbnacle frumious bandersnatch quibble zorptang",
		"max_suggestions":     10,
		"min_confidence":      0.2,
		"include_constraints": true,
	}
	resp := postSuggest(t, cfg.MDEMGEndpoint, body)
	constraints, _ := resp["constraints"].([]any)
	if len(constraints) > 5 {
		t.Errorf("gibberish context surfaced %d constraints — gate may be over-corrected (false-positive flood)", len(constraints))
	}
}

func postSuggest(t *testing.T, endpoint string, body map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/memory/suggest", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/memory/suggest: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/memory/suggest returned %d", httpResp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(httpResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func requireServiceUp(t *testing.T, cfg TestConfig) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	r, err := client.Get(cfg.MDEMGEndpoint + "/healthz")
	if err != nil {
		t.Skipf("MDEMG service not available at %s: %v", cfg.MDEMGEndpoint, err)
	}
	_ = r.Body.Close()
}
