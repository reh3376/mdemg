# Sprint BENCH-SIDECAR-APPLY-001 — `--apply-tsdb`: benchmark persistence without the manual psql step

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | BENCH-SIDECAR-APPLY-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~0.5 dev-day |
| Parent | Twice-disclosed follow-up (FT-BENCH-REFRESH-001 + APE-REFLECT-EVAL-REFRESH-001): `--persist-tsdb` writes a SQL sidecar but does NOT INSERT — the operator must run `psql < sidecar.sql` by hand, and both sprints' automation stumbled on it (the second time a background watcher's apply step silently didn't fire) |

## 2. Problem Statement

`run_benchmark --persist-tsdb` renders the V0012 INSERTs to a sidecar file and prints "apply with: psql …". Every consumer (operator, FT-RECURSIVE gate stage, session automation) must remember the second step; forgetting it leaves `benchmark_runs` stale while the report JSON looks complete — the exact stale-panel class FT-BENCH-REFRESH-001 exists to prevent. Both recent benchmark runs required manual recovery of this step.

## 3. Scope & Constraints

**In scope**: new `--apply-tsdb` flag — after writing the sidecar (kept as the audit artifact), connect via psycopg using the same `TSDB_*` env DSN pattern as `uvts_runner._tsdb_dsn()` and execute the rendered statements in one transaction. Failure is **non-fatal**: WARN + keep the sidecar + the "apply with" hint (report JSON remains the primary artifact — mirrors the UVTS best-effort pattern). Unit tests (statements executed + committed; graceful failure). Live Tier-3 tiny run proving rows land with zero manual steps, followed by surgical cleanup of the smoke rows (restore-state rule). Docs: feature-doc recipe drops the manual step; CHANGELOG.
**Out of scope**: changing the sidecar format, the Go-side jobhealth wiring, or FT-RECURSIVE controller stages (they can adopt the flag later).
**Constraints**: no hardcoded connection values (env-driven DSN, same defaults as the UVTS runner); sidecar file always written regardless (audit trail); psycopg absence → WARN + skip apply (not a hard dep for JSON-only users).

## 4. Dependencies

✅ `persist.py` exposes `render_benchmark_runs_insert` + `render_benchmark_results_inserts`; ✅ psycopg installed (UVTS runner uses it); ✅ TSDB up locally for the smoke.

## 5. Implementation Plan (sequential)

- **E0** plan (this doc).
- **E1** `persist.py::apply_to_tsdb(report, dsn) -> int` — renders the same statements, executes in one transaction, returns statement count; import-guarded psycopg.
- **E2** runner: `--apply-tsdb` flag (implies sidecar write first); success prints "tsdb applied: N statements (run_id …)"; failure prints WARN + existing hint.
- **E3** unit tests (fake connection: executed+committed; psycopg-missing and connect-failure graceful paths).
- **E4** live Tier-3: `--task-filter consulting.classify --rows-per-spec 1 --n-runs 1 --apply-tsdb` → verify rows landed with no manual step → surgical smoke-row cleanup (DELETE by the smoke run_id from both tables; count-verified before/after — the restore-state rule).
- **E5** docs: `docs/features/ft-benchmark-freshness.md` recipe updated; CHANGELOG; sprint post.

## 6. Testing Plan

Tier 1: E3 unit tests. Tier 2: `pytest neural/` suite green. Tier 3: E4 live end-to-end insert + verified cleanup.

## 7. Commit Strategy

`docs(E0)` → `feat(E1+E2+E3)` → `docs(E4 evidence + E5)`.

## 8. Verification Checklist

pytest green · live rows landed via flag alone · smoke rows cleaned + count restored · docs updated · pushed.

## 9. Documentation Update

Feature doc recipe; CHANGELOG Fixed; sprint post. CLAUDE.md: amend the FT-BENCH-REFRESH-001 note's disclosed follow-up as CLOSED.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Double-apply (flag + manual psql) duplicates rows | Low | benchmark_runs PK rejects the run row; documented; statements run in one tx so a PK conflict aborts cleanly |
| DSN env mismatch in odd environments | Low | Same env names/defaults as the UVTS runner (established pattern); WARN + sidecar preserved on failure |
| Smoke pollutes the Latest Run panel | Low | Cleanup step removes the smoke run immediately; panel returns to `ed062ea8` |

## 11. Rollback

Revert commits; flag is additive (default off — existing behavior unchanged).

## 12. Documents Accessed

`neural/benchmarks/{persist.py,run_benchmark.py}`; `docs/tests/uvts/runners/uvts_runner.py` (DSN pattern); FT-BENCH-REFRESH-001 + APE-REFLECT-EVAL-REFRESH-001 posts (the disclosures); V0012 schema.
