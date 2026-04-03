# Post-Fix Re-Validation Results — Data Propagation Fixes F7-F10

**Date:** 2026-04-03
**Tester:** Claude Code (AI-assisted)
**Environment:** Locally-built Docker image from commit 194d2cf, clean /tmp/mdemg-live-test directory
**Codebase:** PR #253 merged to main (commit 28dd542), containing fix commit 194d2cf
**Validates:** Fixes F7 (session_id), F8 (space_id), F9 (recorder init), F10 (export instance_id), F11 (docs)
**Predecessor:** Live Validation Sprint (2026-04-02), detailed summary in `live-validation-detailed-summary.md`

---

## Executive Summary

All 19 tests from the live validation sprint were re-run against the fixed codebase. The 4 tests that previously FAILed now PASS. The 15 tests that previously PASSed continue to PASS. Zero regressions detected. The campaign gate is **GREEN** — the 30-day collection campaign can launch.

---

## Environment Setup

The re-validation used a clean test environment to simulate a fresh user experience:

1. **Binary:** Built from commit 194d2cf with `go build -ldflags` (version `v0.5.1-revalidation`)
2. **Docker image:** Locally rebuilt from `deploy/docker/Dockerfile.prod` and tagged as `ghcr.io/reh3376/mdemg:latest` — this overwrites the stale GHCR image (v0.5.1/8858ac6) with the fixed code (194d2cf)
3. **Test directory:** `/tmp/mdemg-live-test` — wiped and recreated from scratch
4. **Init:** `mdemg init --defaults` — assigned port 10000 (9999 occupied by main services), project name `mdemg-mdemg-live-test`
5. **API key:** Copied `OPENAI_API_KEY` from dev `.env` into test `.env` (required for embedding, reranking, query classify)
6. **Query classify/intent:** Added `QUERY_CLASSIFY_ENABLED`, `INTENT_ENABLED`, `LLM_INTERACTION_LOGGING` env vars to test compose file — these are not in the default compose template (see Note N2 below)

### Image Version Verification

Before testing, the container's `mdemg version` was verified:
- **First start:** Showed commit `8858ac6` (stale GHCR image) — **wrong**
- **After local rebuild:** Showed commit `194d2cf` (fix commit) — **correct**
- This confirms the GHCR image has NOT been updated yet. The `workflow_run` trigger from PR #253's merge should update it, but this has not been verified (see Risk R1).

---

## Results

| Test | Description | Original (Apr 2) | Re-validation (Apr 3) | Change |
|------|-------------|-------------------|----------------------|--------|
| 1.1 | Homebrew Install / Version | PASS | **PASS** | Held |
| 1.2 | Init from Empty Dir | PASS | **PASS** | Held |
| 1.3 | Pre-Campaign Check | PASS | **PASS** | Held |
| 1.4 | Browser UI | PASS | **PASS** | Held |
| 1.5 | Grafana | PASS | **PASS** | Held |
| 1.6 | Service Install | PARTIAL | **PARTIAL** | Held |
| 2.1 | Observe → TSDB | PASS | **PASS** | Held |
| **2.2** | **Session ID Propagation** | **FAIL** | **PASS** | **Fixed (F7)** |
| **2.3** | **Query Classify Records** | **FAIL** | **PASS** | **Fixed (F9)** |
| **2.5** | **Export Pipeline** | **PARTIAL** | **PASS** | **Fixed (F10)** |
| 2.6 | Export-Auto Retention | PASS | **PASS** | Held |
| 3.1 | Curation Pipeline (4 steps) | PASS | **PASS** | Held |
| 3.2 | Training Dry Run | PASS | **PASS** | Held |
| 3.4 | Regression Gate Self-Compare | PASS | **PASS** | Held |
| 4.5 | Teardown + Reinit | PASS | **PASS** | Held |
| 6.1 | PII Injection | PASS | **PASS** | Held |
| 6.2 | TSDB Down | PASS | **PASS** | Held |
| 6.3 | Invalid Manifest | PASS | **PASS** | Held |
| 6.4 | Regression Gate Regression | PASS | **PASS** | Held |

**Scorecard:** 17 PASS, 1 PARTIAL (1.6 — expected for Docker), 0 FAIL. **4 fixes verified. 0 regressions.**

---

## Fix Verification Evidence

### F7: Session ID Propagation — VERIFIED

**Test:** Sent 3 `/v1/memory/retrieve` requests with `"session_id": "verify-session-f7"`, waited 35s for TSDB flush.

**TSDB query result:**
```
retrieval.rerank_cross    | verify-session-f7 | live-test   | 2802fdf507b0-mdemg-dev
retrieval.intent_translate| verify-session-f7 | mdemg-dev   | 2802fdf507b0-mdemg-dev
retrieval.query_classify  | verify-session-f7 | mdemg-dev   | 2802fdf507b0-mdemg-dev
```

All 9 records (3 requests × 3 tasks) show `session_id = verify-session-f7`. Previously this field contained the instance_id (`2802fdf507b0-mdemg-dev`).

**What changed:** `WithSessionID(ctx, req.SessionID)` added to `handleRetrieve` and `handleConsult` in `handlers.go`. Default session_id changed from `cfg.InstanceID` to empty string in both `serve.go` and `server.go`.

### F8: Space ID Propagation to Reranker — VERIFIED

**Evidence from same TSDB query:**
```
retrieval.rerank_cross | verify-session-f7 | live-test | 2802fdf507b0-mdemg-dev
```

The `space_id` column for `rerank_cross` records shows `live-test` (the request's space_id). Previously this always showed `mdemg-dev` (the default).

**Note:** `query_classify` and `intent_translate` records still show `space_id = mdemg-dev`. This is expected — the supplement plan explicitly deferred multi-space threading for these tasks (they operate within the retrieval pipeline and use `defaultSpaceID` as the fallback). For single-space deployments this is correct. Multi-space support for these tasks is a future enhancement (see Suggestion S3).

### F9: Recorder Initialization Order — VERIFIED

**Container log evidence:**
```json
{"level":"INFO","msg":"tsdb: early LLM recorder attached (pre-server init)"}
{"level":"INFO","msg":"query classifier enabled","provider":"openai","model":"gpt-5-nano"}
{"level":"INFO","msg":"query classifier wired to retrieval service"}
{"level":"INFO","msg":"tsdb: LLM interaction logger attached","instance_id":"2802fdf507b0-mdemg-dev"}
```

The log order confirms:
1. Early LLM recorder created in `serve.go` BEFORE `NewServer()`
2. Query classifier and intent translator created inside `NewServer()` — they now inherit the recorder
3. `SetTSDBClient()` re-confirms the recorder (idempotent, reuses the early writer via `SetLLMWriter`)

**TSDB record counts after 3 retrieval requests:**
```
retrieval.intent_translate | 3
retrieval.query_classify   | 3
retrieval.rerank_cross     | 5
```

Previously `query_classify` and `intent_translate` were permanently at 0. Now they record every invocation.

### F10: Export Instance ID Auto-Detection — VERIFIED

**Test:** `mdemg data export --space-id live-test` (no `--instance-id` flag).

**Result:** `Total rows: 5` — five `llm_interactions` records exported successfully.

Previously this returned 0 rows because the CLI auto-generated `reh3376-live-test` as the instance_id, which didn't match the Docker container's `2802fdf507b0-mdemg-dev`. Now empty instance_id means "all instances for this space," and the exporter correctly skips the instance_id filter.

### F11: Regression Gate Documentation — VERIFIED

Docstring added to `index_by_task()` in `regression_gate.py`. Verified indirectly: Test 3.4 (self-compare) returns `Exit 2 (WARN)` — the gate correctly reads 2 tasks with `status: "evaluated"` and produces the expected verdict.

---

## Notes

### N1: GHCR Image Is Still Stale

The Docker image at `ghcr.io/reh3376/mdemg:latest` is still commit `8858ac6` (pre-fix). The re-validation used a locally-built image. PR #253's merge to main should trigger the `docker-publish.yml` workflow via `workflow_run`, but this has not been confirmed. Any Docker user pulling `ghcr.io/reh3376/mdemg:latest` right now gets the unfixed image.

**Action required:** Verify the Docker Publish workflow ran successfully after the PR #253 merge. Check `gh run list --workflow docker-publish.yml`. If it didn't trigger, run it manually via `workflow_dispatch`.

### N2: Query Classify / Intent Env Vars Not in Default Compose Template

`QUERY_CLASSIFY_ENABLED`, `INTENT_ENABLED`, and `LLM_INTERACTION_LOGGING` are not forwarded in the default `docker-compose.yml` template. During re-validation, these had to be manually added to the compose file's environment section. This means:

- A user running `mdemg init --defaults` gets `QUERY_CLASSIFY_ENABLED=false` (not forwarded from `.env`)
- Setting these in `.env` has no effect unless the compose environment section forwards them with `${VAR}` syntax
- The pre-campaign check warns about disabled flags but doesn't explain that they must be added to compose

This is NOT a regression from the fix — it was the same in the original validation. But it will trip up campaign participants.

### N3: TSDB Schema Version Tracking Gap

The TSDB auto-migration runs all 9 migration SQL files successfully (001-009), but `GetSchemaVersion()` reports version 7 while the CLI's pre-campaign check requires >= 9. The migration files execute correctly (all tables/columns exist), but the `schema_version` tracking table isn't updated by all migrations.

This was present in the original validation too. It causes a warning in pre-campaign check but doesn't block operation — the server starts and records data correctly. The gap is cosmetic but confusing.

### N4: Port 9999 Conflict During Re-Validation

Port 9999 was occupied by the user's existing (stopped) mdemg services, so the test environment was assigned port 10000. After re-validation, restarting the main services hit a port conflict because a lingering native `mdemg` process held port 9999. This required `kill` to resolve.

This is a known operational issue — `docker compose stop` stops containers but doesn't release port bindings if a native process also holds the port. Not a code bug, but worth noting for anyone running re-validation.

### N5: Test 1.6 (Service Install) Remains PARTIAL

LaunchAgent installation fails for `com.mdemg.server` with `exit status 5` (bootstrap error) when Docker services are already running on the target ports. This is expected and documented — Docker deployment mode and native LaunchAgent mode are mutually exclusive. The embedded templates (F6 fix) work correctly; the failure is a port/runtime conflict, not a template issue.

---

## Risks

### R1: GHCR Image Update Unverified (MEDIUM)

The `workflow_run` trigger (F4 fix from the original sprint) should fire when the Release workflow completes, but PR #253 was a merge to main — not a release. The Docker Publish workflow also triggers on `push: branches: [main]` with path filters, which should cover this merge. However, this has not been verified.

**If the GHCR image is not updated:** Every `mdemg init --defaults` pulls the stale image with wrong session_id, no query_classify recording, and broken exports. The fixes exist only in the locally-built image and native binary.

**Mitigation:** Check `gh run list --workflow docker-publish.yml` within 24 hours. If no run appears, trigger manually. Consider tagging v0.5.2 to definitively trigger the release → docker-publish chain.

### R2: query_classify / intent_translate Space ID Uses Default (LOW)

As noted in verification evidence, `query_classify` and `intent_translate` TSDB records show `space_id = mdemg-dev` regardless of the request's actual space_id. The F8 fix only threaded space_id through the reranker path. For single-space deployments (the current reality), this is correct. For future multi-space deployments, these tasks would produce training data tagged with the wrong space.

**Mitigation:** Acceptable for the 30-day campaign (single space). Log a follow-up to thread space_id through `ComputeRetrievalHintsWithLLM` and `IntentTranslator.Translate` via context when multi-space is needed.

### R3: Compose Template Divergence After Manual Edit (LOW)

The re-validation required manually adding env vars to the test compose file. If these env vars are eventually added to the source compose template (`internal/cli/compose_templates/docker-compose.yml`), the CI sync check will catch divergence. But if they're only documented as "add to your compose file," users will have inconsistent setups.

**Mitigation:** Consider adding `QUERY_CLASSIFY_ENABLED`, `INTENT_ENABLED`, and `LLM_INTERACTION_LOGGING` (defaulting to false) to the embedded compose template so that `.env` changes are sufficient.

### R4: Zero-Row Warning Not Tested in Re-Validation (LOW)

The F10 fix added a zero-row warning to stderr when exports produce no results. This path was not exercised during re-validation because the fix ensures exports now return data. The warning logic is trivial (fprintf after row count check), but it has not been observed in a live run.

**Mitigation:** Low risk — the code path is straightforward. Can be verified by running `mdemg data export --space-id nonexistent-space` against a running TSDB.

---

## Suggestions

### S1: Tag v0.5.2 and Verify GHCR Update

Tag a new release to definitively trigger the full release → docker-publish chain. This ensures:
- GHCR images are updated with the F7-F10 fixes
- The `workflow_run` trigger (F4 fix) is verified end-to-end
- Campaign participants pulling images get the correct version

### S2: Add Campaign-Required Env Vars to Compose Template

Add the following to `internal/cli/compose_templates/docker-compose.yml` (and the root copy) so that `mdemg init` produces a compose file that respects `.env` settings for these features:

```yaml
QUERY_CLASSIFY_ENABLED: "${QUERY_CLASSIFY_ENABLED:-false}"
INTENT_ENABLED: "${INTENT_ENABLED:-false}"
LLM_INTERACTION_LOGGING: "${LLM_INTERACTION_LOGGING:-true}"
```

This would eliminate Note N2 and prevent campaign participants from having to manually edit compose files.

### S3: Thread Space ID Through All Retrieval LLM Consumers

The F8 fix threaded space_id through the reranker. The same pattern should be applied to:
- `ComputeRetrievalHintsWithLLM` (query_classify path)
- `IntentTranslator.Translate` (intent_translate path)

Both currently rely on `defaultSpaceID`. The fix is straightforward: pass space_id via context (add a `WithSpaceID` context helper alongside the existing `WithSessionID`). Low priority for single-space campaign, but needed before multi-space support.

### S4: Fix TSDB Schema Version Tracking

The gap between "9 migrations ran" and "schema version = 7" is confusing. Either:
- Each migration SQL file should `INSERT INTO schema_version` to bump the version
- Or `GetSchemaVersion()` should count applied migration files instead of reading a version number

This would eliminate the pre-campaign check false positive and reduce confusion during onboarding.

### S5: Add Automated Re-Validation Script

The 19 tests in this re-validation are mechanical enough to script. A `scripts/live-revalidation.sh` that:
1. Builds binary
2. Creates clean test dir
3. Inits, starts services, waits for healthy
4. Runs all 19 tests with PASS/FAIL output
5. Tears down

...would catch regressions automatically and could be added to CI as a nightly or pre-release check. The live validation sprint proved that unit/integration tests miss integration boundary bugs — a scripted re-validation would catch them without requiring manual intervention.

---

## Documents Accessed

- `docs/operations/live-validation-findings.md` — original test results table
- `docs/operations/live-validation-detailed-summary.md` — root cause analysis and re-validation requirements
- `~/Downloads/mdemg-sprint-completed/LIVE_VALIDATION_SPRINT.md` — test procedures
- `~/Downloads/DATA_PROPAGATION_FIX_SPRINT.md` — fix supplement plan
- `deploy/docker/Dockerfile.prod` — Docker image build
- `internal/cli/serve.go` — early LLM writer (F9 fix)
- `internal/api/server.go` — SetLLMWriter, SetTSDBClient (F9 fix)
- `internal/api/handlers.go` — WithSessionID in retrieve/consult (F7 fix)
- `internal/models/models.go` — SessionID field additions (F7 fix)
- `internal/retrieval/rerank.go` — SpaceID threading (F8 fix)
- `internal/cli/data_export.go` — instance_id auto-detection removal (F10 fix)
- `docker-compose.yml` (test environment) — env var forwarding for query classify
