# JIMINY-ACTIONABILITY-001 — Sprint Post

The near-term surfacing lever from JIMINY-RELEVANCE-001's diagnostic: guidance is ignored because it is *not actionable* (abstraction class ≈90% of guidance, ignored 54–66%; actionable ≈10%, ignored 27–35%). Bias what `Guide()` surfaces, and how it is phrased, toward the actionable.

## Epics (plan: 0–6)

- **Epic 1 — baseline + gauge + A/B harness:** `baseline_composition.md` (live 30d numbers); surfaced-composition gauge `mdemg_jiminy_surfaced_actionable_fraction`/`_abstraction_fraction` (Grafana panel added in GRAFANA-AUDIT-002); reusable A/B harness `scripts/jiminy_actionability_ab.py`. ✅
- **Epic 2 — Lever A (surface reweighting):** per-type sort weight + min-actionable quota + abstraction cap, all default-preserving (`applyActionableComposition`). ✅
- **Epic 3 — Lever B (directive synthesis):** `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` renders abstractions as imperative directives, bounded prompt, reuses the existing synthesizer. ✅
- **Epic 4 — live A/B:** Lever B works (imperative narratives); Lever A modest (surfaced actionable 6.7%→10.5%) because retrieval surfaces no actionable candidates for most contexts (`epic4_live_ab.md`). **This triggered Lever C's contingency.** ✅
- **Epic 5 — Lever C (retrieval-side constraint inclusion):** built as a follow-on (RRF-SCALE-001-safe constraint-inclusion mechanism). See `leverc/` + `ab_results.md`. ✅
- **Epic 6 — docs:** feature doc `docs/features/jiminy-actionability.md` (shipped under this name — the plan's `guidance-actionability.md` is reconciled to the shipped name, which CLAUDE.md + CHANGELOG already reference; not renamed to avoid breaking those references), CHANGELOG, CLAUDE.md note, this post. ✅

## Outcome

- **Lever B is the per-phrasing win** (works regardless of candidate type).
- **Lever A was bottlenecked upstream**, which **Lever C** addresses by guaranteeing actionable candidates enter the retrieval pool (the diagnosed root cause).
- All levers ship **default-off**; operator enables per the A/B evidence.

## Documents Accessed
- `internal/jiminy/service.go`, `internal/jiminy/synthesizer.go`, `internal/jiminy/guidance_prompt.go`, `internal/api/rsic_adapters.go`, `internal/consulting/service.go`
- `docs/development/jiminy-actionability-001/{sprint_plan,baseline_composition,epic4_live_ab,ab_results}.md`
- Live mdemg-dev (`/v1/jiminy/guide`, `constraint_outcomes`, the surfaced gauge)
