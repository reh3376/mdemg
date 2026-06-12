# Sprint Plan — DORMANT-CENSUS-001: The Standing Dormancy Guarantee

## 1. Header & Metadata
2026-06-12 · branch `reh3376_dev01` · Roadmap Q3 — the FINAL committed
sprint, deliberately sequenced last · effort 4d est · risk medium (route
prunes are API-surface changes; the signal wire touches guidance
ranking).

## 2. Problem Statement
The quarter's bug classes (24-day Hebbian no-op, 9-week guidance
dormancy, dead actuators, zero-write tables, the flat-dead surprise
chain) share one shape: **a writer or surface with no consumer, invisible
until someone looks**. This sprint turns the one-off looking into CI:
a machine-checkable endpoint↔consumer inventory (187 routes; recon
flagged 58 zero-production-consumer — with KNOWN false positives:
`/v1/jiminy/classify` is called by pre-write-check.py in /strict,
`/v1/conversation/snapshot` by pre-compact.sh, `/v1/skills/` by the
skill-recall path, `/v1/memory/ingest-codebase` by INGEST-EXEC-001 —
proving greps are not an inventory) plus the adjudicated prune/wire
pass, headlined by `SignalLearner.GetStrength`: a fully-shipped Hebbian
persistence layer (V0024 SignalState, supervised 30s flush, startup
hydration, live emission/response stream since HOOKWIRE-001) whose read
side has ZERO production callers.

## 3. Scope & Constraints
**In**: (1) `docs/api/route_consumer_inventory.json` — one entry per
route: consumers (hook/cli/mcp/script/dashboard/internal), uats_specs,
disposition (ACTIVE / OPERATOR_SURFACE / INTERNAL / DEFERRED:<trigger> /
PRUNED) — and `scripts/verify_route_consumers.py`, merge-blocking in CI:
extracts the live route table from server.go and fails on bidirectional
drift (unlisted route, or inventory entry for a removed route).
*Disclosed deviation from the roadmap's "UATS consumers field": a single
inventory covers all 187 routes including the ~55 spec-less ones; per-spec
fields can backfill later.* (2) Orchestrator verification of all 58
flagged routes before disposition. (3) **WIRE SignalLearner.GetStrength**
(lane-2 adjudication adopted): strength blends into the Guide() sort as
a within-priority tiebreaker — `score = (1-w)·confidence + w·strength`,
`JIMINY_SIGNAL_STRENGTH_WEIGHT` default 0.2, 0 = off; selection/filtering
untouched, ordering only. (4) Prune pass limited to UNAMBIGUOUS legacy
(verified zero consumers + superseded-by-named-successor: /api/graph/*,
/viz/topology, /v1/feedback→jiminy/feedback, /v1/alerts/grafana→native
evaluator — each individually re-verified); PREDICTS/FORESHADOWS removed
from allowed relationship types (never created anywhere; Note-03/08
re-spec will re-add deliberately if Y2 ever lands). (5) Lane-3 verdict
OVERRIDES recorded: ft_* tables = KEEP (named sinks per the
FT-RECURSIVE-001 spec — not tombstone); gated-off features = intentional.
**Out**: removing operator escape hatches or internal/eval surfaces
(they get dispositions, not deletion); UATS spec edits beyond what
prunes require; the deferred-Grafana panel follow-ups (documented).

## 4. Dependencies
3 recon lanes (a52df2bc routes / ac6551ae signal / a628c8cd writers);
internal/api/server.go:2358-2623 (route table); internal/ape/
signal_learner.go; internal/jiminy/service.go:933-940 (sort);
scripts/verify_config_consumers.py (gate precedent); ci.yml.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** inventory + gate (generate from route table;
seed consumers from recon; verify the 58 flagged routes one-by-one —
orchestrator greps per route, not trusted lane output; CI step) ·
**Epic 2** signal wire (config + sort blend + nil guards + tests) ·
**Epic 3** prune pass (verified legacy routes + handler code; PREDICTS/
FORESHADOWS; UATS spec adjustments if any spec covered a pruned route) ·
**Epic 4** live Tier 3 (gate green in CI; pruned routes 404 live;
guidance ordering responds to strength — set weight high on a test call
and observe reorder; signal stream values logged) · **Epic 5** docs
(feature doc, CHANGELOG, post), push.

## 6. Testing Plan
T1: gate self-test (synthetic drift both directions fails); sort-blend
unit tests (weight 0 = pure confidence; strength tiebreak within
priority); inventory JSON schema check. T2: full `go test ./internal/...`;
gate green against the real tree. T3 (live): pruned routes return 404;
`/v1/jiminy/guide` ordering shifts under JIMINY_SIGNAL_STRENGTH_WEIGHT
extremes; SignalState strengths inspected in Neo4j (variance evidence).

## 7. Commit Strategy
Per-epic · lint each · push once (auto-PR) · summary · CI watch.

## 8. Verification Checklist
- [ ] Inventory covers all 187 routes; gate merge-blocking + green
- [ ] All 58 flagged routes verified; false positives corrected in inventory
- [ ] GetStrength wired (weight config, 0=off); ordering live-verified
- [ ] Prune list: each route individually re-verified before removal
- [ ] PREDICTS/FORESHADOWS removed from allowed types
- [ ] ft_* KEEP override + triggers recorded in inventory
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 5 (never cut).

## 10. Risks & Mitigations
Pruning a route someone uses → only superseded-by-named-successor
legacy, each re-verified; dispositions (not deletions) for everything
ambiguous; 404s are loud, and restoration is a revert. Signal blend
destabilizes guidance order → tiebreaker-only within priority, weight
0.2 conservative, 0 = off; live-verified before push. Inventory rots →
that's the point of the merge-blocking bidirectional gate.

## 11. Documents Accessed
ROADMAP:57; the 3 lane reports; FT-RECURSIVE-001 spec (ft_* sinks);
prior follow-up records (RSICStore.CleanupExpired — now verified WIRED,
closing that standing follow-up).

## 12. Rollback Procedures
Inventory + gate: one CI step, removable. Prunes: git revert restores
routes. Signal wire: weight 0 disables without code change.
