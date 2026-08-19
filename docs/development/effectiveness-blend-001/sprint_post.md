# EFFECTIVENESS-BLEND-001 — Sprint Post

**Arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase B2
**Shipped**: 2026-08-18
**Ship state**: code + tests + docs shipped default-OFF; passive re-measurement + operator flip decision deferred

## What shipped

1. **3-way blend in Lever C** — `Service.activationEnrichLeverC` extended from `(1-wa)*cosine + wa*activation` to `(1-wa-we)*cosine + wa*activation + we*effectiveness`. When both weights are 0 → byte-identical to Phase B1.
2. **1 new config knob** — `JIMINY_LEVER_C_EFFECTIVENESS_WEIGHT` (default 0 in code AND `.env`; enable via `.env` after operator-directed live smoke).
3. **Weight-clamp** — if operator over-specifies `wa+we > 1`, `we` is clamped to `1-wa` so activation wins. Pin-tested.
4. **Fail-open contract preserved** — B1's contract of "no signal usable → return raw input" is preserved. Fail-open triggers when BOTH activation and effectiveness signals are unusable (nil retriever/error AND nil persistence/empty cache).
5. **Boot log extended** — `jiminy: lever c activation ... weight=... eff_weight=...`.
6. **4 new pin tests** (10 total for `activationEnrichLeverC` now):
   - `TestActivationEnrichLeverC_EffectivenessOnly_ReordersByRate` — activation OFF, effectiveness ON; reranks correctly.
   - `TestActivationEnrichLeverC_ThreeWayBlend` — both signals ON at `wa=0.3, we=0.3`; blend arithmetic verified.
   - `TestActivationEnrichLeverC_WeightClamping` — `wa=0.6, we=0.6` → `we` clamped to 0.4; blend arithmetic verified.
   - `TestActivationEnrichLeverC_EffectivenessNilPersistenceIsSafe` — nil persistence → identity (B1 contract).
7. **Test helper** `seedEffectivenessCache(s, spaceID, rates)` — pre-populates the shipped `effPriorCache` so tests can exercise the effectiveness path without a live Neo4j driver. Requires `JiminySurfaceEffectivenessPriorWeight > 0` + non-nil persistence (the outer gate at `effectivenessPriorRates`) — both set on the test Service (non-nil `&PersistenceStore{}` is never invoked because the cache hit returns early).

## Recon findings (verified live on mdemg-dev before writing code)

Applied `must-validate-all-claims-before-commit`.

| Claim | Verification | Verdict |
|-------|--------------|---------|
| Phase B2 original scope (effectiveness prior) is unshipped | live grep + code inspection | ❌ **already shipped** as JIMINY-CORPUS-001 Lever B (final-sort multiplier at `service.go:3720-3721`) |
| `JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT=0.3` is active in `.env` | grep `.env` + code default | ✅ active (not overridden; running default 0.3) |
| GUIDANCE_OUTCOME sink has real signal | live cypher: 26 constraints ≥5 samples on mdemg-dev, rates in `[0.0, 0.385]` | ✅ real signal |
| The shipped effectiveness prior applies at Lever-C SELECTION | code trace: applied only at final sort, after Lever C selection is frozen | ❌ gap — the reason B2 wasn't fully "already shipped" |

**Re-scoped B2**: the effectiveness signal is available AT THE FINAL SORT (shipped) but NOT AT LEVER-C SELECTION (this sprint). This sprint extends the effectiveness prior into Lever C's blend at the same site as B1's activation. The two application sites (Lever-C blend + final-sort multiplier) compose multiplicatively via the sort key.

## Live Tier-3 evidence

**Query**: "mermaid diagram markdown table" (targets `seg74inomjt5kx` = `markdown-mermaid-tables-and-charts`, live effectiveness 0.0 / 20 followed).

**BASELINE (both flags OFF)** — Lever C items:
```
[0] correction   bxz6hssryy6jg6 conf=0.6506  Markdown memory files ...
[1] constraint   wwecxwkuqmlkdr conf=0.6296  NEVER use mdemg db start
[2] constraint   seg74inomjt5kx conf=0.4092  Always create tables with mermaid
```

**CANDIDATE (`wa=0.3, we=0.3`)** — Lever C items:
```
[0] correction   bxz6hssryy6jg6 conf=0.3090  Markdown memory files ...     ← 0.65 → 0.31
[1] constraint   wwecxwkuqmlkdr conf=0.4144  NEVER use mdemg db start      ← 0.63 → 0.41
[2] constraint   seg74inomjt5kx conf=0.5182  Always create tables ...      ← 0.41 → 0.52
```

**Mechanism verified**: `debug.enriched=3` confirms the function fired; all 3 items' Confidence values shifted; the shift magnitudes are consistent with `(1-wa-we)*cosine + wa*activation + we*effectiveness` arithmetic modulo the mock effectiveness/activation values the substrate returned. Note: the DISPLAY ORDER shown above is set by the downstream final sort (Lever B multiplier × type weight × guidanceSortKey) — the blend feeds Confidence, and the final sort then reranks; both stages compose.

## Decisions

| Decision | Rationale |
|----------|-----------|
| Shape A (3-way blend in `activationEnrichLeverC`) over Shape B (full refactor) or Shape C (declare complete) | Operator-selected. Small footprint (~200 LOC + tests), preserves B1 contract, testable in isolation. |
| Default OFF in code AND `.env` | Behavior-changing feature per HEBB-ETA-001 rule. |
| Reuse `Service.effectivenessPriorRates` (shipped Lever B fetch) | Single-source: same cache, same TTL, same min-samples gate. Two application sites, one signal. |
| Weight-clamp `we = min(we, 1 - wa)` (activation wins) | Deterministic, avoids negative cosine coefficient. Alternative "reject config" would break at startup. |
| Fail-open only when NO signal usable | B1 contract preserved (nil-retriever/activation-error identity); B2 nil-persistence also identity. If one signal works and the other doesn't, blend continues with the working one (its coefficient survives, the failed one's = 0). |
| Test helper `seedEffectivenessCache` over persistence mock | `PersistenceStore` is concrete (not interface); mocking would require refactor scope. Cache-hit path returns before persistence.GetConstraintEffectiveness invocation. |

## Follow-ups (disclosed, deferred)

1. **[Passive] EFFECTIVENESS-BLEND-AB-001**: enable `JIMINY_LEVER_C_EFFECTIVENESS_WEIGHT=0.3` for a 168h window against real production traffic. Measure follow-rate delta vs baseline. Data-decide flip vs no-flip. NOT urgent — JIMINY-CEILING-BREAK-2 T+168h re-check on 2026-08-19 owns the primary substrate-quality signal.
2. **[Small]** URL override `?leverc_effectiveness=<w>` for per-request A/B measurement without env flip.
3. **[Phase B3]** Precision-confidence weighting: extend the blend to also fold `activation_confidence` (multiplicatively OR as a 4th term). Additive on top of B1+B2.
4. **[Phase C]** Layer/edge-aware surfacing — separate arc; independent of B2.

## Arch rules pinned

- **When extending a shipped signal to a new application site**, single-source the fetch (reuse the existing cache/computer); do NOT duplicate the persistence-layer fetch or the min-samples gating. This sprint reuses `Service.effectivenessPriorRates` from JIMINY-CORPUS-001 unchanged — the two application sites (Lever-C blend + final-sort multiplier) share one cache read per request.
- **Fail-open contract for multi-signal blends**: identity only when NO signal is usable. If ONE signal fails and another works, blend continues with the working one. Sprint tests distinguish "signal-requested-but-unusable → coefficient 0" from "signal-not-requested → coefficient 0" — both paths converge to the same numerical result but the identity-return short-circuit only fires when ALL signals are unusable.
- **When adding a config field that gates a signal path in a multi-signal blend**, extend the blend function's docstring to describe (a) the new signal's contribution to the formula, (b) the composition with existing signals (multiplicative? additive? clamped?), and (c) the shipped downstream site that consumes the same signal (Lever B multiplier here — composition via sort key).

## Documents Accessed

- `internal/jiminy/service.go` (`activationEnrichLeverC` extended; `effectivenessPriorRates` 3807; `effPriorCache` fields 81-82; `effPriorCacheEntry` 3790)
- `internal/jiminy/persistence.go` (`GetConstraintEffectiveness` 185; `PersistenceStore` struct 15)
- `internal/jiminy/lever_c_activation_test.go` (existing B1 tests + 4 new B2 tests)
- `internal/config/config.go` (`JiminyLeverC*` block; `JiminySurfaceEffectivenessPrior*` fields 361-363)
- `internal/api/server.go` (jiminy boot log 1214-1220)
- Live cypher-shell queries on mdemg-dev (per-constraint effectiveness distribution)
- Live `/v1/jiminy/guide` smoke (baseline vs candidate on mermaid query)
- `docs/development/effectiveness-blend-001/sprint_plan.md`
- `CLAUDE.md` (JIMINY-CORPUS-001 pin, ACTIVATION-DRIVEN-DISCOVERY-001, GUIDANCE-OUTCOME-SINK-INVESTIGATE-001, HEBB-ETA-001)
