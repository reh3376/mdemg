# Guidance Training Corpus (JIMINY-RELEVANCE-001)

**Status:** shipped (Epics 1, 2, 4, 5; Epic 3 — human review — lands with
HITL-REVIEW-001). **Sprint:** `docs/development/jiminy-relevance-001/`.

This feature is the **collection + measurement infrastructure** for a 3–6 month
effort to build a trustworthy training corpus that can eventually raise Jiminy
guidance follow-rate toward the operator's >90% goal. It does **not** retrain a
model (that is a future-triggered sprint) and it does **not** change what
guidance gets surfaced (that is the sibling `jiminy-actionability-001`). It makes
sure that, starting now, every guidance interaction is **captured, honestly
labelled, honestly measured, and curatable**.

---

## Why

The Step-1 diagnostic (`docs/development/jiminy-relevance-001/diagnostic_ignored_population.md`)
established three facts on live data (30-day window, 2,561 outcomes):

1. **We could not retrain toward the goal, because the training evidence did not
   exist.** MDEMG persisted *verdicts* everywhere (the `constraint_outcomes`
   telemetry table, Neo4j `GUIDANCE_OUTCOME` edges) but the *evidence* nowhere:
   the `action_summary` the agent reports to `/v1/jiminy/feedback` was used to
   classify the outcome and then **thrown away**. So every
   `(surfaced-guidance → agent-action → did-they-follow)` training triple was
   unrecoverable. Production emitted the perfect training signal on every prompt
   and we discarded it.

2. **~51% of the labels we did keep were heuristic noise.** When the LLM outcome
   classifier was unavailable, the system fell back to a heuristic that
   default-credited everything as `partial_compliance`. A model trained on those
   labels learns the heuristic, not the goal.

3. **">90% follow rate" is partly the wrong target.** Much guidance is
   *correctly* ignored — it is advisory ("Foundational principle: …"), not an
   actionable directive. The meaningful, reachable metric is **"follow rate on
   guidance that *should* have been followed"**, which you cannot even compute
   without the action evidence from (1).

The operator decided to **collect and curate for 3–6 months before any retrain.**
This feature is that infrastructure.

---

## Choices

- **New hypertable, not an extension of `constraint_outcomes`.** `constraint_outcomes`
  is a tight, high-volume telemetry row; the training evidence is bulky text
  (guidance + action snapshots). They have different lifecycles, so the corpus
  lives in its own `guidance_training_rows` table.
- **Evidence columns only — no gold-grading columns.** Human/auto *gold* grades
  are a separate concern, owned by **HITL-REVIEW-001**'s `review_grades` table,
  joined back to this table on `item_id = row_id`. Keeping them apart means this
  table stays append-only production evidence and the gold overlay can evolve
  independently.
- **Forward-only, no backfill.** The historical evidence does not exist to
  recover (it was discarded). The clock starts at deploy.
- **Reuse the existing LLM classifier for relabelling**, not a separate model —
  same production verdicts, less config surface.
- **"Should-follow" = actionable guidance types (`constraint`/`correction`).**
  These are the items the diagnostic found are followed ~2× better and are the
  ones that should be followed; the `pattern`/`learning`/`concept` abstraction
  class is the correctly-ignored-advisory population, excluded from the
  denominator. Certified-gold grounding (preferring human verdicts) is layered in
  when HITL-REVIEW-001 lands.
- **TSDB-sourced curation gets its own path**, not a retrofit of the file-based
  UAITS `paradigm_router`. The corpus artifact it emits can later be fed to the
  router as a file input.

---

## How it works

### 1. Capture (Epic 1) — `guidance_training_rows` (TSDB V0027)
On every `/v1/jiminy/feedback`, for each guidance item with a non-`unknown`
outcome, `jiminy.Service.RecordOutcome` emits one evidence row through a
buffered, async writer (`internal/tsdb/guidance_training_rows_writer.go`, the
V0022 reinforcement-writer pattern — bounded FIFO buffer, drop counter,
`registerWriterStats`). The row carries:

| Column | Meaning |
|---|---|
| `guidance_content` | snapshot of the surfaced guidance text (truncated, UTF-8-safe) |
| `action_summary` | the agent-action text — *the thing previously discarded* |
| `guidance_type` | the actionability class (`constraint`/`correction`/`pattern`/`learning`/`concept`) |
| `source_role_type`, `source_layer` | best-effort bounded Neo4j lookup of the source node (the 172:1 abstraction:constraint signal); `""`/NULL when unresolved |
| `outcome_type`, `similarity`, `classifier_source` | the audited verdict + how it was produced |
| `constraint_code`, `guidance_id`, `session_id`, `space_id` | correlation |
| `row_id` | CUIDv2; HITL-REVIEW-001 `review_grades.item_id` joins here |

The writer never blocks the feedback hot path; the source-node lookup is bounded
by `GUIDANCE_CORPUS_SOURCE_LOOKUP_TIMEOUT_MS`. The table has 365-day retention +
30-day compression (no unbounded hypertable).

### 2. Label quality (Epic 2) — the auto-relabel job
A supervised, periodic job (`internal/api/guidance_audit.go`, `job_name='guidance-audit'`)
samples recent rows whose `classifier_source` is heuristic/blank, re-classifies
them with the LLM classifier, and updates the label **in place** — but only when
the new verdict came from a real mechanism (`llm`/`tier1`/`explicit`); if the
classifier itself falls back to heuristic (LLM down), the row is left for the
next run. It runs once `GUIDANCE_AUDIT_INITIAL_DELAY_SEC` after startup (first
cleanup doesn't wait a full day), then every `GUIDANCE_AUDIT_INTERVAL_HOURS`. The
gauge `mdemg_guidance_corpus_heuristic_label_fraction` makes the 51%→0 progress
observable.

### 3. Honest measurement (Epic 4) — should-follow follow rate
The alert rule `guidance_should_follow_rate_low` and the Grafana panel
"Should-Follow Follow Rate" (on `mdemg-jiminy`) compute the follow rate over the
**actionable** guidance types only (`constraint`/`correction`), excluding
correctly-ignored advisory. The SQL aggregates + `COALESCE`s so an idle window
returns one non-NULL row (no false alerts); it reads the `time` column; it has a
unique `Service` label. Fires a medium alert when the rate drops below
`GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` over `GUIDANCE_SHOULD_FOLLOW_LOOKBACK_HOURS`.
(Certified-gold grounding via the `review_grades` join is a documented follow-up,
added when HITL-REVIEW-001 lands.)

### 4. Curation (Epic 5) — `mdemg data curate-guidance`
Builds a versioned, labeled corpus from `guidance_training_rows`: prefers gold
labels (the `review_grades` LEFT JOIN, when present) over auto-labels, filters by
label quality, distribution-summarizes the production `guidance_type × outcome`
mix, optionally leak-audits against held-out sources, and writes
`corpus.jsonl` + `manifest.json`. The manifest's `gold_fraction` and distribution
are what the retrain trigger reads.

### How the three sibling sprints fit together
- **This sprint (jiminy-relevance-001)** changes what we **store and measure**.
- **HITL-REVIEW-001** is the human-in-the-loop review + live-reinforcement
  platform; the guidance corpus is its first reviewable dataset (this sprint's
  Epic 3). Human grading produces the certified-gold labels this corpus prefers.
- **jiminy-actionability-001** changes what we **surface** (biasing guidance
  toward actionable directives) — the near-term lever to lift follow-rate without
  waiting for the corpus.

### The retrain FUTURE-TRIGGER (not built here)
When the corpus reaches a trustworthy, distribution-matched threshold per
implicated call site — default target `GUIDANCE_CORPUS_RETRAIN_MIN_GOLD_ROWS`
(proposed 2,000/task) over a ≥3-month window, leak-audit clean — open the retrain
sprint: `ft-guidance-001` (guidance synthesis, for the not-actionable failure
mode) and advance **FT-CLASSIFY-002** (constraint classification, for the
wrong-constraint-surfaced mode). This is the CLAUDE.md "recursive-retraining loop
(FT Phases 6/7/9 — NOT STARTED)". This sprint only emits the signal that makes
the threshold observable.

---

## How to use

### Watch the corpus fill and the labels clean up
- Grafana `mdemg-jiminy`: the **"Should-Follow Follow Rate"** panel (green ≥0.9 =
  the goal, red <0.5 = the alert floor).
- Gauges: `mdemg_guidance_corpus_rows_enqueued_total`,
  `mdemg_guidance_corpus_heuristic_label_fraction` (should trend to ~0 as the
  auto-relabel job runs), `mdemg_tsdb_writer_*{writer="guidance_training_rows"}`.

### Curate a corpus snapshot
```bash
# Default: real-labelled rows from mdemg-dev over the last 180 days.
mdemg data curate-guidance --version v1

# Human-graded gold only (once HITL-REVIEW-001 grading has accumulated):
mdemg data curate-guidance --version v1-gold --min-label-quality gold

# With a leak audit against a held-out eval:
mdemg data curate-guidance --version v1 \
    --against training_data/eval/valid_clean.jsonl
```
Outputs land in `training_data/guidance_corpus/<version>/` (`corpus.jsonl` +
`manifest.json`). Inspect `manifest.json` → `gold_fraction`,
`label_source_breakdown`, `guidance_type_x_outcome`.

> **Requires `psycopg2`** (same as the other TSDB scripts, e.g.
> `jiminy_effectiveness_report.py`): `pip install psycopg2-binary`.

### Configuration
| Env var | Default | Purpose |
|---|---|---|
| `GUIDANCE_CORPUS_ENABLED` | `true` | persist evidence at feedback time |
| `GUIDANCE_CORPUS_WRITER_FLUSH_INTERVAL_SEC` | `30` (floor 5) | writer flush cadence |
| `GUIDANCE_CORPUS_WRITER_BUFFER_SIZE` | `1000` (0=unbounded) | FIFO buffer cap |
| `GUIDANCE_CORPUS_MAX_CONTENT_BYTES` | `8192` (floor 256) | snapshot truncation |
| `GUIDANCE_CORPUS_SOURCE_LOOKUP_TIMEOUT_MS` | `300` (0=disable) | bounded source role/layer lookup |
| `GUIDANCE_AUDIT_ENABLED` | `true` | run the auto-relabel job |
| `GUIDANCE_AUDIT_INTERVAL_HOURS` | `24` (floor 1) | relabel cadence |
| `GUIDANCE_AUDIT_SAMPLE_SIZE` | `50` (floor 1) | rows relabelled per run |
| `GUIDANCE_AUDIT_INITIAL_DELAY_SEC` | `60` (0=skip) | delay before the first relabel run |
| `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` | `0.5` (0=disable) | should-follow alert floor |
| `GUIDANCE_SHOULD_FOLLOW_LOOKBACK_HOURS` | `168` (floor 1) | should-follow window |

### Rollback
`GUIDANCE_CORPUS_ENABLED=false` stops capture; `GUIDANCE_AUDIT_ENABLED=false`
stops relabelling; `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR=0` disables the alert. The
V0027 table is additive — disabling simply stops accumulation; already-captured
rows are inert.
