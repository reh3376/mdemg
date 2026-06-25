# JIMINY-ACTIONABILITY-001 — Epic 4 Live A/B

Live Tier-3 against the real binary + mdemg-dev. Same 6 contexts, levers OFF (Arm A, current defaults) vs ON (Arm B). Measures the **surfaced** composition (the lever's direct effect); the outcome-side follow-rate is a multi-week signal not measurable in one session.

## Arm config

- **Arm A (off):** all defaults (`JIMINY_SURFACE_*` no-op, `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=false`).
- **Arm B (on):** `JIMINY_SURFACE_ACTIONABLE_WEIGHT=1.5`, `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION=0.4`, `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION=0.6`, `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`.

## Result — surfaced actionable fraction (Lever A)

| | total items | actionable | fraction | distribution |
|---|---|---|---|---|
| Arm A | 15 | 1 | **0.067** | constraint 1, learning 14 |
| Arm B | 19 | 2 | **0.105** | constraint 2, pattern 8, learning 8, concept 1 |

Per-query, Lever A pulled an actionable into the surfaced set **where one existed in the candidate pool** (TSDB-migration query 0/5→1/4; commit-without-lint query 0/1→1/3) and was a no-op where the pool had none.

## Result — directive synthesis (Lever B)

Confirmed working. With `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`, the synthesized narrative is imperative:

> "...it is **imperative** to adhere strictly to the established cleanup procedures... You **MUST** clean up the Neo4j spaces in a controlled manner... You **MUST NOT** delete nodes..."

(Arm A narratives are advisory/descriptive prose.)

## Verdict (against `baseline_composition.md`)

- Milestone 1 (surfaced abstraction fraction → ≤60%): **not met by Lever A alone** — 93.3% → 89.5%, a modest 3.8pp shift.
- Milestones 2–3 (outcome follow-rate): not measurable in-session (multi-week signal).

**Honest read:** Lever A is mechanically correct but low-impact, because the **binding constraint is upstream** — retrieval simply does not surface actionable (`constraint`/`correction`) candidates for most contexts (the substrate is ~9.6% actionable and those nodes rarely rank into the candidate pool). The min-actionable quota cannot manufacture actionables the candidate set lacks.

**Lever B is the real win this sprint:** directive synthesis renders the (still ~90% abstraction) surfaced set as imperative directives — it makes guidance actionable in *phrasing* regardless of *type*, directly attacking the "ignored because not-actionable" root cause without depending on the candidate pool.

## Follow-up

The highest-leverage next step is **biasing retrieval toward actionable nodes** (a role-scoped retrieval boost for `constraint`/`correction`), so the surfacing levers have actionables to promote. Tracked as **jiminy-actionability-002** (retrieval-side), distinct from this sprint's surfacing-side levers.

## Shipping decision

Code ships with levers **default-off** (config-driven; this A/B did not justify a default flip for Lever A). Operator recommendation: enable **Lever B** (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`) — it works and is the higher-impact lever; hold Lever A's quota/cap until retrieval-side actionable bias (jiminy-actionability-002) gives it candidates to work with.
