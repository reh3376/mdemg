# Sprint JIMINY-CORPUS-001 — Raise guidance follow rate (data + repetition + phrasing)

## 1. Header & Metadata

- **Sprint ID:** JIMINY-CORPUS-001
- **Sprint line:** `docs/development/jiminy-corpus-001/`
- **Date opened:** 2026-07-03
- **Target version:** minor (guidance-quality; behavior changes are default-flagged where risky)
- **Estimated effort:** ~1.5 dev-days (8 sequential epics)
- **OpenAI spend:** $0 (local Fable 5 execution)
- **Risk level:** Medium–High — E2 mutates the protected `mdemg-dev` substrate (tombstone-only, reversible, operator-gated); E3/E4 change guidance surfacing/labelling behavior.

## 2. Problem Statement

DASHBOARD-TRUTH-001 made the Jiminy metrics honest — they now truthfully read a guidance follow rate of ~0.17 (live 7d: 0.172 over 1,621 constraint outcomes; should-follow 0.142). The score is genuinely low, and the investigation (three Fable-5 agents, 2026-07-03) localised the cause: Lever C (`JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=true`) correctly redirected the guidance surface onto the `role_type='constraint'` partition — but that partition is **~half junk** and a tiny set of nodes **repeats relentlessly regardless of task relevance**, so correctly-ignored noise dominates the outcome stream.

Live evidence (mdemg-dev, 2026-07-03):
- **140** live constraint-role nodes; ≥32 clearly junk by coarse pattern (`doc_template` 17, `bash_error` 5, `sprint_status` 4, `pr_status` 3, `build_succeeded_fabricated` 3), more with de-duplication (duplicate `CONSTRAINT 1–5` sets under two constraint_codes).
- **Repetition: 37 distinct source nodes → 1,826 guidance rows in 7 days (~49× each)**, fired irrespective of relevance → relevance failure measured as compliance failure.
- Lever B (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED`) is OFF (default), against the JIMINY-ACTIONABILITY-001 operator recommendation.

The fix is data (stop + remove junk), repetition control, labelling honesty, and phrasing.

## 3. Scope & Constraints

**In scope:** the four near-term follow-rate movers — (E1) stop junk being promoted to constraint role; (E2) purge existing junk; (E3) repetition control; (E4) ignored↔not_applicable relevance gate in the outcome classifier; (E5) enable Lever B.

**Out of scope (disclosed follow-ups):**
- `RetrieveForJiminy` role_type adapter gap (why `correction`-type guidance has zero rows ever) — deeper retrieval-adapter fix, its own sprint.
- HITL curation (SME-authored `suggested_guidance`) — the JIMINY-RELEVANCE-001 3–6-month arc toward >0.9.
- New retrieval/candidate-composition changes beyond Lever B/C already shipped.

**Constraints:** sequential epics; docs before implementation; **fix the source (E1) BEFORE pruning (E2)** per the data-hygiene rule (`feedback_prune_nonconforming_data.md`); E2 uses tombstone-only (`is_archived`), never hard-delete, with full backup + `LIMIT 5` preview + operator sign-off, never circumventing `mdemg-dev` deletion protection; no hardcoded values (config knobs with defaults); CUIDv2 for any minted ids; 3 testing tiers incl. live Tier-3; never leave test failures; never commit to `main`; lint before commit. Behavior changes (E3/E4) ship behind config with sane defaults; disclose live before/after.

## 4. Dependencies

- Live stack: mdemg :9999, Neo4j `mdemg-neo4j-1` (space `mdemg-dev`, protected), TSDB `mdemg-timescaledb-1`, llama-server :8102.
- Shipped prerequisites on `main`: Lever C (JIMINY-ACTIONABILITY-001), the honest metrics (DASHBOARD-TRUTH-001 E5: pie/should-follow read `constraint_outcomes`), the guidance corpus writer (`guidance_training_rows`, JIMINY-RELEVANCE-001), trust EMA (JIMINY-EFFECTIVENESS-001), the outcome classifier + `constraint_outcomes` sink.
- Backup/restore path (BACKUP-RESTORE-VERIFY-001) for the E2 pre-purge snapshot.
- Investigation record: `docs/development/dashboard-truth-001/investigation_findings.md` (Jiminy section).

## 5. Implementation Plan (sequential epics + gates)

**E0 — Sprint plan (this doc) + commit.**

**E1 — Stop junk becoming constraints (forward-fix first).** Root-cause the ConvObs→`role_type='constraint'` promotion path (the junk nodes carry `obs_type=NULL`, promoted from conversation observations; `Build/test succeeded:…` are the pre-HOOKWIRE-001 fabricated-observation class). Add a classification/promotion gate so non-constraint content (build/test/error status, PR/sprint notes, doc/template dumps) is not promoted to constraint role. Config-driven predicate; log rejections. **Gate:** a synthetic junk observation is NOT promoted to constraint; a genuine constraint still is; unit-tested.

**E2 — Purge existing junk (operator-gated, tombstone-only).** (1) Full backup: `mdemg data export` / pg_dump + Neo4j snapshot to `.mdemg-backup-*`. (2) Produce the exact candidate list (content + surfacing count + constraint_code) via read-only query; **show the operator a `LIMIT 5` preview and the full list; STOP for sign-off.** (3) On sign-off, tombstone (`is_archived=true`, `archive_reason='jiminy_corpus_junk_purge'`, `archived_at`) — never hard-delete; de-dupe the duplicate `CONSTRAINT 1–5` sets (keep one code, tombstone the duplicate). **Gate:** small-batch (LIMIT 5) verified first; tombstoned nodes drop out of the constraint partition; live constraint count falls; follow rate re-measured after a window.

**E3 — Repetition control.** Per-node surfacing cooldown / session dedup (suppress a node ignored ≥N consecutive times in a session) + per-constraint-effectiveness surfacing prior (chronically-ignored nodes decay in surfacing rank). Config: cooldown count, decay weight, all default-sane. ⚠️ RRF-SCALE-001-safe: gate on effectiveness/similarity signals, never a hardcoded RRF score. **Gate:** distinct-nodes/rows ratio rises (less repetition); a chronically-ignored node stops dominating; unit + live.

**E4 — ignored↔not_applicable relevance gate.** In the outcome classifier, when guidance-vs-action similarity is below the LOW threshold, classify `not_applicable` (correctly-irrelevant), not `ignored`. Config threshold (reuse/mirror `JIMINY_OUTCOME_SIMILARITY_LOW`). **Gate:** an irrelevant surfacing lands `not_applicable`; a relevant-but-unfollowed one stays `ignored`; the should-follow rate reflects real actionable compliance; unit + live.

**E5 — Enable Lever B** (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true` in `.env`; confirm the compose/UI expose it). Imperative directive phrasing via the existing `jiminy.synthesize` (no new hot-path LLM call). **Gate:** live warm guidance renders imperative directives; no synthesis-timeout regression.

**E6 — Testing (3 tiers).** See §6.

**E7 — Documentation (never cut).** `docs/features/jiminy-actionability.md` (or a new corpus doc), CLAUDE.md note, CHANGELOG, `post.md`.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** promotion gate (junk rejected / constraint accepted); repetition cooldown + effectiveness-prior ordering; relevance-gate classification (below-LOW → not_applicable); config defaults for every new knob.
- **Tier 2 (integration):** the promotion path end-to-end (observe junk → not a constraint node); the outcome classifier over a fixture set produces the corrected label distribution; guidance surfacing dedup over a repeated-node fixture.
- **Tier 3 (live — required):** (1) pre/post backup verified; (2) E2 small-batch LIMIT-5 tombstone verified before full; (3) live constraint-node count falls after purge; (4) **windowed follow rate measured before → after** the corpus clean + repetition control (disclose the window; continuous guidance means before/after windowed, not synchronous A/B; attribute per-epic by measuring after each); (5) Lever B renders imperative directives on a live warm call. Observe via TSDB `constraint_outcomes`/`guidance_training_rows` + Grafana Jiminy panels (now honest).

## 7. Commit Strategy

One commit per epic on `reh3376_dev01` (E0 plan; E1 forward-fix; E2 purge — backup + tombstone scripts + the reviewed list; E3/E4/E5 behavior; E6 tests folded where natural; E7 docs). Push once at the end → auto-PR. Surprise live-smoke bugs get their own fix-commit. Sprint summary to the PR comment. The purge (E2) records the tombstoned node ids for reversibility.

## 8. Verification Checklist

- [ ] E1 promotion gate: junk not promoted, genuine constraint promoted (unit + live)
- [ ] E2 backup taken + verified before any mutation; LIMIT-5 preview shown + operator sign-off obtained; full tombstone reversible (`is_archived`, `archive_reason`); duplicate CONSTRAINT sets de-duped
- [ ] Live constraint-node count falls; tombstoned nodes leave the surfacing partition
- [ ] E3 repetition control: distinct/rows ratio rises; chronically-ignored node decays
- [ ] E4 relevance gate: irrelevant→not_applicable, relevant-unfollowed→ignored
- [ ] E5 Lever B on; imperative directives render live; no synthesis-timeout regression
- [ ] Windowed follow rate measured before→after; per-epic attribution disclosed
- [ ] `go build ./...` + `go test ./...` + `golangci-lint run ./...` green
- [ ] CLAUDE.md note + CHANGELOG + feature doc + post.md updated

## 9. Documentation Update — E7 above

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Purge tombstones a genuine constraint | Low | High | Operator sign-off on the exact list; tombstone-only (reversible via `is_archived=false`); small-batch LIMIT-5 first |
| Purge refills (junk re-promoted) | Medium | Medium | E1 (source fix) ships BEFORE E2; a post-purge re-check confirms no new junk promoted |
| Repetition cooldown suppresses a genuinely-relevant constraint | Medium | Medium | Cooldown keyed on consecutive *ignored* outcomes in a session, not blanket; effectiveness prior decays slowly; config-tunable; live-observe |
| Relevance gate mislabels real ignores as not_applicable | Medium | Medium | Threshold mirrors the existing `JIMINY_OUTCOME_SIMILARITY_LOW`; unit test both directions; the should-follow metric already separates actionable types |
| Follow-rate lift not cleanly attributable (continuous guidance) | High | Low | Measure after each epic over a defined window; the purge lift is mechanically attributable |
| Protected-space mutation | — | High | Never hard-delete; tombstone-only; `mdemg-dev` deletion protection never circumvented; backup first |

## 11. Documents Accessed

- `docs/development/dashboard-truth-001/investigation_findings.md` (Jiminy root-cause)
- `internal/jiminy/{service.go,stats.go,codegen.go}` (promotion, surfacing, outcome classification), `internal/consulting/`
- TSDB `guidance_training_rows`, `constraint_outcomes`; Neo4j `role_type='constraint'` nodes + `GUIDANCE_OUTCOME` edges
- `.env` (Lever C on, Lever B off), `internal/config/config.go`
- Memory: `feedback_prune_nonconforming_data.md`, `feedback_small_batch_first.md`, `feedback_no_hardcoded_values.md`

## 12. Rollback Procedures

- **E2 purge:** tombstones are reversible — `MATCH (n) WHERE n.archive_reason='jiminy_corpus_junk_purge' SET n.is_archived=false REMOVE n.archive_reason`. Full backup in `.mdemg-backup-*` as the last resort. No hard-delete performed.
- **E1/E3/E4 code:** contained to `internal/jiminy` (+ config); revert the epic commit; all behavior behind config knobs (set to prior default to restore).
- **E5 Lever B:** unset `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` (default off) + restart.
