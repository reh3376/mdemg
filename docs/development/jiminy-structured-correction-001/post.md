# JIMINY-STRUCTURED-CORRECTION-001 — Sprint Post (2026-07-20)

## Summary
Propagates the `Incorrect`/`Correct`/`Context` triple from `POST /v1/conversation/correct` into the graph substrate as first-class fields — previously discarded during the free-text join. Downstream synthesis (Lever B) now renders "Do <correct> — not <incorrect>" without regex-parsing the joined content string, and the bridge-authored corrections from JIMINY-CONTRADICTED-BRIDGE-001 flow through cleanly with zero bridge-specific code changes.

## What shipped
- **E0** — sprint plan (v1.0 12-section format).
- **E1** — `conversation.Service.Correct` writes `structured_data.correction = {incorrect, correct, context}` on the L0 obs. Extracted `buildCorrectionObserveRequest` helper for testability. Retired the dead `metadata_*` param flatten in `createObservationNode` with a comment explaining prior intent (the CREATE cypher never referenced those params — meta_keys=[] on a fresh L0 obs). 6 unit tests: structured populated, content backward-compatible, context omission, metadata preserved, obs_type/tags/AgentID pins.
- **E2** — `CreateCorrectionNodes` parses `structured_data.correction` on the L0 and sets `correction_incorrect`/`correction_correct`/`correction_context` as top-level properties on the new L1 correction node. Absent-safe for old-format L0. Full go test ./... green; no regression on the 23 existing correction gate tests.
- **E3** — `mdemg corrections rehydrate-structured` backfill CLI (idempotent, dry-run-previewable, batched). Regex tolerates embedded pipes in Context; malformed existing structured_data starts fresh (defensive). 6 Tier-1 tests: happy-path 4-case truth table, unparseable 4-case skip, merge fresh/preserves/overwrites/malformed-defensive.
- **E4** — `GuidanceItem` gains three optional structured fields; Lever C `fetchActionableCandidates` reads the L1 properties in the same query; `buildGuidancePrompt` renders correction items as "Do <correct> — not <incorrect>. (Context: <ctx>)" when structured is present, falls back to raw Content when not; `directiveSynthesisInstruction` (Lever B) preserves BOTH sides of the contrast verbatim. 3 prompt-rendering pins: structured present, fallback-to-Content, partial (Correct-only) form.
- **E5** — live Tier-3 across all 4 modified surfaces on `mdemg-dev`. Real end-to-end, no mocks:
  - `POST /v1/conversation/correct` → L0 `wgrs82t8fvpunspgg12pbnqk` has structured all-three-fields.
  - `POST /v1/memory/consolidate` → L1 `igtz7vhy844cngs49xxv2bhw` has structured all-three-fields.
  - `mdemg corrections rehydrate-structured --space-id mdemg-dev --dry-run=false` → parsed 1/33 (the bridge-authored obs from JIMINY-CONTRADICTED-BRIDGE-001), WARN-skipped 32 free-form hand-authored corrections. Linked L1 updated too.
  - Lever B synthesized narrative preserved both sides of the contrast: "You must fix every discovered test failure immediately" (from correction_correct) + "Previously, there was a tendency to label failures as pre-existing" (from correction_incorrect). Cited the exact L0 obs node id from E5-A.
- **E6** — canonical docs: CLAUDE.md architecture note; CHANGELOG `[Unreleased] > Added`; `docs/features/jiminy-actionability.md` §Follow-up section; this post.

## Commits (on `reh3376_dev01`)
1. `docs(jiminy-structured-correction-001): E0 — sprint plan` — `93ca3ee`
2. `feat(jiminy-structured-correction-001): E1 — Correct persists structured correction fields` — `797477d`
3. `feat(jiminy-structured-correction-001): E2 — CreateCorrectionNodes propagates structured to L1` — `557edf6`
4. `feat(jiminy-structured-correction-001): E3 — rehydrate-structured backfill CLI` — `3869570`
5. `feat(jiminy-structured-correction-001): E4 — Lever B + prompt render use structured fields` — `c840581`
6. `docs(jiminy-structured-correction-001): E5 — live Tier-3 verification` — `0eb084a`
7. `docs(jiminy-structured-correction-001): E6 — CLAUDE.md/CHANGELOG/feature/post`

## Live evidence highlights
| Surface | Pre-sprint | Post-sprint |
|---|---|---|
| L0 obs from POST /v1/conversation/correct carries structured Incorrect/Correct/Context | joined string only | all three fields as first-class JSON keys under structured_data.correction |
| L1 correction node exposes structured pair | not present | correction_incorrect/_correct/_context set from L0 |
| Backfill for historical L0 corrections | manual authorship required | idempotent `mdemg corrections rehydrate-structured` CLI parses the 1 template-matching row, honestly WARN-skips 32 free-form |
| Lever B narrative shape | LLM had to parse joined string | LLM sees "Do Y — not X" pair directly; narrative preserves both sides |
| HITL Preview text | draft_incorrect/draft_correct in Item.Meta (bridge-populated; unchanged) | unchanged from JIMINY-CONTRADICTED-BRIDGE-001 (bridge already carried them) |

## Lessons captured
1. **Template regex is a data-recovery contract, not a data-normalization guarantee.** Expect the majority of historical rows to not match the template — free-form corrections captured via /v1/conversation/observe won't follow the joined "CORRECTION: Incorrect: X | Correct: Y" shape. Backfill's job is to be honest about parseable vs unparseable, never to force a shape onto free-form data.
2. **When adding structured fields that mirror existing free-text, make downstream consumers PREFER the structured form when present and FALL BACK to the free-text otherwise.** Never require both, since backfill can never be 100%. `buildGuidancePrompt`'s correction branch demonstrates this pattern cleanly.
3. **Merge into existing JSON blobs, never replace.** The L0 obs's `structured_data` may already carry constraint-detector fields (`constraint_code`, `detected_constraints`) — the backfill's `mergeStructuredCorrection` helper preserves them. Dropping other keys would silently regress the constraint-detection pipeline.
4. **Dead code that LOOKS like persistence is worse than no persistence.** The `metadata_*` flatten in `createObservationNode` gave the impression that ObserveRequest.Metadata reached the graph — but the CREATE cypher never referenced the params, so nothing was ever persisted. Retired with a comment explaining prior intent + the requirement that a future feature must add BOTH sides.
5. **Extract before test.** `buildCorrectionObserveRequest` was extracted from `Correct` specifically so Tier-1 tests could pin the contract without a live Neo4j driver. This pattern (helper extraction for testability) is now standard for anything that mixes deterministic logic with a stateful call.
6. **Test the artifact of a live rebuild, not the intent.** E5-D initially saw an empty narrative; the fix wasn't code — it was waiting long enough for the LLM synthesis to complete (~30s). Live testing requires patience; timeouts are not proof of code failure.

## Non-goals (respected)
- No LLM-assisted parse of free-form corrections (deferred; the fallback path handles them).
- No schema migration (schemaless graph; properties additive on the `MemoryNode:Correction` label).
- No behavior change for non-correction items (Lever B prompt change is guarded on `type == correction` AND structured presence).

## Follow-ups
- **LLM-assisted content-parser for free-form corrections.** Call the LLM once per unparseable L0 to extract Incorrect/Correct from operator-authored durable rules. Only justified if the free-form corrections' fallback rendering is measurably worse than structured — which E5-D suggests is a MODEST gap, since the LLM already reads and understands the free-form content.
- **`applied_node_id` column on `contradicted_correction_drafts`** (deferred from JIMINY-CONTRADICTED-BRIDGE-001 post) — this sprint didn't touch it; still open.
- **Cache staleness on `/v1/jiminy/latest`** — noticed during E5-D; pre-existing warm store behavior. Not a regression but worth a follow-up sprint if the operator flags stale narratives.
- **Idempotency edge on backfill re-runs**: unparseable rows retain their existing `structured_data`, so re-running the backfill re-lists them as candidates and re-WARN-skips them. Cheap but noisy — a `--skip-known-unparseable` flag could suppress. Deferrable.

## Acceptance criteria — all met
- [x] POST /v1/conversation/correct writes structured_data.correction on L0.
- [x] Consolidation propagates the three fields to L1 correction node.
- [x] Backfill CLI populates structured on template-matching L0 corrections + linked L1s; honestly reports parseable vs unparseable.
- [x] Lever B narrative surfaces "Do Y — not X" imperative form when structured fields are present (verified via live LLM synthesis).
- [x] HITL Preview surfaces the pair (Tier-1 pinned; bridge already populates Item.Meta).
- [x] Full test suite green; lint clean.
- [x] Canonical docs updated.
