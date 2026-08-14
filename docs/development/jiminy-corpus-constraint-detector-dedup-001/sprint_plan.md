# JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint code:** `JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001`
- **Date:** 2026-08-14 | **Branch:** `reh3376_dev01`
- **Arc adjacency:** JIMINY-CEILING-BREAK-2 (arc-in-flight, T+168h re-measurement due 2026-08-19). Substrate-mutating retroactive tombstones (E4) MUST be gated on the arc window (see §3).
- **Parent triggers:**
  - `CREATE-CORRECTION-DEDUP-001` (shipped 2026-08-12) — sibling defect at the correction-promotion layer; already ships vector-similarity dedup + `SkippedDup` telemetry.
  - `JIMINY-CORPUS-AUDIT-004` (2026-08-14) — surfaced two live instances of the dual-severity dual-mint class and disclosed this sprint by name.
- **Target commit shape:** 1 code commit (E1+E2+E3), 1 tombstone-batch commit (E4 batch_record + snapshot), 1 docs commit (E6). E5 live Tier-3 runs against the shipped commit before batch.
- **Estimated effort:** ~1.5–2h implementation + operator-scheduled tombstone batch.

## 2. Problem Statement

`internal/conversation/constraint_detector.go` runs on every `/v1/conversation/observe` and produces a `[]DetectedConstraint` slice — one entry per **matching constraint type** (`must`, `must_not`, `should`, `should_not`, `deadline`). The current shape (see `Detect`, lines 134–180) collapses **within a type** via `bestByType` (highest-confidence-per-type wins), but does NOT collapse **across types**. Consequence: one L0 observation whose text triggers regexes in more than one severity bucket emits N `DetectedConstraint` rows → the `observe` handler in `internal/conversation/service.go:397–441` writes N `constraint:<type>` tags → the L1 promoter `CreateConstraintNodes` (`internal/hidden/constraint_nodes.go:152–324`) loops `for _, cType := range obs.cTypes` and mints **one constraint node per type** — all sharing the same content, name, and `constraint_code`, only the `constraint_type` axis differing.

**Two live instances found in the mdemg-dev corpus (JIMINY-CORPUS-AUDIT-004):**

- `z5xgcmv8i60e2aoatnw8i15b` — `constraint_type=must_not` (semantically correct twin)
- `pwa2lmy6qgu81r10r5xch9nv` — `constraint_type=must`  (semantically wrong twin)
- Second-instance pair confirmed by Fable audit: node `qi43sv83g136` with `constraint_code=auto-250af3293675` + its dual-severity twin.

Both pairs share identical content substrate and identical `constraint_code`. The differing `constraint_type` axis is the sole artifact of the dual-mint bug.

`CREATE-CORRECTION-DEDUP-001` catches this class at the promotion layer for corrections via a vector-similarity query. The correct fix for the constraint side is **upstream at the detector**: the detector should emit exactly one canonical `DetectedConstraint` per L0 observation whenever the multi-match originated from one contiguous content substrate.

## 3. Scope & Constraints

**In scope (single detector fix + retroactive cleanup):**

- `internal/conversation/constraint_detector.go` — introduce a canonical-selection step after `bestByType` population, gated on a new config flag.
- `internal/config/config.go` — new `ConstraintDetectorDedupEnabled` bool (default `true`).
- Metric: `mdemg_constraint_detector_multi_emit_suppressed_total{space_id}` counter registered in `internal/metrics/`.
- New pin-test file `internal/conversation/constraint_detector_dedup_test.go` with the 2 live-instance regression fixtures + 1 legitimately-multi-severity content fixture (must NOT collapse when disabled).
- E4 retroactive tombstone (operator-authorized direct-Cypher) of the 2 dup twin nodes — mirrors JIMINY-CORPUS-AUDIT-004 Option B pattern (uniform `archive_reason` prefix + rollback Cypher captured in `batch_record.md`).
- `docs/features/constraint-detector.md` — new (does not yet exist).

**Out of scope:**

- Any change to `CreateConstraintNodes` semantics (the promoter remains a pass-through of the detector's per-type list; correctness is enforced upstream).
- Any change to the shipped `constraint_code` mint flow.
- Any change to `CREATE-CORRECTION-DEDUP-001`'s vector-similarity gate.
- Constraint-side symmetric vector-similarity dedup at the promotion layer (deferred; this sprint solves a different failure mode).

**Arc-safety (explicit answer to the arc-window question):**

- **Code changes (E1/E2/E3) are arc-safe.** They alter detector behavior on new `observe` traffic only; they neither mutate the substrate retroactively nor touch the follow-rate measurement pipeline. `constraint_outcomes` rows previously written for the duplicate nodes remain intact; the code fix simply stops the class from re-appearing.
- **Retroactive tombstones (E4) are substrate-mutating.** Both twin pairs currently produce two rows apiece in the active_constraints denominator and each pair emits duplicate `constraint_outcomes` rows on every surfacing — a small but measurable inflation of the CEILING-BREAK-2 follow-rate baseline. **Recommendation: batch during the arc window (2026-08-14 → 2026-08-19)** — the duplicates inflate the baseline in the direction of dragging follow-rate DOWN (each twin surfaces independently, doubling `ignored` outcomes on the same substrate); removing them BEFORE the re-check gives a cleaner honest reading. Operator sign-off required either way per JIMINY-CORPUS-002 tombstone protocol.

## 4. Dependencies

**Hard prerequisites (all shipped):**

- `CREATE-CORRECTION-DEDUP-001` (2026-08-12) — establishes the "dedup at write, not at read" pattern this sprint mirrors on the constraint side; also establishes the `SkippedDup`-style telemetry field convention.
- `JIMINY-CORPUS-AUDIT-004` (2026-08-14) — precedent for operator-authorized direct-Cypher tombstone batches with `archive_reason` provenance + rollback Cypher; `batch_record.md` + `pre_batch_snapshot.json` file shape.
- `JIMINY-CORPUS-002` — original tombstone-safety protocol (backup + small batch + operator sign-off).

**Soft prerequisites:**

- Metrics registration surface (existing `mdemg_constraint_*` counters — reuse the same `MustRegister` block).
- CLAUDE.md pin-writing convention (audit-004 established the two-arch-rules pin cadence).

**No blockers.** JIMINY-CEILING-BREAK-2 is arc-in-flight but its T+168h re-check is the natural verification event for E4 (see §6 Live).

## 5. Implementation Plan

Sequential epics. E1 → E2 → E3 land in a single code commit. E4 is a separate operator-authorized commit after E5 verifies the fix on live traffic.

### E1 — Detector fix (in-detector canonical single-emit)

`internal/conversation/constraint_detector.go`:

- After the existing `bestByType` population loop, if `s.cfg.ConstraintDetectorDedupEnabled && len(bestByType) > 1`, collapse to a single `DetectedConstraint` via **severity precedence**:
  - Order: `must_not` > `must` > `should_not` > `should` > `deadline`. Rationale: `must_not` outranks `must` because a prohibition is strictly more constraining than an obligation over the same substrate (the AUDIT-004 twin pair `z5xgcm`/`pwa2lm` proves this — the `must_not` twin was semantically correct, the `must` twin was the spurious noun-phrase match on "must-constraint" in the ruling text).
  - Ties within the same precedence bucket (impossible today since precedence is total-ordered by type, but pin the invariant): break by higher `Confidence`, then by earlier regex-pattern-index.
- Increment `mdemg_constraint_detector_multi_emit_suppressed_total{space_id}` by `len(bestByType) - 1` when a collapse occurs. Log `slog.Debug("constraint detector: multi-emit collapsed to canonical", obs_id, chosen_type, suppressed_types)`.
- Signature stays `[]DetectedConstraint` — the returned slice length becomes 1 (or 0) when dedup is enabled. Callers in `service.go:397–441` iterate the slice unchanged; the loop-body executes exactly once so the tag-append + tier-set + `constraint_code` gen paths remain semantically identical to the shipped single-severity case.
- **Fail-open**: dedup selection is deterministic and cannot fail; no error path. If the config flag is false, current behavior is byte-identical to today.

`internal/config/config.go`: add `ConstraintDetectorDedupEnabled bool` with env `CONSTRAINT_DETECTOR_DEDUP_ENABLED`, default `true`. Wire into `NewConstraintDetector` via `internal/conversation/service.go:94` (extend the constructor signature to accept the flag, or read it from `svc.cfg` in `Detect`).

**Perf budget (hot path):** the collapse is O(1) over a map of size ≤5 (there are only 5 possible constraint types). Zero regex work, zero I/O. Bench pin in E2 confirms detector-latency-p99 unchanged within noise.

### E2 — Pin tests

`internal/conversation/constraint_detector_dedup_test.go` (new):

- `TestDetectorDedup_LiveInstance1_MustNotWins` — feed the exact content substrate of `z5xgcmv8i60e2aoatnw8i15b`/`pwa2lmy6qgu81r10r5xch9nv` pair. Assert `len(result) == 1`, `result[0].ConstraintType == "must_not"`.
- `TestDetectorDedup_LiveInstance2_MustNotWins` — feed the content of the 2nd audit-found instance. Assert same shape.
- `TestDetectorDedup_LegitimatelyMultiSeverity_NotCollapsedWhenDisabled` — content that is genuinely 2 rules ("You must always use CUIDv2. You must never commit to main."). With `ConstraintDetectorDedupEnabled=false`, assert both `must` and `must_not` are emitted (regression pin on the disable path).
- `TestDetectorDedup_LegitimatelyMultiSeverity_CollapsedWhenEnabled_DocumentsTradeOff` — same content, dedup on: assert `len(result) == 1`, `must_not` wins by precedence. Comment documents the known limitation: **the detector cannot distinguish "one rule with mixed language" from "two rules in one obs"** — this is a deliberate trade-off. Operators authoring genuinely-multi-rule observations should submit them as separate `observe` calls, one per rule.
- `TestDetectorDedup_SingleSeverity_NoOpWhenAlreadyOne` — no collapse, no metric increment.
- `TestDetectorDedup_ConfigDefault_TrueSafe` — pin the safe default.
- `TestDetectorDedup_ConfigDisabled_ByteIdenticalToPreSprint` — with flag off, `Detect` returns byte-identical output to today.

Extend `internal/conversation/bench_test.go` with `BenchmarkDetector_MultiSeverityContent` — assert dedup-enabled p99 within +/-10% of dedup-disabled (hot-path budget).

### E3 — Metric

Register `mdemg_constraint_detector_multi_emit_suppressed_total` (CounterVec labelled `space_id`) in the same metrics-registration block that hosts the existing `mdemg_constraint_*` counters. Add a pin test that the metric name + label set is stable.

### E4 — Retroactive tombstone (operator-authorized, JIMINY-CORPUS-AUDIT-004 shape)

- Pre-batch snapshot: full JSON dump of the 4 target node_ids (`z5xgcm…`, `pwa2lm…`, and the 2nd-instance pair) → `docs/development/jiminy-corpus-constraint-detector-dedup-001/pre_batch_snapshot.json`.
- `batch_record.md` — per-node: current state, disposition (`TOMBSTONE_DUPLICATE_DUAL_SEVERITY`), rollback Cypher.
- Uniform `archive_reason` prefix: `jiminy_corpus_constraint_detector_dedup_001_operator_option_B_dual_severity_dual_mint`.
- Per-pair disposition: keep the semantically-correct twin (the one whose severity survives the E1 precedence rule — typically `must_not`); tombstone the wrong twin via direct Cypher.
- Small-batch discipline: 4 nodes in one transaction; operator sign-off logged in `batch_record.md`.

### E5 — Live Tier-3

Order matters: land E1+E2+E3 first, verify on live traffic, then run E4.

- Build + lint + unit clean.
- Deploy to mdemg-dev; grep server logs for `constraint detector: multi-emit collapsed to canonical` after the next natural burst of observe traffic (or seed 1 synthesized multi-severity obs).
- Confirm `mdemg_constraint_detector_multi_emit_suppressed_total > 0` via `/v1/metrics/snapshot`.
- Query Neo4j: `MATCH (c:MemoryNode {role_type:'constraint'}) WHERE c.created_at > $deploy_ts RETURN c.node_id, c.constraint_type, c.content` — assert no new dual-severity twins appear for content substrates that would have previously produced them.
- Only after all four green: proceed to E4.

### E6 — Docs

- `docs/features/constraint-detector.md` (new): purpose, pattern list, single-canonical-emit invariant, severity precedence, config knob, metric, known trade-off. Cross-link to `create-correction-dedup-001`.
- CLAUDE.md pin (append to JIMINY-CORPUS lineage).
- CHANGELOG entry.
- `sprint_post.md` after the batch closes.

## 6. Testing Plan

**Tier 1 — Unit (fast, pre-commit):**

- 7 new pin tests in `constraint_detector_dedup_test.go` (§E2).
- Existing `constraint_detector_test.go` suite green (regression floor).
- `go build ./...` + `golangci-lint run ./internal/conversation/... ./internal/config/... ./internal/metrics/...` clean.

**Tier 2 — Integration:**

- `internal/conversation/service_test.go` — extend to seed the 2 live-instance content substrates and assert exactly one `constraint:<type>` tag on the resulting observation.
- `internal/hidden/pipeline_test.go` — run the full L0 → CreateConstraintNodes pipeline on the live-instance fixtures; assert exactly 1 constraint node emerges per obs.
- Bench: `BenchmarkDetector_MultiSeverityContent` within ±10% of baseline.

**Tier 3 — Live (mdemg-dev):**

- Post-deploy log grep + metric scrape (§E5).
- Post-E4 batch: `MATCH (c) WHERE c.node_id IN [<4 targets>] RETURN c.is_archived, c.archive_reason` — verify tombstone landed with the expected reason string.
- CEILING-BREAK-2 T+168h re-check (2026-08-19) — record whether follow-rate baseline shifted after the 4 nodes left the active denominator; data point for CEILING-BREAK-2's honest steady-state estimate.

## 7. Commit Strategy

Sequential commits on `reh3376_dev01`:

1. `JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 E1+E2+E3: canonical single-emit in constraint detector` — code + pins + metric registration. All unit + integration tests green pre-commit.
2. `JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 E4: retroactive tombstone of 4 dual-severity dual-mint duplicates` — `batch_record.md`, `pre_batch_snapshot.json`, no code. Commit body captures operator sign-off + rollback Cypher.
3. `JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 E6: docs + CLAUDE.md pin + CHANGELOG` — `docs/features/constraint-detector.md` (new), CLAUDE.md pin append, sprint_post.md scaffold.

Commit 2 does NOT land unless commit 1 has passed E5 live verification. No amends; if E4 needs adjustment the fix is a new commit.

## 8. Verification Checklist

- [ ] `ConstraintDetectorDedupEnabled` default true; env var honored.
- [ ] Severity precedence pinned by test.
- [ ] 2 live-instance content substrates produce exactly 1 detection each.
- [ ] Legitimately-multi-rule content pin exists (documents the known trade-off).
- [ ] `mdemg_constraint_detector_multi_emit_suppressed_total{space_id}` exposed on `/v1/metrics/snapshot`, increments on collapse.
- [ ] Bench: hot-path p99 within ±10% of pre-change baseline.
- [ ] `go build ./...` clean; lint clean.
- [ ] Live grep on mdemg-dev shows collapse log line after next multi-severity observe.
- [ ] Pre-batch snapshot JSON captured for all 4 tombstone targets.
- [ ] Rollback Cypher captured in `batch_record.md` (per-node).
- [ ] Post-batch Neo4j spot-check confirms exactly the intended 4 nodes archived with the expected `archive_reason` prefix.
- [ ] CLAUDE.md pin added; CHANGELOG entry present; sprint_post.md drafted.
- [ ] CEILING-BREAK-2 T+168h re-check noted the tombstone timing (for baseline attribution).

## 9. Documentation Update (Epic 6)

`docs/features/constraint-detector.md` — new file. Sections:

- Purpose (hot-path pattern layer for `/v1/conversation/observe`).
- Pattern list (verbatim from `initPatterns`) + confidence ladder.
- Observation-type confidence boost table.
- **The single-canonical-emit invariant** (this sprint's contract): one L0 obs → ≤1 `DetectedConstraint`.
- **Severity precedence**: `must_not` > `must` > `should_not` > `should` > `deadline` — with the audit-004 rationale.
- Config: `CONSTRAINT_DETECTOR_DEDUP_ENABLED` (default true).
- Metric: `mdemg_constraint_detector_multi_emit_suppressed_total{space_id}`.
- Known trade-off (multi-rule-in-one-obs cannot be split by the detector).
- Cross-links: `constraint-nodes.md` (promoter), `create-correction-dedup-001/sprint_post.md` (sibling defect at correction layer).

CLAUDE.md pin appended under the JIMINY-CORPUS lineage.
CHANGELOG.md entry under Unreleased.

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Collapsing legitimately-different rules that share one L0 obs | Medium | Documented as a known trade-off; pinned by test; operators guided to split multi-rule observations into separate `observe` calls. |
| Hot-path perf regression | Low | Collapse is O(1) over a ≤5-element map, no regex work, no I/O. Bench pin in E2 catches any regression. |
| Arc-window collision with CEILING-BREAK-2 T+168h re-measurement | Low-Medium | E4 timing recorded in `batch_record.md` so re-check can attribute any baseline shift correctly. Recommended: run E4 within the arc window to give the re-check a cleaner honest reading. |
| Precedence choice wrong (e.g., `must` should outrank `must_not` in some domain) | Low | Test pin documents the choice; if a future audit surfaces a counterexample, the precedence table is a single-line change guarded by the same flag. |
| Tombstone of the wrong twin (fingerprint mixup between the pair) | Low | Pre-batch snapshot captures full JSON of BOTH twins per pair; rollback Cypher un-archives the exact node_id; small batch (4 nodes) + operator sign-off. |
| Fail-closed on flag misconfiguration | None | Flag default is `true`; setting to `false` reverts to pre-sprint state; no regression path beyond re-opening the class. |

## 11. Rollback Procedures

**Code (E1/E2/E3):**

- Fastest revert: set `CONSTRAINT_DETECTOR_DEDUP_ENABLED=false` in `.env`, restart. Instantly reverts to pre-sprint detector behavior; no data path affected.
- Full code revert: `git revert <commit-1-sha>`.

**Tombstones (E4):**

- Per-node rollback Cypher captured in `batch_record.md`:
  ```cypher
  MATCH (c:MemoryNode {node_id: $nodeId, space_id: $spaceId})
  SET c.is_archived = false,
      c.archive_reason = null,
      c.archived_at = null,
      c.archived_by = null
  ```
- `pre_batch_snapshot.json` provides full-state restore if any property beyond the archive fields needs to be reinstated.

**Metric (E3):**

- Removing the metric is a code revert. `/v1/metrics/snapshot` gracefully handles a series that stops appearing.

## 12. Documents Accessed

- `internal/conversation/constraint_detector.go` — the regex pattern list + `Detect` loop being fixed.
- `internal/conversation/service.go:380–442` — the observe-path callsite that consumes detector output.
- `internal/hidden/constraint_nodes.go` — the L1 promoter that mints one node per emitted type (unchanged by this sprint; consumes the fixed detector output).
- `internal/hidden/constraint_gate.go` — reference for the fail-open pattern reused conceptually.
- `internal/conversation/constraint_detector_test.go` — existing pin suite that must stay green.
- `docs/development/create-correction-dedup-001/sprint_post.md` — sibling sprint at the promotion layer; pattern for config knob + telemetry field + pin-test file naming.
- `docs/development/jiminy-corpus-audit-004/sprint_post.md` + `batch_record.md` — precedent for operator-authorized direct-Cypher tombstone batches with rollback Cypher + snapshot; source of the 2 live node_ids and the 2nd-instance pair.
- CLAUDE.md pin lineage: JIMINY-CORPUS-001, JIMINY-CORPUS-002, JIMINY-CORPUS-AUDIT-004, JIMINY-CEILING-BREAK-2, CREATE-CORRECTION-DEDUP-001.
