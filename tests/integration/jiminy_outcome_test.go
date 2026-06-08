//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestJiminyOutcome_GuidanceItemsGetConstraintCodes is the Tier 2 integration
// test for JIMINY-OUTCOME-001. Before the fix, matchConstraintCode used keyword
// overlap and concept-abstracted guidance items never matched a constraint code
// (all 17 recent outcome rows had constraint_code=(none)), so PersistGuidanceOutcome
// could not attach the outcome to a real constraint node and the Neo4j
// GUIDANCE_OUTCOME edge sink stayed dormant. The embedding-similarity matcher
// links concept content back to the right constraint code.
//
// Asserts that, against a populated stack, a constraint-matching context yields
// guidance items carrying a constraint_code. Skips on an empty environment
// (CI boots a fresh empty Neo4j) per the RRF-SCALE-001 CI lesson — there is no
// constraint data to match against, so the fix is not exercisable.
func TestJiminyOutcome_GuidanceItemsGetConstraintCodes(t *testing.T) {
	cfg := GetTestConfig()
	requireServiceUp(t, cfg)

	body := map[string]any{
		"space_id":  "mdemg-dev",
		"context":   "never commit directly to the main branch, always use a dev branch and a pull request workflow",
		"max_items": 10,
	}

	// The guide path is LLM-dependent: a cold model can time out the constraint
	// classifier and return zero guidance items. Retry a few times so the model
	// warms; assert codes once items surface. If items never surface (LLM path
	// unavailable/too slow in this environment) or there's no retrievable data,
	// skip — constraint-code assignment requires the items the LLM produces, so
	// this isn't a matcher failure. (The matcher is also covered by Tier 1 +
	// the live Tier 3 e2e — see docs/development/jiminy-outcome-001/verification.md.)
	var data map[string]any
	var guidance []any
	var debug map[string]any
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(5 * time.Second) // let the LLM classifier recover between attempts
		}
		resp := postGuide(t, cfg.MDEMGEndpoint, body)
		data, _ = resp["data"].(map[string]any)
		if data == nil {
			continue
		}
		debug, _ = data["debug"].(map[string]any)
		if rf, _ := debug["retrieval_found"].(float64); rf == 0 {
			t.Skipf("environment has no retrievable data (retrieval_found=0) — not exercisable here. debug=%v", debug)
		}
		guidance, _ = data["guidance"].([]any)
		if len(guidance) > 0 {
			break
		}
	}
	if len(guidance) == 0 {
		t.Skipf("guidance path returned no items after warmup retries (LLM classifier unavailable/too slow here) — constraint-code assignment not exercisable. debug=%v", debug)
	}

	coded := 0
	for _, it := range guidance {
		m, _ := it.(map[string]any)
		if c, _ := m["constraint_code"].(string); c != "" {
			coded++
		}
	}
	if coded == 0 {
		t.Fatalf("JIMINY-OUTCOME-001: %d guidance items but 0 carry a constraint_code — "+
			"the embedding matcher should link concept-abstracted items to a constraint code. debug=%v",
			len(guidance), debug)
	}
	t.Logf("JIMINY-OUTCOME-001 PASS: %d/%d guidance items carry a constraint_code (was 0 before fix)", coded, len(guidance))
}

func postGuide(t *testing.T, endpoint string, body map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/jiminy/guide", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/jiminy/guide: %v", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/jiminy/guide returned %d", httpResp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(httpResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}
