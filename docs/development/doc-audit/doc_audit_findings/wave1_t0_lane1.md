# Wave 1 / Lane 1 — CLAUDE.md + 00_README_v2.md (audited @ 831eb0a)

## CLAUDE.md — CURRENT (lane verdict DRIFT_MAJOR reversed on re-verification)
Lane attributed the 8101/8102 discrepancy to CLAUDE.md; orchestrator
re-verification shows CLAUDE.md correctly describes live production
(llama-server :8102, serving since 2026-05-03). Verified clean: TSDB
schema 26, all cited endpoints (jiminy/*, admin/breakers, eventgraph/*),
config defaults (JIMINY_OUTCOME_LLM_ENABLED, CONSULTING_CLASSIFY_TIMEOUT_MS
30000, J17_SIDECAR_TIMEOUT_MS 1000, SPARSE/CONTEXT/EVENTGRAPH flags).

## 00_README_v2.md — DRIFT_MAJOR
Version ledger frozen at v5.12 (2026-04-30): the newest entry's header
text presents `mlx_lm.server :8101` routing as the cutover state. The
Phase 13.5 runtime cutover (2026-05-03), MODEL-DIST-001/002,
FT-RECURSIVE-000, and FT-CLASSIFY-002 have no version entries — in the
doc chartered as "canonical plan + full history." The STATUS blockquote
at top IS maintained (DOC-TRUTH-001/FT-RECURSIVE-000 patched it).
Fix proposal: add v5.13+ ledger entries (append-only, R-LT-4-clean).

## CODE finding (for the fix batch, not a doc edit)
`internal/cli/compose_templates/docker-compose.yml:84` —
`LLM_ENDPOINT` defaults to `http://host.docker.internal:8101/v1` (dead
port post-13.5). Same stale-8101 class as the benchmark config fixed in
FT-CLASSIFY-002. Docker deployments without an explicit .env override
point at nothing.
