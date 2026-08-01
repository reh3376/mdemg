# MDEMG ROADMAP — Q4 2026 (formalized 2026-08-01)

<!-- Generated 2026-08-01 as a retrospective + forward-looking synthesis of
     the Q4 arc (started with the Q4-frontier deep-dive on 2026-07-27
     under `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
     — which the deep-dive itself named "scoping, not commitment," with
     the actual roadmap deferred to this doc). Sprint dates: 2026-07-27
     through 2026-08-01. Rules pinned to CLAUDE.md this quarter: 25+.
     PRs merged: 554-565 (13 in the operator-directed post-audit arc) +
     ~9 in the pre-arc Q4-frontier top-6 window. -->

## 1. State-of-the-System Verdict

Q4 started with the Q4-frontier deep-dive (2026-07-27) — a 6-candidate
scoping exercise the deep-dive itself framed as "the substrate is
mature; the operator's north star ('increase the likelihood of better
decisions') needs measurement integrity + connection-layer quality
before more capability." The premise was: **Q3 shipped the plumbing;
Q4 ships what the plumbing is FOR**. That framing held.

**Verdict as of 2026-08-01**: the connection-layer defects the Q4
deep-dive named — corpus contamination, tier1 systematic mislabeling,
context-mismatch classifier readings, abstract-over-concrete retrieval
drift, reverse-lookup ("what consumes X?") failure, empty-code
classifier attribution — are all **either fixed or measurement-honest**
(the honest ceiling turned out to be lower than the operator's stated
>90% target, and the recalibration to a realistic 15-25% band is
itself the deliverable). The 15%-baseline follow-rate is preserved
but three of its four upstream defects are removed; the fourth (Lever
C surface volume) is a design trade-off the operator has already
adjudicated. The Q4-frontier arc's fanout of 13 disclosed follow-ups
(all shipped this quarter) shows the "small-batch shipping + verify
+ pin rule + move on" cadence held throughout — no half-finished
implementations, no ambient TODO backlog.

**Q4-shipped strategic capability**: **GO-IMPLEMENTS-001** — the last
Q3 deferral — closed. 267 Go IMPLEMENTS pairs discovered on mdemg's
own tree in 472ms; 188 landed in Neo4j; the structural RRF column +
IMPLEMENTS edge-attention weight (0.70) now have real Go data to
walk. Two-quarters-open capability gap closed on the last day of Q4.

**Q4-shipped strategic hygiene**: the **DORMANT-CENSUS family is now
three-strong** — routes (Q3), TSDB tables (Q3 close), metrics (Q4).
Every declaration surface with a plausible drift class has an
adjudication registry + build-time CI drift check. Same forcing-
function pattern that ended the Q3 silent-failure class now extends
to build-time drift.

**Q4-shipped strategic tightening**: the **HITL platform matured**
from a single-dataset guidance corpus to an auto-graded, sample-
recorded infrastructure (HITL-CURATION-002 pre-arc) whose SME
suggestions now feed the training corpus (REVIEW-SUGGESTED-GUIDANCE-
CONSUME-001). Correction nodes now carry constraint codes (CORRECTION-
CODE-GEN-001) closing the last "0/N of a role type is codified" gap.

**Standing residuals** (deliberate; each has a documented trigger):
- **PLUGIN-HYGIENE** — needs an operator disposition call (ship
  first-party plugins in image + forward env, OR document native-only
  + `/v1/scraper` actionable error). No code work possible without
  the decision.
- **CONTEXT-LIVE-001** / **HEBB-ETA-001** — stretch-tier research
  work carried over from Q3; each blocked on a valid prerequisite
  (score-scale contract stability / SURPRISE-TOPK + CoactivateSession
  semantics).
- **FG-2 carry-forward extraction + successor foundational
  document** — Q3 explicitly named as the highest-leverage neglected
  *strategic* workstream; still 3/5 fork readiness.
- **60→49 IN_USE_TSDB_ONLY residual** (post DORMANT-METRICS-CLEANUP-
  001) — 49 metric declarations are still wired but quiet (error/
  failure paths that just haven't triggered). Keep-as-is is the
  right call per the sprint's own pin — removing them loses the
  signal.

---

## 2. Phases (what SHIPPED)

Q4 didn't have a top-down phase plan the way Q3 did; the shape was
"deep-dive → 6 top candidates → shipping cadence with cascading
follow-ups." Below is the arc grouped thematically. All sprint dirs
under `docs/development/`; feature docs under `docs/features/` where
applicable.

### PHASE 1 — Measurement Integrity (Q4-frontier top-4 + disclosed follow-ups; 2026-07-27 → 2026-07-30)

Anchor question: **is the ~11% Jiminy follow rate real, or noise?**

- **HITL-CURATION-002** — the HITL platform's second major sprint.
  Six sequential epics: `internal/review/autograder.go`; `GET
  /v1/review/candidates` bulk-fetch; per-dataset `AutogradePromptHinter`;
  `mdemg review autograde` CLI; `mdemg review cadence` digest;
  `alert.HITLCurationStalledRule`. **Load-bearing invariant** at TWO
  layers: every auto-graded row's `grader_id` starts with
  `review.AutoGraderPrefix` (`"auto:"`, pin-tested) AND the CLI
  ALWAYS POSTs `reinforce:false`. **Auto-grade NEVER triggers the
  substrate reinforcement side-effect**; only operator-confirmed
  grades mint L0 corrections / adjust trust scores. Live E1 drill:
  4 rows landed with `reinforcement_applied=false`; contradicted
  drafts stayed pending 5/5 post-autograde. Sprint dir:
  `docs/development/hitl-curation-002/`. Feature doc:
  `docs/features/hitl-auto-curation.md`.

- **CLASSIFIER-CONSISTENCY-001** — the deep-dive framed 25%-heuristic
  fallback on `constraint_outcomes` vs 0.03% on the training corpus
  as "chronic"; investigation **reframed it** — the split is entirely
  GUIDANCE-AUDIT-001's asymmetric retroactive relabeling, and the
  25% headline was ONE 2026-07-21 multi-sprint-day burst driven by
  LLM saturation (baseline 0-1%). Shipped as the durable observability
  layer: `alert.HeuristicShareRule` fires when heuristic fraction
  exceeds `HEURISTIC_SHARE_THRESHOLD` (0.05) over 24h. Live: rule
  count 29→30, live query 0.0062 (2/321) → CLEAR. ⚠️ **Rule
  pinned**: burst-vs-chronic distinction — a deep-dive can be right
  about the CLASS of failure while wrong about the TEMPO; check per-
  day trends, not aggregate-window slices. Sprint dir:
  `docs/development/classifier-consistency-001/`.

- **DRIFT-TRIGGER-001** — deep-dive framed as "validate the shipped
  drift → actuator → cycle chain end-to-end"; investigation **reframed
  it** — the chain didn't exist. `ft_production_drift` was purely
  observational, and the only path to `Gate.EvaluateTrigger` was
  RSIC pattern 29 (`training_data_ready` — DATA-availability signal,
  not MODEL-quality). Shipped as capability work: pattern 31
  (`production_drift_detected`) emits `trigger_training_pipeline`
  keyed on drift. Live Tier-3: seeded synthetic drift 0.4655 on
  mdemg-dev, pattern fired with correct fields, Gate suppressed (0
  cycles opened, actuator OFF — every safety guard preserved).
  Sprint dir: `docs/development/drift-trigger-001/`. Feature doc:
  `docs/features/drift-triggered-retrain.md`.

- **JIMINY-CEILING-INVESTIGATION-001** — research-class (n=16
  hand-classified). **Zero genuine ignores across 16 samples.** The
  ~11% follow rate is the noise floor produced by three compounding
  defects: (A) corpus contamination (~55% of surface volume goes to
  non-rules), (B) tier1 systematic mislabeling (1% follow rate over
  102 real-durable-rule events), (C) context-mismatch labeled as
  ignored (~50% of remaining ignored). Realistic ceiling if all
  three fixed: **50-70%** on real durable rules under proper
  measurement — the operator's stated >90% target may itself be
  miscalibrated. This investigation spawned the JIMINY-* fix arc
  that followed. Sprint dir:
  `docs/development/jiminy-ceiling-investigation-001/`.

- **JIMINY-CLASSIFIER-CONTEXT-001** — Ceiling defect **C**. Extends
  `classifySystemPrompt` with a `contextMismatchCreditClause`
  routing "rule doesn't govern this action's context" to
  `not_applicable` (filtered from `constraint_outcomes`). ⚠️
  **Architectural rule pinned**: prompt extension via resolver
  method + default-off gate — preserves ULTS `system_prompt_hash`
  pin (default-off render byte-identical). Sibling to the shipped
  JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (must_not credit).
  Live-verified via 3 fixtures; passive re-measurement pending 3-7d.
  Sprint dir: `docs/development/jiminy-classifier-context-001/`.

- **RETRIEVAL-QUALITY-AUDIT-001** — research-class (n=15 real
  operator queries). Total helpful@5: 45/75 = 60%; median helpful@5:
  4/5; failures cluster on specific query shapes. Six follow-ups
  disclosed (three actionable: REVERSE-LOOKUP, LAYER-BALANCE,
  DIVERSITY). Sprint dir:
  `docs/development/retrieval-quality-audit-001/`.

- **JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001** (pre-Q4 close of
  the JIMINY-ACTIONABILITY line) + **JIMINY-ACTIONABILITY-INVERSION-
  001** — kept as reference; both fold into the arc's diagnostic
  chain.

### PHASE 2 — Retrieval Quality Cluster (RETRIEVAL-QUALITY-AUDIT-001 follow-ups; 2026-07-29 → 2026-07-30)

- **RETRIEVAL-DIVERSITY-001** — post-rerank near-duplicate suppression.
  RQA cluster D. Live q04 (2× pre-bash-check + 2× sql) → 3 diverse
  results (both duplicates dropped). ⚠️ **Rule pinned**: diversity-
  filter design intent = "prefer diverse coverage over completeness"
  — `MinOutput=1` default means topK=5 → 3 diverse > 5 with dups.
  Sprint dir: `docs/development/retrieval-diversity-001/`.

- **RETRIEVAL-LAYER-BALANCE-001** — RQA cluster C. Two-phase (E1
  concrete-recall pool addition + E2 quota promoter that bypasses
  rerank). Live q14 "CUIDv2 rules": baseline 0/5 concrete → top slot
  = `L1 constraint "[must] Always use CUIDv2 for identifiers"`
  (the actual answer). Guards q10 4/5→5/5, q08 unchanged. ⚠️
  **Rule pinned**: when a retrieval intervention aims to surface a
  specific candidate class, first identify WHICH STAGE filters it
  out (recall / RRF / rerank / truncation) — interventions at
  earlier stages CAN'T fix problems caused by later stages; the LLM
  rerank overrides fused scores entirely. Sprint dir:
  `docs/development/retrieval-layer-balance-001/`.

- **RETRIEVAL-REVERSE-LOOKUP-001** — RQA cluster A. Filesystem-grep-
  at-query-time (Go-native, NO shell-out) + post-rerank quota
  injection. Live q11 "what consumes constraint_outcomes?":
  baseline 1/10 consumers → 3/10 (including `GuidanceEffectiveness`
  function). ⚠️ **Three rules pinned**: (1) when the answer isn't
  in the substrate, no substrate-side indexing helps; (2) the
  reverse-ref quota MUST be query-shape gated; (3) per-file HitCount
  CAP=3 flattens the writer-bias. Sprint dir:
  `docs/development/retrieval-reverse-lookup-001/`.

### PHASE 3 — Guidance Quality Cluster (JIMINY-CEILING follow-ups; 2026-07-29 → 2026-07-30)

- **JIMINY-CORPUS-002** — Ceiling defect **A**. 5 new patterns in
  `ConstraintPromotionGate` + retroactive tombstone of 5 confirmed
  non-rule constraint nodes on mdemg-dev. Live constraint count
  64→59. Fully reversible (`is_archived=true`, nodes preserved).
  ⚠️ **Rules pinned**: new reject patterns must be paired with
  negative unit tests using real constraint text; retroactive
  tombstone preferred over delete. Sprint dir:
  `docs/development/jiminy-corpus-002/`.

- **JIMINY-TIER1-BYPASS-001** — Ceiling defect **B**. Bypass tier1
  (embedding-sim) for the follow/ignore decision; keep tier1 only
  for `not_applicable` pre-gate. **Post-flip live**: 100% llm-
  source rows / 0% tier1 (correct — sub-naThreshold NotApplicable
  is filtered from constraint_outcomes). ⚠️ **Rule pinned**:
  embedding-similarity between rule text and action text is
  DEFINITIONALLY blind to follows ("committed to reh3376_dev01"
  follows "never commit to main" but embeddings differ). Sprint
  dir: `docs/development/jiminy-tier1-bypass-001/`.

- **JIMINY-CODE-BACKFILL-001** — DORMANT-CENSUS-002 follow-up #3 /
  JIMINY-CEILING follow-up #4. 221 empty-code rows / 7d = 15.8%
  taxonomy: 175 EXPECTED empty (pattern/concept/learning/decision
  don't map to codified rules), 26 mis-tagged, 21 correction-typed
  against nodes without codes. Ship: `FindConstraintCodeForNode`
  defensive backfill + `empty_code_taxonomy_test.go` pin (disjoint
  partition + every GuidanceType classified — CACHE-KEY-002-shape
  forcing function). ⚠️ **Rule pinned**: empty constraint_code is
  EXPECTED for 8 of 9 GuidanceTypes; do NOT filter as data holes.
  Sprint dir: `docs/development/jiminy-code-backfill-001/`.

- **CORRECTION-CODE-GEN-001** — JIMINY-CODE-BACKFILL-001 follow-up.
  New `BootstrapCorrectionCodes` mirrors the shipped `BootstrapCodes`
  for constraints. **Live mdemg-dev: 35/35 correction nodes now
  carry codes** (0/35 pre-sprint). Sample: `always-use-cuidv2`,
  `always-follow-12-section-format` (mnemonic) + `auto-<hash>`
  fallbacks. Two downstream lookups widened to `role_type IN
  ['constraint','correction']`; taxonomy pin flipped
  `GuidanceCorrection` from without-codes to with-codes. ⚠️ **Rule
  pinned**: when B3 startup goroutine bootstraps across two
  categories, each phase gets its OWN context (shared 60s starved
  the second phase — live-caught 11/35 codified on first attempt).
  Sprint dir: `docs/development/correction-code-gen-001/`.

### PHASE 4 — Data Hygiene + Build-Time Drift Checks (DORMANT-CENSUS family; 2026-07-29 → 2026-07-31)

- **DORMANT-CENSUS-002** — TSDB writer↔reader inventory + forcing
  function. 26 tables adjudicated; 24 IN_USE + 2 DORMANT_TO_REMOVE
  (`ft_benchmarks`, `ft_hitl_decisions`). Merge-blocking CI drift
  check. Column-level dormant surface disclosed: `review_grades.
  suggested_guidance` populated 11% with ZERO code readers →
  spawned follow-up #7. Sprint dir:
  `docs/development/dormant-census-002/`.

- **REVIEW-SUGGESTED-GUIDANCE-CONSUME-001** — DORMANT-CENSUS-002
  follow-up #2 (column-level). `scripts/curate_guidance_corpus.py`
  now consumes the dormant `review_grades.suggested_guidance`
  column, emitting synthetic corpus rows length-gated at 40 chars
  (default). **Live mdemg-dev**: 173-char config.go SME rule
  emitted; raised gate (500 chars) correctly skipped it via
  `sme_suggestions_skipped_short` counter. Also fixed a pre-existing
  curator bug (`_table_exists("review_grades")` was too coarse —
  the code assumed a scalar `gold_outcome` column that never landed;
  schema stores it in `gold_dimensions` JSONB — curator crashed
  every run). ⚠️ **Rules pinned**: capability detection at the
  COLUMN level not the TABLE level; dormant-column consumers must
  be length-gated. Sprint dir:
  `docs/development/review-suggested-guidance-consume-001/`.

- **FT-DORMANT-CLEANUP-001** — DORMANT-CENSUS-002 follow-up #1.
  V0032 migration DROPs `ft_benchmarks` (superseded by
  `benchmark_runs`+`benchmark_results` V0012) + `ft_hitl_decisions`
  (superseded by `review_grades` V0028). Schema version 31→32.
  ⚠️ **Rule pinned**: do NOT rewrite historical migration files;
  DROP-only migrations add a new file. Sprint dir:
  `docs/development/ft-dormant-cleanup-001/`.

- **DORMANT-CENSUS-003** — DORMANT-CENSUS-002 disclosed follow-up
  #4. Extends the DORMANT-CENSUS pattern to metrics-registry
  gauges. 152 declared metrics; both false-positive classes handled
  (histogram-derivative expansion for `_p95`/`_p99`/`_bucket`/`_sum`/
  `_count`; snapshot-reader recognition). Two disposition classes:
  IN_USE + IN_USE_TSDB_ONLY (the recorder writes every metric to
  `metric_samples`, so nothing is truly dormant just because no
  dashboard reads it). Zero UNREVIEWED at first-pass adjudication.
  Sprint dir: `docs/development/dormant-census-003/`.

- **DORMANT-METRICS-CLEANUP-001** — DORMANT-CENSUS-003 follow-up.
  Two-gate HARD-DEAD rule (zero writer sites in `internal/**/*.go`
  AND zero samples/7d). **7 metrics removed** (`cms_dedup_skips_total`,
  `cms_learning_edge_failures_total`, `cms_recall_total`,
  `cms_resume_total`, `jiminy_guide_timeout_total`,
  `retrieval_cache_hits_total`, `retrieval_cache_misses_total`).
  11 wired-but-quiet kept as IN_USE_TSDB_ONLY. Live-caught during
  CI cycle: 3 UOBS/UOTS specs still asserted the removed metrics
  → fixed via a follow-up spec-scrub commit. ⚠️ **Rule pinned**:
  HARD-DEAD requires BOTH gates — either alone misses cases.
  Sprint dir: `docs/development/dormant-metrics-cleanup-001/`.

- **METRICS-VERIFIER-UOBS-UOTS-001** — DORMANT-METRICS-CLEANUP-001
  CI-cycle disclosed follow-up. Verifier's `CONSUMER_ROOTS`
  extended with UOBS + UOTS spec paths; new auto-promotion:
  IN_USE_TSDB_ONLY → IN_USE when a spec references the metric
  (one-directional; operator-set DORMANT_* wins). 4 correct
  auto-promotions (`neo4j_graph_total_spaces`, `probe_latency_ms`,
  `tsdb_pool_*`). Distribution: 92→96 IN_USE, 53→49 IN_USE_TSDB_
  ONLY. ⚠️ **Rules pinned**: when removing a metric, still scan
  UOBS/UOTS specs; auto-promotion is one-directional. Sprint dir:
  `docs/development/metrics-verifier-uobs-uots-001/`.

### PHASE 5 — Capability Closeouts (RELEASE-HYGIENE-001, GO-IMPLEMENTS-001; 2026-07-28 → 2026-07-31)

- **RELEASE-HYGIENE-001** — Q4-frontier candidate #5 shipped as two
  paired closeouts. E1 RELEASE-PIN: `docker-publish.yml` never
  triggered directly on tag pushes → added `push: tags: ['v*']`
  trigger. E2 GRAFANA-SHIP: fresh Homebrew installs got blank
  Grafana because compose mounts pointed at paths that only exist
  in a cloned repo → new `internal/cli/grafana_templates` package
  `//go:embed all:staged`s a mirror. Idempotent + operator-edit-safe
  (sha256 compare — files with different content preserved, not
  overwritten). New CI check `make verify-grafana-embed` fails on
  drift. ⚠️ **Rules pinned**: `//go:embed` can't cross package
  boundaries; when a compose service mounts cwd-relative paths, the
  operator's cwd MUST contain those paths after `mdemg init`.
  Sprint dir: `docs/development/release-hygiene-001/`.

- **GO-IMPLEMENTS-001** — Q3-deferred capability finally shipped
  (see feature doc `docs/features/go-implements.md` if extended
  further; sprint dir `docs/development/go-implements-001/`). Real
  `go/types`-backed analyzer replaces the nil-stub. **Live mdemg-
  dev**: 96 packages / 109 interfaces / 1464 concretes → 267 pairs
  in 472ms → **188 IMPLEMENTS edges landed in Neo4j** (2→188).
  Structural RRF column + edge-attention (IMPLEMENTS weight 0.70)
  now have real Go data. Sample edges verified real (`Server
  IMPLEMENTS DevSpaceServer`, gRPC embed patterns). New CLI
  `mdemg symbols analyze-go-implements --root <path> --space-id
  <id>` with `--dry-run`. ⚠️ **Three rules pinned**: Go IMPLEMENTS
  is implicit and tree-sitter CAN'T detect it; `relativizePath`
  MUST produce the leading-slash shape; filter empty interface
  before pairing.

### One flake fix worth noting

- **SCRAPER-TEST-TX-VISIBILITY-001** — race-fix for
  `TestStore_CreateIngestedAsRelationship` (setup used `sess.Run`
  inside deferred-close session; MemoryNode CREATE only committed
  at session close, AFTER `CreateIngestedAsRelationship`'s MATCH
  ran → silent read-your-writes race). Fix: use `sess.ExecuteWrite`
  which commits synchronously. Pre-existing bug exposed by PR 563
  CI cycle. Bundled into DORMANT-METRICS-CLEANUP-001's follow-up
  commit.

### Q4 shipping totals

- **~22 sprints shipped this quarter** (all merged to `main`; PRs
  554-565 in the operator-directed post-audit arc + ~9 in the
  pre-arc Q4-frontier top-6 window)
- **25+ rules pinned** to `CLAUDE.md` (durable operator-facing
  invariants across retrieval, guidance, hygiene, and CI)
- **3 new build-time drift checks live** (routes, TSDB tables,
  metrics) — DORMANT-CENSUS family complete for known declaration
  surfaces
- **Q4-frontier deep-dive's 6 top candidates**: all shipped +
  spawned 13 follow-ups all shipped + 5 CI-cycle-disclosed side
  fixes all shipped
- **1 Q3 capability deferral closed**: GO-IMPLEMENTS-001

---

## 3. Ranked next-quarter sprints

Q4's arc naturally emptied its priority queue. The candidates below
are the deferred residuals + Q3 rollovers + Q4-execution-disclosed
follow-ups.

| # | Sprint | Effort | Impact class | One-line justification |
|---|---|---|---|---|
| 1 | **PLUGIN-HYGIENE decision** | 1-2d after decision | operational (production-deployability) | Named Q3 deferral, still open — the compose template forwards zero PLUGINS_/SCRAPER_ env vars and Dockerfile.prod ships no plugins dir; plugins + scraper are undeployable in the documented-primary Docker deployment. Needs operator disposition call BEFORE code work (ship first-party plugins in image + forward env, OR document native-only + return actionable `/v1/scraper` error). |
| 2 | **CONTEXT-LIVE-001** | 5d | stretch (benchmark-parity) | Q3 stretch tier; still valid. Phase-B fingerprint refresh (`RefineWithCoactivations` zero callers; 76,906 nodes on v1 vs v3 queries); version-guard ContextColumn; default server-side fingerprint derivation; QueryClassifier→category dispatch; fix consensus denominator; 120q UVTS A/B gate. |
| 3 | **FT-RECURSIVE-004 → FT-RECURSIVE-005** integration expansion | ~4d | direct (feed the recursive-retrain loop) | The FT-RECURSIVE-004 base already ships; the natural next step is broadening the drift-triggered retrain to more benchmark task groups + surface a promotion dashboard tile. Concrete follow-up from DRIFT-TRIGGER-001's disclosed sprint list. |
| 4 | **HITL-CURATION-003** — extension paths | ~2-3d | direct (corpus quality lever) | HITL-CURATION-002 shipped the auto-grading substrate; capacity for next-step curation datasets (guardrail rows already gold-labeled 59+, correction bridge drafts, contradicted-drafts) is now the operator's most-effective corpus quality lever. |
| 5 | **HEBB-ETA-001** (idea 02) | 5d | research (PC-ladder gateway) | Q3 stretch; sequencing behind SURPRISE-TOPK-001 + CoactivateSession semantics fix (both shipped Q3). Precision-weighted Hebbian η behind a flag; unlocks unreachable CONTRASTS_WITH/COMPOSES_WITH. |
| 6 | **GO-IMPLEMENTS-002 — audit the 267→188 gap** | 1-2d | direct (analyzer honesty) | Q4 disclosed follow-up. 79 pairs' source or target isn't in the SymbolNode graph; categorize (proto-generated / test-only / ingest filter) + either accept the ingest filter as-is with docs, or widen ingest to include the missing files. |
| 7 | **JIMINY-FOLLOW-RATE-REMEASURE** — passive audit | 1d | measurement (verify the ceiling arc) | 3-7d post JIMINY-CORPUS-002 / TIER1-BYPASS / CORRECTION-CODE-GEN → re-run the S3 sample categorization; verify the honest 15-25% steady-state prediction is right; document the actual ceiling. |
| 8 | **STRICT-SCOPE** | 2d | operational (dormant-risk today) | Q3 deferral. Global `~/.mdemg/.jiminy-strict-mode` arms /strict for ALL concurrent conversations; `pre-write-check.py` hardcodes `mdemg-dev`. Trigger: first multi-session /strict use. |
| 9 | **FG-2 carry-forward extraction** | ~3-4d | strategic (Q3-flagged neglected workstream) | Highest-leverage neglected strategic workstream at ~3/5 fork readiness per Q3 doc. Depends on DOC-TRUTH-001 (shipped Q3) — no prerequisite blocker remains. |
| 10 | **RETRIEVAL-QUALITY-AUDIT-002 or cross-space audit** | 3d | research (retrieval-quality validation) | RQA-001 disclosed follow-up #6 — same 15 queries against `whk-wms` or `lnl-demo-whk` to check whether Q4 retrieval fixes generalize beyond mdemg-dev. |

*Next in line (small):* the disclosed follow-ups from METRICS-VERIFIER-UOBS-UOTS-001, GO-IMPLEMENTS-001 (auto post-ingest hook), and the automatic per-space UVTS re-run gate.

---

## 4. Explicitly deferred (updated from Q3)

Q3's explicit-deferral list is largely still accurate; Q4 closed
some and left others deferred with unchanged rationale.

**Closed this quarter** (either shipped or superseded):
- ✅ **GO-IMPLEMENTS-001** — SHIPPED (2026-07-31, this quarter)
- ✅ **DORMANT-CENSUS-002 → -003** — SHIPPED both passes
- ✅ **HIDDEN-CHURN-003** — SHIPPED late Q3 (per Q3 §5)
- ✅ **FT recursive-retrain loop Phase 6a/6b/7/9** — 6a/6b SHIPPED
  Q3, 7 SHIPPED Q4-adjacent (per drift-trigger-001 tie-in), 9
  base-shipped

**Still explicitly deferred with unchanged rationale**:
- **PLUGIN-HYGIENE** — waiting on operator disposition
- **CONTEXT-LIVE-001**, **HEBB-ETA-001** — stretch tier; prerequisites
  now met (score-scale stable; SURPRISE-TOPK + CoactivateSession
  shipped), so both are genuinely next-quarter-shippable
- **STRICT-SCOPE** — trigger not yet fired
- **FG-2 carry-forward extraction** — strategic, deferred by choice
- **Embedding fine-tune workstream** — waits on stable retrieval
  baseline (post CONTEXT-LIVE)
- **Note 09 outreach** — operator-judgment, not engineering
- **HOOKSRV-001 (server-side hook orchestration)** — waiting for
  HOOKWIRE/HOOKSYNC stability window
- **AUTH-USABLE-001 / AUTH-SCOPE-001 full sprints** — quick fixes
  shipped Q3; full auth model still deferred (localhost single-
  operator, no silent-failure mechanism)
- **HYGIENE-SWEEP-001 batch** — cosmetic tier
- **JIMINY-RRF-001** — enabling; deferred behind direct-impact

**New explicit deferrals from Q4 execution**:
- **60→49 IN_USE_TSDB_ONLY residual** (post DORMANT-METRICS-
  CLEANUP-001) — keep as-is; removing loses error/failure signal
  (the sprint's own pin)
- **Automatic post-ingest hook for GO-IMPLEMENTS-001** — operator-
  invoked CLI is sufficient at current cadence; wire only if
  IMPLEMENTS-driven retrieval quality proves out

---

## 5. Post-roadmap follow-ups (this quarter's disclosures)

New sprints surfaced by Q4 execution itself; will be recategorized
in the Q5/2026 roadmap. All small (≤2d).

- **GO-IMPLEMENTS-002** — audit the 267→188 emitted-vs-landed gap
  (79 pairs' symbols aren't in the SymbolNode graph); disposition:
  document + accept, or widen ingest
- **HITL analytics tile** — DORMANT-CENSUS-002-family idea; a
  Grafana panel over `review_grades` for grade-cadence + auto-vs-
  operator split visibility
- **JIMINY-FOLLOW-RATE-REMEASURE** — 3-7d post-arc passive
  categorization to verify the honest 15-25% steady-state prediction
- **UVTS "who implements X?" retrieval-quality re-baseline** — now
  that Go IMPLEMENTS edges exist, measure the before/after lift on
  interface-related queries
- **DORMANT-METRICS-CLEANUP-002** — future review pass on the 11
  wired-but-quiet metrics kept as IN_USE_TSDB_ONLY (only if 30+
  days of continued zero-emission argues for removal; information
  cost is real)

---

## Annex — Q4 rules pinned (durable operator-facing invariants)

Below is the running list of rules pinned to `CLAUDE.md` this
quarter. Each has a canonical "when this class of problem recurs,
this is the fix" statement.

**Retrieval class**:
- When a retrieval intervention aims to surface a specific candidate
  class into top-K, first identify WHICH STAGE filters it out (recall
  / RRF fusion / rerank / truncation) — earlier-stage interventions
  can't fix later-stage causes (LLM rerank overrides fused scores)
- When the answer isn't in the substrate (function-level summaries
  don't index SQL/symbol strings from bodies), no substrate-side
  indexing helps — the fix must reach an authoritative external
  source
- Reverse-ref quota promoter MUST be query-shape gated (`IsReverseLookupQuery`);
  firing on every query displaces natural top-K with irrelevant grep matches
- For reverse-lookup grep ranking, CAP per-file hit count at a small
  constant (writer bias otherwise)
- Diversity-filter design intent = "prefer diverse coverage over
  completeness" — MinOutput=1 default is deliberate

**Guidance class**:
- Empty constraint_code is EXPECTED for 8 of 9 GuidanceTypes; do NOT
  filter as data holes
- Adding a new GuidanceType enum requires updating one of the two
  taxonomy maps in the same PR (pin-tested)
- Tier1 embedding-sim is DEFINITIONALLY blind to follows (`committed
  to reh3376_dev01` follows `never commit to main` but embeddings
  differ); tier1's correct role is only the `not_applicable` pre-gate
- New reject patterns must be paired with negative unit tests using
  real constraint text that MENTIONS the junk keywords
- Retroactive tombstone preferred over delete for corpus cleanup
- B3 startup goroutine bootstraps get their OWN context per phase
  (shared budget starves the second phase)

**Hygiene / drift-check class**:
- Capability detection at the COLUMN level, not the TABLE level
  (schema evolves to store values in JSONB)
- Dormant-column consumers must be length-gated (free-text carries
  mixed content)
- HARD-DEAD metric verdict requires BOTH zero writer sites AND zero
  7d samples — either alone misses cases
- When removing a metric declaration, DELETE the inventory entry
  (metrics differ from TSDB tables — no historical migration files)
- When removing a metric, scan UOBS/UOTS specs for orphaned
  assertions in the same PR
- Auto-promotion (IN_USE_TSDB_ONLY → IN_USE) is one-directional;
  operator-set DORMANT_* wins
- Do NOT rewrite historical migration files; DROP-only migrations
  add a new file
- REMOVED disposition requires verifier vocabulary extension

**Go-analysis class**:
- Go IMPLEMENTS is implicit; tree-sitter CAN'T detect it — route
  through `GoTypesAnalyzer`
- `relativizePath` MUST produce the leading-slash shape the tree-
  sitter ingest uses (silent-drop class if it drifts; pin-tested)
- Filter empty interface (`any`) before pairing (N×M noise class)

**Meta class**:
- When bypassing a fast-path classifier, verify the downstream path's
  cost + latency lift is small
- Burst-vs-chronic distinction — a deep-dive can be right about the
  CLASS of failure while wrong about the TEMPO
- When predicting a rate lift from denominator filtering, model the
  numerator's volume-coupling too (fixed-numerator arithmetic
  overpredicts)
- A "wash" A/B is a legitimate outcome; strict mean gate applying
  parity-within-noise IS a "no-flip" verdict

---

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q3.md` (format template)
- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (Q4 arc anchor)
- `CLAUDE.md` (pinned rules)
- `CHANGELOG.md` (shipped sprints, Q4 window)
- All Q4 sprint post.md files under `docs/development/`
