//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestGuardrailValidate_CapturesInteractionInTSDB is Sprint FT-LORA-B's Tier 3
// E2E evidence: POST /v1/memory/guardrail/validate must land a row in the
// llm_interactions hypertable tagged task_name='guardrail.evaluate'.
//
// Prerequisites (test is skipped if not satisfied):
//   - Live MDEMG server reachable at TEST_MDEMG_ENDPOINT or http://localhost:9999
//   - Server started with GUARDRAIL_ENABLED=true, LLM_INTERACTION_LOGGING=true,
//     and a usable LLM provider (OPENAI_API_KEY or OLLAMA_URL)
//   - TSDB reachable via TEST_TSDB_* env (see tsdb_test.go)
//   - TEST_GUARDRAIL_LIVE=1 (explicit opt-in — avoids flakiness in default runs)
//
// Uses dedicated space_id="mdemg-test-guardrail"; cleans up via
// `mdemg data clean --space-id mdemg-test-guardrail --force` on teardown
// (runner responsibility; this test does not shell out).
func TestGuardrailValidate_CapturesInteractionInTSDB(t *testing.T) {
	if os.Getenv("TEST_GUARDRAIL_LIVE") != "1" {
		t.Skip("TEST_GUARDRAIL_LIVE != 1, skipping live guardrail TSDB-capture test")
	}
	skipIfNoTSDB(t)

	cfg := GetTestConfig()
	const spaceID = "mdemg-test-guardrail"
	const taskName = "guardrail.evaluate"

	// Baseline: count rows for this task before the call.
	pool := newTestClient(t).Pool()
	before := countGuardrailRows(t, pool, spaceID, taskName)

	// POST a diff with a clearly-bad pattern (plaintext password) so the LLM
	// has a reason to emit a violation. The test does NOT assert on the
	// response body beyond 200 OK — only on the TSDB side-effect.
	body := map[string]any{
		"space_id":      spaceID,
		"files_changed": []string{"main.go"},
		"diff": "--- a/main.go\n+++ b/main.go\n@@\n+password := \"hunter2\"\n",
	}
	buf, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		cfg.MDEMGEndpoint+"/v1/memory/guardrail/validate",
		bytes.NewReader(buf),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("TEST_API_KEY"); key != "" {
		req.Header.Set("X-API-Key", key)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/memory/guardrail/validate: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("guardrail validate: status %d, want 200", resp.StatusCode)
	}

	// TSDB flush is asynchronous (batched by llm_writer). Poll up to 10s for
	// at least one new row to appear.
	deadline := time.Now().Add(10 * time.Second)
	var after int
	for time.Now().Before(deadline) {
		after = countGuardrailRows(t, pool, spaceID, taskName)
		if after > before {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if after <= before {
		t.Fatalf("no new llm_interactions row for task_name=%q space_id=%q after 10s (before=%d, after=%d)",
			taskName, spaceID, before, after)
	}

	// Verify the latest row has provider + model populated (not empty).
	row := latestGuardrailRow(t, pool, spaceID, taskName)
	if row.Provider == "" {
		t.Errorf("llm_interactions.provider empty; expected openai or ollama")
	}
	if row.ModelName == "" {
		t.Errorf("llm_interactions.model_name empty")
	}
	if row.TokensIn == 0 && row.TokensOut == 0 {
		t.Errorf("both tokens_in and tokens_out are 0; token accounting not wired")
	}
}

// guardrailRow is a subset of llm_interactions columns used for assertions.
type guardrailRow struct {
	Provider  string
	ModelName string
	TokensIn  int
	TokensOut int
}

func countGuardrailRows(t *testing.T, pool *pgxpool.Pool, spaceID, taskName string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM llm_interactions WHERE space_id = $1 AND task_name = $2",
		spaceID, taskName,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count llm_interactions: %v", err)
	}
	return n
}

func latestGuardrailRow(t *testing.T, pool *pgxpool.Pool, spaceID, taskName string) guardrailRow {
	t.Helper()
	var r guardrailRow
	err := pool.QueryRow(context.Background(),
		`SELECT provider, model_name, COALESCE(tokens_in, 0), COALESCE(tokens_out, 0)
		 FROM llm_interactions
		 WHERE space_id = $1 AND task_name = $2
		 ORDER BY time DESC LIMIT 1`,
		spaceID, taskName,
	).Scan(&r.Provider, &r.ModelName, &r.TokensIn, &r.TokensOut)
	if err != nil {
		t.Fatalf("select latest llm_interactions row: %v", err)
	}
	return r
}

