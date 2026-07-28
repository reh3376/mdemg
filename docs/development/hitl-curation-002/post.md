# HITL-CURATION-002 — Sprint Post

**Date:** 2026-07-27 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive Finding 3 (HITL curation stalled at
2 grades/week) + Finding 2 (25%-heuristic-in-outcomes signal-quality issue).

## Verdict

**Shipped.** All six epics landed in sequence (E1–E6, six commits under
`reh3376_dev01`). The load-bearing invariant (auto-grade NEVER triggers
the substrate reinforcement side-effect) is pin-tested at TWO layers and
verified live on `mdemg-dev`: 6 auto-grades landed, all with
`grader_id='auto:mdemg-llm-v1@dev'` and `reinforcement_applied=false`;
contradicted drafts remained 5/5 pending post-autograde (invariant proven —
zero substrate mutation from an auto-grade).

## What shipped (by epic)

**E1 — Autograder + bulk-candidates endpoint + CLI** (commit `8ecb6c30`):
`internal/review.Autograder` (rated-rubric only; DPO/ranked rejected
explicitly), `GET /v1/review/candidates` (deterministic iteration
bypassing the human-sampler bias), `ReviewableDataset.AutogradePromptHinter`
optional interface (dataset-specific typology guidance),
`contradicted_drafts` hint, `mdemg review autograde` CLI. 15 unit tests
pin the invariant + parse + gate.

**E2 — Per-call-site hints + guardrail hint** (commit `1048e908`):
`llmCallSiteDataset` implements `AutogradePromptHinter` with dispatch on
`site.task` via `llmAutogradeHints` map; guardrail-specific hint teaches
the pass/warn/block typology.

**E3 — `mdemg review cadence` CLI** (commit `7b5ae953`): compact digest
of pending items across all datasets; text + JSON output; 4 unit tests
pin empty-state / populated / JSON-roundtrip / actionable-field.

**E4+E5 — Stall alert + drill sign-off** (commit `40fdf4c2`):
`alert.HITLCurationStalledRule(minPending, lookbackHours)`; distinct
Service `hitl-curation`; MEDIUM severity; ForDuration 24h; EXCLUDES
`auto:%` grader_id from operator count. 2 targeted unit tests +
sweep coverage.

**E6 — Docs epic** (this commit): feature doc, CHANGELOG entry, CLAUDE.md
architectural pin, this post.

## Live Tier-3 evidence (mdemg-dev)

**E1 drill on 5 pending `contradicted_drafts`:**

| # | Pre-tune verdict | Post-tune verdict (shipped prompt) |
|---|---|---|
| 1 | `dr=0 pq=0 conf=0.80` (correct: session log) | Same, better rationale citing hint's typology |
| 2 | `dr=0 pq=0 conf=0.80` (correct) | Same, better rationale |
| 3 | `dr=0 pq=0 conf=0.80` (correct) | Same |
| 4 | `dr=4 pq=? conf=0.90` (wrong — approved ambiguous) | **LOW conf=0.00** — left pending (safer) |
| 5 | `dr=4 pq=3 conf=0.90` (wrong — approved log) | Unchanged (semantic move the prompt can't kill) |

**Pre-tune:** 3/5 defensible, 2/5 clearly wrong.
**Post-tune:** 4/5 defensible; item 5 remains a hallucination bounded by
the invariant. Ship default-off; operator gate stands.

**Live landing (post-tune, --dry-run=false):**
- 4 `review_grades` rows written, all with `grader_id='auto:mdemg-llm-v1@dev'`
- All 4 rows `reinforcement_applied=false` (invariant held)
- Contradicted-drafts table: 5/5 still `pending` (auto-grade never
  flipped status)
- Fresh `GET /v1/review/candidates` still returns 5 (dataset queries
  status, not grades — correct behavior)

**E2 drill on 3 pending `llm:guardrail.evaluate` rows:**
All 3 auto-graded to `correctness=4 format=4 helpfulness=4 conf=0.90`.
Structural finding: for empty-verdict outputs, the model reads
`{"violations":[],"warnings":[]}` as "guardrail said no problem, therefore
correct" — it can't cross-check against the action. Item 2's rationale
was a clear hallucination ("adds a new field" when there was no new
field). **The invariant + NoopSink on all `llm:*` datasets makes this
zero substrate risk** — auto-grade produces training-signal-quality
metadata, not live reinforcement.

**E3 cadence live output:** rendered 16 datasets with 2,887 total pending
items (dominated by the 200-item sampler caps on `llm:*` datasets;
contradicted_drafts at 5). Both text and JSON formats verified.

**E4 alert live-verified:**
- Rule count 28 → 29 (HITL rule loaded)
- Live stall query: 0 operator grades (7d) + 7 auto grades (7d) + 5
  pending drafts → `stall_signal=5` (matches pending count)
- Threshold=5, Operator=gt → `5 > 5` is FALSE → doesn't fire at boundary;
  any additional pending draft OR lowered threshold triggers immediately.
  Boundary-correct.

## Rules pinned

1. **Auto-grade rows enrich the corpus but NEVER mutate the substrate.**
   Every auto-grade row's `grader_id` starts with `review.AutoGraderPrefix`
   (`"auto:"`); every autograde CLI POST uses `reinforce:false`. Both
   layers are pin-tested. The reinforcement side-effect (mint L0 obs,
   adjust trust, etc.) is operator-only.
2. **Datasets teach the autograder their typology, not just their rubric.**
   Rubric anchors describe the SCORING scale; the optional
   `AutogradePromptHinter` supplies the item TYPOLOGY (what a rule looks
   like vs a log). The E1 dry-run proved anchors alone insufficient.
3. **The `hitl_curation_stalled` rule EXCLUDES the `auto:%` grader_id**
   from the operator-grade count — else the sprint's own auto-clear would
   suppress the stall signal. Changing this without adding a separate
   operator-signal source silently disarms the alert.
4. **LLM-output `correctness` is not reliably autogradable.** For any
   `llm:*` dataset, correctness requires ground truth the model can't
   independently derive. Autograde produces confidence-marker metadata;
   operator remains the arbiter of correctness. The invariant makes this
   safe.

## Structural limitations (known)

- **Item 5 class**: high-confidence hallucination on completion logs
  ("EXECUTED: deleted N rows" → "this is a durable rule about pruning").
  Prompt tuning can reduce but not eliminate. Bounded by the invariant.
- **`llm:*` correctness axis**: covered above. Format-validity-only
  autograde variant is a follow-up candidate.
- **Sampler-cap 200 in cadence output**: LLM datasets report exactly 200
  pending because `ReviewSampleSize` caps the FetchCandidates limit.
  Not a real count of "all pending" — cadence would need a separate
  count-only endpoint to be accurate at scale. Follow-up.

## Follow-ups disclosed

- **HITL-CURATION-002.1 (format-validity-only autograde variant)**: adds
  a `--dimensions` flag or per-dataset config to restrict which rubric
  dimensions the autograder writes; useful for `llm:*` datasets where
  correctness is unreliable.
- **Cadence-scheduler wiring**: the sprint plan named a supervised
  scheduler but the plan explicitly deferred it (env var
  `HITL_CADENCE_SCHEDULE_CRON` was proposed but not wired). The cadence
  CLI is fully invokable manually / via external cron; a supervised
  scheduler would be a small follow-up.
- **Gold-fraction metric + dashboard row**: the sprint plan named
  `mdemg_hitl_gold_fraction{dataset_id}` gauge + a Grafana panel; not
  shipped this sprint (the stall alert covers the primary detection
  need). Follow-up if operator wants panel visibility.
- **Cadence sampler-cap accuracy**: see limitations above.

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (parent trigger, Finding 3)
- `docs/development/hitl-curation-002/sprint_plan.md` (this dir)
- `internal/review/{rubric,scoring,dataset,sink}.go` (HITL-REVIEW-001
  platform surface being extended)
- `internal/api/{handlers_review,contradicted_drafts_dataset,llm_dataset,
  server}.go` (dataset registrations, endpoint wire)
- `internal/alert/rules.go` (ORPHAN-ALERT-001 + NODE-DROP-CALIBRATION-001
  pattern used for the stall rule)
- `internal/llmclient/client.go` (Client construction pattern)
- `internal/cli/{corrections,synergy,root}.go` (CLI patterns:
  cobra.Command registration, resolveEndpoint)
- Live TSDB queries against `review_grades`, `contradicted_correction_drafts`
- Live process inspection: mdemg serve, llama-server
- Post-restart log inspection for rule-count verification
