# Sprint Plan — UXTS-CI-001: Wire the Dormant Forcing Functions

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | UXTS-CI-001 |
| Sprint line | `docs/development/uxts-ci-001/` |
| Date opened | 2026-06-11 |
| Branch | `reh3376_dev01` |
| Roadmap slot | Q3 Phase 2 (committed; "the forcing functions were built and never wired") |
| Estimated effort | 4 dev-days |
| OpenAI spend | $0 |
| Risk level | Medium (CI surgery; a wrong hard-fail blocks every future PR — each gate is proven locally AND on this PR's own CI before merge) |

## 2. Problem Statement

The gates against the dominant historical bug class exist and are dormant:

1. **No TimescaleDB in CI** → the 8 `tsdb`-tagged contracts (incl. both
   eventgraph federation specs and `metrics_snapshot`) never run on PRs.
   (Roadmap said "23" — recon counts 8 on disk across UATS+UOTS; the
   figure was aspirational. Disclosed.)
2. **UOTS and UVTS steps are zombies** — both run with
   `continue-on-error: true` (ci.yml:263, :278); they can never block a
   merge.
3. **`test-ubench-contract` is merge-blocking nowhere** — the forcing
   function built specifically to prevent the Phase 10 guardrail gap
   class has no CI invocation.
4. **`ults_runner --verify-hashes` never runs** — prompt drift across the
   16 production LLM call sites (each spec pins `system_prompt_hash` +
   source file:line) is undetected until an eval regresses.
5. **~750 neural/ Python tests have zero CI** (pytest+ruff configured in
   `neural/pyproject.toml`, never invoked).
6. **The UxTS drift checker covers 10 of 15 frameworks** (ULTS, UITS,
   UTDS, UAITS, UNTS omitted) and `UXTS_FRAMEWORK_MATRIX.md` is 51 days
   stale (UATS off by +96 specs).

## 3. Scope & Constraints

**In scope**: the 7 roadmap deliverables above; remediation of any spec
that fails once un-excluded (fix or re-tag with rationale); this PR's own
CI run as the Tier 3 proof.

**Out of scope (disclosed)**: making UVTS a *quality* gate (semantic
scoring needs an embedder + a seeded corpus CI doesn't have — the
roadmap explicitly allows "un-zombie OR delete"; decision recorded in
Epic 2); the ~30 full-corpus UATS failures living in tags that REMAIN
excluded (`llm_required`, `unts`, `sidecar_required` — they require
services CI legitimately lacks); macOS/MLX-dependent neural tests (CI is
ubuntu; mlx-importing tests are excluded by collection, documented).

**Constraints**: every converted/added gate must be green on this PR
before it can merge (self-proving); no hardcoded versions where the
compose file is the source of truth; sequential epics; surprises = own
fix commits.

## 4. Dependencies

- `.github/workflows/ci.yml` (neo4j service block as the service
  template), compose template (TSDB image/env), `Makefile` targets,
  `docs/tests/{ults,ubench}/runners/`, `scripts/verify_uxts_drift.py`,
  `docs/development/UXTS_FRAMEWORK_MATRIX.md`, `neural/pyproject.toml`.
- AUTO_MIGRATE handles TSDB schema in CI (V0001..V00xx on first boot).

## 5. Implementation Plan (sequential epics)

**Epic 0** — plan + recon committed.

**Epic 1 — TSDB service in CI + un-exclude `tsdb`**
- Add `timescaledb` service to the test job (image pinned to the compose
  template's version; health-checked), wire `TSDB_*` env for the server
  step, drop `tsdb` from the UATS `--exclude-tag` list.
- Local proof: `--include-tag tsdb` run green against a fresh TSDB
  container; CI proof on this PR.

**Epic 2 — Un-zombie UOTS; decide UVTS**
- UOTS: remove `continue-on-error`; fix whatever fails (that's the
  point).
- UVTS: data-decided disposition — semantic grading in CI (no embedder,
  no seeded corpus, `MDEMG_ALLOW_NO_MLX=1`) measures nothing; **delete
  the CI step** and document that UVTS gates live changes via the
  documented local flow (`make test-uvts-quick` + A/B harness), per the
  roadmap's "un-zombie or delete" option. Recorded here + CHANGELOG.

**Epic 3 — New merge-blocking gates**
- `test-ubench-contract` job (pure-Python, no live deps).
- `ults_runner --verify-hashes` step (prompt-drift tripwire for the 16
  LLM sites).
- `neural-tests` job: `ruff check neural/` + `pytest neural/` with
  mlx-dependent modules excluded on linux (collection-level ignore or
  marker; exact mechanism decided at execution and pinned in the job).

**Epic 4 — Drift checker + matrix**
- Extend `verify_uxts_drift.py` to ULTS, UITS, UTDS, UAITS (+UNTS via its
  registry); refresh `UXTS_FRAMEWORK_MATRIX.md` counts (UATS 124→actual);
  ensure the checker runs hard-fail in CI.

**Epic 5 — Tier 3 = this PR's CI run**
- The live system for CI work is GitHub Actions itself: every gate green
  on the PR, with the UATS step now executing the tsdb-tagged specs
  (visible in the step log), UOTS hard-failing-capable, ubench/ults/
  neural/drift steps present and green.

**Epic 6 — Documentation (final epic — never cut)**
- CHANGELOG, CLAUDE.md (CI gate inventory note), sprint post.

## 6. Testing Plan

Tier 1 = each runner/checker invoked locally exactly as CI will. Tier 2 =
full local `make test-api`-equivalent with the new tag set + drift checker
run. Tier 3 = the PR's own Actions run (real CI, real services, real
gates) — failures there are the deliverable working.

## 7. Commit Strategy

One commit per epic; gate-breakage fixes get their own commits; single
push at the end (auto-PR) — then iterate on the PR's CI until green.

## 8. Verification Checklist

- [ ] CI test job boots TimescaleDB; server starts with TSDB_ENABLED
- [ ] UATS step runs WITHOUT the `tsdb` exclude; tsdb-tagged specs pass in CI
- [ ] UOTS step hard-fails on failure (and is green)
- [ ] UVTS step removed with documented rationale
- [ ] `test-ubench-contract` merge-blocking and green
- [ ] `--verify-hashes` step present and green (prompt drift tripwire live)
- [ ] neural ruff+pytest job green (mlx exclusions documented in-job)
- [ ] Drift checker covers 14+UNTS frameworks, matrix refreshed, CI hard-fail
- [ ] This PR's full CI run green (the Tier 3 gate)
- [ ] CHANGELOG + CLAUDE.md + post.md

## 9. Documentation Update — Epic 6 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| New hard gates block future PRs on flakiness | Medium | High | Every gate proven on THIS PR repeatedly; flaky cases re-tagged/skipped with rationale, never silently soft-failed |
| TSDB service slows CI | Low | Low | Single container + AUTO_MIGRATE; UATS already runs the server |
| neural tests import mlx on ubuntu | High | Medium | Collection-level exclusion decided at execution; job documents what is excluded and why |
| Hash verification fails on legitimate prompt changes | Medium | Low | That IS the gate: the failing dev updates the spec hash in the same PR (documented workflow) |

## 11. Documents Accessed

Recon (agent, 2026-06-11): `.github/workflows/ci.yml` (services 104-111,
UATS 228-233, UOTS 262-267, UVTS 277-283), `Makefile` (test-ubench-contract
292-298), `docs/tests/ults/runners/ults_runner.py` (:477 --verify-hashes,
:37-54 task list), `neural/pyproject.toml`, `scripts/verify_uxts_drift.py`
(:43-74), `docs/development/UXTS_FRAMEWORK_MATRIX.md`, tsdb-tag census
(8 specs), `/tmp/api-report.json` (443 cases, 0 fail post-UATS-GAP-001).

## 12. Rollback Procedures

CI-only changes — revert commits; no data or runtime-config changes.
Each gate is independently revertable (separate steps/jobs).
