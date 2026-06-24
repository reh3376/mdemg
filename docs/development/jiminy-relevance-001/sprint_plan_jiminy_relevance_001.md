# Sprint Plan — JIMINY-RELEVANCE-001: Persist the Evidence + Measure the Right Thing (the 3–6-month guidance-corpus collection infrastructure)

## 1. Header & Metadata

- **Sprint ID:** JIMINY-RELEVANCE-001
- **Line dir:** `docs/development/jiminy-relevance-001/`
- **Date:** 2026-06-23
- **Branch:** `reh3376_dev01` (PR to `main`; never commit to `main`)
- **Target version:** **v0.11.0** (minor bump — additive feature: new hypertable + writer + curation surface + Grafana panel/rule; current released tag v0.10.1, CHANGELOG `[Unreleased]`). **Ships as a coordinated pair with HITL-REVIEW-001, which also recommends v0.11.0** — the two CHANGELOG entries read as one capability ("we now persist the evidence AND can human-certify + live-reinforce it"). Decided at execution from merge order; if the pair splits across a v0.11.0 cut, the later one takes v0.12.0.
- **TSDB schema:** **26 → 27** (one additive migration for `guidance_training_rows`; bump `TSDB_REQUIRED_SCHEMA_VERSION` to **whichever applies** in `internal/config/config.go` — CI Build gate fails otherwise). ⚠️ **Migration-number coordination with HITL-REVIEW-001:** that sprint *also* proposes migration `027` (`027_review_grades.sql`). **Whichever merges first takes `027`; the second renumbers to `028`** and bumps `TSDB_REQUIRED_SCHEMA_VERSION` to `28` in the same PR. Mechanical rebase — called out here so it is not discovered at merge time.
- **Effort:** ~2–3d (Epic 3 is now a THIN consumer of the HITL-REVIEW-001 platform — register the guidance dataset + rubric — not a build-the-tool epic).
- **Risk:** medium — net-new write path on a hot per-prompt endpoint (`/v1/jiminy/feedback`) and a measurement-redefinition that operators will read off Grafana; getting the writer wrong loses evidence silently, so the binding gate is a **live, fully-populated row observed in TSDB**.
- **Lineage:** the **Option-B follow-up disclosed in JIMINY-EFFECTIVENESS-001**, scoped by the **Step-1 diagnostic** `docs/development/jiminy-relevance-001/diagnostic_ignored_population.md`. This sprint is the diagnostic's **Step 2 (collection + measurement infrastructure)**. The human-grading capability the diagnostic envisioned has been **generalized into the standalone HITL-REVIEW-001 platform** (`docs/development/hitl-review-001/sprint_plan_hitl_review_001.md`); this sprint is its **first consumer** (Epic 3 registers the guidance corpus as the platform's first reviewable dataset). The diagnostic's Step 3 (a model retrain) is an explicit, documented **FUTURE-TRIGGER — NOT in this sprint** (§3 Out-of-scope, §5 forward references).

## 2. Problem Statement

The Step-1 diagnostic established (live `mdemg-dev`, 30-day window, 2,561 outcome rows) that **MDEMG cannot retrain its guidance toward the operator's follow-rate goal today, because the training evidence does not exist.** We persist *verdicts* everywhere (`constraint_outcomes` rows, Neo4j `GUIDANCE_OUTCOME` edges) and *evidence* nowhere:

- **Finding 1 (the binding constraint):** no store holds the surfaced **guidance text**, its **source-node role_type/layer**, or the **agent-action text**. The `action_summary` POSTed to `/v1/jiminy/feedback` is classified and then **discarded**. Every historical `(context → surfaced-guidance → did-the-agent-follow)` triple is **unrecoverable** — production emits the perfect signal every prompt and we throw the evidence away.
- **Finding 4:** ~51% of the labels we *do* have are non-LLM heuristic defaults (blank/heuristic `classifier_source`, incl. 747 `partial_compliance` rows defaulted at sim 0.32). A retrain on those learns the heuristic, not the goal.
- **TL;DR #4:** ">90% follow rate" as stated is partly the wrong target — some guidance is *correctly* ignored. The meaningful, reachable metric is **"follow rate on guidance that *should* have been followed"**, which **requires the action evidence (Finding 1) to even measure.**

The operator has decided to **collect & curate training data for 3–6 months before any retrain.** This sprint therefore ships the **collection + measurement infrastructure**: start accumulating a trustworthy, production-distribution `(context, surfaced-guidance, action, audited-outcome)` corpus, raise label quality, and measure follow-rate honestly — so that in 3–6 months a retrain has the data it needs. It does **not** retrain a model and it does **not** change guidance composition (both forward-referenced in §3).

## 3. Scope & Constraints

**In scope (sequential epics — do NOT parallelize; docs-before-implementation within reason):**

1. **Persist the evidence (Epic 1)** — at feedback time, capture per guidance item: a content snapshot of the surfaced guidance, its source-node `role_type`/`layer`, the `action_summary` text, the audited verdict + classifier source, and correlation ids. New TSDB hypertable + buffered CopyFrom writer + schema-version bump. **Forward-only** — no historical backfill (the evidence is unrecoverable per Finding 1; we start the clock now).
2. **Raise label quality (Epic 2)** — reduce the ~51% heuristic-default share by widening LLM-classifier coverage, and add a **periodic audit-sampling job** that produces gold labels on a sample (strong-model and/or operator). Cadence/thresholds config-driven.
3. **Register the guidance corpus as the first HITL-REVIEW-001 reviewable dataset (Epic 3)** — a THIN consumer epic: implement the guidance `ReviewableDataset` against the HITL-REVIEW-001 registry — (a) `FetchCandidates` reads from the `guidance_training_rows` table Epic 1 creates, and (b) declare the guidance-specific rubric dimensions (`relevance`, `actionability`, `outcome_label_correctness`) — then register it. The rubric ENGINE, sampling engine, `:9999/ui` review surface, `review_grades` gold store, and the guidance reinforcement SINK are all OWNED BY HITL-REVIEW-001 (its first concrete sink is the guidance corpus). This epic wires the data source + rubric shape and registers; it does NOT build or rebuild any of those mechanics. **Gated on HITL-REVIEW-001 being merged.**
4. **Measure the right thing (Epic 4)** — instrument **"follow rate on should-follow guidance"** (separate correctly-ignored advisory items from genuine misses), surfaced as a Grafana panel + an alert-evaluator rule. Depends on Epic 1's action evidence + Epic 2's auto-labels (and, where available, the human-certified grades that HITL-REVIEW-001 persists in its `review_grades` table for this dataset).
5. **Curation pipeline (Epic 5)** — extend `mdemg data curate` (UAITS spec-driven) to emit the labeled guidance corpus from the new evidence table, **leak-audited** (reuse the `scripts/audit_eval_leakage.py` pattern) and **distribution-matched**. This is the artifact that matures over 3–6 months.
6. **Documentation (Epic 6 — final, never cut)** — `docs/features/guidance-training-corpus.md`, CHANGELOG, a `post.md` stub, and a CLAUDE.md Architecture-Notes entry.

**Out of scope (documented as such, with forward references):**

- **The retrain itself.** That is a **future sprint**, triggered when the corpus reaches a trustworthy, distribution-matched threshold **per implicated call site**: the **guidance-SYNTHESIS** site (for the *not-actionable* failure mode, Findings 2 & 3) and **FT-CLASSIFY-002** (for the *wrong-constraint-surfaced* failure mode). This is the CLAUDE.md **"recursive-retraining loop (FT Phases 6 / 7 / 9 — NOT STARTED)"**, which this corpus feeds. **Concrete future-trigger condition** (state it, don't build it): *≥ N audited-gold rows per implicated task at production distribution with leak-audit clean and a held-out split — default target `GUIDANCE_CORPUS_RETRAIN_MIN_GOLD_ROWS` (proposed default 2,000/task) over a ≥ 3-month window — at which point open `ft-guidance-001` (synthesis) and advance FT-CLASSIFY-002 (classify).* This sprint only emits the gauge that makes that threshold observable; it does not act on it.
- **Fixing guidance COMPOSITION** — biasing retrieval toward actionable `constraint`/`correction` types, or synthesizing abstractions into imperative directives. This is a **real, near-term lever** (Finding 2: 90% of surfaced guidance is the abstraction class, ignored 53–65%; Finding 5: the **172:1 abstraction:constraint node ratio**; the RRF-SCALE-001 "retrieval surfaces emergent_concept abstractions, not raw constraint nodes" class) that could move follow-rate **now, without waiting 3–6 months**. It is a **strong candidate for a PARALLEL near-term sprint — proposed name `jiminy-actionability-001`.** It is held out of THIS sprint deliberately so collection+measurement and composition do not entangle (one changes what we *store/measure*, the other changes what we *surface* — coupling them would make each impossible to validate).

**Constraints:**

- **No-hardcoding:** every new value is an env var / config field with a sensible default.
- **CUIDv2** for any new identifier (never UUID); library `github.com/nrednav/cuid2` (in `go.mod`).
- **TSDB migration ⇒ bump `TSDB_REQUIRED_SCHEMA_VERSION`** to whichever applies (27, or 28 if HITL-REVIEW-001 took `027` first — see §1 coordination note) in `internal/config/config.go` (CI validator fails Build otherwise).
- **Forward-only**, no backfill (evidence unrecoverable).
- **Sequential epics**, gates between each.
- **Live Tier-3 required** (real binary + real Neo4j + TSDB + llama-server, observable output).
- TSDB writer follows the **buffered + CopyFrom (V0019 / `constraint_outcomes_writer.go`) pattern** — guidance evidence is per-prompt volume; must self-register via `registerWriterStats` so it joins the `mdemg_tsdb_writer_*` flush-stats plane (TSDB-CONSUME-001).
- Alert-rule SQL contract (TSDB-CONSUME-001): the new rule must ALWAYS return one non-NULL row (aggregate + `COALESCE`, never `ORDER BY … LIMIT 1`); the new table's time column is `time`; give the rule a **unique `Service` label** (dispatcher cooldown key is `(Service, Severity)`).
- New hypertable MUST get retention/compression in the migration (V0025 policy — no new unbounded hypertable).

## 4. Dependencies & Pre-Conditions

- **Code:** `internal/api/handlers_jiminy.go` (`handleJiminyFeedback` ~214 — the capture site has `req.ActionSummary` + the per-item classification); `internal/jiminy/service.go` (`RecordOutcome` ~1439 — already iterates items with `item.Content`, `item.Type`, `item.SourceNodes`, `cr.Outcome`, `cr.Confidence`, `cr.Source`; the natural emit point); `internal/jiminy/types.go` (`GuidanceFeedbackRequest` carries `action_summary`; `GuidanceItem` carries `Content`/`Type`/`SourceNodes`/`ConstraintCode`); `internal/tsdb/constraint_outcomes_writer.go` (writer template); `internal/tsdb/migrations/` (latest `026_…`; add `027_…` or `028_…` per the §1 coordination note); `internal/config/config.go` (schema-version + new config block); `internal/api/server.go` (writer construction + wiring an adapter, the `SetGuidanceTrainingWriter`-style hook); `internal/alert/evaluator` rule set; **HITL-REVIEW-001's `internal/review/` registry + `ReviewableDataset` interface** (Epic 3's only new touch-point — implement + register the guidance dataset); `internal/cli/data_curate.go` + `training/paradigm_router.py` (UAITS curation); `scripts/audit_eval_leakage.py` (leak-audit pattern).
- **Hard cross-sprint dependency — HITL-REVIEW-001.** Epic 3 cannot be implemented until the HITL-REVIEW-001 platform exists. **Explicit cross-sprint ordering:** (1) **JIMINY-RELEVANCE-001 Epic 1** lands first — it creates `guidance_training_rows`, the table HITL-REVIEW-001's guidance `FetchCandidates` reads from; (2) **HITL-REVIEW-001** ships the platform (registry + rubric engine + sampler + `/v1/review/*` + `:9999/ui` review tab + `review_grades` store) **and the guidance reinforcement sink** (its Epic 5, the first concrete sink); (3) **JIMINY-RELEVANCE-001 Epic 3** registers the guidance dataset against the now-merged platform. So Epic 3 is **effectively gated on HITL-REVIEW-001 being merged.** Epics 1, 2, 4, 5, 6 of this sprint have no HITL dependency and can proceed in order; only Epic 3 waits.
- **Pre-conditions:** `JIMINY_ENABLED=true` on the live stack; TSDB reachable at schema 26; llama-server :8102 up (for the LLM classifier + audit-sampling); the source-node `role_type`/`layer` are resolvable from `item.SourceNodes` (Neo4j lookup or carried on the item — Epic 1 decides, see open option); **for Epic 3 only: HITL-REVIEW-001 merged** (its `review_grades` table, registry, and guidance sink live).
- **Data dependency:** the source-node `role_type`/`layer` join (Finding 5's actionability signal) requires either reading it off the `GuidanceItem`/`SourceNodes` at emit time or a Neo4j lookup; the writer must not block the feedback hot path on Neo4j (resolve before enqueue, tolerate "unknown").

## 5. Implementation Plan (sequential epics + gates)

**Epic 0 — Plan (this document).** Gate: plan written, format-conformant, scope reconciled with the diagnostic.

**Epic 1 — Persist the evidence (the binding constraint, Finding 1).**
- Migration `027_guidance_training_rows.sql` (or `028_…` per the §1 coordination note): new hypertable carrying **only the evidence-capture columns** (guidance content snapshot, source role_type/layer, `action_summary`, verdict, classifier_source, + correlation ids — full list in the wire step below); `create_hypertable('time')`, indexes, **retention + compression policies** (V0025 contract), `UPDATE tsdb_schema_meta … '27'` (or `'28'`). ⚠️ **No gold-grading columns here** — human/auto gold grades live in HITL-REVIEW-001's `review_grades` hypertable keyed by `(dataset_id, item_id)`, NOT on `guidance_training_rows`; this table is the evidence source the guidance `ReviewableDataset.FetchCandidates` reads, and `item_id` joins back to its `row_id`.
- `internal/tsdb/guidance_training_rows_writer.go`: buffered `GuidanceTrainingRow` + `CopyFrom`, `registerWriterStats("guidance_training_rows", …)`, config-driven flush interval + buffer size, FIFO eviction + drop counter.
- Config block (`internal/config/config.go`), no-hardcoding: `GUIDANCE_CORPUS_ENABLED` (default `true`), `GUIDANCE_CORPUS_WRITER_FLUSH_INTERVAL_SEC` (default `30`, floor `5`), `GUIDANCE_CORPUS_WRITER_BUFFER_SIZE` (default `1000`, `0`=unbounded), `GUIDANCE_CORPUS_MAX_CONTENT_BYTES` (default `8192` — truncate guidance/action snapshots, append `…[truncated]`). **Bump `TSDB_REQUIRED_SCHEMA_VERSION` 26 → 27.**
- Wire from `RecordOutcome` (per item, after `cr` is computed, gated on `GUIDANCE_CORPUS_ENABLED`): emit one row carrying `guidance_content` (snapshot of `item.Content`), `guidance_type`, `source_role_type`/`source_layer` (resolved from `item.SourceNodes`), `action_summary` (`req.ActionSummary`), `outcome_type`, `similarity`, `classifier_source`, `constraint_code`, `guidance_id`, `session_id`, `space_id`, `instance_id`, and a CUIDv2 `row_id`. Resolve role_type/layer **before** enqueue; never block the hot path (timeout-bounded lookup → "unknown" on miss).
- 3 Prometheus counters: `mdemg_guidance_corpus_rows_{enqueued,dropped}_total{space_id}` + `_flush_failure_total`.
- **Gate G1:** `go build ./...` green; writer + config unit tests pass; `registerWriterStats` joins the flush plane; schema-version CI check passes locally.

**Epic 2 — Raise label quality (Finding 4).**
- Widen LLM-classifier coverage: ensure the LLM classifier (`s.classifier`) is the path for as many items as the budget allows so fewer rows fall to the heuristic default — the `classifier_source`/`outcome_type` captured by Epic 1 is then a real LLM verdict, not a heuristic guess. Expose the gating as config (e.g. `GUIDANCE_CORPUS_LLM_LABEL_MIN_SHARE` target, observed via a gauge — *measure* the heuristic-default share, don't silently leave it at 51%).
- **Periodic auto-relabel job:** a scheduled job (jobhealth-reported, supervised per SUPERVISOR-002 — route through the supervisor, never a bare `go func()`) that samples N recent `guidance_training_rows` whose `classifier_source` is heuristic/blank and re-labels them with a strong model, **updating the row's `classifier_source`/`outcome_type` in place** (this raises label quality on the evidence table; it is NOT a gold-grade — human/auto *gold* grading is HITL-REVIEW-001's `review_grades`). Config: `GUIDANCE_AUDIT_ENABLED` (default `true`), `GUIDANCE_AUDIT_INTERVAL_HOURS` (default `24`), `GUIDANCE_AUDIT_SAMPLE_SIZE` (default `50`), `GUIDANCE_AUDIT_MODEL` (default the strong teacher used elsewhere, e.g. `gpt-5.4-mini`), `GUIDANCE_AUDIT_MIN_LATENCY_BUDGET_MS` (≥ 15000 per house rule). Report to jobhealth as `job_name='guidance-audit'`.
- **Gate G2:** auto-relabel job runs once live, upgrades ≥ 1 heuristic-labelled row to an LLM verdict observable in TSDB; heuristic-default-share gauge emits; unit tests for the sampler (deterministic sample given a seed) pass.

**Epic 3 — Register the guidance corpus as the first HITL-REVIEW-001 reviewable dataset.** ⚠️ **Gated on HITL-REVIEW-001 being merged** (see §4 cross-sprint ordering). This is a THIN consumer epic — it wires the data source + rubric shape and registers; it builds **none** of the platform mechanics (rubric engine, sampler, `:9999/ui` review surface, `review_grades` store, the reinforcement sink), which are owned by HITL-REVIEW-001.
- **Why this epic exists:** the corpus must become a human-**CERTIFIED** one — the calibration anchor the LLM auto-classifier (Epic 2) is measured against, the chosen/rejected preference signal for the eventual DPO/retrain, and the only ground truth that can distinguish "should-follow" from "correctly-ignored advisory" for Epic 4. HITL-REVIEW-001 supplies that capability generically; this epic makes the guidance corpus its first registered dataset.
- **Implement `ReviewableDataset` for guidance** (`internal/review/datasets/guidance.go`, against HITL-REVIEW-001's `internal/review/` interface + registry): (a) **`FetchCandidates`** reads un-graded (or re-gradable) rows from the **`guidance_training_rows` table Epic 1 creates** — surfacing the guidance content snapshot, source role_type/layer, action_summary, and the current auto-label as a `ReviewItem` whose `item_id` is the row's `row_id`; (b) **declare the guidance-specific rubric dimensions** — `relevance`, `actionability`, `outcome_label_correctness` (0–4 anchored; the last is what the guidance reinforcement sink reads to decide the corrected outcome). The rubric ENGINE (0–4 → normalized 0–1, versioned anchors, `rubric_version` pinning) is HITL-REVIEW-001's; this epic only names the three dimensions + their anchor text and hands them to the platform.
- **Register** the guidance dataset with the HITL-REVIEW-001 registry at server construction (behind the platform's dataset-enable config). No new env vars, no new endpoints, no new migration in this epic — the data source is Epic 1's table; the gold store, sampler, UI, and sink are the platform's.
- **The reinforcement sink itself (guidance grade → trust EMA + `GUIDANCE_OUTCOME` + node confidence) is OWNED BY HITL-REVIEW-001 Epic 5** (its first concrete sink, reusing the `RecordOutcome` reinforcement path). Epic 3 does **not** rebuild reinforcement; once the dataset is registered, grading a guidance item through the platform reinforces the live substrate via that sink.
- **Gate G3:** with HITL-REVIEW-001 merged, the **guidance dataset appears in the review tool** (`GET /v1/review/datasets` lists it; `GET /v1/review/next?dataset_id=guidance` returns a real item sourced from `guidance_training_rows`) and a real grade submitted through the platform writes a `review_grades` row + reinforces trust on the live substrate (verified via HITL-REVIEW-001's own reinforcement/audit path). Unit test: the guidance `ReviewableDataset` maps a `guidance_training_rows` row → `ReviewItem` correctly and declares the three rubric dimensions.

**Epic 4 — Measure the right thing (TL;DR #4).**
- Define **"follow rate on should-follow guidance"**: numerator = items classified `followed` (or gold-followed); denominator = items deemed *should-follow* (actionable `constraint`/`correction`, OR — preferentially — any item whose **certified-gold** grade says it was a genuine miss; i.e. **exclude correctly-ignored advisory items**, using certified-gold as ground truth where available and falling back to auto-labels otherwise). Compute over `guidance_training_rows` (Epic 1's action evidence + Epic 2's LLM labels) **LEFT JOINed to HITL-REVIEW-001's `review_grades`** on `(dataset_id='guidance', item_id = row_id)` for the certified-gold ground truth (where graded). This join is read-only against the platform's table — no write into `review_grades` from this sprint.
- Grafana panel "Should-Follow Follow Rate" on `mdemg-rsic` (or `mdemg-graph-topology`, alongside the existing Jiminy effectiveness panels), windowed, idle-safe.
- Alert-evaluator rule (`internal/alert/evaluator`): fires when should-follow follow rate drops below `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` (default `0.5`) over `GUIDANCE_SHOULD_FOLLOW_LOOKBACK_HOURS` (default `168`). **Unique `Service` label** (e.g. `guidance-should-follow`); aggregate + `COALESCE` so it always returns one non-NULL row; reads the new table's `time` column; pinned with a SQL contract test.
- **Gate G4:** the rule's SQL returns a non-NULL row on an idle window (pin test); the panel renders against live data; the metric is computed only over should-follow items (verified against a hand-checked sample).

**Epic 5 — Curation pipeline.**
- Extend `mdemg data curate` (UAITS spec-driven, `internal/cli/data_curate.go` + `training/paradigm_router.py`) to emit the labeled guidance corpus from `guidance_training_rows`: a new UAITS spec / paradigm config that reads the evidence table, prefers gold labels over classifier labels, **leak-audits** the output (reuse `scripts/audit_eval_leakage.py` against the relevant sources), and **distribution-matches** (preserve the production `guidance_type` × `outcome` distribution, or document the deliberate re-balancing). Output a versioned corpus artifact under `training_data/` with a manifest (row counts, label-source breakdown, leak-audit report, distribution summary).
- Config-driven thresholds: `GUIDANCE_CORPUS_LEAK_MAX_OVERLAP` (default per existing audit pattern), `GUIDANCE_CORPUS_MIN_GOLD_FRACTION` (default `0.0` now — it grows over the 3–6 months as HITL-REVIEW-001 grading accumulates; the retrain trigger reads it). Curation prefers **certified-gold** (HITL-REVIEW-001 `review_grades`, joined on `item_id = row_id`) over the row's LLM `classifier_source` label, and records the label-source breakdown in the manifest.
- **Gate G5:** `mdemg data curate --spec <guidance spec>` emits a corpus artifact + manifest live; leak-audit runs clean (or reports overlap, exit-coded); the manifest shows the production distribution + the certified-gold/LLM-label source breakdown.

**Epic 6 — Documentation (final, never cut).**
- `docs/features/guidance-training-corpus.md` (Why / Choices / How it works / How to use).
- CHANGELOG `[Unreleased] → Added` under v0.11.0.
- `docs/development/jiminy-relevance-001/post.md` stub.
- CLAUDE.md Architecture-Notes entry (the JIMINY-RELEVANCE-001 paragraph: what was persisted, the new table/writer/rule, the should-follow metric, that the guidance corpus is **HITL-REVIEW-001's first registered reviewable dataset** (human grading + live reinforcement owned there), and the explicit out-of-scope forward references to `jiminy-actionability-001` + the retrain trigger).
- **Gate G6:** docs present; CHANGELOG + CLAUDE.md updated; tree clean.

## 6. Testing Plan (3 tiers — unit + integration + live Tier-3)

**Tier 1 (unit):**
- Writer: enqueue → flush → `CopyFrom` called with the expected `pgx.Identifier` + columns; FIFO eviction past buffer size increments `dropped`; content truncation at `MAX_CONTENT_BYTES`; `Stats()` registered.
- Config: all new env vars parse, defaults correct, floors enforced (flush ≥ 5s, latency budget ≥ 15000ms), `TSDB_REQUIRED_SCHEMA_VERSION` default = 27; config scanner sees every new knob (no-hardcoding).
- Auto-relabel sampler: deterministic sample for a fixed seed; in-place `classifier_source`/`outcome_type` update shape (only touches heuristic/blank rows).
- Guidance `ReviewableDataset` (Epic 3, thin): a `guidance_training_rows` row maps to a `ReviewItem` correctly (`item_id = row_id`, content/role/action/auto-label populated); the three rubric dimensions (`relevance`, `actionability`, `outcome_label_correctness`) are declared. (The rubric ENGINE + sampler are HITL-REVIEW-001's — tested there, not here.)
- Should-follow metric: numerator/denominator selection excludes correctly-ignored advisory, prefers certified-gold ground truth from the `review_grades` LEFT JOIN; SQL contract (returns one non-NULL row on empty input).
- CUIDv2 used for `row_id` (regex-validated, not UUID).

**Tier 2 (integration):** `go test ./internal/tsdb/... ./internal/jiminy/... ./internal/config/... ./internal/alert/...`; `golangci-lint run ./...` (0 issues); migration applies cleanly against a TimescaleDB container (CI `tsdb`-tagged); UATS contract for `/v1/jiminy/feedback` still green (`make test-api`) — the feedback path now also emits a corpus row but the response contract is unchanged.

**Tier 3 (LIVE — required):** on the running stack (real `bin/mdemg` + real Neo4j + TSDB + llama-server):
- **Persist-evidence smoke:** run a real prompt → guidance surfaced via hook → real `/v1/jiminy/feedback` with a real `action_summary` → **observe a fully-populated `guidance_training_rows` row in TSDB** carrying the guidance content snapshot, source role_type/layer, the action text, and the audited outcome + classifier_source (the binding deliverable — evidence that was previously discarded is now stored).
- **Auto-relabel job:** trigger the auto-relabel job live; observe ≥ 1 heuristic-labelled row upgraded to an LLM verdict in TSDB + a jobhealth `guidance-audit` success row.
- **Guidance dataset registered (Epic 3 integration, requires HITL-REVIEW-001 merged):** the **guidance dataset appears in the review tool** — `GET /v1/review/datasets` lists `guidance`; `GET /v1/review/next?dataset_id=guidance` returns a real item sourced from `guidance_training_rows`; and a real grade submitted through the platform **reinforces trust** on the live substrate + writes a `review_grades` row (the grade→live-reinforcement→audit→reversal e2e is HITL-REVIEW-001's binding deliverable; this sprint verifies the guidance dataset *participates* in it). If HITL-REVIEW-001 is not yet merged at this sprint's Tier-3 run, Epic 3's live check is deferred to the HITL merge per the cross-sprint ordering and noted in the PR.
- **Metric:** observe the "Should-Follow Follow Rate" gauge/panel populate from real rows; confirm the alert rule evaluates without rule-health noise on an idle window (TSDB-CONSUME-001 class).
- **Curation:** run `mdemg data curate` against the live table; confirm a corpus artifact + manifest + leak-audit report are produced.
- **Restore state** after destructive checks; confirm the feedback hot path latency is unaffected (writer is async).

## 7. Commit Strategy

Conventional commits, one per logical unit / epic: `feat(jiminy-relevance-001): persist guidance training evidence (V0027 + writer)`, `feat(jiminy-relevance-001): guidance label-quality auto-relabel job`, `feat(jiminy-relevance-001): register guidance corpus as HITL-REVIEW dataset`, `feat(jiminy-relevance-001): should-follow follow-rate metric + panel + rule`, `feat(jiminy-relevance-001): guidance corpus curation surface`, `docs(jiminy-relevance-001): feature doc + CHANGELOG + post + CLAUDE.md`. (The Epic 3 commit lands only after HITL-REVIEW-001 is merged — see the cross-sprint ordering; the other epic commits have no such gate.) gofmt/vet + lint each; push once at the end (auto-PR fires — do NOT manually create the PR); add the sprint summary to the PR comments. **Live-surprise fixes get their own fix-commit** (per the Phase 11.6.2 precedent in CLAUDE.md) — never silently fold them into the epic commit.

## 8. Verification Checklist

- [ ] `go build ./...` green; `golangci-lint run ./...` 0 issues
- [ ] Migration applies (evidence-capture columns only — **no** `gold_*` columns); takes `027` or `028` per the §1 HITL-REVIEW-001 coordination note; `TSDB_REQUIRED_SCHEMA_VERSION` bumped to whichever applies; CI schema-version validator passes
- [ ] New hypertable has retention + compression policies (no unbounded table)
- [ ] Writer buffered + CopyFrom + `registerWriterStats` (joins `mdemg_tsdb_writer_*`); 3 Prometheus counters emit
- [ ] All new values are env vars / config fields with sensible defaults; config scanner clean (no-hardcoding)
- [ ] `row_id` is CUIDv2 (not UUID)
- [ ] Tier 1 unit + Tier 2 integration + UATS (`make test-api`) green
- [ ] **LIVE:** a fully-populated `guidance_training_rows` row (guidance content + source role/layer + action text + audited outcome) observed in TSDB from a real prompt→feedback cycle
- [ ] **LIVE:** auto-relabel job upgrades ≥ 1 heuristic-labelled row to an LLM verdict + jobhealth success row
- [ ] **HITL dep:** Epic 3 lands only after HITL-REVIEW-001 is merged; guidance `ReviewableDataset` implemented + registered; `GET /v1/review/datasets` lists `guidance` and `/next` returns a `guidance_training_rows`-sourced item; guidance dataset→`ReviewItem` mapping unit-tested
- [ ] **LIVE (Epic 3, post-HITL-merge):** a real grade on a guidance item reinforces trust on the live substrate + writes a `review_grades` row (or deferred-to-HITL-merge note in the PR)
- [ ] **LIVE:** should-follow follow-rate panel populates (certified-gold from the `review_grades` join preferred); alert rule returns one non-NULL row on an idle window (unique `Service`)
- [ ] **LIVE:** `mdemg data curate` emits a leak-audited, distribution-summarized corpus artifact + manifest
- [ ] Feedback hot-path latency unaffected (async writer)
- [ ] `docs/features/guidance-training-corpus.md` + CHANGELOG (v0.11.0) + `post.md` + CLAUDE.md Architecture-Notes entry
- [ ] Out-of-scope forward references documented (`jiminy-actionability-001`; retrain FUTURE-TRIGGER → FT Phases 6/7/9 + FT-CLASSIFY-002)
- [ ] Working tree clean; pushed; auto-PR created; sprint summary on PR; epics executed sequentially with gates

## 9. Documentation Update (final epic — never cut)

**Epic 6** delivers `docs/features/guidance-training-corpus.md` with the four mandatory sections:
- **Why:** the binding constraint (Finding 1 — evidence discarded; no retrain possible without it), the 3–6-month operator-decided collection strategy, and why human certification (via HITL-REVIEW-001) — not just a second auto-classifier — is the calibration anchor + preference signal.
- **Choices:** new hypertable vs extending `constraint_outcomes` (recommended new table — see §3 open option); evidence-capture columns only (gold grades live in HITL-REVIEW-001's `review_grades`, joined on `item_id = row_id` — **not** on `guidance_training_rows`); distribution-matching policy; what "should-follow" means and how certified-gold grounds it.
- **How it works:** the emit path from `RecordOutcome`, the buffered writer, the auto-relabel job, the guidance corpus registered as **HITL-REVIEW-001's first reviewable dataset** (grading + live reinforcement owned there), the `review_grades` join for certified gold, the should-follow metric definition, the curation pipeline.
- **How to use:** the env vars, how the guidance dataset shows up in the HITL review tool at `:9999/ui` (see HITL-REVIEW-001's feature doc for the grading/reinforcement workflow), `mdemg data curate` invocation for the corpus, where the artifact + manifest land, how to read the should-follow panel, and the retrain FUTURE-TRIGGER condition.

Plus CHANGELOG (v0.11.0 Added — coordinated with HITL-REVIEW-001), `post.md` stub, and a CLAUDE.md Architecture-Notes paragraph. (The grading rubric doc belongs to HITL-REVIEW-001, not this sprint.)

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Bulky text on a hot per-prompt write path bloats TSDB / slows feedback | High | Async buffered writer (never blocks the hook); `GUIDANCE_CORPUS_MAX_CONTENT_BYTES` truncation; retention + compression in the migration (V0025 contract); `enqueued/dropped` counters make pressure observable. |
| Evidence silently not captured (the failure we're fixing, recurring) | High | Binding live Tier-3 gate is a **fully-populated row in TSDB**; flush-failure counter + the `mdemg_tsdb_writer_*` plane; loud warn on Neo4j role/layer-lookup miss → "unknown" (never drop the row). |
| Should-follow metric encodes a wrong denominator (re-introduces the >90%-as-stated error) | Med | Numerator/denominator pinned in unit tests; live verified against a hand-checked sample; exclude correctly-ignored advisory explicitly; documented in the feature doc. |
| Alert rule generates rule-health noise (TSDB-CONSUME-001 class) | Med | Aggregate + `COALESCE` (always one non-NULL row), `time` column, unique `Service`; pin test on an idle window. |
| Label quality stays ~51% heuristic, corpus is noisy | Med | Epic 2 widens LLM coverage + auto-relabels heuristic rows in place; HITL-REVIEW-001 adds human-certified gold (joined via `review_grades`); the heuristic-default-share gauge measures progress; curation prefers certified-gold over LLM labels. |
| Cross-sprint coupling — Epic 3 blocked / migration-number collision with HITL-REVIEW-001 | Med | Explicit cross-sprint ordering (§4): Epic 1 lands first, HITL ships, then Epic 3 registers; Epics 1/2/4/5/6 have no HITL dependency so the sprint progresses regardless; the `027`/`028` migration-number rebase is called out in §1 so it is not discovered at merge time; if HITL slips, Epic 3's live check defers to the HITL merge (noted in the PR). |
| Scope creep into composition/retrain/platform-building | Med | Explicit out-of-scope with forward references; the human-grading platform is HITL-REVIEW-001 (Epic 3 is a thin consumer, builds none of it); `jiminy-actionability-001` (composition) and the retrain trigger are named but not built here; sequential epics keep the lines disentangled. |
| Distribution skew in the curated corpus | Low | Curation distribution-matches the production `guidance_type × outcome` distribution (or documents deliberate rebalancing in the manifest); leak-audited. |

## 11. Documents Accessed

- `docs/development/jiminy-relevance-001/diagnostic_ignored_population.md` (Step-1 diagnostic — factual basis; all 5 findings + TL;DR)
- `docs/development/jiminy-effectiveness-001/sprint_plan_jiminy_effectiveness_001.md` (house style/voice; the Option-B disclosure)
- `docs/development/hitl-review-001/sprint_plan_hitl_review_001.md` (the standalone human-in-the-loop review + live-reinforcement platform this sprint's Epic 3 consumes; the `ReviewableDataset`/`ReinforcementSink` interfaces, the `review_grades` table, the guidance sink, the `027`/`028` migration coordination, the shared-v0.11.0 recommendation)
- `docs/development/eventgraph-001/` (TSDB-schema-change pattern: migration + buffered writer + schema-version bump)
- `internal/api/handlers_jiminy.go` (`handleJiminyFeedback` ~214 — capture site; `action_summary` currently classified then discarded)
- `internal/jiminy/service.go` (`RecordOutcome` ~1439 — per-item iteration, the natural emit point)
- `internal/jiminy/types.go` (`GuidanceFeedbackRequest`, `GuidanceItem`)
- `internal/tsdb/constraint_outcomes_writer.go` + `internal/tsdb/migrations/011_constraint_outcomes.sql`, `…/026_constraint_outcomes_classifier_source.sql` (writer + migration templates; latest = 026)
- `internal/config/config.go` (`TSDB_REQUIRED_SCHEMA_VERSION` = 26; config-block pattern)
- `internal/cli/data_curate.go` + `training/paradigm_router.py` (UAITS curation surface); `scripts/audit_eval_leakage.py` (leak-audit pattern)
- CHANGELOG.md / git tags (current v0.10.1 → next minor v0.11.0)
- CLAUDE.md (recursive-retraining loop FT Phases 6/7/9 NOT STARTED; FT-CLASSIFY-002; RRF-SCALE-001; TSDB-CONSUME-001 alert-SQL contract; SUPERVISOR-002 supervisor rule; V0025 retention policy; no-hardcoding / CUIDv2 rules)

## 12. Rollback Procedures

- **Feature flag:** `GUIDANCE_CORPUS_ENABLED=false` stops all corpus writes without a code change; `GUIDANCE_AUDIT_ENABLED=false` stops the auto-relabel job; the alert rule is disabled by setting `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR=0` (rule convention: `0` disables). Epic 3's dataset registration is gated by HITL-REVIEW-001's own dataset-enable config — disabling it there removes the guidance dataset from the review tool without touching this sprint's code.
- **Migration:** the new table migration is **additive only** (new table + policies) — it touches no existing table and is safe to leave in place even if the feature is disabled. No down-migration is run in production; if a full revert is required, drop `guidance_training_rows` manually and revert `TSDB_REQUIRED_SCHEMA_VERSION` to the prior value (operator action, off the hot path). The HITL-REVIEW-001 `review_grades` table is owned + reverted by that sprint.
- **Forward-only data:** there is no historical data to restore (none was ever captured); disabling the feature simply stops accumulation — already-captured rows are inert and harmless.
- **Code revert:** reverting the sprint commits removes the writer/handler wiring; the table (if created) becomes an orphan with retention policies that age it out. No destructive operation on existing `constraint_outcomes` / Neo4j data at any point (this sprint only *adds*).
