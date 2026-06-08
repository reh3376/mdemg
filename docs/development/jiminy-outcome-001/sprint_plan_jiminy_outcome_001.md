# Sprint JIMINY-OUTCOME-001 — Revive the Neo4j GUIDANCE_OUTCOME Edge Sink

> **Status:** DRAFT — awaiting user approval before implementation.
> **Type:** P1 correctness fix (cognition pipeline — completes the guidance-loop revival begun in RRF-SCALE-001).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | JIMINY-OUTCOME-001 |
| **Sprint line** | `docs/development/jiminy-outcome-001/` |
| **Date opened** | 2026-06-04 |
| **Target version** | v0.11.2 (patch — bugfix, no new feature surface) |
| **Estimated effort** | 1–1.5 dev-days |
| **OpenAI / LLM spend** | $0 (embeddings via the configured local/stub embedder; no new LLM call sites) |
| **Risk level** | Low–Medium. Touches `matchConstraintCode` in the Jiminy guidance path. Bounded: the embedding path falls back to the existing keyword matcher, and the whole feature is config-gated. Worst case is a wrong constraint_code on an outcome row — bounded by the similarity threshold + the existing `findConstraintNodeID` resolution (a non-existent code resolves to no node → no edge, same as today). |
| **Priority** | P1 — this is **Follow-up A** from RRF-SCALE-001: the other half of the guidance-loop revival. RRF-SCALE-001 revived the TSDB outcome sink; this revives the Neo4j `GUIDANCE_OUTCOME` edge sink so per-constraint effectiveness *graph* stats (`GetConstraintEffectiveness`) update again. |

## 2. Problem Statement

The Neo4j `GUIDANCE_OUTCOME` edge sink has been dormant since **2026-04-12** (893 edges, no growth). RRF-SCALE-001 revived guidance surfacing + the **TSDB** `constraint_outcomes` sink, but the **Neo4j** edge sink stayed dead. Root cause, confirmed by live diagnosis (RRF-SCALE-001 verification + this sprint's investigation):

The guidance→feedback→outcome path attaches an outcome to a constraint node only when the guidance item carries a **`constraint_code`** that `PersistGuidanceOutcome` can resolve to a real `role_type='constraint'` node via `findConstraintNodeID`. That code is assigned in `Guide()` by **`matchConstraintCode`**, which links a guidance item to a constraint code by **keyword overlap (≥3 shared significant words)**.

Retrieval surfaces **emergent_concept** abstractions (L2–L5) of constraints, not the raw constraint nodes. Concept-abstracted content (e.g. *"Core understanding: commit, before…"*) does **not** share 3+ literal words with the raw constraint text (e.g. *"never commit directly to the main branch…"*), so `matchConstraintCode` returns empty → no `constraint_code` → `PersistGuidanceOutcome` falls back to the concept SourceNode → its `WHERE obs_type IN [...] OR role_type='constraint'` filter rejects `emergent_concept` → **no edge**.

Live evidence: across 17 fresh outcome rows (RRF-SCALE-001 verification), **`constraint_code` was `(none)` for every one** — confirming `matchConstraintCode` matched nothing. The keyword matcher's own code comment notes that keyword-list content embeds at a ~0.33 cosine ceiling vs ~0.70 for natural language; the content is *already* normalized to natural language (`normalizeGuidanceContent`) for exactly this reason, but `matchConstraintCode` still uses keyword overlap, not embeddings.

**The downstream machinery already works** — `PersistGuidanceOutcome` + `findConstraintNodeID` correctly create an edge on the real constraint node *when a code is present*. The single weak link is the matcher. Fix the matcher → the existing machinery revives the Neo4j sink with edges on the *correct* nodes (so `GetConstraintEffectiveness`, which reads `role_type='constraint'` edges, works too).

## 3. Scope & Constraints

### In scope
1. **Embedding-similarity constraint-code matching** (Option 1, selected from the grounded fork — see §11):
   - Extend `loadSpaceConstraintCodes` to also load each coded constraint's `embedding`.
   - Add `Embedding []float32` to `constraintCodeEntry`.
   - In `Guide()`, embed the (already natural-language-normalized) guidance item content and select the best constraint code by **cosine similarity** above a config threshold, instead of (with fallback to) keyword overlap.
   - **Fallback chain:** embedding similarity → keyword overlap (the existing matcher) when (a) the embedder is unavailable, (b) the guidance item has no embeddable content, or (c) a coded constraint has no embedding (13 of 111 in `mdemg-dev`). Never regress below today's keyword behavior.
2. **Config knob:** `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` (default derived empirically in Epic 1 from the live concept↔constraint similarity distribution; provisional 0.55–0.65) — no-hardcoding rule.
3. **3-tier testing** (unit + integration + live e2e) — acceptance bar is a **fresh Neo4j `GUIDANCE_OUTCOME` edge on a real `role_type='constraint'` node**, dated today.
4. **Documentation** — fix doc, CHANGELOG, CLAUDE.md note, post.md.

### Out of scope
- **Retrieval ranking** (making raw constraint nodes outrank their concept abstractions) — a much larger retrieval-tuning effort; Option 1 deliberately avoids it by matching concepts *back* to constraint codes via embeddings.
- **Follow-up B (LLM synthesis timeout)** and **Follow-up C (`/v1/jiminy/latest` JSON control-char escaping)** — tracked separately; may be folded into a later maintenance pass. (If C proves to break the real hook's `guidance_id` capture, it gets escalated on its own — but it is not this sprint's concern.)
- **Batch re-embedding of the 13 coded constraints lacking embeddings** — they fall back to keyword matching; an `embeddings backfill` run is the operator remedy, out of scope here.
- **Changing `PersistGuidanceOutcome`'s node-type allow-list** (Option 4) — rejected: would attach outcomes to concepts that `GetConstraintEffectiveness` can't read.

### Constraints
- Sequential epics (`feedback_sequential_epics.md`).
- No-hardcoded-values rule (`feedback_no_hardcoded_values.md`) — the similarity threshold is config-driven with an empirically-derived default.
- Tier 3 live testing required (`feedback_live_testing_required.md`) — acceptance is the **observed fresh Neo4j edge on a real constraint node**, not a unit test.
- Rigorous verification (`feedback_rigorous_verification.md`) — confirm the edge exists AND lands on a `role_type='constraint'` node AND `GetConstraintEffectiveness` reflects it.
- Never regress the keyword matcher — embedding is additive with fallback.

## 4. Dependencies

- **RRF-SCALE-001** (merged) — revived guidance surfacing + the TSDB sink; this sprint completes the loop on the Neo4j side.
- **`jiminy.Service.embedder`** (`embeddings.Embedder`) — present; the `OutcomeClassifier` already uses it for embed→cosine matching (strong in-package precedent, `outcome_classifier.go:187` `cosineSimilarity`).
- **`cosineSimilarity`** helper — already in the jiminy package.
- **Constraint node embeddings** — 98 of 111 coded constraints in `mdemg-dev` carry embeddings (88%); the rest fall back to keyword.
- **`PersistGuidanceOutcome` + `findConstraintNodeID`** — unchanged; the existing, working downstream that this sprint feeds correctly-coded items into.
- **Live stack** (Neo4j + TSDB + embedder) for Epic 2 live-verify.

## 5. Implementation Plan

### Epic 0 — Sprint plan (~0.1 day)
Commit this plan. No code.

### Epic 1 — Embedding-similarity matcher (~0.5 day)
- **Capture the live similarity distribution first** (grounding the threshold default): for a handful of real "commit to main" / "no hardcoded values" / "CUIDv2" guidance items, compute cosine similarity between the item embedding and the matching constraint's embedding vs non-matching constraints. Pick a default threshold that admits true matches and rejects the rest (provisional 0.55–0.65). Record in the post/verification.
- `loadSpaceConstraintCodes`: add `c.embedding` to the Cypher `RETURN`; populate `constraintCodeEntry.Embedding` (parse the Neo4j vector → `[]float32`).
- Matching in `Guide()` (around service.go:879): replace the per-item `matchConstraintCode(item, constraints)` call with an embedding-aware matcher:
  - Embed the item content (reuse `s.embedder`; batch the item-content embeds in one call where the embedder supports it, else per-item — bounded by `JiminyMaxItems`, ~10).
  - `bestCode = argmax cosineSimilarity(itemEmb, entry.Embedding)` over entries with an embedding; accept iff `best ≥ cfg.JiminyConstraintCodeSimThreshold`.
  - **Fallback:** if no embedding match (or embedder/embedding unavailable), call the existing `matchConstraintCode` keyword path. Net behavior ≥ today.
- New config: `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` (float, default from the distribution; zero-value fallback to a safe constant).
- Tier 1 unit tests: embedding match selects the right code above threshold; below-threshold → keyword fallback → empty; embedder-nil → keyword path; constraint-without-embedding skipped in the embedding pass; threshold config override honored. Use synthetic embeddings (deterministic vectors) so tests don't need a live embedder.
**Gate:** unit tests green; lint clean; `go build ./...` clean.

### Epic 2 — Tier 2 integration + Tier 3 live e2e (~0.4 day)
- **Tier 2 integration** (`-tags=integration`, skip-on-empty per the RRF-SCALE-001 CI lesson): against real Neo4j with seeded constraint+embedding nodes, the matcher assigns the correct `constraint_code` to a concept-abstracted guidance item.
- **Tier 3 live e2e (the acceptance bar):**
  1. Baseline the Neo4j `GUIDANCE_OUTCOME` edge count + latest timestamp (893 / Apr 12).
  2. Rebuild + restart; warm the model; run the full warm→latest→feedback loop on a constraint-matching context.
  3. Confirm a **fresh `GUIDANCE_OUTCOME` edge** appears, **incident on a `role_type='constraint'` node** (not an `emergent_concept`), dated today.
  4. Confirm the matched `constraint_code` on the outcome (TSDB row + Neo4j edge) corresponds to the semantically-correct constraint.
  5. Confirm `GET /v1/constraints/effectiveness` (or `GetConstraintEffectiveness`) now reflects the new outcome.
- Transcript → `docs/development/jiminy-outcome-001/verification.md`.
**Gate:** fresh Neo4j edge on a real constraint node, dated today; effectiveness reflects it.

### Epic 3 — Documentation (~0.2 day, never cut)
- `docs/features/` — update the guidance/constraint-effectiveness feature doc (or add a note) on embedding-based constraint-code matching.
- `CHANGELOG.md` Unreleased entry.
- `CLAUDE.md` — note under the Jiminy/score-scale section: constraint-code matching is embedding-based (keyword fallback); the guidance loop's Neo4j sink depends on it.
- `docs/development/jiminy-outcome-001/post.md` — epic-by-epic, acceptance check-off, the loop-revival completion (TSDB from RRF-SCALE-001 + Neo4j here), forward-looking (B, C still open).

## 6. Testing Plan (3 tiers — required)

**Tier 1 — Unit (target 8–12):** embedding matcher selection above/below threshold; keyword fallback paths (nil embedder, no item content, constraint without embedding); threshold config override; never-regress (an input the keyword matcher matched today still matches). Deterministic synthetic embeddings.

**Tier 2 — Integration (`-tags=integration`, skip-on-empty):** seeded constraint+embedding nodes in real Neo4j → matcher assigns the correct code to a concept-abstracted item.

**Tier 3 — Live e2e (Epic 2):** real binary + live stack → fresh Neo4j `GUIDANCE_OUTCOME` edge on a `role_type='constraint'` node, dated today; effectiveness reflects it. Transcript in `verification.md`.

## 7. Commit Strategy
Sequential commits per epic on `reh3376_dev01`; auto-PR. Epic 1 = matcher + config + Tier 1. Epic 2 = integration test + verification.md. Epic 3 = docs. Surprise bugs in live smoke get their own fix-commit (precedent). Sprint summary on PR after Epic 3.

## 8. Verification Checklist
- [ ] `loadSpaceConstraintCodes` loads constraint embeddings; `constraintCodeEntry.Embedding` populated.
- [ ] Matcher selects the correct code by cosine ≥ threshold; falls back to keyword when embeddings unavailable; never regresses keyword behavior.
- [ ] Threshold is config-driven (`JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD`) with an empirically-derived, documented default; zero-value fallback.
- [ ] Tier 1 unit tests green; `golangci-lint run ./...` clean; `go build ./...` clean.
- [ ] Tier 2 integration: matcher assigns the correct code for a concept-abstracted item (skip-on-empty in CI).
- [ ] Tier 3 live: a **fresh** `GUIDANCE_OUTCOME` edge appears on a `role_type='constraint'` node, dated today (sink dead since Apr 12).
- [ ] The matched `constraint_code` is semantically correct for the context.
- [ ] `GetConstraintEffectiveness` / `/v1/constraints/effectiveness` reflects the new outcome.
- [ ] No regression in existing jiminy/consulting test suites.
- [ ] CHANGELOG, CLAUDE.md, post.md, verification.md, feature doc updated.
- [ ] Sprint summary on PR.

## 9. Documentation Update — Epic 3 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Embedding matcher assigns a *wrong* constraint code (semantic near-miss) | Medium | Medium | Threshold tuned from the live distribution (Epic 1); `findConstraintNodeID` resolves only real codes; keyword fallback unchanged. A wrong-but-plausible code is bounded — it attaches the outcome to a related constraint, not a random node. Tier 1 + live spot-check the matched code's semantics. |
| Per-item embedding adds latency to `Guide()` | Low | Low | ≤ `JiminyMaxItems` (~10) embeds, batched where supported; `Guide()` already embeds for retrieval + the OutcomeClassifier already embeds. Negligible vs the existing LLM synthesis cost. |
| 13/111 constraints lack embeddings → those never match via embedding | Medium | Low | Keyword fallback covers them; document `mdemg embeddings backfill` as the operator remedy. |
| Threshold too low → spurious codes flood outcomes | Low | Medium | Conservative default from the distribution; config-driven; Tier 1 below-threshold test; live spot-check. |
| Concept→constraint is many-to-one or ambiguous | Low | Low | argmax picks the single best; if best < threshold, no code (same as today). One outcome per item, as today. |
| Embedder unavailable in some deploy (stub/CI) | Low | Low | Fallback to keyword; the feature degrades to today's behavior, never worse. |

## 11. Documents Accessed
- `internal/jiminy/service.go` — `Guide` (constraint-code assignment loop ~879), `matchConstraintCode` (2432), `loadSpaceConstraintCodes` (2385), `constraintCodeEntry` (2378), `significantWordSet`, embedder field (34)
- `internal/jiminy/persistence.go` — `PersistGuidanceOutcome`, `findConstraintNodeID` (the working downstream this feeds)
- `internal/jiminy/outcome_classifier.go` — `cosineSimilarity` (187), the embed→cosine→optional-LLM precedent
- `internal/jiminy/retrieval_source.go` — `normalizeGuidanceContent` natural-language normalization (the ~0.70-cosine prep)
- `internal/consulting/service.go` — `findApplicableConstraints` (where concept items become "constraints")
- `internal/config/config.go` — `JiminyMinConfidence` / `J17Enabled` (config style); new `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD`
- Live diagnostics: 111 coded constraints (98 with embeddings); 17 fresh outcome rows all `constraint_code=(none)`; Neo4j `GUIDANCE_OUTCOME` 893 / last Apr 12; constraint→concept abstraction edges (`ABSTRACTS_TO`/`GENERALIZES`)
- RRF-SCALE-001 `verification.md` + `post.md` (Follow-up A definition)

## 12. Rollback Procedures
- **Config:** set `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` very high (e.g. 1.1) to disable embedding matches → pure keyword behavior (today's state). Instant, no redeploy logic change.
- **Code revert:** the Epic 1 commit is self-contained (matcher + config + loader); revert restores the keyword-only matcher. No schema changes, no migrations, no data mutation.
- **Data:** outcomes are forward-only + additive; rolling back simply stops new Neo4j edges (returns to the dormant-but-stable prior state). Existing edges/rows untouched.

---

## Files to be created/modified (anticipated)

**New:**
- `docs/development/jiminy-outcome-001/sprint_plan_jiminy_outcome_001.md` (Epic 0)
- `docs/development/jiminy-outcome-001/verification.md` (Epic 2)
- `docs/development/jiminy-outcome-001/post.md` (Epic 3)
- `tests/integration/jiminy_outcome_test.go` (Epic 2, skip-on-empty)

**Modified:**
- `internal/jiminy/service.go` — `loadSpaceConstraintCodes` (load embedding), `constraintCodeEntry` (+Embedding), the `Guide()` matching loop (embedding-aware + keyword fallback), possibly a new `matchConstraintCodeByEmbedding` helper
- `internal/jiminy/service_test.go` (or a new `constraint_code_match_test.go`) — Tier 1
- `internal/config/config.go` — `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD`
- `CHANGELOG.md`, `CLAUDE.md` — Epic 3
- (possibly) `docs/features/<guidance-or-constraint-effectiveness>.md`

## Acceptance Criteria
1. The full guidance→feedback→outcome loop produces a **fresh Neo4j `GUIDANCE_OUTCOME` edge on a real `role_type='constraint'` node**, dated today — the sink dormant since Apr 12 is observably revived.
2. The matched `constraint_code` is semantically correct for the guidance context (live spot-check).
3. `GetConstraintEffectiveness` / `/v1/constraints/effectiveness` reflects the new outcome.
4. The similarity threshold is config-driven with an empirically-derived default; no hardcoded magic number.
5. Embedding matching never regresses the keyword matcher (fallback verified); the feature degrades gracefully without an embedder.
6. Combined with RRF-SCALE-001 (TSDB sink), the constraint-effectiveness loop is fully revived across **both** sinks.
