# Sprint Plan — HITL-REVIEW-001: A General-Purpose Human-in-the-Loop Dataset Review + Live-Reinforcement Tool

## 1. Header & Metadata

- **Sprint ID:** HITL-REVIEW-001
- **Line dir:** `docs/development/hitl-review-001/`
- **Date:** 2026-06-23
- **Branch:** `reh3376_dev01` (PR to `main`; never commit to `main`)
- **Target version:** **v0.11.0** (minor bump — additive feature: new hypertable + writer + `/v1/review/*` surface + UI tab + reinforcement-sink registry). **Recommendation: share v0.11.0 with JIMINY-RELEVANCE-001.** Both ship in the same collection-infrastructure arc, both are additive, and the two CHANGELOG entries read as one capability ("we now persist the evidence AND can human-certify it"). If HITL-REVIEW-001 lands *after* a v0.11.0 cut, take **v0.12.0**. Decided at execution from the actual merge order (current released tag v0.10.1, CHANGELOG `[Unreleased]`).
- **TSDB schema:** **26 → 27** (one additive migration `027_review_grades.sql`; `TSDBRequiredSchemaVersion` bumped 26 → 27 in `internal/config/config.go` — the CI Build gate fails otherwise). ⚠️ **Coordination with JIMINY-RELEVANCE-001:** that sprint *also* claims `027` for `guidance_training_rows`. Whichever merges first takes `027`; the second renumbers to `028` and bumps to `28` in the same PR. This is a mechanical rebase, called out here so it is not discovered at merge time.
- **Effort:** ~4–5d (the generic registry + sink interface + reversible-audit write path + UI tab + Playwright e2e make this wider than a single-dataset grader).
- **Risk:** medium-high — the reinforcement sink **mutates the live, protected `mdemg-dev` space** (trust scorer + `GUIDANCE_OUTCOME` edge + node confidence). A wrong apply silently corrupts the cognitive substrate, so the binding gates are **idempotency, an observable audit row, and a verified reversal** — all on the live system.
- **Lineage:** generalizes **JIMINY-RELEVANCE-001 Epic 3** (human-in-the-loop gold grading of the guidance corpus) into a **standalone, dataset-agnostic platform**. Where JIMINY-RELEVANCE-001 envisioned a guidance-specific grading surface, this sprint builds the reusable substrate (registry + rubric + sampler + endpoints + reversible reinforcement) and registers the guidance corpus as its **first** reviewable dataset. JIMINY-RELEVANCE-001 Epic 3 then becomes "register the guidance corpus against the HITL-REVIEW platform" instead of building a bespoke surface. See §4 for the precise cross-sprint ordering.

## 2. Problem Statement

The JIMINY-RELEVANCE-001 diagnostic (`diagnostic_ignored_population.md`, live `mdemg-dev`, 2,561 outcome rows) established that **~51% of the labels MDEMG holds are non-LLM heuristic defaults — noise** (Finding 4), and that a retrain on them "would learn the heuristic, not the goal." The fix it scoped (Epic 3) was a human-certified, anchored-rubric grading surface that turns a sampled subset into trustworthy gold.

But that requirement is **not unique to the guidance corpus.** Every curated dataset MDEMG produces — SFT rows, DPO preference pairs, RAFT, curriculum (the four paradigms the Python UAITS `paradigm_router` emits, invoked by `internal/cli/data_curate.go`) — has the same need: a developer must be able to **review items against a reproducible standard, certify them, and have that judgment count.** Building a bespoke grader per dataset is the wrong shape; the diagnostic's grading surface should be built **once**, generically.

There is a second, larger gap the diagnostic implies but JIMINY-RELEVANCE-001 leaves to "the eventual retrain": **a human grade today does nothing to the live system.** It would sit as a `gold_*` column waiting months for a retrain. Yet MDEMG already has the machinery to act on a judgment **immediately** — the trust scorer (`TrustScorer.RecordOutcome`, now an EMA per JIMINY-EFFECTIVENESS-001), the `GUIDANCE_OUTCOME` edge writer, and node-confidence reinforcement, all reachable through the existing `/v1/jiminy/feedback` path (`handleJiminyFeedback` → `RecordOutcome`). A certified human judgment "this guidance was genuinely followed / genuinely ignored" is a **higher-quality outcome signal than the production auto-classifier** — and feeding it into the live cognitive substrate is exactly the kind of human-corrected reinforcement MDEMG's purpose ("help developers make better decisions") calls for.

**This sprint builds the general-purpose Human-in-the-Loop review tool: a dataset-agnostic review + grading platform, native inside the `:9999` server, whose certified grades (a) persist as auditable gold and (b) optionally reinforce the LIVE cognitive substrate — auditably and reversibly.** The headline capability is the live-reinforcement loop. The guidance corpus is the first registered dataset; SFT/DPO/RAFT/curriculum follow via the same interface.

## 3. Scope & Constraints

**In scope (sequential epics — do NOT parallelize):**

1. **Dataset registry + grade persistence core (Epic 1)** — a Go `ReviewableDataset` interface + a registry; the `review_grades` TSDB hypertable + buffered writer + migration `027` + the schema-version bump.
2. **Normalized anchored rubric + scoring engine (Epic 2)** — versioned 0–4-per-dimension anchored rubric → 0–1 normalized score, with per-paradigm rubric *shapes* (rated for SFT/guidance; **ranked** chosen/rejected for DPO).
3. **Active + stratified sampling (Epic 3)** — prioritize the most informative items (low auto-confidence / signal disagreement), stratified across natural strata. Config-driven.
4. **`/v1/review/*` endpoints (Epic 4)** — list datasets, fetch next sampled item, submit grade, reverse a grade; gated by `AUTH_API_KEYS`.
5. **Reinforcement sinks (Epic 5)** — a generic `ReinforcementSink` interface + the **first concrete sink** (guidance corpus → reuse the jiminy feedback reinforcement: trust + `GUIDANCE_OUTCOME` + node confidence), **auditable + reversible + idempotent + dry-run**. SFT/DPO sinks scoped lightly as next consumers.
6. **Review UI (Epic 6)** — a "Review" tab in `internal/api/ui/` (vanilla JS, functional-not-flashy) + Playwright e2e.
7. **Documentation (Epic 7 — final, never cut)** — `docs/features/hitl-review.md`, CHANGELOG, `post.md` stub, CLAUDE.md Architecture-Notes entry.

**Out of scope (documented, with forward references):**

- **A polished / "designed" UI.** Explicitly de-scoped. The Review tab is a **functional developer tool** in the existing hand-rolled vanilla-JS style (`internal/api/ui/tabs/`), navigable and correct, not visually refined. No framework, no build step, no design system.
- **A 6th Docker container.** Explicitly rejected — the platform is **native inside the existing `:9999` server**, because the live-reinforcement write access (trust scorer, `GUIDANCE_OUTCOME`, node confidence, Neo4j/TSDB pools) already lives there. A separate service would have to re-establish all of it.
- **The retrain itself / the recursive-retraining loop.** That is downstream (CLAUDE.md "recursive-retraining loop, FT Phases 6/7/9 — NOT STARTED" + FT-CLASSIFY-002). This tool produces certified gold *for* that loop; it does not run it.
- **Full SFT/DPO/RAFT/curriculum sinks.** Only the guidance sink is *implemented* this sprint. The SFT sink (curation accept/reject) and DPO sink (chosen/rejected preference + a recursive-retrain-trigger nudge) are scoped lightly (signatures + a stub registration path) and forward-referenced as the next consumers — the platform is built so they are an interface implementation, not a refactor.
- **Inter-rater calibration across multiple human graders / consensus voting.** Single-grader-per-item this sprint; `grader_id` is recorded so multi-grader agreement is a later additive sprint.

**Constraints:**

- **No-hardcoding:** every new value is an env var / config field with a sensible default (concrete names in §5).
- **CUIDv2** for every new identifier (`grade_id`, etc.) via `github.com/nrednav/cuid2`; **never UUID**.
- **TSDB migration ⇒ bump `TSDBRequiredSchemaVersion`** (26 → 27) in `internal/config/config.go` (CI validator fails Build otherwise — and see the `027`/`028` coordination note in §1).
- **Protected-space rules respected:** `mdemg-dev` has hardcoded deletion protection; reinforcement *adds/updates* — it must never delete protected nodes, and the reversal path must respect the same protection (reversal *restores prior state*, it does not delete).
- **Buffered writer pattern** (V0019 / `constraint_outcomes_writer.go`): `review_grades` is low-volume (human-paced) — a buffered CopyFrom writer that **self-registers via `registerWriterStats`** so it joins the `mdemg_tsdb_writer_*` flush-stats plane (TSDB-CONSUME-001).
- **New hypertable MUST get retention/compression** in the migration (V0025 policy — no new unbounded hypertable). Audit/forensic retention (180d) since these are reversal records.
- **Reinforcement on the hot path is human-paced**, so synchronous reinforcement (not buffered) is acceptable *for the sink call*, but the audit row write goes through the buffered writer; the apply itself must be idempotent and bounded.
- **Sequential epics**, gates between each.
- **Live Tier-3 required** (real binary + real Neo4j + TSDB + llama-server, observable output) + **Playwright e2e** for the UI (house rule: all UI/UX dev includes Playwright e2e).

## 4. Dependencies & Pre-Conditions

- **Code touch-points:**
  - `internal/jiminy/service.go` (`RecordOutcome` ~1439; `TrustScorer.RecordOutcome`; the `GUIDANCE_OUTCOME` outcome writers) — the guidance `ReinforcementSink` reuses this path.
  - `internal/api/handlers_jiminy.go` (`handleJiminyFeedback` ~214 — the existing reinforcement entrypoint the guidance sink mirrors).
  - `internal/jiminy/types.go` (`GuidanceFeedbackRequest`, `GuidanceItem`).
  - `internal/tsdb/constraint_outcomes_writer.go` (the buffered + CopyFrom + `registerWriterStats` writer template); `internal/tsdb/migrations/` (latest `026_…`; add `027_review_grades.sql`).
  - `internal/config/config.go` (`TSDBRequiredSchemaVersion` = 26; the `FromEnv()` config-block pattern; the config scanner the no-hardcoding test asserts).
  - `internal/api/server.go` (writer construction + wiring the registry + a `SetReviewWriter`-style hook; route registration; `AUTH_API_KEYS` gating).
  - `internal/api/ui/` — `index.html` (tab nav), `main.js`, `state.js`, `api.js`, `tabs/*.js` (add `tabs/review.js`).
  - `internal/cli/data_curate.go` + `training/paradigm_router.py` (the SFT/DPO/RAFT/curriculum curation surface the SFT/DPO sinks will eventually call accept/reject into — referenced, not implemented this sprint).
- **Pre-conditions:** `JIMINY_ENABLED=true` on the live stack (the guidance sink needs the jiminy service); TSDB reachable at schema 26; Neo4j up (trust scorer + `GUIDANCE_OUTCOME` writes); `AUTH_API_KEYS` set (the `/v1/review/*` surface is admin-gated like `/v1/admin/breakers`).
- **Cross-sprint ordering (state explicitly):**
  1. **JIMINY-RELEVANCE-001 Epic 1** (persist guidance evidence — the `guidance_training_rows` table) **must exist before the guidance dataset has reviewable items.** HITL-REVIEW-001's guidance `FetchCandidates` reads from `guidance_training_rows`. Until that table is populated, the guidance dataset registers but returns an empty candidate queue (the platform itself is testable against a **synthetic/stub dataset** — see §6 — so HITL-REVIEW-001 does not *block* on JIMINY-RELEVANCE-001 Epic 1 for its own gates).
  2. **HITL-REVIEW-001 must ship before JIMINY-RELEVANCE-001 Epic 3 can register against it** — Epic 3 becomes "implement `ReviewableDataset` for the guidance corpus and register it," which requires this platform to exist.
  - **Net ordering:** JIMINY-RELEVANCE-001 Epic 1 → HITL-REVIEW-001 (full) → JIMINY-RELEVANCE-001 Epic 3. HITL-REVIEW-001's own live gate uses the guidance dataset if `guidance_training_rows` is populated, else the stub dataset; either way the **reinforcement** live gate uses a real guidance item (a real `constraint_outcomes`/`GUIDANCE_OUTCOME`-eligible row exists today independent of JIMINY-RELEVANCE-001 Epic 1).

## 5. Implementation Plan (sequential epics + gates)

**Epic 0 — Plan (this document).** Gate: plan written, v1.0 format-conformant, scope reconciled with the diagnostic + JIMINY-RELEVANCE-001, the `027`/`028` schema coordination noted.

---

**Epic 1 — Dataset registry + grade persistence core.**

The reusable substrate: the registry interface, the registry itself, and the audit/grade hypertable.

- **The `ReviewableDataset` interface** (`internal/review/dataset.go`), concretely:

  ```go
  // ReviewableDataset is implemented once per dataset type (guidance, sft, dpo, …).
  // The platform code (sampling, endpoints, persistence) is written once against it.
  type ReviewableDataset interface {
      // ID is the stable dataset key (e.g. "guidance", "sft", "dpo").
      ID() string
      // DisplayName for the UI dataset picker.
      DisplayName() string

      // FetchCandidates returns up to limit un-graded (or re-gradable) review items
      // for this dataset, honoring the active/stratified sampling hints. The platform
      // calls this; the dataset decides where items come from (TSDB table, file, etc.).
      FetchCandidates(ctx context.Context, q CandidateQuery) ([]ReviewItem, error)

      // Rubric returns the versioned rubric SHAPE for this dataset (rated vs ranked,
      // its dimensions + anchors). Per-paradigm — DPO returns a ranked rubric.
      Rubric() Rubric

      // Sink returns the ReinforcementSink that defines what "apply this grade to the
      // live system" MEANS for this dataset. May return a no-op sink (gold-only).
      Sink() ReinforcementSink
  }

  // ReviewItem is one thing to be reviewed.
  type ReviewItem struct {
      ItemID      string            // stable per-item id within the dataset
      Content     string            // the primary text under review (guidance text, SFT completion, …)
      Context     string            // surrounding context shown to the grader (query, action_summary, …)
      AutoLabel   string            // the current auto-classifier verdict (e.g. "ignored")
      AutoScore   float64           // auto-classifier confidence [0,1]
      Signals     map[string]float64// secondary signals for active sampling (e.g. "similarity")
      Stratum     string            // the natural stratum this item belongs to (for stratified sampling)
      // For ranked paradigms (DPO): Alternatives holds the chosen/rejected pair.
      Alternatives []ReviewAlternative
      Meta        map[string]string // dataset-specific passthrough (constraint_code, guidance_id, node ids …)
  }

  type ReviewAlternative struct {
      AltID   string
      Content string
  }

  // CandidateQuery carries the sampling hints from Epic 3.
  type CandidateQuery struct {
      SpaceID         string
      Limit           int
      Strata          []string  // dimensions to stratify across
      UncertaintyBand float64   // prefer items whose AutoScore is within this band of 0.5
      ExcludeGraded   bool      // skip items already graded at the current rubric_version
  }
  ```

- **The registry** (`internal/review/registry.go`): `Register(ReviewableDataset)` + `Get(id)` + `List()`. Datasets register at server construction (guidance sink wired in Epic 5; a `stubDataset` registered behind `REVIEW_STUB_DATASET_ENABLED` for self-testing).

- **Migration `027_review_grades.sql`** — the `review_grades` hypertable (keyed conceptually by `(dataset_id, item_id)`):

  | column | type | notes |
  |---|---|---|
  | `time` | `timestamptz NOT NULL` | hypertable time dim (= `graded_at`) |
  | `grade_id` | `text NOT NULL` | **CUIDv2** |
  | `dataset_id` | `text NOT NULL` | e.g. `guidance` |
  | `item_id` | `text NOT NULL` | stable item key within the dataset |
  | `space_id` | `text NOT NULL` | |
  | `gold_score` | `double precision NOT NULL` | normalized 0–1 |
  | `gold_dimensions` | `jsonb NOT NULL` | per-dimension 0–4 scores + anchors hit; for DPO, the ranking |
  | `grader_id` | `text NOT NULL` | human handle or `auto:<model>` |
  | `rubric_version` | `text NOT NULL` | pinned per grade (e.g. `gr-v1`) |
  | `graded_at` | `timestamptz NOT NULL` | |
  | `reinforcement_applied` | `boolean NOT NULL DEFAULT false` | did this grade write to the live system |
  | `reinforcement_detail` | `jsonb` | **exactly what was applied** — the reversal payload (prior trust, prior confidence, edge id, sink id, applied verb) |
  | `reversed` | `boolean NOT NULL DEFAULT false` | reversal flag (a reversal writes a *new* row referencing this one AND sets this) |
  | `reverses_grade_id` | `text` | non-null on a reversal row → the grade it undoes |
  | `instance_id` | `text` | |

  Plus: `create_hypertable('review_grades','time')`; index on `(dataset_id, item_id, time DESC)` (idempotency lookup) + `(grade_id)`; **retention 180d + compression** (V0025 contract — audit/forensic class); `UPDATE tsdb_schema_meta SET version='27'`.

- **`internal/tsdb/review_grades_writer.go`** — buffered `ReviewGradeRow` + `CopyFrom`, `registerWriterStats("review_grades", …)`, config-driven flush + buffer, FIFO eviction + drop counter. Plus a **synchronous point read** `LatestGradeForItem(dataset_id, item_id)` (NOT buffered — idempotency + reversal need read-your-writes; flush-then-read or a direct pooled query).

- **Config block** (`internal/config/config.go`), no-hardcoding: `REVIEW_ENABLED` (default `true`), `REVIEW_WRITER_FLUSH_INTERVAL_SEC` (default `15`, floor `5`), `REVIEW_WRITER_BUFFER_SIZE` (default `500`, `0`=unbounded), `REVIEW_MAX_CONTENT_BYTES` (default `16384` — truncate stored snapshots), `REVIEW_STUB_DATASET_ENABLED` (default `false` — a synthetic dataset for self-test/dev). **Bump `TSDBRequiredSchemaVersion` 26 → 27.**

- **Gate G1:** `go build ./...` green; registry + writer + config unit tests pass; `registerWriterStats` joins the flush plane; migration applies against a TimescaleDB container; schema-version CI check passes locally.

---

**Epic 2 — Normalized anchored rubric + scoring engine.**

- **`Rubric`** (`internal/review/rubric.go`), per-paradigm flexible:

  ```go
  type RubricKind int // RubricRated | RubricRanked

  type Rubric struct {
      Version    string          // pinned on every grade (e.g. "gr-v1")
      Kind       RubricKind      // Rated (SFT/guidance) or Ranked (DPO)
      Dimensions []RubricDimension // for Rated
      // Ranked datasets ignore Dimensions; the grade is a chosen/rejected ordering.
  }

  type RubricDimension struct {
      Key     string            // e.g. "relevance", "actionability", "outcome_label_correctness"
      Anchors [5]string         // WRITTEN anchor description for levels 0,1,2,3,4 — a reproducible standard
  }

  // Score converts a submitted grade into the normalized 0–1 gold_score + the
  // gold_dimensions jsonb. For Rated: mean(dimension scores)/4. For Ranked: a
  // 1.0/0.0 chosen/rejected encoding (with a confidence the grader sets).
  func (r Rubric) Score(g GradeSubmission) (gold float64, dims map[string]any, err error)
  ```

  - **Rated** (guidance, SFT): each dimension scored **0–4**, each level carrying a **written anchor** (e.g. for `actionability`: `0` = "advisory prose, no action implied"; `4` = "names a specific, executable action"). Normalized: `mean(dims)/4 ∈ [0,1]`.
  - **Ranked** (DPO): the grader picks chosen vs rejected among `ReviewItem.Alternatives` (+ an optional 0–4 confidence). `gold_score` encodes the ordering; `gold_dimensions` records the chosen/rejected alt ids.
  - **Guidance specifically** is *rated + outcome-label-corrected*: dimensions `relevance`, `actionability`, **`outcome_label_correctness`** (0–4: "the auto verdict was exactly wrong" … "the auto verdict was exactly right") — the last dimension is what the reinforcement sink reads to decide the corrected outcome.

- **Rubric text is versioned in-repo**: `docs/development/hitl-review-001/rubric_v1.md` (the written anchors for each registered dataset's dimensions). `rubric_version` (`gr-v1`) pinned on every grade; a grade against a stale `rubric_version` is flagged (not certified-current).
- **Scoring engine** validates the submission shape against the dataset's rubric kind (a Ranked submission to a Rated rubric is a 400), computes `gold_score`, emits `gold_dimensions`.
- **Config:** `REVIEW_RUBRIC_VERSION` (default `gr-v1`) — the *current* rubric version; grades not at this version are not "certified-current."
- **Gate G2:** rubric→normalized-score math unit-tested (rated mean/4 bounds + rounding; ranked encoding; mismatch rejection); the versioned `rubric_v1.md` present; anchors are non-empty for every dimension (a test asserts no blank anchor).

---

**Epic 3 — Active + stratified sampling.**

- **Sampler** (`internal/review/sampler.go`) consumes `[]ReviewItem` from a dataset's `FetchCandidates` and orders/selects so developer time hits the most informative items:
  - **Active:** prioritize items where `AutoScore` is **uncertain** (within `REVIEW_ACTIVE_UNCERTAINTY_BAND` of 0.5) OR where `AutoScore` **disagrees with a secondary signal** (e.g. `Signals["similarity"]` high but `AutoLabel="ignored"` — the Finding-3 signature).
  - **Stratified:** spread the selection across the dataset's natural `Stratum` values (for guidance: `guidance_type × outcome`) so no stratum is starved.
  - Selection is **deterministic given a seed** (testability) and capped at `REVIEW_SAMPLE_SIZE`.
- **Config** (no-hardcoding): `REVIEW_SAMPLE_SIZE` (default `200`), `REVIEW_ACTIVE_UNCERTAINTY_BAND` (default `0.4` — pull items whose `AutoScore ∈ [0.5−band/2, 0.5+band/2]` or that disagree with their signal), `REVIEW_STRATA_DEFAULT` (default empty → use the dataset's declared strata), `REVIEW_SAMPLE_SEED` (default `0` → time-seeded; non-zero → deterministic).
- **Gate G3:** sampler unit tests — uncertain/disagreeing items preferred within the band; strata coverage (no stratum starved when items exist); deterministic for a fixed seed; cap honored.

---

**Epic 4 — `/v1/review/*` endpoints.**

All gated by `AUTH_API_KEYS` (like `/v1/admin/breakers`); `REVIEW_ENABLED=false` → 503.

- `GET /v1/review/datasets` — list registered reviewable datasets (`{id, display_name, rubric_version, rubric_kind, candidate_count}`).
- `GET /v1/review/next?dataset_id=<id>&space_id=<id>` — fetch the next sampled item (dataset `FetchCandidates` → sampler → top item not graded at current `rubric_version`), returns the item + its rubric shape + the auto-label.
- `POST /v1/review/grade` — submit a grade. Body: `{dataset_id, item_id, space_id, grader_id, dimensions{…}|ranking{…}, reinforce bool, dry_run bool}`. Flow: **validate** (rubric shape) → **score** (Epic 2) → **idempotency check** (`LatestGradeForItem`: a non-reversed grade at the current `rubric_version` for this `(dataset_id,item_id)` → 409 unless `force`) → **persist** the gold row → **if `reinforce` (default from `REVIEW_REINFORCE_DEFAULT`) and not `dry_run`**: call the dataset's `Sink.Apply` (Epic 5), record `reinforcement_detail` → **audit**. `dry_run=true` returns the sink's **preview** (what *would* be applied) without writing.
- `POST /v1/review/reverse` — `{grade_id}` → look up the grade, call `Sink.Reverse(reinforcement_detail)` (restore prior state), write a **reversal row** (`reverses_grade_id` set) and set `reversed=true` on the original. Idempotent (reversing an already-reversed grade is a 409/no-op).
- **Config:** `REVIEW_REINFORCE_DEFAULT` (default `false` — reinforcement is **opt-in per submission**; a grade is gold-only unless `reinforce:true` is sent, so the default path never mutates the live system by accident), `REVIEW_DRY_RUN_DEFAULT` (default `false`).
- **Gate G4:** UATS contract specs for all four endpoints (list/next/grade/reverse) green (`make test-api`), incl. the 503-when-disabled and 401/403-when-unauthed variants and the 409 idempotency variant; `dry_run` returns a preview without a TSDB row.

---

**Epic 5 — Reinforcement sinks (the headline capability).**

- **The `ReinforcementSink` interface** (`internal/review/sink.go`), concretely:

  ```go
  // ReinforcementSink defines what "apply this grade to the live system" MEANS for a
  // dataset. Implementations MUST be idempotent (keyed by grade_id), reversible, and
  // support a dry-run preview. A no-op sink (NoopSink) makes a dataset gold-only.
  type ReinforcementSink interface {
      // Preview returns a human-readable description of what Apply WOULD do, plus the
      // structured detail that Apply would persist — without mutating anything.
      Preview(ctx context.Context, g Grade) (ReinforcementPreview, error)

      // Apply mutates the live system per the grade. It MUST be idempotent on
      // g.GradeID (re-applying the same grade is a no-op returning the prior detail)
      // and MUST capture, in the returned detail, EVERYTHING needed to Reverse it
      // (prior values, created edge ids, the sink verb). Respects protected-space rules.
      Apply(ctx context.Context, g Grade) (ReinforcementDetail, error)

      // Reverse undoes a prior Apply using its persisted detail, restoring prior state.
      // Idempotent: reversing an already-reversed application is a no-op.
      Reverse(ctx context.Context, detail ReinforcementDetail) error
  }

  type ReinforcementPreview struct {
      Summary string                 // "would set GUIDANCE_OUTCOME=followed, trust 0.40→0.46, confidence +0.05"
      Detail  ReinforcementDetail    // the would-be detail (no mutation)
  }

  type ReinforcementDetail struct {
      SinkID      string            // which sink applied it
      GradeID     string            // idempotency key
      Verb        string            // e.g. "guidance_outcome:followed"
      PriorState  map[string]any    // prior trust/confidence/edge presence — the reversal payload
      Applied     map[string]any    // what was set (edge id created, new trust, …)
  }
  ```

- **First concrete sink — `GuidanceSink`** (`internal/review/sink_guidance.go`): reuses the existing jiminy reinforcement path. From a certified guidance grade it derives the **corrected outcome** (`outcome_label_correctness` dimension → `followed | partial | ignored | contradicted`) and applies it exactly as `/v1/jiminy/feedback`→`RecordOutcome` would, capturing for reversal:
  - **trust** — `TrustScorer.RecordOutcome` (the EMA from JIMINY-EFFECTIVENESS-001) moves trust; `PriorState` records the pre-apply trust so `Reverse` restores it (reversal sets trust back, it does not re-EMA).
  - **`GUIDANCE_OUTCOME` edge** — created/updated on the real constraint node (the JIMINY-OUTCOME-001 `constraint_code` resolution); `Applied` records the edge id so `Reverse` can delete the *created* edge (or restore the prior edge props if it pre-existed).
  - **node confidence** — the confidence nudge; `PriorState` records the prior value.
  - **idempotency:** keyed by `grade_id` — re-applying a guidance grade for the same item returns the stored detail, no second edge.
  - **dry-run:** `Preview` computes the would-be outcome + the trust/confidence deltas + "would create edge X" without writing.
  - **protected-space:** all operations are add/update on `mdemg-dev`; reversal restores prior state (deleting only edges *this apply created*) — never a protected-node delete.
- **NoopSink** for gold-only datasets.
- **Lightly-scoped next sinks (signatures + registration path only, NOT implemented):**
  - **`SFTSink`** — a certified SFT grade ≥ threshold marks the row **accepted** for curation (an accept/reject signal `mdemg data curate` reads); below threshold → **rejected/excluded**. Reversal flips the flag back.
  - **`DPOSink`** — a certified ranking writes the **chosen/rejected** preference into the DPO pair store and emits a **recursive-retrain-trigger nudge** (a marker the FT recursive loop reads). Reversal removes the pair.
  - These are documented as the next consumers; their concrete bodies are a follow-up (`hitl-review-002`).
- **Config:** `REVIEW_REINFORCE_DEFAULT` (Epic 4, default `false`), `REVIEW_GUIDANCE_SINK_ENABLED` (default `true`), `REVIEW_GUIDANCE_CONFIDENCE_NUDGE` (default `0.05` — the node-confidence delta; mirrors the existing jiminy nudge), `REVIEW_SINK_DRY_RUN_ENFORCE` (default `false` — when `true`, *all* sink applies require an explicit non-dry-run, a belt-and-suspenders safety flag).
- **Gate G5:** `GuidanceSink` unit tests (Apply captures full reversal detail; Reverse restores prior trust/confidence + deletes only self-created edges; idempotent on `grade_id`; Preview mutates nothing); the SFT/DPO sink signatures compile + register behind their flags.

---

**Epic 6 — Review UI + Playwright e2e.**

- **`internal/api/ui/tabs/review.js`** (vanilla JS, matching `tabs/training_data.js` style — no framework): a **functional** Review tab —
  - **dataset picker** (from `GET /v1/review/datasets`),
  - **item view** — the surfaced content + context + the auto-label, one item at a time (from `GET /v1/review/next`),
  - **rubric form** — rendered from the item's rubric shape (Rated: per-dimension 0–4 radios with the written anchor text inline; Ranked: chosen/rejected picker),
  - a **`reinforce` checkbox** (default from `REVIEW_REINFORCE_DEFAULT`) + a **dry-run "preview"** button that shows the sink preview before applying,
  - **submit + progress** (graded count / sample size),
  - a **reverse** affordance for the last submitted grade.
- Wire the tab into `index.html` nav + `main.js`/`state.js`/`api.js` (the existing tab-registration pattern).
- **Playwright e2e** (house rule): a spec that loads `:9999/ui`, opens the Review tab, fetches an item, fills the rubric, previews (dry-run, asserts no write), submits (asserts the grade lands), and reverses (asserts undo) — driven against the live server.
- **Gate G6:** the Review tab loads + grades an item end-to-end in the browser; Playwright e2e green; visual polish explicitly NOT a gate (functional only).

---

**Epic 7 — Documentation (final, never cut).**
- `docs/features/hitl-review.md` (Why / Choices / How it works / How to use).
- `docs/development/hitl-review-001/rubric_v1.md` (the versioned anchored rubric — if not already created in Epic 2).
- CHANGELOG `[Unreleased] → Added` under v0.11.0.
- `docs/development/hitl-review-001/post.md` stub.
- CLAUDE.md Architecture-Notes entry (the HITL-REVIEW-001 paragraph: the registry + sink interfaces, the `review_grades` table + reversal/audit design, the guidance sink reusing the jiminy reinforcement path, the env vars, the cross-sprint relationship to JIMINY-RELEVANCE-001, the lightly-scoped SFT/DPO follow-ups).
- **Gate G7:** docs present; CHANGELOG + CLAUDE.md updated; tree clean.

## 6. Testing Plan (3 tiers — unit + integration + live Tier-3)

**Tier 1 (unit):**
- **Registry:** `Register`/`Get`/`List`; duplicate-id rejection; the `stubDataset` round-trips.
- **Writer:** enqueue → flush → `CopyFrom` with expected identifier + columns; FIFO eviction increments `dropped`; content truncation at `REVIEW_MAX_CONTENT_BYTES`; `LatestGradeForItem` read-your-writes; `registerWriterStats` registered.
- **Config:** every new env var parses, defaults correct, floors enforced (flush ≥ 5s); `TSDBRequiredSchemaVersion` default = 27; config scanner sees every new knob (no-hardcoding).
- **Rubric/scoring:** rated mean/4 bounds + rounding; ranked encoding; Ranked-submission-to-Rated-rubric rejected; every dimension has a non-empty anchor; `rubric_version` pinned on output.
- **Sampler:** uncertain/disagreeing items preferred within band; stratum coverage; deterministic for fixed seed; cap honored.
- **GuidanceSink:** Apply captures full reversal detail (prior trust + confidence + edge id); Reverse restores prior trust/confidence and deletes only self-created edges; idempotent on `grade_id` (second Apply is a no-op returning the stored detail); Preview mutates nothing.
- **CUIDv2:** `grade_id` is CUIDv2 (regex-validated, not UUID).

**Tier 2 (integration):** `go test ./internal/review/... ./internal/tsdb/... ./internal/config/... ./internal/api/...`; `golangci-lint run ./...` (0 issues); migration `027` applies cleanly against a TimescaleDB container (CI `tsdb`-tagged); **UATS contract** for all four `/v1/review/*` endpoints green (`make test-api`), incl. 503-disabled / 401-unauthed / 409-idempotency / dry-run-no-write variants; the existing `/v1/jiminy/feedback` UATS still green (the guidance sink reuses but does not alter that handler's contract).

**Tier 3 (LIVE — required; the binding gate):** on the running stack (real `bin/mdemg` + real Neo4j + TSDB + llama-server):
- **The headline live test:** a developer **grades a REAL guidance item via `:9999/ui`** with `reinforce:true` → observe in **live Neo4j** the trust score move (EMA signature) AND a `GUIDANCE_OUTCOME` edge created/updated on the real constraint node AND the node-confidence nudge → observe in **live TSDB** a fully-populated `review_grades` audit row (`gold_score`, `gold_dimensions`, `grader_id`, `rubric_version`, `reinforcement_applied=true`, `reinforcement_detail` carrying the reversal payload) → **`POST /v1/review/reverse`** that grade and **verify the reversal undid it** (trust restored to prior, the created edge gone, a reversal row written, `reversed=true` on the original). This single end-to-end run — grade → live mutation observed → audit row observed → reversal verified — is the sprint's binding deliverable.
- **Dry-run safety:** submit a grade with `dry_run:true` → the sink **preview** returns the would-be deltas → confirm **no** Neo4j mutation and **no** `review_grades` row.
- **Idempotency:** re-submit the same item's grade → 409 (or stored-detail no-op) → confirm **no second** `GUIDANCE_OUTCOME` edge.
- **Playwright e2e:** the UI spec (Epic 6) runs green against the live server.
- **Restore state** after the destructive checks (the reversal *is* the restore for the reinforcement test; confirm `mdemg-dev` is back to its pre-test trust/edge state); confirm protected-space rules were never violated.

## 7. Commit Strategy

Conventional commits, one per logical unit / epic:
- `feat(hitl-review-001): dataset registry + review_grades persistence (V0027 + writer)`
- `feat(hitl-review-001): normalized anchored rubric + scoring engine`
- `feat(hitl-review-001): active + stratified review sampling`
- `feat(hitl-review-001): /v1/review/* endpoints (list/next/grade/reverse)`
- `feat(hitl-review-001): reinforcement sink interface + guidance live-reinforcement sink`
- `feat(hitl-review-001): review UI tab + Playwright e2e`
- `docs(hitl-review-001): feature doc + rubric_v1 + CHANGELOG + post + CLAUDE.md`

gofmt/vet + lint each; push once at the end (auto-PR fires — do NOT manually create the PR); add the sprint summary to the PR comments. **Live-surprise fixes get their own fix-commit** (Phase 11.6.2 precedent) — never fold them silently into an epic commit. **Propose options in the plan, decide at execution, disclose in the PR** (the open decisions in §3 / below).

## 8. Verification Checklist

- [ ] `go build ./...` green; `golangci-lint run ./...` 0 issues
- [ ] Migration `027` (or `028` per the coordination note) applies; `TSDBRequiredSchemaVersion` bumped 26 → 27 (or 28); CI schema-version validator passes
- [ ] `review_grades` hypertable has retention (180d) + compression (no unbounded table)
- [ ] Writer buffered + CopyFrom + `registerWriterStats` (joins `mdemg_tsdb_writer_*`); drop/flush counters emit
- [ ] `ReviewableDataset` + `ReinforcementSink` interfaces implemented; registry round-trips; guidance dataset + sink registered; SFT/DPO sink signatures compile behind flags
- [ ] All new values are env vars / config fields with sensible defaults; config scanner clean (no-hardcoding)
- [ ] `grade_id` is CUIDv2 (not UUID)
- [ ] Rubric versioned (`rubric_version` pinned on every grade); rated 0–4→0–1 + ranked DPO shapes both covered; every dimension has a written anchor
- [ ] Active + stratified sampler config-driven (`REVIEW_*`); deterministic for a fixed seed; unit-tested
- [ ] `/v1/review/*` gated by `AUTH_API_KEYS`; `REVIEW_ENABLED=false` → 503; UATS green (incl. 409 idempotency + dry-run + unauthed variants)
- [ ] **Reinforcement is auditable + reversible + idempotent + has a dry-run preview**; reinforcement is **opt-in** per submission (`REVIEW_REINFORCE_DEFAULT=false`)
- [ ] Tier 1 unit + Tier 2 integration + UATS (`make test-api`) green
- [ ] **LIVE:** a developer grades a real guidance item via `:9999/ui` (reinforce) → trust + `GUIDANCE_OUTCOME` + confidence move in live Neo4j → `review_grades` audit row in live TSDB → **reversal verified to undo it**
- [ ] **LIVE:** dry-run preview mutates nothing; idempotent re-grade creates no second edge
- [ ] **LIVE:** Playwright e2e for the Review UI green against the running server
- [ ] Protected-space rules never violated (reversal restores; never a protected-node delete)
- [ ] `docs/features/hitl-review.md` + `rubric_v1.md` + CHANGELOG (v0.11.0) + `post.md` + CLAUDE.md Architecture-Notes entry
- [ ] Cross-sprint ordering documented (JIMINY-RELEVANCE-001 Epic 1 → HITL-REVIEW-001 → JIMINY-RELEVANCE-001 Epic 3)
- [ ] Working tree clean; pushed; auto-PR created; sprint summary on PR; epics executed sequentially with gates

## 9. Documentation Update (final epic — never cut)

**Epic 7** delivers `docs/features/hitl-review.md` with the four mandatory sections:
- **Why:** the diagnostic's 51%-noise finding generalized — every curated dataset needs human-certified review against a reproducible standard, built once; and the headline capability — a certified human judgment should reinforce the **live** cognitive substrate immediately (not wait months for a retrain), reusing the trust scorer + `GUIDANCE_OUTCOME` + node-confidence machinery.
- **Choices:** native-in-`:9999` vs a 6th container (**native** — the write access already lives in the server; 6th container rejected); generic `ReviewableDataset`/`ReinforcementSink` interface vs per-dataset graders (**generic** — write the platform once); gold-columns-on-a-dedicated-`review_grades`-table vs annotation-on-the-source-table (**dedicated table** — datasets vary, the audit/reversal record is uniform); reinforcement **opt-in per submission** + reversible-by-design (safety over convenience); functional-not-flashy UI (de-scoped beauty); the rubric's per-paradigm flex (rated vs ranked DPO).
- **How it works:** the registry + the three sink methods (Preview/Apply/Reverse), the `review_grades` columns incl. `reinforcement_detail` as the reversal payload, the active+stratified sampler, the four endpoints, the guidance sink's reuse of the jiminy reinforcement path, idempotency keyed by `grade_id`.
- **How to use:** the env vars (`REVIEW_*`), the grading workflow at `:9999/ui`, how to enable reinforcement per grade, the dry-run preview, how to reverse a grade, and how a new dataset type registers (implement `ReviewableDataset` + a `ReinforcementSink`).

Plus `rubric_v1.md`, CHANGELOG (v0.11.0 Added), `post.md` stub, and the CLAUDE.md Architecture-Notes paragraph.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Reinforcement corrupts the live protected `mdemg-dev` substrate (wrong/double apply) | **High** | Reinforcement is **opt-in per submission** (`REVIEW_REINFORCE_DEFAULT=false`); **idempotent on `grade_id`** (no double-apply); **dry-run preview** before any write; **fully reversible** (`reinforcement_detail` captures prior state); the binding live gate is a *verified reversal*; reversal restores prior state, never deletes protected nodes. |
| Reversal can't actually undo the apply (lost prior state) | **High** | `Apply` MUST capture EVERYTHING needed to reverse (prior trust, prior confidence, created-edge id, verb) in `reinforcement_detail` *before* mutating; a Tier-1 test asserts the captured detail round-trips; the live gate verifies trust restored + edge gone. |
| `027` schema-version collision with JIMINY-RELEVANCE-001's `027` | Med | Called out in §1 — whichever merges first takes `027`; the second renumbers to `028` + bumps to 28 in the same PR (mechanical). |
| Generic interface over-fits the guidance case, doesn't actually flex to DPO | Med | Rubric `Kind` (Rated/Ranked) + `ReviewItem.Alternatives` are designed for DPO from the start; the SFT/DPO sink *signatures* compile this sprint (proving the interface holds) even though their bodies are a follow-up. |
| Bulky stored snapshots bloat `review_grades` | Low | `REVIEW_MAX_CONTENT_BYTES` truncation; retention 180d + compression (V0025); human-paced volume is low; `enqueued/dropped` counters observable. |
| UI scope creep into "polish" | Low | Explicitly de-scoped — functional only; visual polish is NOT a gate; vanilla-JS, no framework/build step. |
| Live reinforcement gate blocked because guidance evidence isn't populated yet | Med | The platform self-tests against the `REVIEW_STUB_DATASET_ENABLED` synthetic dataset for its non-reinforcement gates; the *reinforcement* live gate uses a real `GUIDANCE_OUTCOME`-eligible item (these exist today, independent of JIMINY-RELEVANCE-001 Epic 1). |
| Auth gap exposes a live-mutation surface | Med | All `/v1/review/*` gated by `AUTH_API_KEYS` like `/v1/admin/breakers`; UATS asserts 401/403 unauthed. |

## 11. Documents Accessed

- `docs/development/jiminy-relevance-001/diagnostic_ignored_population.md` (the 51%-noise diagnostic — the motivation; the need for human-certified, live-reinforcing review)
- `docs/development/jiminy-relevance-001/sprint_plan_jiminy_relevance_001.md` (the sibling sprint — house style/voice; its Epic 3 is the thing this platform generalizes; the cross-sprint ordering)
- `internal/api/handlers_jiminy.go` (`handleJiminyFeedback` ~214 — the existing reinforcement entrypoint the guidance sink mirrors)
- `internal/jiminy/service.go` (`RecordOutcome` ~1439; `TrustScorer.RecordOutcome` EMA; the `GUIDANCE_OUTCOME` outcome writers — what the guidance sink reuses)
- `internal/jiminy/types.go` (`GuidanceFeedbackRequest`, `GuidanceItem`)
- `internal/tsdb/constraint_outcomes_writer.go` + `internal/tsdb/migrations/026_constraint_outcomes_classifier_source.sql` (writer + migration templates; latest = 026; `registerWriterStats` pattern)
- `internal/config/config.go` (`TSDBRequiredSchemaVersion` = 26; the `FromEnv()` config-block + no-hardcoding scanner pattern)
- `internal/api/ui/` (`index.html`, `main.js`, `state.js`, `api.js`, `tabs/*.js` — the vanilla-JS tab pattern, e.g. `tabs/training_data.js`)
- `internal/cli/data_curate.go` + `training/paradigm_router.py` (the SFT/DPO/RAFT/curriculum curation surface the SFT/DPO sinks will eventually feed)
- CHANGELOG.md / git tags (current v0.10.1 → next minor v0.11.0; JIMINY-RELEVANCE-001 targets v0.11.0)
- CLAUDE.md / MEMORY.md (no-hardcoding; CUIDv2-not-UUID; TSDB schema-version CI gate; V0025 retention; TSDB-CONSUME-001 alert-SQL/writer-stats contract; protected-space `mdemg-dev` rules; JIMINY-EFFECTIVENESS-001 trust EMA; JIMINY-OUTCOME-001 `constraint_code` resolution; `AUTH_API_KEYS` admin-gating pattern; mandatory 3-tier + live testing; Playwright e2e for UI; recursive-retraining loop FT Phases 6/7/9 — the downstream consumer)

## 12. Rollback Procedures

- **Feature flag:** `REVIEW_ENABLED=false` makes the entire `/v1/review/*` surface return 503 and stops the UI tab from functioning — no code change. `REVIEW_GUIDANCE_SINK_ENABLED=false` disables live reinforcement while leaving gold-only grading intact. `REVIEW_REINFORCE_DEFAULT` defaults `false`, so the *default* path never mutates the live system.
- **Per-grade reversal:** any individual live reinforcement is undone via `POST /v1/review/reverse {grade_id}` (restores prior trust/confidence, removes the self-created `GUIDANCE_OUTCOME` edge, writes a reversal audit row). This is the primary, designed rollback for the headline capability.
- **Migration:** `027` (or `028`) is **additive only** (new table + policies) — touches no existing table, safe to leave in place if the feature is disabled. No down-migration in production; a full revert drops `review_grades` manually and reverts `TSDBRequiredSchemaVersion` to 26 (operator action, off any hot path).
- **Code revert:** reverting the sprint commits removes the registry/endpoints/sink/UI wiring; the table (if created) becomes an orphan that retention ages out. **No destructive operation on existing `constraint_outcomes` / Neo4j data at any point** — this sprint only *adds* rows and *adds/reverses* reinforcement edges it created; it never touches pre-existing protected-space data except via the reversible, idempotent sink.

---

### Open decisions (propose options here; decide at execution; disclose in the PR)

1. **Reversal of a `GUIDANCE_OUTCOME` edge that pre-existed the apply.** Option A — `Apply` only ever *creates* a fresh edge (sink-owned), so `Reverse` always safely deletes it; if an edge already exists, the apply updates props and `PriorState` stores the old props for restore. Option B — refuse to apply when an outcome edge already exists (treat as already-reinforced). **Lean A** (update + store-prior-props restore is more general); decide once the live `GUIDANCE_OUTCOME` cardinality is confirmed.
2. **Where the guidance dataset's items come from before JIMINY-RELEVANCE-001 Epic 1 lands.** Option A — `FetchCandidates` reads `constraint_outcomes` directly (available today, but lacks the guidance *text* snapshot the diagnostic flagged as missing). Option B — read `guidance_training_rows` (richer, but depends on JIMINY-RELEVANCE-001 Epic 1). **Lean B for the real corpus, A as an interim source** so the platform is exercisable before Epic 1; the sink (reinforcement) works against a real outcome-eligible item either way.
3. **`gold_dimensions` jsonb schema for the Ranked (DPO) case.** Whether to reuse the same `{dim: score}` shape with a synthetic `preference` dimension, or a distinct `{chosen, rejected, confidence}` object. **Lean distinct object** (clearer for the DPO consumer); pin in the rubric doc.
4. **Version coordination** — share v0.11.0 with JIMINY-RELEVANCE-001 vs take v0.12.0. **Lean shared v0.11.0** (same arc); decide from the actual merge order (§1).
