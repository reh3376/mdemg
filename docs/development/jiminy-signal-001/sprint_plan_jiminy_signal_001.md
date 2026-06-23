# Sprint Plan — JIMINY-SIGNAL-001: Honest Jiminy Health Signals

## 1. Header & Metadata
2026-06-15 · branch `reh3376_dev01` · P0 (operator-bumped ahead of doc-currency)
· first of two sprints decomposed from the "guidance not reaching agent"
investigation · effort ~1–1.5d · risk low-medium (changes a CRITICAL alert
condition + a health gauge that feeds RSIC's health score — every change makes
a lying signal honest; no guidance behavior changes here).

## 2. Problem Statement
Two Jiminy health signals are lying, in opposite directions, and between them
they hid a real problem AND raised a false alarm (root-caused via two subagent
investigations, file:line verified):

- **False-positive CRITICAL** — "Jiminy Pipeline Critical — guidance not
  reaching agent" fires ~8×/day on a HEALTHY Jiminy. It is not a delivery check;
  it's an RSIC reflection insight (`self_reflect.go:331`, pattern
  `synergy_jiminy_unhealthy`) gated on `!report.JiminyHealthy`, where
  `JiminyHealthy = cfg.JiminyEnabled && jiminySvc != nil` (`server.go:793`) is
  set only inside a synergy block (`self_assess.go:185`) that is conditionally
  skipped — when skipped, the bool stays at Go zero-value `false`, so a
  delivering Jiminy is reported "down." Live: `/v1/jiminy/latest` returns fresh
  guidance; the alert text actively misdirects triage.
- **Inflated follow-rate gauge** — `mdemg_jiminy_follow_rate = 0.725`
  (`stats.go:34-46`, `GetGuidanceStats`) uses
  `count(DISTINCT CASE WHEN outcome='followed' THEN guidance_id END) /
  count(DISTINCT guidance_id)` over Neo4j `GUIDANCE_OUTCOME` edges. 59/217
  guidance_ids carry multiple outcome types, so a guidance_id lands in the
  "followed" numerator if **any** edge is followed — systematically inflating.
  The honest dashboard panels (TSDB `constraint_outcomes`, one row per outcome:
  903 followed / 908 partial / 1752 ignored / 20 contradicted) show ~**0.18**.
  The gauge says 0.725 — a ~4× lie. **RSIC's `scoreGuidance` consumes the
  inflated gauge** (`self_assess.go:450`, 50% weight), so the health score
  believes guidance is healthy and masks the real effectiveness problem.

This sprint makes both signals honest. It does NOT change guidance behavior —
that's JIMINY-EFFECTIVENESS-001 (the real ~18%-effectiveness / trust-decay fix,
which needs these honest signals to validate against).

## 3. Scope & Constraints
**In:**
1. **Fix the false CRITICAL** — (a) make `JiminyHealthy` a REAL operability
   signal, not `enabled && svc != nil`: wire it to the health-prober signal that
   already reports `"jiminy":"ok"` in `/healthz` (or a recent
   `guide_calls`/`latest_served` delta / `latest_age` staleness). (b) Guard the
   `synergy_jiminy_unhealthy` insight so it cannot fire on an UNASSESSED
   (zero-value) flag — add `report.SynergyAssessed` set true only inside the
   `self_assess.go:185` block, require `SynergyAssessed && !JiminyHealthy`. (c)
   **Rename** the alert text away from "guidance not reaching agent" to an
   accurate "Jiminy service unavailable — catastrophic-forgetting risk"
   (`task_dispatch.go:823`).
2. **De-inflate the follow-rate gauge** so the gauge, the dashboard panels, and
   RSIC's `GuidanceHealth` all agree. **Options (pick at execution):**
   - **Option A (minimal, Neo4j edge-level):** drop the `DISTINCT guidance_id`
     dedup in `stats.go` — count outcomes edge-level. Removes the inflation
     (0.725 → ~0.38) with a one-query change, but still differs from the panels'
     0.18 (Neo4j `GUIDANCE_OUTCOME` is a constraint_code-resolved SUBSET of the
     fuller TSDB `constraint_outcomes`).
   - **Option B (faithful, TSDB source):** route `GetGuidanceStats` /
     `GuidanceHealth` through `constraint_outcomes` TSDB (the panels' source) so
     gauge = panels = RSIC. Requires adding a TSDB pool to `StatsCollector`
     (currently Neo4j-only) — more plumbing.
   - **Lean B** if the plumbing is contained; else **A + a disclosed follow-up**
     to unify on TSDB. Either way the inflation bug is killed and the divergence
     is documented.
**Out:** the guidance-relevance / trust-decay behavioral fix (→
JIMINY-EFFECTIVENESS-001); the J17 T1-promotion bootstrap; any change to how
guidance is surfaced or classified. No new alert rules (so no new evaluator
Service-label/SQL-contract surface).

**Constraints:** no-hardcoding (any new threshold a config knob); the alert
keeps a UNIQUE `Service`; Tier 3 live required; making RSIC consume the honest
gauge will LOWER the guidance dimension of the health score — that is correct
(it was masked) and must be disclosed.

## 4. Dependencies
`internal/ape/self_reflect.go:331` (insight), `self_assess.go:185` (synergy
block + `JiminyHealthy` set) + `:450` (RSIC consumes follow-rate),
`internal/ape/synergy_reader.go:68`, `internal/api/server.go:793` (jiminyCheck);
`internal/ape/task_dispatch.go:823` (alert text); `internal/jiminy/stats.go:23-95`
(`GetGuidanceStats` — the inflated gauge); the health-prober `/healthz` jiminy
signal; `constraint_outcomes` TSDB (Option B); `deploy/docker/grafana/dashboards/
mdemg-jiminy.json` (the honest panels — the agreement target).

## 5. Implementation Plan
Epic 0 plan · **Epic 1** false-CRITICAL fix — real `JiminyHealthy` probe +
`SynergyAssessed` guard + alert rename; unit tests (insight does NOT fire when
unassessed; fires only on a real down-probe with synergy lines present) ·
**Epic 2** de-inflate the gauge (Option A or B) — corrected query/source; a unit
test pinning the new math against a fixture matching the live distribution
(multi-outcome guidance_ids no longer double-credited); confirm RSIC reads the
honest value · **Epic 3 (live Tier 3)** — rebuild, restart, observe: the
`synergy_jiminy_unhealthy` CRITICAL stops firing on the healthy live Jiminy
(watch `~/.mdemg/alerts/current.json` across ≥1 RSIC cycle); `mdemg_jiminy_
follow_rate` now ≈ the panel value (query both, confirm they agree within a few
pp); RSIC `GuidanceHealth` reflects the honest rate · **Epic 4** docs (feature
doc note / CHANGELOG / post), push.

## 6. Testing Plan (3 tiers)
T1: insight-guard unit (unassessed flag → no insight; assessed + real-down +
synergy lines → insight); gauge-math unit (fixture with multi-outcome
guidance_ids → honest rate, not inflated). T2: `go test ./internal/ape/...
./internal/jiminy/...`; `golangci-lint`; config scanner if a knob added. T3
(live): on the running stack — CRITICAL no longer fires across an RSIC cycle;
`follow_rate` gauge ≈ dashboard panel (the two now agree); spot-check
`/healthz` jiminy + the alert file.

## 7. Commit Strategy
Per-epic · gofmt/vet + lint each · push once · summary · CI watch.

## 8. Verification Checklist
- [ ] `JiminyHealthy` is a real operability signal; insight guarded on `SynergyAssessed`
- [ ] `synergy_jiminy_unhealthy` CRITICAL no longer fires on a healthy live Jiminy (≥1 RSIC cycle observed)
- [ ] Alert text renamed off "guidance not reaching agent"
- [ ] Follow-rate gauge de-inflated; gauge ≈ dashboard panels (agree within a few pp)
- [ ] RSIC `GuidanceHealth` consumes the honest rate (disclosed: health score drops, correctly)
- [ ] Unit tests pin both fixes; go build + lint green
- [ ] Feature-doc note + CHANGELOG + post

## 9. Documentation Update — Epic 4 (never cut).

## 10. Risks & Mitigations
Making the gauge honest drops RSIC's guidance health → that is the POINT (it was
masking the real problem); disclose loudly, don't soften. A real probe for
`JiminyHealthy` could itself false-negative under load → reuse the existing
health-prober signal (already battle-tested in `/healthz`), don't invent a new
probe. Renaming the alert could orphan dashboards/runbooks referencing the old
text → grep for the old string and update references. Option B plumbing balloons
scope → fall back to Option A + disclosed TSDB-unification follow-up.

## 11. Documents Accessed
The two subagent root-cause reports (J17 T1 + dashboard metrics); the file:line
sites in §4; live TSDB `constraint_outcomes` + Neo4j `GUIDANCE_OUTCOME`; the
mdemg-jiminy Grafana dashboard JSON; `/healthz` + `~/.mdemg/alerts/current.json`.

## 12. Rollback Procedures
All changes are code reverts (alert condition/text, gauge query/source, the
`SynergyAssessed` field). No schema, no migration, no data change. Reverting
restores the prior (lying) signals.
