# JIMINY-ACTIONABILITY-001 — Lever C Live A/B (Epic 5d)

Controlled A/B on the live binary (same build, same substrate moment), `scripts/jiminy_actionability_ab.py`, 6 fixed contexts against mdemg-dev. Measures the **surfaced** composition (the lever's direct effect).

## Result

| Arm | Config | Surfaced **actionable** fraction | Abstraction fraction | items / actionable |
|---|---|---|---|---|
| A | Lever C **off** | **11.1%** | 88.9% | 27 / 3 |
| B | Lever C **on** (`SIM_FLOOR=0.30`, `INCLUDE_TOPK=5`) | **47.7%** | **52.3%** | 44 / 21 |

**4.3× lift; +36.6pp.** This **clears the ≤60%-abstraction milestone** from `baseline_composition.md` — the milestone Lever A alone could not move (Lever A: 6.7%→10.5% in Epic 4).

Per-call debug confirms the mechanism fires: `leverc_actionable_merged: 5` (the role-filtered cosine query finds the rare actionable nodes; the merge is real, not substrate drift).

## Relevance (the flooding-risk check)

The surfaced constraints are **query-relevant**, not noise. For "writing a new TSDB migration and bumping the schema version", Lever C surfaced:
- "[must] Fixes schema version tracking by updating the database metadata" — directly on-topic
- "Implemented timestamp_format enum validation (4 formats)" — schema/validation
- "When creating a JSON schema/contract/test spec … use UxTS" — schema-related
- (+ 2 marginally-related: DATAPRUNE audit, a testing-blindspot note)

At `SIM_FLOOR=0.30` a couple of marginal constraints surface; operators wanting tighter relevance can raise the floor (~0.35–0.40). The default 0.30 already yields mostly-relevant results.

## Verdict (against `baseline_composition.md`)

1. **Surfaced abstraction fraction → ≤60%: MET** (88.9%→52.3%). ✅ — the in-session measurable Lever C was built to move.
2. Follow-rate rise / ignore-rate fall in `constraint_outcomes`: a multi-week signal, not measurable in-session (the surfaced fraction is the in-session proxy).
3. No actionable-set follow-rate regression: N/A — Lever C *adds* actionables.

## Live-smoke fix (own commit)

The first Arm-B run showed `leverc_actionable_merged` absent (merge no-op) — `db.index.vector.queryNodes(top-50)` then role-filter returns ~nothing because actionables are ~0.1% of nodes and never rank into the global top-50. Replaced with `vector.similarity.cosine` over the role-filtered set directly (guarantees the top-K actionables). The apparent first-run "lift" was substrate drift; the controlled re-run above is the real result.

## Shipping

Lever C ships **default-off** (`JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=false`). **Operator recommendation: enable it** — it is the lever that actually moves the actionable composition (the whole line's goal). `SIM_FLOOR=0.30` default; raise for tighter relevance.
