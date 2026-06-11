# UXTS-CI-001 — Sprint Post

Closed: 2026-06-11 · Branch: `reh3376_dev01` · Roadmap: Q3 Phase 2.

## Shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan + CI recon | `05b0431` |
| 1–3a | TSDB in CI + tsdb un-excluded; UOTS un-zombied; UVTS step deleted; ULTS hash gate + UBENCH contract merge-blocking; ULTS/UOTS rot remediated | `03cbe29` |
| 3b | neural pytest+ruff job + the rot it caught | `edd4c51` |
| 4 | Drift checker → all 15 frameworks; matrix refreshed; drift gate un-zombied | `59bb608` |
| 5–6 | This PR's CI run green with all gates live; docs | (docs commit + PR) |

## The thesis, proven five times in one sprint

Every dormant gate that was armed immediately caught real rot:

1. **ULTS hash gate** → 4 genuine prompt drifts (ape.reflect,
   hidden.reclassify, jiminy.evaluate_llm, retrieval.rerank_cross) that
   had accumulated across shipped sprints — plus its own parser couldn't
   read annotated source locations (5/17 specs failing on an artifact).
2. **UOTS un-zombied** → the grafana_neo4j_dashboard spec was stale
   against the GRAFANA-AUDIT-001 restructure, and the 'artifact' spec
   type was unimplemented (runner honestly failed it; now implemented).
3. **neural CI job** → two silently-failing tests (MoE-era
   `synth-qwen3.6-local` label predating the 2026-04-22 pivot; a
   scalar-vs-array prompt-hash assumption) + 32 ruff findings incl. dead
   batched mx.arrays in the GRPO adapter.
4. **Drift checker** → the matrix was 51 days stale (UATS off by +96
   specs) and the checker step was itself a continue-on-error zombie.
5. **(From UATS-GAP-001, same week)** the deep-merge variant pitfall and
   the deployment-dependent breaker variant.

## Disposition decisions (data-decided, disclosed)

- **UVTS step deleted**, not converted: CI has a stub embedder, no LLM,
  and no seeded lnl_demo corpus — semantic grades there are noise. UVTS
  remains the live gate (`make test-uvts-quick/full` + `uvts_ab_compare`
  per-question regression gate).
- Roadmap's "23 tsdb-tagged contracts" → 8 exist on disk (6 UATS +
  2 UOTS); the figure was aspirational.
- Sidecar app tests (torch/sentence-transformers, multi-GB) stay
  local-only; mlx-dependent RL tests excluded on ubuntu (Apple-Silicon
  dependency) — both documented in the job.

## Verification

- Local: ULTS 17/17 exit 0; UOTS 11/11 exit 0; UBENCH contract green;
  neural 610 passed + ruff clean; drift checker 15/15.
- Tier 3: this PR's own GitHub Actions run with every new gate
  executing (see PR checks).

## Documents Accessed

`sprint_plan_uxts_ci_001.md` §11; `.github/workflows/{ci,uxts-canonical-specs}.yml`;
`docs/tests/{ults,ubench,uits,utds,uaits}/`; `docs/api/api-spec/uots/`;
`scripts/verify_uxts_drift.py`; `docs/development/UXTS_FRAMEWORK_MATRIX.md`;
`neural/` (pyproject, training tests); local gate runs.
