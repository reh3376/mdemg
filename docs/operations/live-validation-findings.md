# Live Validation Session 1-6: Findings
**Date:** 2026-04-02
**Tester:** Claude Code (AI-assisted)
**Environment:** Dev build from reh3376_dev01, locally-built Docker images
**Codebase:** commit 8858ac6 + live-validation fixes

## Results

| Test | Expected | Actual | Status |
|------|----------|--------|--------|
| 1.1 Homebrew Install | v0.5.1 | v0.5.1, commit 8858ac6 | PASS |
| 1.2 Init from Empty Dir | 5 services, compose written | All criteria met after fixes | PASS (after fix) |
| 1.3 Pre-Campaign Check | 8 checks run | 1 pass, 3 fail (expected), 4 warn (expected) | PASS |
| 1.4 Browser UI | Dashboard loads | 200 OK | PASS |
| 1.5 Grafana | Login page | 200 OK | PASS |
| 1.6 Service Install | 4 LaunchAgents | 2 installed, sidecar fails (expected for Docker) | PARTIAL |
| 2.1 Observe | 200 + node_id | 200 + node_id + surprise_score | PASS |
| 2.2 Session ID Propagation | session_id = "test-session-001" | session_id = instance_id (wrong) | **FAIL** |
| 2.3 Query Classify | query_classify records > 0 | 0 records | **FAIL** |
| 2.5 Export Pipeline | Archive with manifest | Works with explicit instance-id only | PARTIAL |
| 2.6 Export-Auto Retention | 2 archives, symlink | Correct | PASS |
| 3.1 Curation Pipeline | 4 steps complete | All 4 steps produce valid output | PASS |
| 3.2 Training Dry Run | Config printed | Manifest validates, LoRA rank resolved | PASS |
| 3.4 Regression Gate Self-Compare | Exit 2 (WARN) | Exit 2 (WARN) | PASS |
| 4.5 Teardown + Reinit | Idempotent | All 5 services healthy | PASS |
| 6.1 PII Injection | Export blocked | EXPORT BLOCKED: 1 violation | PASS |
| 6.2 TSDB Down | Error, exit 1 | Connection refused, exit 1 | PASS |
| 6.3 Invalid Manifest | Exit 1, rejection | Exogenous ratio rejection | PASS |
| 6.4 Regression Gate Regression | Exit 1 (FAIL) | FAIL with 15% regression detail | PASS |

## Findings (Bugs Fixed In-Sprint)

### Finding #1: neural-sidecar uses `build: ./neural` in compose (FIXED)
**Severity:** P0 — blocks all non-repo-checkout users
**Root cause:** `docker-compose.yml` neural-sidecar service used `build: ./neural` instead of `image:` reference.
**Fix:** Changed to `image: ghcr.io/reh3376/mdemg-neural-sidecar:latest` with commented `build:` for dev.

### Finding #2: Fresh Neo4j has no schema — `mdemg serve` crash loop (FIXED)
**Severity:** P0 — blocks all fresh installs
**Root cause:** Docker entrypoint runs `./mdemg serve` without `--auto-migrate`. The `TSDB_AUTO_MIGRATE` env var only triggered TSDB migrations, not Neo4j migrations.
**Fix:** Added `AUTO_MIGRATE` env var support in `serve.go` + set `AUTO_MIGRATE=true` in compose.

### Finding #3: GHCR mdemg image is v0.3.4 (March 30) — massively outdated
**Severity:** P0 — all Docker users get ancient image
**Root cause:** Finding #4 (below).

### Finding #4: Docker Publish workflow never triggers on release (FIXED)
**Severity:** P0 — Docker images never updated on new releases
**Root cause:** `release.yml` creates GitHub Release using `GITHUB_TOKEN`, which by design doesn't trigger other workflows (`docker-publish.yml`).
**Fix:** Changed `docker-publish.yml` trigger from `release: types: [published]` to `workflow_run: workflows: ["Release"] types: [completed]`.

### Finding #5: `data` CLI commands don't load `.env` file (FIXED)
**Severity:** P1 — all `mdemg data` commands fail for Docker users
**Root cause:** `data_check.go`, `data_export.go`, `data_export_auto.go` never call `godotenv.Load()`. Also, CLI uses `TSDB_PORT` (Docker internal 5432) not `TSDB_HOST_PORT` (host-mapped port).
**Fix:** Added `PersistentPreRun` with `godotenv.Load()` to parent `data` command. Added `TSDB_HOST_PORT` fallback in `tsdbConfigFromEnv()`.

### Finding #6: `mdemg service install` fails for Homebrew users (FIXED)
**Severity:** P1 — LaunchAgent templates not found outside repo checkout
**Root cause:** Reads from `packaging/launchd/` on disk, no embedded fallback.
**Fix:** Created `internal/cli/launchd_templates/` with `embed.FS`, fall back to embedded when disk path missing.

## Findings (Bugs Documented — Not Fixed Yet)

### Finding #7: session_id not propagated to TSDB
**Severity:** P1 — training data has wrong session_id
**Symptom:** `session_id` column contains instance_id instead of the session ID from API requests.
**Impact:** Session-level analysis of training data impossible.

### Finding #8: space_id not propagated to TSDB for reranking
**Severity:** P1 — training data has wrong space_id
**Symptom:** `space_id` in llm_interactions shows "mdemg-dev" (default) instead of the space used in API calls.
**Impact:** Multi-instance data isolation broken in TSDB.

### Finding #9: Query classify/intent translate not recording to TSDB
**Severity:** P2 — missing training data for classifier/translator tasks
**Symptom:** Zero `retrieval.query_classify` and `retrieval.intent_translate` records despite features being enabled.
**Impact:** Cannot train LoRA adapters for these tasks.

### Finding #10: Export instance_id auto-detection mismatch
**Severity:** P2 — export silently produces empty results
**Symptom:** CLI auto-generates instance_id from username pattern, Docker containers use hostname pattern. No match = empty export.
**Impact:** `mdemg data export` without explicit `--instance-id` returns nothing.

### Finding #11: regression_gate requires `status: "evaluated"` in task results
**Severity:** P3 — documentation gap
**Symptom:** `index_by_task()` filters for `status == "evaluated"`, which is correct but not obvious to manual report creation.
**Impact:** None for automated pipeline (evaluate_ft sets it), only for manual testing.

## Documents Accessed

- `docker-compose.yml` — compose service definitions
- `internal/cli/compose_templates/docker-compose.yml` — embedded compose copy
- `internal/cli/serve.go` — auto-migrate env var handling
- `internal/cli/tsdb.go` — TSDB config from env
- `internal/cli/data.go` — data command registration
- `internal/cli/service_darwin.go` — LaunchAgent install
- `internal/tsdb/exporter.go` — export query logic
- `.github/workflows/docker-publish.yml` — Docker CI trigger
- `.github/workflows/ci.yml` — embedded file sync checks
- `neural/training/regression_gate.py` — gate logic
- `neural/training/evaluate_ft.py` — evaluation output format
