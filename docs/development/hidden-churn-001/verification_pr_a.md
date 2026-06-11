# HIDDEN-CHURN-001 PR-A — Verification

**Date:** 2026-06-11 · live stack, space `mdemg-dev`.

## Tier 1
matchTheme (exact/threshold/claimed/best-of); range pin test (phase 22 ∈
node-creation range); hidden suite green; lint 0 (dead churn helper removed).

## Tier 3 — the churn test
Two consecutive live consolidations:
- Run 1: 23 themes, sample of 5 node_ids captured.
- Run 2: **23 themes, 5/5 sampled node_ids SURVIVED**, `themes_created: 0`
  — every cluster matched an existing theme and updated in place. Before
  this fix, all 23 were destroyed and recreated with new IDs every cycle.
- Surviving theme spot-check: 7 member edges, freshly rewired this run,
  real cosine weights (avg 0.893 — HIDDEN-WEIGHT compounding).
- Emergence: `EMERGENCE_ENABLED=true` live; "dynamic emergence enabled"
  logged; the automated path now includes phase 22 (pin test guards it).

## PR-B (declared, next)
Coverage retune (config maxThemes/min-samples, density assignment, gauge
+ alert), childless-L2 repair (10,395 live), `mdemg concepts trace`,
surface ThemesUpdated in the consolidate API response.
