# EFFECTIVENESS-BLEND-001 — Sprint Plan

**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase B2

## 1. Header & Metadata

- **Sprint ID**: `EFFECTIVENESS-BLEND-001`
- **Arc**: JIMINY-SUBSTRATE-NATIVE-001 (Phase B2)
- **Author**: reh3376 / claude
- **Date**: 2026-08-18
- **Branch**: `reh3376_dev01`
- **Estimated wall-clock**: ~2 hours
- **Sprint format**: v1.0 (12-section)

## 2. Problem Statement

Phase B2's original scope ("Hebbian effectiveness prior via GUIDANCE_OUTCOME reinforcement") is **substantially already shipped** as JIMINY-CORPUS-001 Lever B: `Service.effectivenessPriorMultiplier` reads per-node followed/surfaced rates from GUIDANCE_OUTCOME edges via `GetConstraintEffectiveness`, applied as a multiplier in the final sort at `service.go:3720`. Currently active on mdemg-dev at `JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT=0.3` (default). Live: 26 constraints with ≥5 samples, effectiveness rates in `[0.0, 0.385]`, real signal.

The remaining architectural gap: **the shipped effectiveness prior applies only in the FINAL sort — never inside Lever C's own reranking**. Phase B1 (ACTIVATION-DRIVEN-DISCOVERY-001) added activation-driven reranking at the Lever-C level via `activationEnrichLeverC` with blend `(1-w_a)*cosine + w_a*activation`. Effectiveness is a substrate signal orthogonal to activation (context-agnostic historical outcome vs context-specific graph centrality); folding it into the same blend gives Lever C selection access to BOTH substrate signals at once, not just at the final sort where selection is already frozen.

## 3. Scope & Constraints

### In scope
1. **Extend `activationEnrichLeverC`** (`internal/jiminy/service.go`) to fetch effectiveness rates via the shipped `effectivenessPriorRates` and include them in the blend.
2. **3-way blend**: `blended = (1 - w_a - w_e) * cosine + w_a * activation + w_e * effectiveness`. When both weights are 0 → identity (byte-identical to pre-sprint). When `w_a > 0` OR `w_e > 0` → reweight fires. Guard: if `w_a + w_e > 1`, clamp `w_e = 1 - w_a` (activation wins if operator over-specifies).
3. **1 new config knob**: `JIMINY_LEVER_C_EFFECTIVENESS_WEIGHT` (default 0 in code AND `.env` — behavior-changing default off per HEBB-ETA-001 rule).
4. **Fail-open**: nil persistence, empty effectiveness map, node not in map → the effectiveness term becomes 0 for that node (uniform `(1-w_e)*cosine` fall-back — no differential penalty on data-sparse nodes, mirroring the shipped Lever B contract).
5. **Extend `activationEnrichLeverC` docstring** to describe the 3-way blend + composition with the final-sort effectiveness prior (they compose multiplicatively via the sort key).
6. **Update boot log** — extend the existing `jiminy: lever c activation` line to include `eff_weight`.
7. **Pin tests**: default-off byte-identical; effectiveness-only mode (activation off, effectiveness on) reranks correctly; combined mode; weight-clamping when sum > 1.

### Out of scope
- **Refactoring the shipped Lever B (final-sort multiplier)** — Shape B in the recon discussion; left alone. This sprint's blend and Lever B compose multiplicatively; both can be tuned independently.
- **New CO_ACTIVATED_WITH signal path** — B1's activation already consumes it; not double-consumed here.
- **Phase B3 precision-confidence weighting** — additive follow-up; deferred.

### Hard invariants
- **`JiminyLeverCActivationEnabled=false && effectivenessWeight=0` → byte-identical to Phase B1** (pin-tested).
- **Actionable coverage preserved** — reweighting reorders; never filters.
- **RRF-SCALE-001-safe**: effectiveness rate is followed/surfaced over GUIDANCE_OUTCOME (stable [0,1]), NEVER the RRF Score.
- **Fail-open**: nil persistence / empty rate map → effectiveness contribution is 0, blend falls back to `(1-w_e)*cosine`.
- **Effectiveness prior in the final sort is UNCHANGED** — this sprint adds a NEW application site (inside Lever C), does NOT modify the shipped one.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ `activationEnrichLeverC` (ACTIVATION-DRIVEN-DISCOVERY-001, 2026-08-18)
- ✅ `effectivenessPriorRates` + `GetConstraintEffectiveness` (JIMINY-CORPUS-001, 2026-07-03)
- ✅ GUIDANCE_OUTCOME sink populated (JIMINY-OUTCOME-001, 2026-06-08 — verified 8,517 edges on mdemg-dev)

**Downstream**:
- Phase B3 (precision-confidence via `activation_confidence`) — additive.

## 5. Implementation Plan

### Epic 1: config knob (~10min)
- Add `JiminyLeverCEffectivenessWeight float64` field to `Config` in `internal/config/config.go` alongside the existing `JiminyLeverCActivation*` fields.
- Init from `atof("JIMINY_LEVER_C_EFFECTIVENESS_WEIGHT", 0.0)`; clamp `[0, 1]`.
- Wire into Config literal in `FromEnv`.

### Epic 2: 3-way blend in Lever C (~45min)
- Modify `Service.activationEnrichLeverC` in `internal/jiminy/service.go`:
  - Rename var `w` → `wa` for clarity.
  - Read `we := s.cfg.JiminyLeverCEffectivenessWeight` clamped to `[0, 1]`.
  - Early return unchanged if `!s.cfg.JiminyLeverCActivationEnabled` AND `we <= 0` (still fully identity when both signals off).
  - When ONLY effectiveness enabled (activation flag off, we > 0): skip the `retriever.ExpandSeedsByActivation` call entirely (activation map is empty; blend becomes `(1-we)*cosine + we*effectiveness`).
  - Clamp `we = min(we, 1 - wa)` — if operator over-specifies, activation wins.
  - Fetch effectiveness rates via `s.effectivenessPriorRates(ctx, spaceID)` (nil-safe: nil map → all effectiveness=0, same fail-open shape).
  - Blend: `enriched[i].Confidence = (1 - wa - we) * orig + wa * act + we * effRate`.
  - Stable sort by Confidence DESC.

### Epic 3: boot log + docstring (~10min)
- Extend the boot log line at `internal/api/server.go` `jiminy: lever c activation` to include `eff_weight`.
- Update `activationEnrichLeverC` docstring to describe the 3-way blend, composition with the final-sort effectiveness prior, and the clamp semantics.

### Epic 4: tests (~30min)
- New tests in `internal/jiminy/lever_c_activation_test.go`:
  - `TestActivationEnrichLeverC_EffectivenessOnly_ReordersByRate` — activation flag OFF, `we=0.5`, mock returns rate map that flips ordering.
  - `TestActivationEnrichLeverC_ThreeWayBlend` — both enabled at `wa=0.3, we=0.3`; verify blend arithmetic.
  - `TestActivationEnrichLeverC_WeightClamping` — `wa=0.6, we=0.6` → `we` clamped to 0.4; sum = 1.0; cosine coefficient 0.0.
  - `TestActivationEnrichLeverC_EffectivenessNilMapIsSafe` — persistence nil → all effectiveness=0, no panic.
- Update existing `TestActivationEnrichLeverC_DisabledIsIdentity` to also assert byte-identity when `we=0`.
- Mock effectiveness path: reuse `mockPersistenceStore` if it exists; else pass rates directly by wiring a test-only setter on Service OR use table-driven test with real Service + injected map via unexported accessor.

### Epic 5: live Tier-3 (~15min)
- Build binary, kickstart, boot log confirms `eff_weight=0` (default off).
- Flip `.env` `JIMINY_LEVER_C_EFFECTIVENESS_WEIGHT=0.3` + `JIMINY_LEVER_C_ACTIVATION_ENABLED=true`, kickstart, boot log confirms both.
- One `/v1/jiminy/guide` call with a query that surfaces a known-low-effectiveness constraint (`markdown-mermaid-tables-and-charts` at 0.0 followed rate) — verify it's demoted vs baseline.
- Restore `.env` to defaults post-smoke.

### Epic 6: docs (~10min)
- Update `docs/features/jiminy-lever-c-activation.md` — add "Effectiveness signal" section describing the 3-way blend + the new knob.
- New sprint post at `docs/development/effectiveness-blend-001/sprint_post.md`.
- CLAUDE.md architecture note.
- CHANGELOG entry.

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit
- 4 new tests as listed in Epic 4.
- `TestActivationEnrichLeverC_DisabledIsIdentity` extended: assert `wa=0, we=0` returns byte-identical (input Confidence + ordering preserved).

### Tier 2 — Integration
- `go build ./...` clean.
- `go test ./internal/retrieval/... ./internal/jiminy/... ./internal/config/... ./internal/api/...` all green.

### Tier 3 — Live end-to-end (mdemg-dev)
- Boot log baseline: `eff_weight=0`.
- Boot log with flag: `eff_weight=0.3`.
- `/v1/jiminy/guide` baseline vs candidate on real query — assert `markdown-mermaid-tables-and-charts` (0/20 followed) is demoted when candidate flag is on and it happens to be in the Lever C pool.
- `.env` restored to defaults; boot log confirms `eff_weight=0`.

## 7. Commit Strategy

- 1 primary commit for the sprint code (Epics 1-4).
- Docs (Epic 6) in same commit for cohesion — this is a small sprint.
- Fix-commit if live smoke uncovers a surprise defect (Phase 11.6.2 precedent).

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/jiminy/ ./internal/config/ ./internal/api/` clean
- [ ] `go test ./internal/jiminy/... ./internal/config/... ./internal/api/...` green
- [ ] Boot log confirms `eff_weight=0` default
- [ ] Boot log confirms `eff_weight=0.3` when `.env` flipped
- [ ] Live smoke: candidate reranks known-low-effectiveness constraint down
- [ ] `.env` restored to defaults post-smoke
- [ ] Sprint plan lives at `docs/development/effectiveness-blend-001/`
- [ ] Feature doc updated
- [ ] CLAUDE.md note added
- [ ] CHANGELOG entry
- [ ] PR sprint-summary comment posted

## 9. Documentation Update

### Files created
- `docs/development/effectiveness-blend-001/sprint_plan.md` (this)
- `docs/development/effectiveness-blend-001/sprint_post.md`

### Files modified
- `internal/config/config.go` — 1 new field + init
- `internal/jiminy/service.go` — `activationEnrichLeverC` extended
- `internal/jiminy/lever_c_activation_test.go` — 4 new tests + 1 extended
- `internal/api/server.go` — boot log line
- `docs/features/jiminy-lever-c-activation.md` — new "Effectiveness signal" section
- `CLAUDE.md` — arch note
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Chronically-ignored actionables (e.g. `markdown-mermaid-tables-and-charts`) get so down-weighted they never surface | Medium | Medium | Default OFF (weight=0); operator flips explicitly; the final-sort Lever B already applies a similar prior — this sprint is additive, not replacing |
| Weight over-specification (`wa+we>1`) produces negative cosine coefficient | Confirmed | Low | Clamp `we = min(we, 1-wa)`; pin-tested |
| Effectiveness rate stale (5min TTL) makes reweighting lag | Low | Low | Same TTL as shipped Lever B; consistent across both application sites |

## 11. Rollback Procedures

- Zero substrate mutation; pure read.
- `JIMINY_LEVER_C_EFFECTIVENESS_WEIGHT=0` → byte-identical to Phase B1.
- Code rollback: revert commit; no schema changes.

## 12. Documents Accessed

- `internal/jiminy/service.go` (`activationEnrichLeverC` 3372+; `effectivenessPriorRates` 3743; `effectivenessPriorMultiplier` 3798; final sort 3720-3721)
- `internal/jiminy/persistence.go` (`GetConstraintEffectiveness` 185; `PersistGuidanceOutcome` 44)
- `internal/jiminy/lever_c_activation_test.go` (existing 6 pin tests)
- `internal/config/config.go` (`JiminyLeverC*` block 361-368; `JiminySurfaceEffectivenessPrior*` fields 361-363)
- `internal/api/server.go` (boot log 1194-1215)
- Live cypher-shell queries on mdemg-dev (effectiveness distribution)
- `CLAUDE.md` (JIMINY-CORPUS-001 pin, JIMINY-OUTCOME-001, ACTIVATION-DRIVEN-DISCOVERY-001, GUIDANCE-OUTCOME-SINK-INVESTIGATE-001)
- `docs/features/jiminy-lever-c-activation.md`

---
