# Phase 90: RSIC Conformance and CI Gating

**Status**: Complete
**Priority**: High
**Date**: 2026-02-16
**Depends On**: Phase 89 (`docs/specs/phase89-rsic-persistence-multi-space.md`)
**Related Handoff Section**: `AGENT_HANDOFF.md` → `RSIC Hardening Phases`
**Gap References**: `docs/development/RSIC_GAP_ANALYSIS.md` — Gap #7 (Spec/Docs Drift), Gap #8 (Test Coverage)

---

## Purpose

Phases 87-89 built the RSIC runtime: orchestration triggers, safety enforcement, persistence, and multi-space correctness. All of these features are validated by **unit tests** (pure logic, no Neo4j) and **UATS contract tests** (HTTP request/response shape). However, neither layer exercises the **full vertical stack** — a cycle that reads real graph signals, dispatches real actions, persists state, and survives a restart.

Additionally, all UATS currently run under `continue-on-error: true` in CI because some specs require an embedding provider that CI doesn't configure. This means **RSIC regressions do not block merges**.

Phase 90 closes both gaps:

1. **Go integration tests** that exercise RSIC end-to-end against a live Neo4j + MDEMG server.
2. **CI pipeline split** that makes non-embedding UATS (including all RSIC specs) merge-gating.
3. **Make targets** for reproducible local RSIC verification.

---

## Scope

- Add 6 Go integration tests for RSIC behavior in `tests/integration/rsic_test.go`.
- Split the CI UATS step into two: merge-gating (no embedding needed) and best-effort (embedding needed).
- Add UATS tag/category system to the runner for selective execution.
- Add `make test-rsic` and `make test-rsic-uats` targets.
- Promote the Phase 87 idempotency draft spec (sequential support).
- Clean up 6 Phase 88 draft stubs still in `drafts/` after promotion.

---

## Design Goals

- Integration tests must be fully self-contained: create test space, run cycle, assert, cleanup.
- CI gating must not require embedding provider — only non-embedding specs block merge.
- RSIC tests should complete in < 30 seconds (no real-time waits for cooldowns).
- All test spaces use the `test-<timestamp>-<suffix>` naming pattern with `t.Cleanup()`.
- Tests must not depend on any pre-existing graph data.

---

## Current State

### What Exists

| Layer | Coverage | Merge-Gating? |
|-------|----------|---------------|
| Unit tests (internal/ape/) | 60+ tests, pure logic | Yes (go test) |
| UATS contract tests | 12 RSIC specs, HTTP shape | No (continue-on-error) |
| Go integration tests | 8 files (retrieval, ingest, etc.) | Yes (go test -tags=integration) |

### What's Missing

| Gap | Impact |
|-----|--------|
| No RSIC integration tests | Full-stack RSIC regressions undetected |
| UATS all under continue-on-error | RSIC contract regressions don't block merge |
| No local RSIC verification target | Developers can't easily validate RSIC changes |
| 1 draft spec needs sequential runner | Idempotency test blocked |
| 6 promoted specs still in drafts/ | Confusing directory state |

---

## Implementation

### 1. Go Integration Tests — `tests/integration/rsic_test.go`

New file with 6 behavior-level tests, all using `//go:build integration` tag.

#### Test 1: `TestRSIC_CycleCreatesHistory`

- Trigger a micro cycle via `POST /v1/self-improve/cycle`
- Assert 200 response with `cycle_id`, `tier: "micro"`, `actions` array
- Query `GET /v1/self-improve/history` and verify the cycle appears
- Verify `GET /v1/self-improve/health` reports `cycles_run >= 1`

#### Test 2: `TestRSIC_DryRunProducesNoDelta`

- Observe 3 nodes into a test space (via `POST /v1/conversation/observe`)
- Trigger a cycle with `dry_run: true`
- Assert response has `dry_run: true` and non-empty `actions`
- Query Neo4j directly — verify no new edges/nodes were created beyond the 3 observations

#### Test 3: `TestRSIC_SafetyBlocksProtectedSpace`

- Trigger a cycle targeting `space_id: "mdemg-dev"` (protected)
- Assert the cycle completes but destructive actions are skipped
- Verify `safety_blocked` count in the cycle outcome > 0 (or actions list shows blocked)

#### Test 4: `TestRSIC_MultiSpaceCycleIsolation`

- Create two test spaces (A and B), observe 3 nodes into each
- Trigger a cycle for space A only
- Verify cycle only touched space A (check actions reference space A)
- Verify space B graph state is unchanged (node count same as after observations)

#### Test 5: `TestRSIC_PersistenceSurvivesFlush`

- Trigger a cycle, note the `calibration` confidence in health response
- Call `GET /v1/self-improve/health` — record `persistence.state_nodes` count
- Assert `persistence.state_nodes > 0` (RSICState nodes exist in Neo4j)
- Directly query Neo4j for `(:MemoryNode:RSICState)` nodes — verify they exist with correct `rsic_type` values

#### Test 6: `TestRSIC_WatchdogStateInHealth`

- Trigger 2 cycles to build watchdog history
- Call `GET /v1/self-improve/health`
- Assert `watchdog` block exists with `decay_score`, `cycles_seen`, `last_cycle_at`
- Assert `orchestration` block exists with `policy_version`
- Assert `persistence` block exists (Phase 89)

#### Helper additions to `helpers_test.go`

```go
// ObserveTestNode creates a minimal observation in the test space.
func ObserveTestNode(t *testing.T, endpoint, spaceID, content string) {
    // POST /v1/conversation/observe with space_id, session_id, content
}

// TriggerRSICCycle triggers an RSIC cycle and returns the response.
func TriggerRSICCycle(t *testing.T, endpoint, spaceID string, opts map[string]any) map[string]any {
    // POST /v1/self-improve/cycle with space_id + opts
}

// GetRSICHealth fetches the RSIC health endpoint.
func GetRSICHealth(t *testing.T, endpoint string) map[string]any {
    // GET /v1/self-improve/health
}
```

---

### 2. CI Pipeline Split — `.github/workflows/ci.yml`

Replace the single UATS step with two steps:

```yaml
- name: Run UATS Contract Tests (Core — no embedding required)
  run: |
    python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
      --spec-dir docs/api/api-spec/uats/specs/ \
      --base-url http://localhost:${{ env.MDEMG_PORT }} \
      --exclude-tag embedding_required
  # No continue-on-error — these MUST pass to merge

- name: Run UATS Contract Tests (Embedding — best effort)
  continue-on-error: true
  run: |
    python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
      --spec-dir docs/api/api-spec/uats/specs/ \
      --base-url http://localhost:${{ env.MDEMG_PORT }} \
      --include-tag embedding_required
```

This requires adding tag-based filtering to the UATS runner (see Step 3).

---

### 3. UATS Runner Tag Filtering

Extend `uats_runner.py` to support a `tags` field in spec configs and CLI filtering.

#### Spec-level tag field

```json
{
  "config": {
    "name": "retrieve_semantic",
    "tags": ["embedding_required"],
    ...
  }
}
```

#### CLI flags

- `--include-tag TAG` — only run specs with this tag
- `--exclude-tag TAG` — skip specs with this tag
- Tags are optional; specs without tags are included by default (unless `--include-tag` is specified)

#### Tag classification for existing specs

**`embedding_required` (specs that need embedding provider):**

- `retrieve_semantic.uats.json`
- `retrieve_hybrid.uats.json`
- `retrieve_vector.uats.json`
- `ingest_codebase_*.uats.json` (4 specs — need embeddings for vector indexing)
- `memory_search.uats.json` (if it uses vector search)
- Any spec whose assertions depend on vector similarity scoring

**No tag (core — merge-gating):**

- All health/readiness specs
- All self_improve (RSIC) specs
- All conversation CMS specs
- All learning management specs
- All admin/system specs
- All memory CRUD specs (non-vector)
- All archive/freshness specs

Estimated split: ~15 specs tagged `embedding_required`, ~93 specs untagged (merge-gating).

---

### 4. Make Targets

Add to `Makefile`:

```makefile
# ============================================================
# RSIC Testing Targets
# ============================================================

.PHONY: test-rsic test-rsic-unit test-rsic-integration test-rsic-uats

# Run all RSIC tests (unit + integration + UATS)
test-rsic: test-rsic-unit test-rsic-integration test-rsic-uats
 @echo "All RSIC tests complete"

# RSIC unit tests only (no server needed)
test-rsic-unit:
 @echo "Running RSIC unit tests..."
 go test -v ./internal/ape/... -run "TestRSIC|TestCalibr|TestDispatch|TestDateTime|TestOrchestration|TestSafety|TestAction|TestWatchdog"

# RSIC integration tests (requires running server + Neo4j)
test-rsic-integration:
 @echo "Running RSIC integration tests..."
 go test -v -tags=integration ./tests/integration/... -run "TestRSIC_"

# RSIC UATS contract tests only
test-rsic-uats:
 @echo "Running RSIC UATS contract tests..."
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url $(BASE_URL) \
  --include-tag rsic
```

Also add an `rsic` tag to all 12 self_improve specs for targeted execution.

---

### 5. Draft Cleanup

#### Promote Phase 87 idempotency spec

The spec `drafts/self_improve_cycle_idempotency.phase87.uats.json` requires sequential execution (two requests with same idempotency key). Add sequential support to the runner:

- New spec config field: `"sequential": true`
- When `sequential: true`, the runner executes variants in declaration order, not in parallel
- Each variant can reference the previous variant's response via `{{prev_response.<field>}}`

After adding sequential support, move the spec to `specs/`.

#### Remove promoted stubs from drafts/

Delete the 6 Phase 88 draft files that were already promoted to `specs/`:

- `drafts/self_improve_cycle_dry_run.phase88.uats.json`
- `drafts/self_improve_health_safety.phase88.uats.json`
- `drafts/self_improve_rollback_list.phase88.uats.json`
- (and any other Phase 88 stubs remaining)

---

### 6. UATS Spec Tagging

Add `tags` array to each spec's config section.

#### Tags to add

| Tag | Meaning | Specs |
|-----|---------|-------|
| `rsic` | RSIC/self-improve endpoint | 12+ self_improve specs |
| `embedding_required` | Needs embedding provider | ~15 vector/semantic specs |
| `cms` | Conversation memory system | ~12 conversation specs |
| `admin` | Admin/system endpoints | ~5 admin specs |

Tags are additive — a spec can have multiple tags (e.g., `["rsic", "phase87"]`).

---

## Acceptance Criteria

1. `go test -tags=integration ./tests/integration/... -run TestRSIC_` — all 6 tests pass
2. `make test-rsic` — runs unit + integration + UATS for RSIC, all pass
3. CI splits UATS into merge-gating and best-effort steps
4. Non-embedding UATS (including all RSIC specs) block merge on failure
5. Embedding-dependent UATS remain continue-on-error
6. `--include-tag` and `--exclude-tag` flags work in UATS runner
7. All 12+ RSIC specs tagged with `rsic`
8. Embedding-dependent specs tagged with `embedding_required`
9. Phase 87 idempotency spec promoted from drafts
10. No orphaned draft stubs remain for already-promoted specs

---

## Verification Plan

### Automated

1. `go build ./...` — zero errors
2. `go vet ./...` — clean
3. `go test -v ./internal/ape/...` — all unit tests pass
4. `go test -v -tags=integration ./tests/integration/... -run TestRSIC_` — all 6 pass
5. `make test-rsic` — complete pass
6. `make test-api` — existing 108+ specs, 100%

### Manual CI Verification

1. Push to `mdemg-dev01` — verify CI runs two UATS steps
2. Verify core UATS step (non-embedding) is merge-gating (no continue-on-error)
3. Verify embedding UATS step has continue-on-error
4. Intentionally break a RSIC spec — verify CI fails the core step

---

## Files Modified/Created Summary

| File | Action | Changes |
|------|--------|---------|
| `tests/integration/rsic_test.go` | **Create** | 6 RSIC core integration tests (~300 lines) |
| `tests/integration/rsic_systems_test.go` | **Create** | 10 systems-level integration tests (~600 lines) |
| `tests/integration/rsic_holistic_test.go` | **Create** | 6 holistic integration tests — full pipeline with Neo4j mutations (~350 lines) |
| `tests/integration/helpers_test.go` | Edit | Add 10 RSIC test helpers: 6 API (TriggerRSICCycleRaw, GetRSICCalibration, GetRSICHistoryFiltered, GetRSICRollbackList, PostRSICRollback, GetRSICSignals) + 4 holistic (SeedHiddenNode, SeedObservationNodes, CountNodesByProperty, RefreshDistributionCache) (~210 lines) |
| `.github/workflows/ci.yml` | Edit | Split UATS into 2 steps (~15 lines changed) |
| `docs/api/api-spec/uats/runners/uats_runner.py` | Edit | Add tag filtering + sequential mode (~60 lines) |
| `Makefile` | Edit | Add 4 RSIC test targets (~20 lines) |
| 12+ UATS spec files | Edit | Add `tags` to config (~1 line each) |
| ~15 UATS spec files | Edit | Add `embedding_required` tag (~1 line each) |
| ~6 draft files | Delete | Remove promoted stubs |
| 1 draft spec | Promote | Move idempotency spec to specs/ |

**Total:** 3 new test files (~1250 lines), ~30 modified files (~150 lines of changes), ~6 deletions

### Holistic Tests — Pipeline Verification

The 6 holistic tests in `rsic_holistic_test.go` close the critical gap where no existing test exercised the full RSIC pipeline past the confidence gate. They seed data via direct Cypher to achieve 2/4 confidence data points (TotalNodes + ConsolidationAgeSec = 0.50 > 0.30 threshold), then verify:

| Test | Pipeline Coverage |
|------|-------------------|
| `ConfidenceGatePassAndReflect` | Assess → Reflect (proves gate passage, insights produced) |
| `TombstoneStaleEndToEnd` | Assess → Reflect → Plan → Dispatch → Execute → Neo4j mutation verified |
| `DryRunPreservesState` | Full pipeline in dry-run mode → zero Neo4j mutations |
| `RollbackReversesTombstone` | Execute → Snapshot → Rollback → mutations reversed |
| `HistoryAndCalibrationReflectExecution` | History/calibration entries reflect real pipeline execution |
| `MultiActionDispatchAndMetrics` | Multiple actions dispatched, Prometheus action-level metrics recorded |
