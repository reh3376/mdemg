# Wave 2 / Lane 2 — operations + guides + api (14 files @ 831eb0a)

8 CURRENT · 4 HISTORICAL_OK (dated records correctly framed; vllm-mlx
superseded banner intact) · 2 DRIFT_MINOR.

## DRIFT_MINOR
- pre-campaign-checklist.md: "schema v8+" vs real requirement 26
  (config.go) — 18 versions behind on a live checklist. FIX-BATCH.
- live-validation-findings.md: P3 doc-gap finding presented alongside
  P0-P1 bugs; framing clarification only.

## CODE finding (re-confirmed independently by this lane)
docker-compose.yml + compose template: LLM_ENDPOINT default
host.docker.internal:8101 + stale Phase-11.6 comment — pre-13.5. Same
finding as wave-1; now double-sourced. FIX-BATCH (code).
