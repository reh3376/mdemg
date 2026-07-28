# HITL-CURATION-002 — Sprint Plan

**Date:** 2026-07-27 | **Branch:** `reh3376_dev01`
**Parent:** HITL-REVIEW-001 (shipped 2026-06-24, PR #TBD)
**Source of trigger:** Q4 frontier deep-dive
(`docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`), Finding 3
(HITL curation stalled at 2 grades/week) and Finding 2 (25%-heuristic-in-
outcomes vs 0.03%-in-corpus split).

## 1. Header & Metadata

Ship an **auto-curation layer over HITL-REVIEW-001** that clears the backlog
of high-confidence-obvious cases (LLM-graded), and surfaces the low-confidence
minority to the operator via a weekly cadence. Effort ~4-5d. Risk medium
(auto-grading a HITL corpus is philosophically loaded — done wrong, it
poisons the training signal it was supposed to curate). Design gates the
risk explicitly: **auto-grade never reinforces the live substrate; only
operator-confirmed grades do that.**

## 2. Problem Statement

HITL-REVIEW-001 shipped a complete platform (`internal/review/` + Review
tab + `/v1/review/*` endpoints + `contradicted_drafts` + `guidance_training_rows` +
16 LLM datasets). **7-day HITL activity: 2 grades total**, both on
`contradicted_drafts`. Meanwhile:

- **5 pending `contradicted_correction_drafts`** waiting since 2026-07-21 —
  well-formed Jiminy-authored contrast pairs; high signal, low curation cost
- **59 `guardrail.evaluate` LLM rows** produced by GUARDRAIL-PRODUCER-001
  (shipped 2026-07-22), 0 graded
- **6,568 `guidance_training_rows`** in the corpus (GUIDANCE-AUDIT-001
  auto-relabeled the heuristic-labeled rows down to 0.03%), but the
  actual **gold-labeled fraction is unknown** — the retrain trigger reads
  `manifest.gold_fraction` from the manifest, not the live grade count

Root cause of the stall: the human-in-loop is not in the workflow.
Curation demands operator attention that doesn't come, and the pipeline
(FT retrain → guidance quality lift → follow-rate rise) is starved of the
input it was designed around.

**This sprint doesn't try to remove the human — it removes the friction
around the human.** Auto-grade clears the boring 80%; the operator reviews
the ambiguous 20% and everything auto-grade flags as low-confidence.

## 3. Scope & Constraints

**In scope:**

- **E1 — Auto-grader for `contradicted_drafts`** (`internal/review/autograde.go` +
  wire into an on-demand CLI + a periodic scheduler behind a default-off flag).
  LLM-graded pass produces a synthetic `GradeSubmission`; confidence gated
  on prompt-return-JSON's own `confidence` field (≥ 0.80 auto-clear; below
  that stays pending). **Auto-grades DO NOT reinforce the substrate** — the
  approval side-effect (mint an L0 correction obs) is gated on `applied_by='operator'`.
  Auto-grade writes a `review_grades` row with `grader_id='auto:<model>@<sha>'`
  and `reinforcement_applied=false`.
- **E2 — Auto-grader for `guardrail.evaluate` LLM output** (extends E1 with
  `LLMOutputRubric` — different dimensions, same confidence-gated pattern).
- **E3 — Weekly HITL cadence surface** — `mdemg review cadence` CLI command
  that produces a compact operator prompt: "N unreviewed items; top-K
  low-confidence-auto-graded, top-K oldest-pending; click-through URLs to
  the Review tab." Feed via a supervised scheduler (opt-in via
  `HITL_CADENCE_SCHEDULE_CRON`, default empty) — the operator gets it in
  their inbox / logs / alert channel (whichever is wired), can act at their
  cadence, no forced turnaround.
- **E4 — Gold-fraction metric** — the retrain trigger currently reads
  `manifest.gold_fraction` (JIMINY-RELEVANCE-001 curation output). Add a
  live gauge `mdemg_hitl_gold_fraction{dataset_id}` = graded/total over
  the last N days, and expose it in a new `mdemg-hitl` Grafana panel.
  Alert rule `hitl_curation_stalled` when the operator-graded rate over
  the last 7 days is zero AND the pending queue has ≥5 items (the two
  live symptoms this sprint targets).
- **E5 — Live Tier-3 drill** — auto-clear the 5 existing pending contradicted
  drafts on `mdemg-dev`; operator spot-checks the auto-grade verdicts against
  their own judgment; sprint ships default-off if the spot-check disagrees
  substantively.
- **E6 — Docs epic (mandatory)** — feature doc, CHANGELOG, CLAUDE.md
  architectural note pinning the "auto-grade never reinforces the substrate"
  invariant.

**Out of scope:**

- **Multi-grader consensus** — the multi-grader vote pattern is HITL-REVIEW-002
  scope; this sprint's auto-grader is single-model, single-verdict.
- **DPO / SFT sinks** — also HITL-REVIEW-002 scope; auto-grade output flows
  into the same `review_grades` table but downstream FT consumers are
  unchanged.
- **`suggested_guidance` field** on grade submission (SME-authored corrective
  examples) — auto-grader produces no corrective text; operator flow retains
  the field.
- **Auto-approve rollout beyond the 3 datasets named** (contradicted_drafts,
  guardrail.evaluate, and — as the CLI-cadence signal only — the guidance
  corpus datasets); extending to the other 15 LLM call sites is a follow-up.

## 4. Dependencies

- **HITL-REVIEW-001** (shipped): the `ReviewableDataset` interface,
  `review_grades` V0028 hypertable, `ReinforcementSink` contract.
- **JIMINY-CONTRADICTED-BRIDGE-001** (shipped): the `contradicted_drafts`
  dataset + `contradicted_correction_drafts` V0030 table.
- **JIMINY-CORRECTION-PRODUCER-001** (shipped): the L1 correction promoter
  the sink depends on.
- **GUARDRAIL-PRODUCER-001** (shipped): the 59+ guardrail rows to grade.
- **LLM endpoint** (`http://127.0.0.1:8102/v1`): auto-grade calls it;
  respects `MDEMG_ALLOW_NO_LLM` opt-out.
- **NOSILENT-001 pattern**: auto-grade scheduler reports to `jobhealth`;
  E4 alert rule uses the shared distinct-Service contract.
- **TSDB-CONSUME-001 pattern**: E4 alert rule uses
  COALESCE(MAX(...),0)+min-count-floor pattern; no `ORDER BY … LIMIT 1`.

## 5. Implementation Plan (sequential epics + gates)

**E1 — Auto-grader for `contradicted_drafts`**
- `internal/review/autograder.go` new package-internal type `Autograder`
  with `Grade(ctx, dataset, item) (GradeSubmission, confidence float64, error)`
  signature.
- LLM prompt renders the item's `Content` (draft correction) + `Context`
  (offending action) + the dataset's `Rubric.Description`; expects strict
  JSON `{dimensions: {durable_rule: 0-4, phrasing_quality: 0-4}, confidence: 0-1, rationale: string}`.
  Uses `llmclient` with `WithTaskName("review.autograde")` for telemetry.
- CLI: `mdemg review autograde --dataset contradicted_drafts --space-id
  mdemg-dev [--min-confidence 0.80] [--dry-run] [--limit 50]`.
- Scheduled hook (supervised via `internal/supervisor/`, opt-in
  `HITL_AUTOGRADE_ENABLED=false` default; interval
  `HITL_AUTOGRADE_INTERVAL_MIN` default 60).
- Writes via existing `/v1/review/grade` endpoint with `grader_id=
  'auto:mdemg-llm-v1@<binary-sha>'` and `reinforcement_applied=false` —
  the sink flow already tolerates this; the reinforcement path checks
  the flag.
- **Gate:** unit test proves the confidence gate; live smoke on 1 draft
  in a scratch space produces exactly one `review_grades` row with the
  right `grader_id` prefix.

**E2 — Auto-grader for `guardrail.evaluate`**
- Reuses the E1 Autograder; register the `llm:guardrail.evaluate` dataset
  in the CLI's dataset list.
- Rubric is `LLMOutputRubric` (existing) — dimensions are different from
  contradicted_drafts, but the prompt-JSON contract is uniform (dimensions
  map + confidence + rationale).
- **Gate:** live smoke on 5 guardrail rows on mdemg-dev; auto-grade succeeds
  at ≥0.80 confidence on the constraint-violation-obvious cases, stays
  pending on borderline.

**E3 — Weekly HITL cadence surface**
- `mdemg review cadence [--out-format=text|json]` — reads pending counts +
  low-confidence auto-graded counts + oldest-pending timestamps across
  datasets; renders a compact text summary or JSON.
- Supervised scheduler (opt-in `HITL_CADENCE_SCHEDULE_CRON`, default empty):
  writes the summary to a file `~/.mdemg/hitl/cadence-<date>.txt` +
  posts a MEDIUM alert `hitl_cadence_ready` when the summary has ≥5 items
  needing review (so the operator sees it via the hook channel).
- **Gate:** invocation produces well-formed output; opt-in scheduler
  writes exactly one file per week (test via short interval override).

**E4 — Gold-fraction metric + curation-stall alert**
- Metric `mdemg_hitl_gold_fraction{dataset_id}` = `count(review_grades over
  7d WHERE grader_id NOT LIKE 'auto:%')` / `NULLIF(pending_count + graded_count, 0)`.
  Emitted by the same live-collectors path as the J17 metrics.
- Alert rule `hitl_curation_stalled` in `internal/alert/rules.go`:
  gates on `pending_count >= HITL_CURATION_STALL_MIN_PENDING` (default 5)
  AND `operator_graded_last_7d == 0`; distinct Service `hitl-curation`,
  Severity MEDIUM, ForDuration 24h (won't flap on a slow curation week).
- Grafana panel row on `mdemg-hitl` (new dashboard file) showing
  pending vs graded per dataset, auto-vs-operator grade split, and the
  gold-fraction gauge.
- **Gate:** rule + panel pass existing sweep tests
  (`TestAllRules_NoLimitOneAntiPattern`, `TestAllRules_DistinctServicePerSeverity`,
  `dashboards_test.go`); metric emits.

**E5 — Live Tier-3 drill on mdemg-dev**
- Run `mdemg review autograde --dataset contradicted_drafts --space-id
  mdemg-dev --dry-run` first (preview mode); operator inspects the 5
  verdicts.
- If operator approves the verdicts substantively, drop `--dry-run` and
  land the auto-graded rows.
- Verify `constraint_outcomes` still shows the same distribution
  (auto-grade DIDN'T reinforce, so no new activity there).
- Verify `review_grades` has 5 new rows with `grader_id='auto:…'`.
- **Gate:** operator sign-off on drill. If disagreement, sprint ships
  E1-E4 default-off + a follow-up task to tune the prompt or dimensions.

**E6 — Docs + CHANGELOG + CLAUDE.md note**
- New `docs/features/hitl-auto-curation.md`.
- CHANGELOG entry under `### Added`.
- CLAUDE.md pin: **"Auto-grade never reinforces the substrate; only
  operator-confirmed grades apply the reinforcement side-effect."** — the
  architectural invariant.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `autograder_test.go` (confidence gate, prompt-JSON parse,
  well-formed GradeSubmission output); `cadence_test.go` (empty state,
  populated state, output format); alert-rule test
  `TestHITLCurationStallRule` (gated on pending + operator-grade absence).
- **Tier 2 (integration):** the existing sweep tests auto-cover the new
  alert rule (LIMIT-1 anti-pattern, distinct services); grafanapin covers
  the new panel; UATS spec `hitl_autograde.uats.json` covers the CLI
  entrypoint via the `POST /v1/review/grade` write path.
- **Tier 3 (live on mdemg-dev):** E5 drill against the 5 existing pending
  drafts, operator sign-off gate. Then a 24h scheduler-on soak to verify
  the periodic auto-grade doesn't produce misclassified rows en masse.

## 7. Commit Strategy

Sequential single-epic commits per operator "no parallelization" rule:
`E1 → E2 → E3 → E4 → E5 → E6` each a separate commit under the
`HITL-CURATION-002` slug. Live-Tier-3 findings that surface bugs get
their own follow-up fix-commits (per the Phase 11.6.2 precedent).

## 8. Verification Checklist

- [ ] `HITL_AUTOGRADE_ENABLED=false` by code default (opt-in only)
- [ ] `HITL_CADENCE_SCHEDULE_CRON` empty by default (opt-in only)
- [ ] Auto-grade rows write `grader_id='auto:...'` prefix (grep-testable)
- [ ] Auto-grade rows write `reinforcement_applied=false` (invariant)
- [ ] Reinforcement side-effect NEVER fires from auto-grade (pin-tested)
- [ ] `mdemg review cadence` renders empty state without erroring
- [ ] `hitl_curation_stalled` rule uses COALESCE(MAX(...)), no LIMIT 1
- [ ] Distinct Service label `hitl-curation` (no collision with existing)
- [ ] Grafana panel added; JSON validates
- [ ] Live Tier-3 drill: 5 pending drafts auto-graded, operator sign-off
- [ ] `go build`, `golangci-lint run ./...`, `go test ./...` full green
- [ ] Feature doc `docs/features/hitl-auto-curation.md` written
- [ ] CHANGELOG entry under `### Added`
- [ ] CLAUDE.md architectural note added

## 9. Rollback Procedures

- All defaults are opt-in — no code path activates without an env var flip.
- Auto-grade rows can be identified by `grader_id LIKE 'auto:%'`;
  deletion is a single SQL statement, non-destructive to operator-graded
  data.
- The reinforcement invariant means rolling back auto-grades has **zero
  substrate side-effect** — no L0 obs was ever minted from an auto-grade.
- Scheduler can be disabled by unsetting `HITL_AUTOGRADE_ENABLED` +
  restart.

## 10. Risks & Mitigations

- **Risk:** Auto-grader systematically approves low-quality drafts, poisoning
  the corpus.
  - **Mitigation:** the reinforcement invariant. Auto-grade produces
    `review_grades` metadata rows; no L0 obs, no substrate mutation. The
    operator's spot-check in E5 gates the ship.
- **Risk:** Auto-grader disagrees consistently with the operator's
  judgment on the same class of items.
  - **Mitigation:** the operator's E5 spot-check catches this before it
    ships. The confidence gate ensures borderline items stay pending
    for the operator, not auto-cleared.
- **Risk:** The `hitl_curation_stalled` alert becomes noisy if the operator's
  natural curation cadence is bi-weekly rather than weekly.
  - **Mitigation:** the ForDuration=24h stops single-week bursts; the
    threshold is config-tunable.
- **Risk:** The auto-grader's LLM prompt hallucinates dimension values
  outside the rubric range.
  - **Mitigation:** Parse-gate at the client: reject non-integer,
    out-of-range, or missing dimensions; count as auto-grade failure
    (item stays pending). Pin-tested.
- **Risk:** Auto-grade runs on the same LLM the substrate is training,
  creating a circular-reference — the model grades its own outputs.
  - **Acknowledgment (not mitigation):** yes, this is a real philosophical
    risk. It's why the sprint gates on the operator-in-loop and the
    invariant. Multi-model auto-grade is HITL-REVIEW-002 scope.

## 11. Documents Accessed

Filled in at post.md.

## 12. Documentation Update

Final epic — never cut (Sprint Plan Format v1.0). Covered by E6.
