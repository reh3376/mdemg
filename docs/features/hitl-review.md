# Human-in-the-Loop Review + Live Reinforcement (HITL-REVIEW-001)

**Status:** shipped. **Sprint:** `docs/development/hitl-review-001/`. **Surface:**
the **Review** tab at `http://localhost:9999/ui/` + the `/v1/review/*` API.

A general-purpose, dataset-agnostic tool for a human SME to **review** curated
datasets against a reproducible rubric, **certify** them (gold), optionally
**reinforce the LIVE cognitive substrate** (auditably + reversibly), and
**capture corrective examples** that become training data. Built once, against a
generic interface; the guidance corpus + all 16 LLM call sites are its first
tenants.

---

## Why

Two gaps the JIMINY-RELEVANCE-001 diagnostic exposed:

1. **Every curated dataset needs human certification against a standard.** ~51%
   of MDEMG's auto-labels were heuristic noise. An auto-classifier can't certify
   itself — only a human-graded gold set, scored on a reproducible anchored
   rubric, can be the trustworthy anchor. And this need isn't unique to one
   dataset (guidance, SFT, DPO, every LLM call site all have it), so the grading
   surface should be built **once, generically**.
2. **A human judgment should act on the live system *now*, not wait months for a
   retrain.** MDEMG already has the machinery to act on an outcome immediately —
   the trust scorer (EMA), node confidence, the `GUIDANCE_OUTCOME` edge. A
   certified human "this guidance was genuinely followed/ignored" is a
   *higher-quality* signal than the production auto-classifier; feeding it into
   the live substrate — **reversibly** — is the headline capability.

---

## Choices

- **Native in the `:9999` server, not a 6th container.** The live-reinforcement
  write access (trust scorer, node confidence, Neo4j/TSDB pools) already lives
  there; a separate service would re-establish all of it. (Operator-decided.)
- **Generic `ReviewableDataset` + `ReinforcementSink` interfaces.** The platform
  (sampling, endpoints, persistence) is written once; a new dataset is an
  interface implementation, not a refactor. Proven by registering 18 datasets
  (guidance + 16 LLM call sites + `contradicted_drafts` from
  JIMINY-CONTRADICTED-BRIDGE-001) with no platform changes.
- **A dedicated `review_grades` table, not gold columns on each source.** Source
  datasets vary; the audit/reversal record is uniform. `review_grades.item_id`
  joins back to a source row (e.g. `guidance_training_rows.row_id`, or an
  `llm_interactions.trace_id`).
- **Reinforcement is opt-in per submission, reversible by design.** Because it
  mutates the protected `mdemg-dev` substrate, `REVIEW_REINFORCE_DEFAULT=false`,
  every apply captures its prior state, and the binding test is a *verified
  reversal*. Gold-only datasets use a `NoopSink`.
- **Functional dev-tool UI, not polished.** Vanilla JS in the existing `:9999/ui`
  shell; no framework/build step. (Operator-decided.)
- **Capture corrective guidance, not just a grade.** Surfaced during live SME
  testing: the highest-value signal is *"what would have been better guidance"* —
  an SME-authored gold example. Stored in `review_grades.suggested_guidance`.

---

## How it works

### The generic core
- **`ReviewableDataset`** (`internal/review/dataset.go`): `ID`, `DisplayName`,
  `FetchCandidates`, `FetchItem`, `Rubric`, `Sink` (+ optional `Description` for
  the UI ⓘ panel). A `Registry` holds them; datasets register at server boot.
- **`Rubric`** (`internal/review/rubric.go` + `scoring.go`): versioned, per-paradigm
  — **Rated** (per-dimension 0–4 with written anchors → normalized `mean/4`) or
  **Ranked** (DPO chosen/rejected). `rubric_version` (`gr-v1`) is pinned on every
  grade. Anchors live in `docs/development/hitl-review-001/rubric_v1.md`.
- **Sampler** (`internal/review/sampler.go`): active (prefer uncertain /
  signal-disagreeing items) + stratified (round-robin across strata), deterministic
  by seed.
- **`review_grades`** (TSDB V0028 + V0029): the gold + reversal-audit row —
  `gold_score`, `gold_dimensions`, `grader_id`, `rubric_version`,
  `reinforcement_applied`/`reinforcement_detail` (the reversal payload),
  `reversed`/`reverses_grade_id`, `suggested_guidance`. Buffered writer + sync
  point reads (`LatestGradeForItem`/`GradeByID`) for idempotency/reversal.

### The endpoints (`/v1/review/*`, admin-gated)
- `GET /datasets` — registered datasets (+ description, rubric, candidate count).
- `GET /next?dataset_id=&space_id=` — the next sampled un-graded item + its rubric.
- `POST /grade` — validate → score → (dry-run? sink **preview**, no write) →
  idempotency (409 on a non-reversed grade at the current rubric_version) →
  optional `Sink.Apply` (reinforce) → persist + audit.
- `POST /reverse {grade_id}` — `Sink.Reverse` (restore prior state) → reversal row
  + mark the original reversed.

### Reinforcement sinks
- **`ReinforcementSink`** = `SinkID` / `Preview` / `Apply` / `Reverse`. Apply MUST
  be idempotent (enforced at the endpoint), capture everything needed to reverse,
  and respect protected-space rules (add/update only).
- **`GuidanceSink`** (the live one): from a certified guidance grade's
  `outcome_label_correctness` it derives the corrected outcome and applies it to
  **live J17 trust** (EMA) + **node confidence** — capturing prior trust + the
  confidence delta so Reverse restores trust exactly and applies the inverse
  delta. (The `GUIDANCE_OUTCOME` edge sink is a disclosed follow-up,
  `hitl-review-002`.)
- **`NoopSink`** (gold-only): the 16 LLM call-site datasets + any dataset whose
  grades are training-data-only.

### The registered datasets
- **Guidance Corpus** — `guidance_training_rows`; live-reinforcing + corrective
  capture; the item view surfaces the auto-verdict (+ confidence + how-labeled),
  guidance type, and provenance so an SME can judge it.
- **16 LLM call sites** (`internal/api/llm_dataset.go`) — every MDEMG LLM call
  site (`ape.reflect`, `consulting.classify`, `jiminy.synthesize`/`evaluate_llm`/
  `codegen`, `hidden.*`, `retrieval.*`, `guardrail.evaluate`, …) reads
  `llm_interactions` (system/user prompt + response), graded gold-only on an
  LLM-output rubric (correctness / format_validity / helpfulness). Each carries a
  description explaining its function + what the SME is judging.

---

## How to use

### Grade in the browser
`http://localhost:9999/ui/` → **Review** → pick a dataset (the **ⓘ** panel
explains its purpose) → an item renders with its rubric → score each dimension →
optionally write **Suggested better guidance** → **Submit** (or **Preview
(dry-run)** first). For the Guidance Corpus you may check **reinforce** to move
the live substrate; **Reverse last grade** undoes it.

### Configuration
| Env var | Default | Purpose |
|---|---|---|
| `REVIEW_ENABLED` | `true` | enable `/v1/review/*` + the UI tab |
| `REVIEW_WRITER_FLUSH_INTERVAL_SEC` | `15` (floor 5) | `review_grades` flush cadence |
| `REVIEW_WRITER_BUFFER_SIZE` | `500` (0=unbounded) | writer buffer cap |
| `REVIEW_RUBRIC_VERSION` | `gr-v1` | current rubric version (certified-current) |
| `REVIEW_SAMPLE_SIZE` | `200` | items the sampler selects |
| `REVIEW_ACTIVE_UNCERTAINTY_BAND` | `0.4` | active-sampling band around 0.5 |
| `REVIEW_SAMPLE_SEED` | `0` (time) | non-zero = deterministic sampling |
| `REVIEW_REINFORCE_DEFAULT` | `false` | per-grade reinforce default (safety) |
| `REVIEW_GUIDANCE_SINK_ENABLED` | `true` | the guidance live-reinforcement sink |
| `REVIEW_GUIDANCE_CONFIDENCE_NUDGE` | `0.05` | node-confidence delta on apply |
| `REVIEW_LLM_DATASETS_ENABLED` | `true` | register the 16 LLM call-site datasets |

### Add a new dataset
Implement `review.ReviewableDataset` (and a `ReinforcementSink`, or `NoopSink`
for gold-only), register it at server construction. The endpoints, sampler, UI,
and persistence work for it with no further changes.

### Rollback
`REVIEW_ENABLED=false` → 503s the whole surface. `REVIEW_GUIDANCE_SINK_ENABLED=false`
→ gold-only (no live reinforcement). Any individual reinforcement →
`POST /v1/review/reverse`. The V0028/V0029 migrations are additive.

---

## Follow-ups
- **`hitl-review-002`**: the `GUIDANCE_OUTCOME` edge sink; the SFT/DPO sinks
  (curation accept/reject + DPO preference + retrain-trigger nudge);
  multi-grader consensus.
- The corrective `suggested_guidance` corpus feeds **`jiminy-actionability-001`**
  (better guidance synthesis) and the recursive-retraining loop.

## Dataset: contradicted_drafts (Sprint JIMINY-CONTRADICTED-BRIDGE-001, 2026-07-20)

**Purpose.** When Jiminy classifies an agent action as `contradicted` (the highest-signal lesson signal in the guidance loop — "the action directly opposes the guidance intent"), the bridge auto-generates a **correction draft** and surfaces it in HITL for operator review. On approve, the sink hands the draft's `Incorrect`/`Correct` pair to `conversation.Service.Correct` — an L0 correction observation is created, and the next consolidation cycle promotes it to an L1 `role_type='correction'` node via `CreateCorrectionNodes` (JIMINY-CORRECTION-PRODUCER-001).

**Rubric (`cd-v1`).** Two 0-4 dimensions:
- `durable_rule`: session-noise (0) / one-off (1) / unclear (2) / probable rule (3) / permanent rule (4). **The sink's decision:** ≥3 approve, ≤1 dismiss, 2 defer (draft stays pending).
- `phrasing_quality`: needs full rewrite (0) → publication-ready (4). Advisory signal for a later synthesis-tuning sprint; does NOT gate the approve/dismiss action.

**How to use.**
1. `GET /v1/review/datasets?space_id=<id>` — verify `contradicted_drafts` is registered (candidate_count > 0 means pending review awaits).
2. `GET /v1/review/next?dataset_id=contradicted_drafts&space_id=<id>` — pull the sampled item + rubric.
3. `POST /v1/review/grade` with `{"dataset_id":"contradicted_drafts","item_id":"<draft_id>","space_id":"<id>","grader_id":"you","dimensions":{"durable_rule":4,"phrasing_quality":3},"reinforce":true}`.
4. On approve: L0 obs created + draft marked `approved` with `applied_obs_id` captured. Next consolidation promotes to L1.
5. On dismiss: draft marked `dismissed`; no substrate mutation.

**Sink semantics.**
- `Preview` (dry-run): describes the exact `conversation.Correct` call that would fire, or the status transition for dismiss/defer. No mutation.
- `Apply`: idempotent per grade_id (endpoint-layer enforces). Mutates the substrate ONLY on approve.
- `Reverse`: resets draft to `pending`. **Deliberately leaves the L0 obs in place** — a reversal is a re-review invitation, not a substrate rollback. Full undo requires `mdemg concepts tombstone` on the L0 obs.

**Config.**

| Env | Default | Purpose |
|---|---|---|
| `JIMINY_CONTRADICTED_BRIDGE_ENABLED` | `true` (post-E5 flip) | Emit draft rows on `OutcomeContradicted` |
| `JIMINY_CONTRADICTED_BRIDGE_WRITER_FLUSH_INTERVAL_SEC` | 30 (floor 5) | Buffered writer flush cadence |
| `JIMINY_CONTRADICTED_BRIDGE_WRITER_BUFFER_SIZE` | 1000 (0=unlimited) | Max buffered rows before FIFO eviction |
| `JIMINY_CONTRADICTED_BRIDGE_MAX_CONTENT_LEN` | 400 | Cap for `draft_incorrect`/`draft_correct` |
| `REVIEW_CONTRADICTED_DATASET_ENABLED` | `true` | Register the HITL dataset (independent of the bridge flag — drafts already in TSDB remain reviewable) |

**Applied identifiers (Sprint CONTRADICTED-BRIDGE-APPLIED-NODE-ID-001, 2026-07-20).** On approve, the sink now records BOTH the L0 observation ID and the MemoryNode ID that carries it: draft rows expose `applied_obs_id` (`obs_id` on the observation record) AND `applied_node_id` (Neo4j `MemoryNode.node_id`). These are distinct CUIDv2s. Pre-sprint code that only had `applied_obs_id` had to re-query Neo4j to walk the *node* (verify L1 promotion, join to graph telemetry); `applied_node_id` is now a direct pointer. HITL response Meta and the underlying V0031-migrated hypertable both carry both fields. Historical rows approved before this sprint keep `applied_node_id=NULL`.

**Live evidence (E5, 2026-07-20).** End-to-end verified on `mdemg-dev`: real contradicted verdict → draft `c8jvgnmkl8zlmr4m58nl7rj3` → HITL surfaced → approve grade → real L0 obs `po2zahas8mh10ahwe0iimmoz` → consolidation → real L1 correction `ymehdkihmj2yiu7t3bywsgxc` (count 32 → 33). See `docs/development/jiminy-contradicted-bridge-001/live_verification.md`.
