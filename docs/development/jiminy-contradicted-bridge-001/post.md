# JIMINY-CONTRADICTED-BRIDGE-001 — Sprint Post (2026-07-20)

## Summary
Bridges Jiminy's `contradicted` outcome (the highest-signal lesson signal in
the guidance loop) into the correction pipeline via HITL-REVIEW-001 →
JIMINY-CORRECTION-PRODUCER-001. Closes the last unblocked autonomous-
learning slot: 23 lifetime contradicted rows that had been dead-ending after
`ApplyNegativeFeedback` now become reviewable correction candidates that,
on operator approve, become real L1 `role_type='correction'` nodes.

## What shipped
- **E0** — sprint plan (v1.0 12-section format).
- **E1** — V0030 hypertable `contradicted_correction_drafts` with status
  enum (pending|approved|dismissed) + buffered async writer with primitives-
  only `RecordDraft` (avoids tsdb→jiminy import cycle); point reads
  (`FetchPendingBySpace`, `FetchByID`, `DedupExists`) + sync status
  transitions (`MarkApproved`, `MarkDismissed`, `ResetToPending`) all
  flush-then-query for read-your-writes. Schema version bumped 29→30.
  Writer always attached; separately-flagged bridge hook is what emits.
- **E2** — bridge hook in `RecordOutcome`. In-process LRU dedup on
  `(guidance_id, action_hash)` where `action_hash = sha256[:8]` over
  normalized `action_summary` (whitespace + case tolerant); template-based
  `Incorrect`/`Correct` render via `clipContent` (whitespace-trim + cap
  at `JiminyContradictedBridgeMaxContentLen`). All async; hot path never
  blocked.
- **E3** — HITL dataset `contradicted_drafts` with the new
  `ContradictedDraftsRubric` (0-4 `durable_rule` + `phrasing_quality`).
  Sink's verdict math: `durable_rule >= 3` → approve, `<= 1` → dismiss,
  `== 2` → defer. Approve calls `conversation.Service.Correct` — the L0
  obs it creates is then promoted to L1 by `CreateCorrectionNodes` on the
  next consolidation cycle. Reverse resets draft to pending; L0 obs stays
  (documented — operator uses `mdemg concepts tombstone` for full undo).
  Exported `review.DimInt` so out-of-package sinks share the same numeric-
  coercion semantics.
- **E4** — 6 bridge-helper Tier-1 tests (hash whitespace/case tolerance,
  clip-content, template shape, LRU dedup with correct eviction reasoning,
  empty-hash guard) + 10 sink-side Tier-1 tests (verdict truth table across
  int/int64/float coercion, Preview branches, Apply error paths, Apply
  doesn't call CorrectService on unclear verdicts). Full `go test ./...`
  green; `golangci-lint run` clean on touched packages.
- **E5** — live Tier-3 on `mdemg-dev`. Full end-to-end verified with no
  mocks in the loop:
  1. Synthesized a real contradicted verdict (never-commit-to-main
     violation; LLM classifier sim=0.85).
  2. Draft `c8jvgnmkl8zlmr4m58nl7rj3` persisted within the writer flush
     window.
  3. `GET /v1/review/datasets` exposed the new dataset with candidate_count=1.
  4. `POST /v1/review/grade` with `durable_rule=4` → real L0 obs
     `po2zahas8mh10ahwe0iimmoz` created; draft flipped to `approved` with
     `applied_obs_id` captured.
  5. `POST /v1/memory/consolidate` → real L1 correction
     `ymehdkihmj2yiu7t3bywsgxc` (count 32 → 33). Content matches drafted
     correction exactly. No bridge-specific code in the promotion path —
     clean pipeline composition on top of JIMINY-CORRECTION-PRODUCER-001.
- **E6** — canonical docs + default flip. `JIMINY_CONTRADICTED_BRIDGE_ENABLED`
  default false→**true** (live-verified unmasked on a fresh boot with the
  `.env` override removed; boot log confirmed `bridge_enabled=true`).
  CLAUDE.md architecture note; CHANGELOG `[Unreleased] > Added`;
  `docs/features/hitl-review.md` new §Contradicted-drafts dataset; this post.

## Commits (on `reh3376_dev01`)
1. `docs(jiminy-contradicted-bridge-001): E0 — sprint plan` — `d369f3c`
2. `feat(jiminy-contradicted-bridge-001): E1 — V0030 hypertable + async writer` — `4e20557`
3. `feat(jiminy-contradicted-bridge-001): E2 — RecordOutcome bridge hook` — `ab97b3d`
4. `feat(jiminy-contradicted-bridge-001): E3 — HITL dataset + Correct sink` — `bbace57`
5. `test(jiminy-contradicted-bridge-001): E4 — unit + adapter tests` — `1e40286`
6. `docs(jiminy-contradicted-bridge-001): E5 — live Tier-3 verification` — `9a67870`
7. `docs(jiminy-contradicted-bridge-001): E6 — CLAUDE.md/CHANGELOG/feature/post + default flip decision`

## Live evidence highlights
| Signal | Pre-sprint | Post-sprint |
|---|---|---|
| Contradicted-outcome captures a lesson | never (dead-ended after weaken) | draft persisted within 30s of the verdict |
| HITL dataset carries pending drafts | did not exist | live, candidate_count > 0 |
| Approve grade mints L0 correction obs | manual `POST /v1/conversation/correct` only | auto via sink; `applied_obs_id` recorded |
| L1 correction node emerges | required manual authorship then consolidation | free composition with JIMINY-CORRECTION-PRODUCER-001; no bridge-specific promotion code |
| `JIMINY_CONTRADICTED_BRIDGE_ENABLED` default | false | true (live-verified unmasked) |

## Lessons captured
1. **New hot-path → HITL bridges should follow this shape.** Async writer
   with primitives-only interface (no import cycle) + LRU dedup on a semantic
   hash (not raw content) + default-off flag flipped only after live smoke
   + HITL-gated substrate mutation. Every future bridge (e.g., a similar
   `partial_compliance` → "clarification draft" surface) can copy this
   scaffolding.
2. **Semantic dedup beats identity dedup.** Hashing normalized text
   (`strings.Fields` + `strings.ToLower`) means whitespace/case jitter in
   the same action doesn't spawn duplicate drafts. Session-scoped LRU +
   DB-side `DedupExists` (7-day window) covers both the fast repeat case
   and cross-process races.
3. **`Preview` must NEVER mutate — even to warm caches.** The dataset test
   pins that Preview does not call the mock CorrectService. Sinks that
   preview via "peek at what would happen" side effects (trust reads, node
   confidence reads) must guarantee those are true no-ops.
4. **Reverse is a review-invitation, not a substrate rollback.** The
   contradicted-drafts sink deliberately leaves the L0 obs in place on
   Reverse — the L0 is real memory the operator authored (via approve),
   and undoing it needs a first-class tombstone action. Sinks whose Apply
   creates immutable substrate rows should document this asymmetry.
5. **`ObserveResponse` carries both `ObsID` and `NodeID`.** For graph-side
   lookups use `NodeID`. The sink currently records `ObsID` (matching the
   existing guidance-dataset pattern for consistency) — additive `applied_node_id`
   is a small follow-up (V0031 ALTER TABLE).
6. **Live-verify default flips too.** After flipping a config default, the
   `.env` override may mask the change. Live-verification of a default
   requires removing the override and confirming the new default is active
   via the boot log (`bridge_enabled=true`) or first-behavior check. The
   E6 unmask-and-restart step caught this correctly.

## Non-goals (respected)
- No full-auto mode (bridge always HITL-gated).
- No LLM-synthesized draft phrasing (templates only for MVP; operator refines
  during review).
- No retroactive processing of the 23 historical `contradicted` rows (bridge
  is forward-only; a `mdemg contradicted backfill` CLI is a follow-up).

## Follow-ups
- **Structured correction propagation** (`Incorrect`/`Correct`/`Context` →
  L1 `structured_data` for first-class synthesis parsing) — orthogonal;
  benefits both the operator-authored and bridge-authored paths.
- **`applied_node_id` column on `contradicted_correction_drafts`** —
  additive V0031 for cleaner graph-side traceability.
- **LLM-synthesized draft phrasing** — a small `contradicted_draft.synthesize`
  call site (analogous to `jiminy.synthesize`) could produce better first-
  draft prose. Only justified if operators report the template output needs
  substantial rewrites at scale.
- **Historical backfill CLI** — `mdemg contradicted backfill` walks
  `constraint_outcomes WHERE outcome_type='contradicted' AND time > <cutoff>`
  and emits drafts for each. Only worth it if the operator wants the 23
  historical lessons captured.

## Acceptance criteria — all met
- [x] `contradicted_correction_drafts` gains a row within seconds of a live
      `contradicted` verdict.
- [x] `/v1/review/datasets` surfaces `contradicted_drafts` with pending count.
- [x] Approving the draft creates a real L0 `obs_type='correction'` MemoryNode
      via `conversation.Service.Correct`.
- [x] Next consolidation promotes the L0 to a fresh L1 `role_type='correction'`
      node (JIMINY-CORRECTION-PRODUCER-001).
- [x] Full test suite green; lint clean.
- [x] Canonical docs updated.
