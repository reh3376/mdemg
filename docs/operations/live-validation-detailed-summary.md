# Live Validation Sprint — Detailed Findings & Solutions

**Date:** 2026-04-02
**Codebase:** v0.5.1 (commit 8858ac6) + live-validation fixes (commit e9f8bc2)
**Scope:** 6 sessions, 19 tests, 11 findings

---

## Executive Summary

The live validation sprint tested every integration boundary that automated tests cannot reach — real Docker services, real TSDB writes, real API flows. It exposed **11 bugs**, 3 of which are P0 (block all new users from using MDEMG). 6 were fixed in-sprint. The remaining 5 are all data propagation bugs where values are lost at component boundaries during TSDB recording.

**Key insight:** Every P0 bug existed since v0.3.0 (Docker deployment introduced). Every automated test passed. Zero users could successfully run `mdemg init` from Homebrew without hitting at least 2 of these.

---

## Findings Fixed In-Sprint

### F1: neural-sidecar `build: ./neural` breaks non-repo installs

| | |
|---|---|
| **Severity** | P0 — blocks ALL Homebrew/binary users |
| **Symptom** | `mdemg init` fails: `unable to prepare context: path "./neural" not found` |
| **Root Cause** | `docker-compose.yml` line 140 uses `build: ./neural` for the neural-sidecar service. The `neural/` directory only exists in a Git checkout, not in Homebrew installs or edge binary downloads. |
| **Fix** | Changed to `image: ghcr.io/reh3376/mdemg-neural-sidecar:latest` with commented `build:` for dev. Applied to both root `docker-compose.yml` and embedded copy in `internal/cli/compose_templates/`. |
| **Files** | `docker-compose.yml:140`, `internal/cli/compose_templates/docker-compose.yml:140` |
| **Risk** | Low. The GHCR image is now the default. Dev users uncomment `build:` as documented. CI sync check ensures both files match. |

### F2: Fresh Neo4j has no schema — server crash loop

| | |
|---|---|
| **Severity** | P0 — blocks ALL fresh installs |
| **Symptom** | `mdemg` container restarts every 2-5s with: `schema version check failed: SchemaMeta missing: run migrations` |
| **Root Cause** | The Docker entrypoint runs `./mdemg serve` without `--auto-migrate`. The `serve.go:82` env var check only looked for `TSDB_AUTO_MIGRATE`, which was added for TSDB migrations. Neo4j schema migrations require the same `autoMigrate` code path but had no env var trigger for Docker. |
| **Fix** | Added `AUTO_MIGRATE` env var check in `serve.go:81-85`. Set `AUTO_MIGRATE=true` in compose environment for the mdemg service. Both `TSDB_AUTO_MIGRATE` and `AUTO_MIGRATE` now trigger the same `autoMigrate` flag. |
| **Files** | `internal/cli/serve.go:81-85`, `docker-compose.yml`, `internal/cli/compose_templates/docker-compose.yml` |
| **Risk** | Low. `AUTO_MIGRATE=true` is a superset of `TSDB_AUTO_MIGRATE=true`. Both are kept for backward compatibility. Migrations are idempotent (skip if already applied). |

### F3+F4: GHCR images never updated on release

| | |
|---|---|
| **Severity** | P0 — Docker users get image from March 30 (v0.3.4-122) |
| **Symptom** | GHCR image shows `mdemg v0.3.4-122` despite 12 releases since then. Docker Publish workflow never appears in Actions history for release events. |
| **Root Cause** | `release.yml` (goreleaser) creates a GitHub Release using `GITHUB_TOKEN`. By GitHub design, events triggered by `GITHUB_TOKEN` do not create new workflow runs (prevents infinite loops). So `docker-publish.yml`'s `release: types: [published]` trigger never fires. |
| **Fix** | Changed `docker-publish.yml` trigger from `release: types: [published]` to `workflow_run: workflows: ["Release"] types: [completed]`. Added `if` condition to only run when the Release workflow succeeded. Also added neural-sidecar build+push steps to the same workflow. |
| **Files** | `.github/workflows/docker-publish.yml` |
| **Risk** | **Medium.** The `workflow_run` trigger uses the default branch's workflow file, not the tag's. If `docker-publish.yml` is broken on `main`, it won't run even if the Release succeeds. Mitigation: `workflow_dispatch` trigger allows manual recovery. Also, the first run after merge will update both images — verify by checking GHCR after the next merge to main. |

### F5: `data` CLI commands don't load `.env` / wrong TSDB port

| | |
|---|---|
| **Severity** | P1 — `mdemg data check/export/export-auto` fail for all Docker users |
| **Symptom** | `mdemg data check --pre-campaign` errors: `connect: connection refused` on port 5432. Docker maps TSDB to host port 5434 (stored in `.env` as `TSDB_HOST_PORT`). |
| **Root Cause** | Two bugs: (1) `data.go` has no `godotenv.Load()` — the `.env` file is never read for data subcommands. Other commands (`daemon.go`, `ingest.go`, `serve.go`) all call it. (2) `tsdbConfigFromEnv()` reads `TSDB_PORT` (Docker internal 5432) not `TSDB_HOST_PORT` (host-mapped port from `.env`). |
| **Fix** | Added `PersistentPreRun` with `godotenv.Load()` to the parent `data` command (affects all subcommands). Added `TSDB_HOST_PORT` as priority fallback in `tsdbConfigFromEnv()`: check `TSDB_HOST_PORT` first, then `TSDB_PORT`, then default 5432. |
| **Files** | `internal/cli/data.go:27-29`, `internal/cli/tsdb.go:36-40` |
| **Risk** | Low. `TSDB_HOST_PORT` is only set in Docker `.env` files. For non-Docker installs (native TSDB at 5432), the fallback chain is unchanged. `PersistentPreRun` on the parent command applies to all `data` subcommands automatically. |

### F6: LaunchAgent templates not embedded in binary

| | |
|---|---|
| **Severity** | P1 — `mdemg service install` fails for all Homebrew/binary users |
| **Symptom** | `read template com.mdemg.server.plist: no such file or directory` |
| **Root Cause** | `service_darwin.go:54-58` reads plist templates from `packaging/launchd/` on disk. This directory only exists in the Git checkout. No embedded fallback exists (unlike compose templates, which were fixed in v0.5.1). |
| **Fix** | Created `internal/cli/launchd_templates/` with `embed.FS` (same pattern as `compose_templates/embed.go`). Copied all 4 plist files. Updated `service_darwin.go:57-62` to try disk first, fall back to embedded. Added CI sync check in `ci.yml` to prevent embedded copies from going stale. |
| **Files** | `internal/cli/launchd_templates/embed.go` (new), `internal/cli/service_darwin.go:57-62`, `.github/workflows/ci.yml` |
| **Risk** | Low. Same proven pattern as compose_templates. CI enforces sync. Neural-sidecar LaunchAgent will still fail for Docker users (references `__PROJECT_DIR__/neural` which doesn't exist) — but this is expected because Docker runs the sidecar inside the compose stack, not as a native LaunchAgent. |

---

## Findings Requiring Follow-Up

### F7: session_id not propagated to TSDB

| | |
|---|---|
| **Severity** | P1 — training data has wrong session_id for all retrieval/consult tasks |
| **Symptom** | `session_id` column in `llm_interactions` contains the instance_id value (`cca2a964e3f5-mdemg-dev`) instead of the API request's session_id (`test-session-001`). |
| **Root Cause** | **Three breaks in the chain:** |

**Break 1: Default session_id set to instance_id**
- `internal/api/server.go:1063` — `llmclient.SetDefaultSessionID(s.cfg.InstanceID)` sets the fallback session_id to the server's instance_id. When no session_id is in context, this value is used.

**Break 2: Retrieve handler doesn't propagate session_id**
- `internal/api/handlers.go:423` — `handleRetrieve` calls `s.retriever.Retrieve(r.Context(), req)` WITHOUT setting session_id in context via `llmclient.WithSessionID()`.
- `internal/models/models.go` — `RetrieveRequest` struct has no `SessionID` field, so even if the API caller sends one, it's dropped during JSON deserialization.

**Break 3: Consult handler doesn't propagate session_id**
- `internal/api/handlers.go:2182` — `handleConsult` has the same issue: no `WithSessionID()` call, and `ConsultRequest` has no `SessionID` field.

**Note:** `handleObserve` (`handlers_conversation.go:63`) correctly calls `llmclient.WithSessionID(r.Context(), req.SessionID)` — this is the one path that works. The pattern exists; it just wasn't applied to retrieve/consult.

| | |
|---|---|
| **Solution** | 1. Add `SessionID string \`json:"session_id,omitempty"\`` to `RetrieveRequest` and `ConsultRequest` in `internal/models/models.go` |
| | 2. In `handleRetrieve` and `handleConsult`, add `ctx := llmclient.WithSessionID(r.Context(), req.SessionID)` and pass `ctx` instead of `r.Context()` to service calls |
| | 3. Change `server.go:1063` to `llmclient.SetDefaultSessionID("")` — empty string is better than a misleading instance_id. Recording with empty session_id is honest; recording with wrong session_id is corrupting. |
| **Impact** | All historical TSDB data has wrong session_id for retrieve/consult tasks. Future data will be correct after the fix. Cannot retroactively fix existing records (the real session_id was never stored). |
| **Risk** | Low fix risk. The `WithSessionID` pattern is proven (used in observe). Adding optional JSON fields is backward-compatible. Changing the default to empty string may surface empty session_id in exports — but empty is truthful, instance_id is not. |

### F8: space_id not propagated to reranker TSDB recording

| | |
|---|---|
| **Severity** | P1 — multi-instance data isolation broken in TSDB |
| **Symptom** | `space_id` in `llm_interactions` shows `mdemg-dev` (RSIC watchdog default) instead of the space used in API calls (`live-test`). |
| **Root Cause** | **Five breaks in the chain:** |

1. `internal/retrieval/rerank.go:24-30` — `RerankRequest` struct has no `SpaceID` field
2. `internal/retrieval/service.go:643-648` — Rerank call site doesn't pass `req.SpaceID` to `RerankRequest`
3. `internal/retrieval/rerank.go:259` — `.WithContext("retrieval.rerank_cross", "")` passes **empty string** as spaceID
4. `internal/llmclient/client.go:150-155` — When spaceID is empty, falls back to `defaultSpaceID`
5. `internal/api/server.go:1062` — `llmclient.SetDefaultSpaceID(s.cfg.RSICWatchdogSpaceID)` sets default to `mdemg-dev`

| | |
|---|---|
| **Solution** | 1. Add `SpaceID string` to `RerankRequest` struct |
| | 2. Pass `req.SpaceID` at call site in `service.go:643-648` |
| | 3. Thread `spaceID` through `Rerank()` → `doRerankWithOpenAI()` / `doRerankWithOllama()` |
| | 4. Pass `req.SpaceID` instead of `""` in `.WithContext()` calls |
| **Impact** | Same as F7 — historical data has wrong space_id. Cannot retroactively fix. This affects every retrieval query that triggered reranking. |
| **Risk** | Low fix risk but **touches hot path** — the reranking code handles every retrieval query. The fix adds a field pass-through, no logic changes. Test with `go test ./internal/retrieval/...` + live TSDB verification. |

**Note:** The same pattern likely affects other LLM consumers (consulting synthesis, jiminy, etc.). A systematic audit of all `.WithContext()` call sites should verify that none pass empty space_id.

### F9: Query classify + intent translate not recording to TSDB

| | |
|---|---|
| **Severity** | P2 — cannot collect training data for classifier/translator tasks |
| **Symptom** | Zero `retrieval.query_classify` and `retrieval.intent_translate` records in TSDB despite features being enabled and retrieval queries being processed. |
| **Root Cause** | **Client initialization ordering bug.** |

The LLM clients for query classifier and intent translator are created **before** the TSDB recorder is attached:

| Order | Line | What Happens |
|-------|------|-------------|
| 1 | `server.go:380` | `intentTrans = retrieval.NewLLMIntentTranslator(...)` — creates llmclient with `recorder = nil` (defaultRecorder is nil at this point) |
| 2 | `server.go:403` | `queryClassifier = retrieval.NewQueryClassifier(...)` — same, `recorder = nil` |
| 3 | `server.go:1060` | `llmclient.SetDefaultRecorder(s.llmWriter)` — attaches recorder, but **only for clients created after this point** |

The `llmclient.New()` function (`client.go:137`) captures `defaultRecorder` **by value** at construction time. Clients created before `SetDefaultRecorder()` permanently have `recorder = nil`. The `recordInteraction()` method (`client.go:208-210`) early-returns when `recorder == nil`.

Other LLM clients (consulting, reranking, etc.) are created later in the initialization sequence and correctly inherit the recorder.

| | |
|---|---|
| **Solution** | **Option A (recommended):** Move `llmclient.SetDefaultRecorder(s.llmWriter)` to execute **before** line 380 in `server.go`. This requires initializing the TSDB writer earlier in the startup sequence. |
| | **Option B:** After the recorder is set, explicitly call `.SetRecorder(s.llmWriter)` on the intent translator's and query classifier's embedded llmclient instances. This requires adding a `SetRecorder()` method or accessor. |
| | **Option C:** Change `llmclient.New()` to use a pointer to `defaultRecorder` instead of copying its value, so late-bound setting works. This is the most architecturally clean but requires careful concurrency analysis. |
| **Impact** | No training data has been collected for `query_classify` or `intent_translate` tasks since their introduction. The fix will start collection; no retroactive data recovery is possible. |
| **Risk** | **Medium.** Option A requires reordering server initialization, which may have cascading dependencies (the TSDB writer needs the database connection, which needs config, etc.). Option B is surgically safe but requires exposing internal client fields. Option C changes the llmclient contract for all consumers. |

### F10: Export instance_id auto-detection mismatch

| | |
|---|---|
| **Severity** | P2 — `mdemg data export` silently returns empty results |
| **Symptom** | `mdemg data export --space-id mdemg-dev` exports 0 rows. Adding `--instance-id cca2a964e3f5-mdemg-dev` exports 1 row. The auto-detected instance_id doesn't match the Docker container's instance_id. |
| **Root Cause** | **Two different auto-detection patterns:** |

| Component | File:Line | Pattern | Example |
|-----------|-----------|---------|---------|
| CLI export | `data_export.go:45-56` | `{os_username}-{space_id}` | `reh3376-mdemg-dev` |
| Docker server | `config.go:3136-3140` | `{container_hostname}-{rsicWatchdogSpaceID}` | `cca2a964e3f5-mdemg-dev` |

The CLI runs on the host (OS username = `reh3376`). The server runs in a Docker container (hostname = `cca2a964e3f5`). These never match.

Additionally, the server uses `rsicWatchdogSpaceID` (always `mdemg-dev`) while the CLI uses the `--space-id` flag value. Even if the hostname matched, the space_id component would diverge for non-default spaces.

| | |
|---|---|
| **Solution** | **Option A (recommended):** Change CLI export auto-detection to use `os.Hostname()` instead of `user.Current().Username`, matching the server's pattern. |
| | **Option B:** Have `mdemg init` write `MDEMG_INSTANCE_ID` to `.env` with the expected Docker container hostname. This is fragile (hostname changes on container recreation). |
| | **Option C:** Remove instance_id filtering from the export query entirely — filter only by space_id and time range. Instance_id was added for multi-instance isolation, but space_id already provides this. |
| **Impact** | Every `mdemg data export` without explicit `--instance-id` silently produces zero rows. Users may not realize data exists but is being filtered out. |
| **Risk** | **Option A risk: Medium.** `os.Hostname()` on macOS returns the machine hostname, not a Docker container hostname. For a Homebrew user running the CLI on the host against a Docker TSDB, the hostname still won't match the container's. **Option C is safest** — just remove instance_id from the default export filter. Users who need it can still pass `--instance-id` explicitly. |

### F11: regression_gate requires `status: "evaluated"` (documentation gap)

| | |
|---|---|
| **Severity** | P3 — affects only manual report creation |
| **Symptom** | `index_by_task()` in `regression_gate.py:48` filters for `status == "evaluated"`. Reports without this field produce empty task indexes, causing the gate to always return WARN (no regressions, no improvements). |
| **Root Cause** | Not a bug — `evaluate_ft.py:398` correctly sets `status: "evaluated"`. The issue is that the contract between evaluate and gate isn't documented, and manual test reports omit it. |
| **Solution** | Add a docstring to `regression_gate.py:43-49` documenting the required `status` field. Optionally: remove the filter (accept all tasks) or warn when zero tasks are indexed. |
| **Risk** | None. |

---

## Systemic Risk Assessment

### Risk 1: Data Corruption Window (HIGH)

Findings F7, F8, and F9 mean that **all TSDB training data collected since v0.3.0 has incorrect session_id and space_id values for retrieval/consult tasks**, and **zero records exist for query_classify and intent_translate tasks**. This affects the quality and usability of data for the 30-day collection campaign.

**Mitigation:** Fix F7-F9 before launching the campaign. The `space_id` backfill migration (009) can partially remediate F8 for records where the correct space_id is inferable. Session_id (F7) cannot be retroactively recovered.

### Risk 2: Silent Export Failure (MEDIUM)

Finding F10 means `mdemg data export` **silently succeeds with 0 rows** instead of failing or warning. A team member running daily exports would believe no data is being collected when it actually is — just under a different instance_id. This directly undermines the campaign's data accumulation goals.

**Mitigation:** Fix F10 before the campaign. At minimum, add a warning when the export query returns 0 rows but the table has data for other instance_ids.

### Risk 3: Docker Image Staleness Recurrence (MEDIUM)

Finding F4 was fixed by changing the trigger to `workflow_run`, but this introduces a new dependency: the Docker Publish workflow only runs if the Release workflow's name exactly matches `"Release"`. If someone renames the workflow, Docker images silently stop updating — the same class of silent failure we just found.

**Mitigation:** Add a scheduled weekly run of Docker Publish (`schedule: cron: '0 6 * * 1'`) as a safety net. If the workflow_run trigger fails, the weekly build catches it.

### Risk 4: Initialization Order Fragility (MEDIUM)

Finding F9's root cause (clients created before recorder is attached) is a time bomb. Any new LLM consumer added early in `server.go` will silently lose TSDB recording. There's no warning, no error, no test that catches it.

**Mitigation:** Either (a) change `llmclient.New()` to use late-binding for the recorder (pointer to global), or (b) add a startup check that verifies all registered LLM clients have a non-nil recorder after initialization completes.

### Risk 5: Embedded File Divergence (LOW)

Findings F1 and F6 required embedding files that are copies of source files (`docker-compose.yml`, `packaging/launchd/*.plist`). CI sync checks catch divergence, but they only run on push — a developer could merge a PR that modifies the source without updating the embedded copy if the CI check is skipped or bypassed.

**Mitigation:** CI checks are already in place. The risk is low but non-zero. Consider generating embedded copies from source during build rather than maintaining manual copies.

---

## Recommended Fix Priority

| Priority | Findings | Effort | Rationale |
|----------|----------|--------|-----------|
| **Sprint blocker** | F7 + F8 + F9 | 2-3 hours | Campaign data quality depends on correct TSDB recording. Fix all three before launching the 30-day collection campaign. |
| **Sprint blocker** | F10 | 30 min | Silent empty exports will confuse every team member. Quick fix: remove instance_id from default export filter. |
| **Next release** | Risk 3 (weekly schedule) | 5 min | Add cron trigger to docker-publish.yml as safety net. |
| **Next release** | Risk 4 (startup check) | 1 hour | Add recorder-nil check after server initialization. |
| **Deferred** | F11 | 10 min | Documentation only. No user impact in automated pipeline. |

---

## Post-Fix Re-Validation Requirement

After the data propagation fix PR merges to main, **all 19 tests from the live validation findings must be re-run** before the 30-day collection campaign launches. This is a gate — no campaign launch without full green.

### Expected Outcome Changes

The 4 tests that FAILed or were PARTIAL due to F7-F10 must now PASS:

| Test | Original Status | Expected After Fix | What Changed |
|------|----------------|-------------------|--------------|
| 2.2 Session ID Propagation | **FAIL** (session_id = instance_id) | **PASS** (session_id = request value) | F7: `WithSessionID` in handlers, default changed to empty |
| 2.3 Query Classify | **FAIL** (0 records) | **PASS** (records > 0) | F9: Recorder initialized before `NewServer()` |
| 2.5 Export Pipeline | **PARTIAL** (explicit instance-id only) | **PASS** (works without --instance-id) | F10: Auto-detection removed, empty = all instances |
| 1.3 Pre-Campaign Check | PASS (3 fail expected) | **PASS** (fewer expected failures) | F9 fixes classify/translate recording |

### Regression Guard

The 15 tests that previously PASSed must still PASS. Any new failure is a regression from the fix and **must be resolved before the campaign launches**:

| Test | Must Remain |
|------|-------------|
| 1.1 Homebrew Install | PASS |
| 1.2 Init from Empty Dir | PASS |
| 1.4 Browser UI | PASS |
| 1.5 Grafana | PASS |
| 1.6 Service Install | PARTIAL (sidecar expected to fail for Docker) |
| 2.1 Observe | PASS |
| 2.6 Export-Auto Retention | PASS |
| 3.1 Curation Pipeline | PASS |
| 3.2 Training Dry Run | PASS |
| 3.4 Regression Gate Self-Compare | PASS |
| 4.5 Teardown + Reinit | PASS |
| 6.1 PII Injection | PASS |
| 6.2 TSDB Down | PASS |
| 6.3 Invalid Manifest | PASS |
| 6.4 Regression Gate Regression | PASS |

### Re-Validation Procedure

1. Build fresh binary from merged main: `go build -o bin/mdemg ./cmd/mdemg`
2. Rebuild Docker images locally: `docker compose build`
3. Teardown and reinit: `docker compose down -v && mdemg init --quick && docker compose up -d`
4. Wait for all 5 services healthy: `docker compose ps`
5. Run all 19 tests in session order (1.x → 2.x → 3.x → 4.x → 6.x)
6. For F7 verification: send retrieve with `session_id`, wait 35s for TSDB flush, verify session_id in `llm_interactions`
7. For F9 verification: trigger retrieval with `QUERY_CLASSIFY_ENABLED=true`, wait 35s, verify `retrieval.query_classify` records exist
8. For F8 verification: send retrieve with non-default space_id, wait 35s, verify `space_id` in rerank TSDB records
9. For F10 verification: run `mdemg data export` without `--instance-id`, verify row count > 0

**Any test that regresses blocks the campaign launch and must be fixed in the same PR or a follow-up hotfix before proceeding.**

---

## Documents Accessed

- `internal/api/server.go` — initialization order, default recorder/session/space setup
- `internal/api/handlers.go` — retrieve/consult handler session_id propagation
- `internal/api/handlers_conversation.go` — observe handler (working reference)
- `internal/llmclient/client.go` — WithContext, WithSessionID, recordInteraction, New()
- `internal/retrieval/service.go` — rerank call site, space_id availability
- `internal/retrieval/rerank.go` — RerankRequest struct, doRerankWithOpenAI/Ollama
- `internal/retrieval/query_classifier.go` — client creation with WithContext
- `internal/retrieval/intent_translator.go` — client creation with WithContext
- `internal/config/config.go` — instance_id auto-detection (server-side)
- `internal/cli/data_export.go` — instance_id auto-detection (CLI-side)
- `internal/tsdb/llm_writer.go` — TSDB INSERT for llm_interactions
- `neural/training/regression_gate.py` — index_by_task status filter
- `neural/training/evaluate_ft.py` — status field in output
