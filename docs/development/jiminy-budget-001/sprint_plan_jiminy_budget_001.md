# Sprint Plan — JIMINY-BUDGET-001 + OUTCOME-ATTRIB-001

## 1. Header & Metadata
2026-06-12 · branch `reh3376_dev01` · Roadmap Q3 Phase 2–3 (last
committed member before DORMANT-CENSUS-001) · effort 3d est · risk low
(config/threading fixes; no schema change).

## 2. Problem Statement
The GUIDANCE-SYNTH-001 starvation class persists for every
non-this-machine install: `Guide()`'s direct-path budget is
`JIMINY_TIMEOUT_MS` default **15s** (`service.go:660`) while live
synthesis runs ~50s — only the warm path got the 90s budget; fresh
installs (cold warm-cache) starve. `/reformulate` hardcodes **10s**
(`handlers_jiminy.go:579`). `config.Validate()` has no budget-coherence
warning. Outcome attribution is contaminated at the source:
`PersistGuidanceOutcome` is called with `sessionID=""`
(`service.go:1524`) so every GUIDANCE_OUTCOME edge has null session;
`JiminyStats.TotalGuidanceIssued` counts guidance *with outcomes*, not
guidance *surfaced* (the gap — feedback never arriving — is invisible);
late feedback drops ("guidance_id expired from tracker") are
Warn-log-only. And `JIMINY_OUTCOME_LLM_MAX_TOKENS` defaults to **100**
— a truncation risk on the classifier's `reasoning` field (truncated
JSON → parse fail → OutcomeUnknown → heuristic fallback, the exact
artifact class JIMINY-OUTCOME-002 made distinguishable).

## 3. Scope & Constraints
**In**: (1) budget derivation — `JIMINY_TIMEOUT_MS` default becomes 0 =
"derive from `JIMINY_WARM_COMPUTE_TIMEOUT_MS`" (90s single source);
explicit positive value still overrides; same derivation for new
`JIMINY_REFORMULATE_TIMEOUT_MS` (default 0 = derive). (2)
`config.Validate()` warning on explicit `JiminyTimeoutMs <
JiminyWarmComputeTimeoutMs`. (3) max_tokens ≥3000 floors on the three
Jiminy LLM knobs (outcome 100→3000, synthesis 2000→3000, evaluate
2000→3000 — generation stops at JSON end; the floor is free insurance
per the standing rule). (4) `feedbackSessionID` into
`PersistGuidanceOutcome`. (5) surface-vs-outcome split:
`mdemg_jiminy_guidance_surfaced_total{space_id}` counter in `Guide()` +
`mdemg_jiminy_feedback_dropped_total{space_id}` on tracker-expiry drops
(durable in metric_samples → coverage ratio computable); JiminyStats
field comment corrected (TotalGuidanceIssued = with-outcomes). (6)
hook-budget coherence: locate `JIMINY_EFFECTIVENESS_TTL_SEC` default;
raise if < 600s; document the 5s-curl / 60s-server / TTL chain.
**Out**: reworking the hook curl timeout (fire-and-forget is correct
post-SUPERVISOR-002); historical edge backfill (forward-only);
effectiveness-formula changes.

## 4. Dependencies
Recon report (ac10971); `internal/jiminy/{service,persistence,stats}.go`;
`internal/api/handlers_jiminy.go:579`; `internal/config/config.go`
(2254/2282/2286/2352/2312/2337 + Validate()); metrics collectors.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** budgets (derive chain + reformulate config +
Validate() warning) · **Epic 2** attribution (sessionID + counters +
TTL audit) · **Epic 3** max_tokens floors · **Epic 4** live Tier 3 +
docs + push.

## 6. Testing Plan
T1: derivation tests (0 → warm budget; explicit override wins);
Validate() warning test; sessionID threading test; floor tests. T2:
full `go test ./internal/...`; scanner green. T3 (live): restart →
guide/feedback round-trip → GUIDANCE_OUTCOME edge carries session_id
(Neo4j check); surfaced counter lands in metric_samples; reformulate
under a >10s synthesis no longer 504s (timeout now 90s-derived).

## 7. Commit Strategy
Per-epic · lint each · push once · summary comment · CI watch.

## 8. Verification Checklist
- [ ] JIMINY_TIMEOUT_MS=0 derives 90s; explicit override honored
- [ ] /reformulate config-driven (derived default)
- [ ] Validate() warns on budget incoherence
- [ ] 3 max_tokens defaults ≥3000
- [ ] GUIDANCE_OUTCOME edges carry session_id (live)
- [ ] surfaced + dropped counters live in metric_samples
- [ ] TTL ≥ 600s or raised; chain documented
- [ ] CHANGELOG + post

## 9. Documentation Update — Epic 4 (never cut).

## 10. Risks & Mitigations
Longer direct-Guide budget holds hook callers? No — hooks use the warm
channel; direct Guide() callers are API consumers who already waited or
timed out client-side. max_tokens floors raise cost? No — completion
stops at natural end; floors only prevent truncation.

## 11. Documents Accessed
ROADMAP:43; recon ac10971 (file:line map); GUIDANCE-SYNTH-001 +
SUPERVISOR-002 records; CLAUDE.md min-floor memory rules.

## 12. Rollback Procedures
Code-only; revert commits. Forward-only attribution.
