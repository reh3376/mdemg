# MDEMG ROADMAP — Q5 2026 (formalized 2026-09-20)

<!-- Generated 2026-09-20 as a retrospective + forward-looking synthesis of
     the Q5 arc. Q4 was formalized 2026-08-01 (post-frontier-scoping deep
     dive); Q5 covers 2026-08-01 → 2026-09-20, ~7 weeks. Shipping window:
     PR #611 (2026-08-12) through PR #669 (2026-09-19), ~58 PRs. Rules
     pinned to CLAUDE.md this quarter: 55+. The Q4 verdict ("substrate
     mature; ship what the plumbing is FOR") held — Q5 executed the
     "measurement honesty" arc the Q4 doc's §3 top-item flagged, closed
     the retrain-vs-substrate-quality gate decision, and delivered the
     Path 2 process-observation platform the design spec called for. -->

## 1. State-of-the-System Verdict

Q5 was **the follow-rate arc quarter**. The operator directive 2026-08-11
("current sub-50% actionable follow rate is a substrate-quality failure,
not a design steady-state — build the arc that owns >80%") reframed
Q4's "honest 15-25% band" as a normalization violation and created
JIMINY-CEILING-BREAK-2 as the master arc. Q5's shipping cadence was
dominated by this arc + its investigation-produced pivots.

**What the arc actually taught us**: the follow-rate ceiling isn't a
classifier-quality problem. It's a **rule-verifiability class mix**
problem — 97% of graded rows in the 168h window were meta/process
rules the classifier CANNOT verify from action-text alone
(JIMINY-CEILING-INVESTIGATION-002, 2026-09-03). No amount of prompt
tuning, retrieval tightening, or LLM retraining moves an aggregate
metric bounded by the class mix. This produced the pivot that shaped
the second half of the quarter:

1. **Design spec** (#157, JIMINY-METRIC-DENOMINATOR-DESIGN-001,
   2026-09-07) — 33-rule taxonomy audit; recommended Path 3 (metric
   partition by verifiability class) then Path 2 (process observers)
   incrementally.
2. **Path 3 shipped** (#158, JIMINY-METRIC-PARTITION-001, 2026-09-10) —
   `verifiability_class` property on constraint/correction nodes;
   4 new class-partitioned gauges; live classifier-only rate = 25.2%
   (vs dishonest aggregate 16.74%).
3. **Retrain gate decision** (#159, PHASE-4B-GATE-DECISION-001,
   2026-09-11) — **DEFER**. Classifier follow rate 17.74% (n=234/30d)
   sits inside a narrow 15-26% cross-class band. Retrain would move
   classifier UP by ~20pp optimistic → ~38%; aggregate moves maybe
   +5pp. Signal density too low (~8/day) for statistical detection
   of a ±5pp lift for ≥30 more days.
4. **Path 2 shipped in full** (JIMINY-PROCESS-OBSERVER-{01..06} arc,
   2026-09-18 → 2026-09-19) — 6 observers, one platform, ~90 LOC per
   matcher, zero platform-shape changes across observers 02-06.
   Every process-verifiable + hybrid rule from the design spec has a
   live grader; `mdemg_jiminy_follow_rate_process_verifiable` has 6
   producers instead of 0.

**Verdict as of 2026-09-20**: the JIMINY-CEILING-BREAK-2 arc has
executed the mechanism the design spec called for. The remaining
delta between "shipped mechanism" and "moved metric" is a **timer
problem**, not a code problem — organic rows need to accumulate
across classes for statistically valid re-measurement. Q4's
15-25% "honest band" framing was correct at the aggregate but
wrong-metric — the honest denominator is per-class, not aggregated,
and the classifier-only class is measurable and improvable in a way
the aggregate is not.

**Q5-shipped strategic capability**: **the process-observation
platform**. 2 hypertables (V0036), 1 endpoint (`POST /v1/process/event`),
1 supervised grader loop, 6 matcher files, 13 config knobs. Marginal
cost of a new observer is 2-3h (proven across observers 02-06). The
next class of durable rule (`sequential-epics`, `never-haiku-for-planning`,
etc.) that couldn't be classifier-graded now has a native
grading path.

**Q5-shipped strategic hygiene**: three UI-adjacent lift-quality
sprints. **JIMINY-HITL-VELOCITY-001** (#141, 2026-08-12) — keyboard-
driven bulk review, ~0.5→40+ items/session. **JIMINY-RULES-UI-001**
(#149, 2026-08-13/14) — dedicated /ui/rules tab; produced the
JIMINY-CORPUS-AUDIT-004 27→31 rewrite batch in production. **PRE-COMMIT-
INVENTORY-GATE-001** (#150, 2026-08-13) — Jiminy fail-CLOSED at git
commit for missing DORMANT-CENSUS adjudications, closing the class
that broke PR #614 and PR #615 at merge-time CI.

**Q5-shipped strategic corpus work**: **corpus curation as the highest-
leverage lever below 40% follow rate**. JIMINY-CORPUS-003 (64→33
constraints), JIMINY-CORRECTION-CORPUS-001 (39→3 corrections),
JIMINY-CORPUS-AUDIT-004 (Fable adjudication → 6 tombstones + 7
rewrites), JIMINY-INFORMATIONAL-CATEGORY-001 (18 human-class rules
marked), JIMINY-CEILING-INVESTIGATION-002-execution Paths 1+4
(9 process-class rules marked informational, META-SCOPE flag flipped
ON). Aggregate corpus went from 72 mixed rules to a curated
26 actionable constraints + 7 informational + 2 actionable corrections
+ 1 informational correction.

**Q5-shipped strategic security**: SEC-TRANCHE-2 (path injection,
`internal/pathsafe.SafeJoinUnderDir`) closed 15 HTTP-reachable
CodeQL alerts; SEC-TRANCHE-3 closed 33 more via structural fix +
dismissal across regex/URL/XSS/allocation/overflow classes.

**Standing residuals** (deliberate; each has a documented trigger):
- **PLUGIN-HYGIENE** — same rationale as Q4; waiting on operator
  disposition call.
- **JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001** — filed by design spec
  #157 as Path 2's other half. The 18 human-class rules have no
  writer to their gauge; needs HITL grading integration. Design
  gap, not sprint gap.
- **HEBB-ETA-001 wire-up** — primitives shipped default-off Q4;
  hot-path observe→confidence-update, RSIC adaptive-override
  action, and A/B benchmark remain (deferred; low priority behind
  arc work).
- **FG-2 carry-forward extraction** — Q4 explicitly named neglected
  strategic workstream; still 3/5 fork readiness. No prerequisite
  blocker remains but no active pressure either.
- **STRICT-SCOPE** — trigger (first multi-session /strict use) still
  hasn't fired.

---

## 2. Phases (what SHIPPED)

Q5's shipping arc was more coherent than Q4's — the JIMINY-CEILING-BREAK-2
master arc gave the quarter a spine, with security + UI-quality +
platform-hygiene work orbiting around it. Below grouped thematically.
All sprint dirs under `docs/development/`.

### PHASE 1 — JIMINY-CEILING-BREAK-2 arc: corpus + retrieval + classifier + UI velocity (2026-08-11 → 2026-08-14)

Anchor operator directive 2026-08-11: **sub-50% follow rate is a
substrate-quality failure, not steady-state — build the arc that
owns >80%**. Sequential phases:

- **JIMINY-CORPUS-003** (Phase 1, 2026-08-11) — 64→33 live constraints
  via 31 tombstones (15 JUNK, 2 STALE, 14 DUPLICATE across 8
  clusters). Reversible via `is_archived=true` + `archive_reason`.
  Corpus curation as highest-leverage lever below 40% follow rate.
- **LEVER-C-TIGHTEN-002** (Phase 2, 2026-08-12) — scope-gate filter
  on retrieval, `internal/jiminy/scope_gate.go`. Data-decided from
  TSDB: similarity distribution for followed/ignored OVERLAPS →
  similarity is NOT a discriminator, action-context match is. 9
  scope families derived from tokens in item Content + request
  context; safe-defaults on both sides.
- **JIMINY-CLASSIFIER-CONTEXT-002** (Phase 3, 2026-08-12) —
  `mechanismScopeCreditClause` hard-precedence gate. When action-
  text doesn't contain the constraint's mechanism-verb, routes to
  `not_applicable` unconditionally. Preserves ULTS `system_prompt_hash`
  pin via byte-identical default-off render.
- **JIMINY-HITL-VELOCITY-001** (Phase 4a, 2026-08-12) — keyboard-
  driven bulk-review UI. `0..4` set every dimension; Space/Enter
  submit; auto-advance with visible cancellation window. Bottleneck
  was operator throughput (~0.5 items/day); post-sprint 40+ items/session.
- **HITL-AUTOGRADE-PREVIEW-001** (2026-08-12) — deferred MVP follow-up.
  `POST /v1/review/autograde-preview` with UI pre-fill for expensive-
  to-compute artifacts.
- **FRAMING-HYGIENE-SWEEP-001** (2026-08-12) — 4 Grafana panels + 5
  pre-arc sprint docs annotated. Retired "honest by design"
  language on aggregate follow rate; embedded arc name in panel
  descriptions.
- **CREATE-CORRECTION-DEDUP-001** (2026-08-12) — cross-role dedup at
  `CreateCorrectionNodes` promotion time. Prevents the JIMINY-
  CORRECTION-CORPUS-001 DUP class from re-accumulating; 0.75
  cosine threshold.
- **JIMINY-CORRECTION-CORPUS-001** (arc-adjacent, 2026-08-12) —
  parallel audit of 39 corrections that Phase 1 missed (constraint-
  only); 39→3 live via 35 tombstones. Expected mechanical lift
  much larger than Phase 1 because corrections drove the DOMINANT
  share of ignored outcomes.
- **JIMINY-INFORMATIONAL-CATEGORY-001** (arc-adjacent, 2026-08-12) —
  `is_informational` property; `mdemg jiminy constraint mark`. Meta/
  epistemic directives route to `not_applicable`, out of the follow-
  rate denominator.
- **ARC-TRAJECTORY-PANEL-001** (2026-08-12) — live-updating trajectory
  panel; embed baseline + target + re-check windows + anti-
  normalization rule in the panel itself.
- **JIMINY-RULES-UI-001** (2026-08-13→14) — /ui/rules tab, 4 endpoints
  under `/v1/jiminy/rules/*`, tombstone-and-recreate semantic
  preserving round-1 immutable-tombstone lock. 4 arch rules pinned
  (UI-completion contract, dashboard contrast, accordion CUIDv2
  keying, rich flag-gated error surface).
- **JIMINY-CORPUS-AUDIT-004** (2026-08-14) — Fable 5 sub-agent (2.7min
  wall-clock) adjudicated 37 live rules; operator applied 6
  tombstones + 1 metadata fix + 7 content rewrites via the shipped
  JIMINY-RULES-UI-001 Save flow (validated the round-1 lock in
  production).
- **JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001** (2026-08-14) — closes
  the dual-severity dual-mint class. Detector collapses to a SINGLE
  canonical emit via severity precedence.
- **JIMINY-CLASSIFIER-META-SCOPE-001** (2026-08-14, SHIP-DORMANT) —
  mention-vs-perform disambiguation. Live smoke revealed 0
  measurable delta; shipped as regression insurance, flag stays OFF
  in code; flipped ON via .env by CEILING-INVESTIGATION-002
  execution 3.5 weeks later.

### PHASE 2 — HITL platform hardening + review-flow correctness (2026-08-13)

The Fable HITL bulk-grade sessions in Phase 1 surfaced 5 review-
flow defects; all closed same-day.

- **REVIEW-GRADE-VALIDATE-ITEM-ID-001** — `POST /v1/review/grade`
  captures `d.FetchItem`'s `found` return; 404 with named class on
  unknown item_id. Closes stealth-orphan-write class.
- **REVIEW-CANDIDATES-DEDUP-409-001 + REVIEW-CANDIDATES-EXCLUDE-ERROR-
  ROWS-001** — contradicted_drafts LEFT JOIN + LLM error/empty
  filtering. Two coupled fixes in one commit.
- **REVIEW-GRADE-NOTES-FIELD-001** — V0034 adds `notes TEXT NOT NULL
  DEFAULT ''` to `review_grades`; `readJSON` decode errors surface
  the offending field name (cross-handler win).
- **RERANK-LENGTH-STRICT-001** — rerank determinism (T=0.0), length-
  mismatch observability, corrective retry. 14/40 rows had been
  silently poisoned; fix applies to openai + ollama provider paths.
- **SCRUB-IDEMPOTENT-001** — `envSecretPattern` value class narrowed
  (excludes `)` and `]` — was blocking every export-auto forever
  on the affected row class). Two arch rules pinned (idempotency;
  multi-branch CLI handlers must not early-return).

### PHASE 3 — Security tranches (2026-08-11 → 2026-08-12)

- **SEC-TRANCHE-2** (path injection, 2026-08-11) — new `internal/
  pathsafe.SafeJoinUnderDir` leaf package. CodeQL doesn't recognize
  regex validation as sanitizer; requires `filepath.Clean` +
  `strings.HasPrefix` containment. Wired at 8 HTTP-reachable sites
  (plugin_handlers, scaffold, backup service + full + partial). 13
  alerts dismissed with rationale; 6 pathsafe pin tests.
- **SEC-TRANCHE-3** (code scanning tranche, 2026-08-12) — 33 open
  CodeQL alerts across 8 rule classes. 11 dismissed with per-alert
  rationale; 21 structural fixes shipping in code; 1 deferred (arc
  no-touch window). 7 arch rules pinned (regex `\b` anchors,
  URL-scheme deny-list, `urlparse().hostname` for host checks,
  isSensitive widening, defensive allocation caps, int32 bounds-
  check, DOM sink CUIDv2 validation).

### PHASE 4 — Post-arc iteration + investigation + design (2026-08-19 → 2026-09-11)

The JIMINY-CEILING-BREAK-2 arc's T+168h passive re-check landed at
**16.74%**, not the 22-35% predicted. The next 3 weeks were
diagnostic + design:

- **PHASE-E1-CORPUS-AUDIT-001** (2026-08-19) — Claude-Code FT corpus
  is ~81% substrate-covered. 2,706-row v2 audited via automated
  retrieval: 2,203 PROVEN_COVERAGE + 503 SUBSTRATE_MISS + 0 error.
- **PHASE-E2-CORPUS-CURATION-001** (2026-08-19) — 503-row stripped
  corpus + manifest. Byte-verbatim preservation of kept rows;
  leak audit CLEAN.
- **HOMEBREW-INSTALLER-QWEN-UPDATE-001 Phase B** (2026-08-19) —
  ModelName-guarded SHA verify + operator publish runbook. Latent
  SHA-verify bug closed BEFORE operator publishes v2.
- **BETA-DOCS-MODEL-VERSIONING-001** (2026-08-19) — doc-only sprint
  explaining v1 vs v2, honest about the shipped activation
  mechanism (edit `.env` `MDEMG_MODEL_PATH` + kickstart, no
  `mdemg model use`).
- **MDEMG-DOCS-INGEST-002** (2026-09-02) — filesystem walker deny-set.
  Live-caught 1 leaked idna LICENSE.md MemoryNode. Retroactive
  tombstone in the follow-up.
- **MDEMG-USAGE-CORPUS-CURATE-002** (2026-09-03) — `STARTS WITH` vs
  `CONTAINS` prefix predicate. Live pre-fix leak: 518/1583 (33%)
  from vendored Python symbol nodes.
- **ADAPTER-SWAP-STANDARDIZE-002** (2026-09-02) — SIGTERM/SIGINT
  handler for `mdemg adapter benchmark`. Wrapper cleanup via
  atomic.Bool guard + re-raise pattern.
- **JIMINY-CEILING-INVESTIGATION-002** (2026-09-03) — read-only 7-
  dimension investigation. **Root cause pinned**: 97% of graded
  rows in the 168h window are from 12 meta/process rule codes
  the classifier CANNOT verify from action-text. Aggregate follow
  rate ceiling is bounded by verifiability class mix, not
  classifier quality. Corollary: BEFORE proposing any classifier-
  side lift, audit that the metric denominator is dominated by
  classifier-verifiable rules.
- **JIMINY-CEILING-INVESTIGATION-002-execution** (2026-09-06) — Paths
  1 + 4 shipped. 9 process-class rules marked informational; META-
  SCOPE flag flipped ON in .env.
- **JIMINY-CORRECTION-SINK-INVESTIGATE-001** (2026-09-07) — the
  correction-outcome 691→3 drop is NOT a bug. Traced to the
  deliberate JIMINY-CORRECTION-CORPUS-001 corpus purge; sink is
  functioning correctly.
- **JIMINY-METRIC-DENOMINATOR-DESIGN-001** (2026-09-07) — design
  sprint feeding retrain gate. Full 33-rule taxonomy audit:
  9 classifier + 4 process + 2 hybrid + 18 human. **Recommendation:
  hybrid — ship Path 3 first (~1 day) then Path 2 incrementally.**
- **JIMINY-METRIC-PARTITION-001** (2026-09-10) — Path 3 shipped.
  V0035 migration; `verifiability_class` on constraint/correction
  nodes; 4 new class-partitioned gauges; `mdemg jiminy constraint
  set-class/list-classes/seed-classes`. Live classifier-only rate
  = 25.2% (vs dishonest aggregate 16.74%).
- **PHASE-4B-GATE-DECISION-001** (2026-09-11) — **DEFER retrain**.
  Cross-class band 15.28% → 26.47% is narrow (11pp), classifier-
  independent within noise. Signal density too low for statistical
  detection of a lift for another 30+ days. Recommended: ship
  Path 2 first observer, wait for organic rows, consider corpus
  growth over retrain.

### PHASE 5 — Path 2 process-observation platform + 6 matchers (2026-09-18 → 2026-09-19)

The Path 2 recommendation from #159. Sequential arc, 6 sprints, 4
days wall-clock.

- **JIMINY-PROCESS-OBSERVER-01** (`lint-before-commit`, 2026-09-18) —
  V0036 hypertables (`process_events`, `process_outcomes`); HTTP
  endpoint `POST /v1/process/event`; grader loop; hook emitter
  with 1s connect / 3s max-time budget; `DatasetBuilder.
  GuidanceEffectivenessByClass` UNION-extended. 6 config knobs
  default OFF. Live-caught migration bug: `add_compression_policy`
  requires ALTER TABLE SET before compression policy (mirrors
  V0025's guarded DO $$ block).
- **JIMINY-PROCESS-OBSERVER-02** (`sequential-epics`, 2026-09-19) —
  hook captures commit message via `git log -1 --format=%B`;
  extract `Epic N` via `(?i)\bepic\s+(\d+)\b`; monotonic check.
  Zero schema change.
- **JIMINY-PROCESS-OBSERVER-03** (`query-mdemg-cms-file-paths`,
  2026-09-19) — hook-side emit for retrieval_call + filesystem_
  search. Word-boundary-anchored bash regex prevents pygrep false-
  match. 300s same-session lookup window.
- **JIMINY-PROCESS-OBSERVER-04** (`unit-integration-e2e-docs`,
  2026-09-19) — filesystem-read matcher class. Bounded via
  `PROCESS_MATCHER_UIED_MAX_FILE_BYTES` (200000, floor 4096);
  path fully constructed from regex-validated input under config-
  driven root.
- **JIMINY-PROCESS-OBSERVER-05** (`must-use-uxts-frameworks-
  consistently`, 2026-09-19) — leanest observer yet; metadata-only,
  no cross-table / filesystem access. UxTS detection via `\.u[a-z]+
  \.json$` + path substring `docs/tests/u`.
- **JIMINY-PROCESS-OBSERVER-06** (`never-haiku-for-planning`,
  2026-09-19) — **arc closeout**. Metadata-only from ANTHROPIC_MODEL
  env var. All 6 observers wired platform-wide (`matcher_count=6`).

### Q5 shipping totals

- **~58 PRs merged** (PR #611 → #669)
- **~40 sprint directories** created under `docs/development/`
- **55+ architectural rules** pinned to CLAUDE.md
- **Zero half-finished implementations** — the small-batch cadence
  held across the full arc
- **Substrate mutations**: constraint corpus 140 → 33 → 26 canonical;
  correction corpus 39 → 3; 18 human-class + 9 process-class rules
  marked informational; 2 hypertables added; 1 endpoint added;
  6 matchers shipped

---

## 3. Ranked next-quarter sprints

Q5's arc emptied its priority queue for the JIMINY-CEILING-BREAK-2
line — the remaining lift is a **timer problem**, not a code problem.
The candidates below are the design-spec siblings + Q4 rollovers +
Q5-execution-disclosed follow-ups.

| # | Sprint | Effort | Impact class | One-line justification |
|---|---|---|---|---|
| 1 | **JIMINY-METRIC-PARTITION per-class alerts + panels** | 1d | direct (observability) | Deferred follow-up from #158. 4 class-partitioned gauges emit; retire aggregate `mdemg_jiminy_follow_rate` alert; ship 4 class-specific alerts + Grafana panel row. Highest-leverage close-the-loop item. |
| 2 | **JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001** | 2-3d | direct (fourth class gauge writer) | Design spec #157's Path 2 sibling. 18 human-class rules currently have no writer; the gauge stays at 0. Ships HITL grading integration so the human class contributes to the honest scoreboard. Blocked on no prerequisite. |
| 3 | **PLUGIN-HYGIENE decision** | 1-2d after decision | operational (production-deployability) | Q3+Q4+now-Q5 named deferral. Compose template forwards zero PLUGINS_/SCRAPER_ env vars; Dockerfile.prod ships no plugins dir. Operator disposition call remains the blocker: ship first-party plugins in image + forward env, OR document native-only + `/v1/scraper` actionable error. |
| 4 | **JIMINY-CEILING-BREAK-2 T+30d passive re-measure** | 0.5d | measurement (verify arc trajectory) | 30 days from arc closeout ≈ 2026-10-19. Re-run the classifier-only + process + hybrid + human class rates over the accumulated post-arc window. Verifies whether the 25.2% classifier-only baseline holds and whether the process gauge landed rows. Blocked on time only. |
| 5 | **RETRAIN-GATE-RE-EVALUATE** (PHASE-4B revisit) | 0.5d gate decision + N-day retrain if greenlit | strategic (compute-lift decision) | Depends on #4. #159 DEFER'd on signal density (~8 classifier rows/day). At ~30 days post-Path-2 the density should support statistical detection of a ±5pp classifier-only lift. Re-derive gate math from that window; only trigger retrain if the classifier-only class rate moved and cross-class band remains narrow. |
| 6 | **HEBB-ETA-001 wire-up** | 3-5d | research (predictive-coding B1) | Primitives shipped default-off Q4; disclosed follow-ups: live observe→confidence-update wiring (hot-path change), RSIC adaptive-override action (`disable_precision_eta`), Grafana panel row, A/B benchmark harness + canary, integration test with seeded fixture graph, multi-sample surprise history. |
| 7 | **CORRECTION-DEDUP-EXTEND-TO-CONSTRAINT-SIDE** | 1d | operational (symmetric promoter dedup) | CREATE-CORRECTION-DEDUP-001 shipped cross-role dedup on the correction promoter. Symmetric constraint-side dedup deferred; ship if telemetry shows constraints being re-minted as dups of corrections. Trigger: `SkippedDup > 0` sustained on constraint promoter. |
| 8 | **JIMINY-PROCESS-OBSERVER-07..N** — new observers as needed | 2-3h each | direct (new-rule coverage) | Marginal cost proven across observers 02-06. When a new durable rule is authored whose verifiability class is process or hybrid, mint a new matcher. No arc — this is now steady-state maintenance. |
| 9 | **FG-2 carry-forward extraction** | 3-4d | strategic (Q3-and-Q4-flagged neglected workstream) | Highest-leverage neglected strategic workstream at ~3/5 fork readiness. No prerequisite blocker remains but no active pressure either. Trigger: operator prioritization. |
| 10 | **UVTS "who implements X?" retrieval-quality re-baseline** | 1d | measurement (retrieval lift) | Now that Go IMPLEMENTS edges exist (Q4 GO-IMPLEMENTS-001 + Q5 GO-IMPLEMENTS-002 fix), measure the before/after lift on interface-related queries. Q4 disclosed follow-up. |

*Next in line (small):* per-request lift on the JIMINY-CEILING-BREAK-2
Phase 3.5 META-SCOPE flag now that it's flipped ON in .env; JIMINY-
RULES-UI Playwright regression suite; retrieval-quality cross-space
audit (RQA-002 against whk-wms / lnl-demo-whk to check whether Q5
fixes generalize).

---

## 4. Explicitly deferred (updated from Q4)

**Closed this quarter** (either shipped or superseded):
- ✅ **HITL-CURATION-003** — SHIPPED Q4-end (2026-08-08)
- ✅ **HITL analytics tile** — SHIPPED Q4-end (2026-08-04,
  HITL-ANALYTICS-TILE-001)
- ✅ **GO-IMPLEMENTS-002 — audit the 267→188 gap** — SHIPPED Q4-end
  (2026-08-05)
- ✅ **DASHBOARD-METRICS-DEEP-DIVE-001** — SHIPPED Q4-end (2026-08-05)
- ✅ **JIMINY-FOLLOW-RATE-REMEASURE passive audit** — SHIPPED as
  JIMINY-CEILING-INVESTIGATION-002 (2026-09-03). Re-measured, root-
  caused, produced the design-spec pivot.
- ✅ **FT-RECURSIVE-004 → -005 integration expansion** — deferred
  further (see below); the Q5 arc's PHASE-4B DEFER makes broader
  drift-triggered retrain a lower priority.

**Still explicitly deferred with unchanged rationale**:
- **PLUGIN-HYGIENE** — waiting on operator disposition
- **HEBB-ETA-001 wire-up** — primitives shipped Q4, remaining
  hot-path/RSIC/A-B work still deferred (low pressure)
- **STRICT-SCOPE** — trigger not yet fired (multi-session /strict
  use)
- **FG-2 carry-forward extraction** — strategic, deferred by choice
- **Embedding fine-tune workstream** — waits on stable retrieval
  baseline (already met; awaiting operator prioritization)
- **HOOKSRV-001** (server-side hook orchestration) — waiting for
  HOOKWIRE/HOOKSYNC stability window
- **AUTH-USABLE-001 / AUTH-SCOPE-001 full sprints** — quick fixes
  shipped Q3; full auth model still deferred
- **HYGIENE-SWEEP-001 batch** — cosmetic tier
- **JIMINY-RRF-001** — enabling; deferred behind direct-impact
- **60→49 IN_USE_TSDB_ONLY residual** (post Q4 DORMANT-METRICS-
  CLEANUP-001) — kept as-is per sprint's own pin

**New explicit deferrals from Q5 execution**:
- **FT retrain (Phase 4b)** — PHASE-4B-GATE-DECISION-001 DEFER'd on
  signal density. Re-evaluate ≥30 days post-Path-2. Not sprint-
  ready until organic rows accumulate through mid-October at
  earliest.
- **JIMINY-CLASSIFIER-META-SCOPE-001 flag flip decision** — flipped
  ON in .env by CEILING-INVESTIGATION-002 execution (2026-09-06);
  code default stays OFF (ULTS pin preserved). Passive re-check at
  T+168h from flip; already validated by later shipping.
- **PROCESS-MATCHER-QUERY-CMS-FIRST MVP caveat** — "exact-token /
  CMS-miss fallback" exception not distinguished. Follow-up sprint
  only if false-positive rate proves unacceptable in the passive-
  observation window.
- **CORRECTION-DEDUP symmetric constraint-side** — deferred to
  candidate #7 above; wait for telemetry trigger.

---

## 5. Post-roadmap follow-ups (this quarter's disclosures)

New sprints surfaced by Q5 execution itself; will be recategorized
in the Q6/2026 roadmap. All small (≤2d).

- **JIMINY-METRIC-PARTITION alert/panel round-up** — the primary
  candidate #1 above; deferred follow-up from #158.
- **Class-differential retrain trigger rule** — proposed by
  PHASE-4B-GATE-DECISION-001: retrain only if classifier drops
  >10pp below other classes. Better than the 70% absolute threshold
  the design spec educated-guessed.
- **Signal density gate** — add to future compute-gate decisions
  per PHASE-4B-GATE-DECISION-001 arch rule (b). <10 rows/day
  organic on a metric class = class isn't ready to gate compute-
  heavy work.
- **Add more L0 correction obs** — JIMINY-CORRECTION-SINK-INVESTIGATE-
  001 disclosed follow-up. Optional; grow the L1 correction corpus
  if a future sprint wants a larger correction sink.
- **JIMINY-CORRECTION-PRODUCER-001 recovery-test drill** — same
  investigation disclosed. Optional; run if operator wants
  confidence the L1 promoter still fires (last L1 correction node
  created 2026-08-14).
- **`mdemg model use` shorthand** — BETA-DOCS-MODEL-VERSIONING-001
  named this as a task-tracked follow-up; not currently shipped.
  HOMEBREW-INSTALLER-QWEN-UPDATE Phase C candidate.
- **Retroactive `is_archived` tombstone for the idna LICENSE.md leak**
  — closed same-day via MDEMG-USAGE-CORPUS-CURATE-002 E3.
- **`mdemg extract-symbols` .venv exclusion audit** — MDEMG-USAGE-
  CORPUS-CURATE-002 disclosed follow-up. Different surface
  (SymbolNode not MemoryNode); separate sprint.
- **UVTS "who implements X?" re-baseline** — Q4 disclosed follow-up
  still outstanding; noted at #10 in §3.
- **CI parity check for HOOKSYNC-001** — PRE-COMMIT-INVENTORY-GATE-001
  disclosed follow-up. Adds a build-time check that hooks +
  templates stay byte-identical modulo `{{SPACE_ID}}`.

---

## Annex — Q5 rules pinned (durable operator-facing invariants)

Below is the running list of rules pinned to `CLAUDE.md` this
quarter. Each has a canonical "when this class of problem recurs,
this is the fix" statement.

**JIMINY-CEILING-BREAK-2 arc class**:
- Corpus curation is the highest-leverage lever when actionable
  follow rate <40%. Audit corpus BEFORE classifier prompt work,
  retrieval tightening, or model swap. Criterion: "would a
  competent developer reminded of this rule before an action
  actually follow it?" — NOT "is this rule true?"
- Follow-rate framing MUST be trajectory language, never "by
  design" language. Panel titles / sprint verdicts / alert-floor
  comments / CLAUDE.md pins that call a sub-50% follow rate
  "honest by design" normalize a substrate-quality failure.
- Corpus-purge audits MUST cover role_type='correction' at parity
  with role_type='constraint'.
- Similarity is not always a discriminator; scope-gate when TSDB
  shows followed/ignored similarity distributions overlap ≥1σ.
- Scope-gating heuristics MUST use safe-defaults on both sides.
- Classifier prompt-extension flags MUST default off in code so
  ULTS `system_prompt_hash` pin is preserved; default-off render
  MUST be BYTE-IDENTICAL to the base const.
- Prompt clause ordering is narrower→broader, strongest-gate-last;
  recency-weighted LLM attention gives LAST clause most influence.
- UI corpus-growth features MUST unlock a keyboard-only flow.
  Auto-advance MUST have a visible cancellation window with
  keyboard cancel.

**Verifiability class taxonomy**:
- Rules of different verifiability classes MUST be graded by
  class-specific signals + metric-partitioned into per-class
  gauges. Aggregate follow rate over mixed classes is dishonest.
- When adding a new rule, `verifiability_class` MUST be set at
  creation.
- New follow-rate arcs MUST specify WHICH class they target.
- Retrain compute MUST gate on the class where LLM improvement
  CAN help (classifier).
- When adding a new node property that RecordOutcome needs to
  read at write time, denormalize into `constraint_outcomes` at
  write time rather than joining Neo4j at aggregation time.

**Process-observation platform class** (all pinned by the
OBSERVER-{01..06} arc):
- Emitters MUST be fire-and-forget async with a short timeout
  (1s connect / 3s max-time; mirrors GUARDRAIL-PRODUCER-001).
- Matchers MUST be fail-open: missing evidence ≠ violation.
- Hook + template mirror move in the same commit (HOOKSYNC-001
  parity is CI-gated).
- Each new observer registers its own `PROCESS_MATCHER_<RULE>_
  ENABLED` flag default-OFF in code AND `.env` (HEBB-ETA-001).
- Matchers MAY read arbitrary `metadata` JSONB via same-session
  prior-event queries.
- Session-scope for `sequential-epics` is intentional — the rule
  targets within-sprint parallel execution.
- Event-source decision framework — prefer hook-side emit if the
  rule targets agent behavior AND session_id correlation is
  required.
- MCP-tool detection via `mcp__` prefix + retrieve-shape substring.
- Bash-side search-binary detection MUST be word-boundary-anchored
  via module-level regex.
- Filesystem-read is valid matcher capability class when bounded
  (config-driven root + MaxFileBytes cap + single-path lookup +
  fail-open).
- Sprint code regex requires ≥1 hyphen (differentiates from
  ISSUE-123 style).
- Tier detection MUST use case-fold + alt spellings.
- Leanest-observer pattern — matchers can grade purely from
  metadata without cross-table / filesystem access.
- UxTS detection uses u-prefix regex + path substring.
- Non-shape-matching files skip, not miss.
- Metadata-only matchers are the leanest observer shape — prefer
  them whenever the graded rule reduces to a single-row metadata
  predicate captured at emit-time.

**Gate-decision class** (from PHASE-4B-GATE-DECISION-001):
- Gate rules with numerical thresholds designed BEFORE data existed
  should be re-derived once real data is available.
- Add "signal density" to future compute-gate decisions — <10
  rows/day organic on a metric class = class isn't ready to gate
  compute-heavy work.
- Retrain compute must show ROI on THE MEASURED CEILING, not on a
  theoretically-improvable subcomponent — cross-class band analysis
  is the honest ROI check before committing to LLM retrain.

**HITL platform class**:
- Review-grade handlers MUST validate item_id exists in the target
  dataset BEFORE writing. `d.FetchItem` returns `(item, found, err)`
  — capture all three, 404 on `!found`.
- HITL candidate-selectors MUST use `LEFT JOIN review_grades ...
  AND r.item_id IS NULL` at current rubric_version.
- HITL candidate-selectors on LLM-derived datasets MUST filter
  `caller_canceled:*`-tagged rows + empty responses.
- `readJSON` decode errors MUST surface the offending field name
  in the response body.
- Additive schema columns for grader/SME-authored metadata follow
  the migration-029 pattern.

**Security class** (SEC-TRANCHE-2/3):
- CodeQL does NOT recognize regex validation as a path-injection
  sanitizer. Every HTTP handler constructing a filesystem path
  MUST route through `pathsafe.SafeJoinUnderDir`.
- `#nosec` is gosec-only; CodeQL ignores it. Close via recognized
  sanitizer OR GitHub code-scanning API dismissal.
- Regex patterns for URL/host detection MUST use `\b` word-
  boundary anchors on the host side.
- URL-scheme deny-lists MUST include `data:` + `vbscript:` alongside
  `javascript:` after lowercase normalization.
- Substring-in-URL checks are unsafe — parse with
  `urlparse(url).hostname` and compare hostnames.
- Every secret-shaped env var (name contains PASSWORD / PASS /
  API_KEY / TOKEN / SECRET) MUST be added to `internal/config/
  yaml_config.go::isSensitive` in the SAME COMMIT.
- Every `make([]T, n)` where `n` traces to an operator-tunable
  value MUST have a defensive cap.
- Every `int32(n)` conversion of a `strconv.Atoi`-derived value
  MUST bounds-check first.
- DOM sinks receiving server-response IDs MUST client-side
  validate the ID shape before use (CUIDv2 regex `^[a-z0-9]{20,32}$`).

**UI-completion class** (JIMINY-RULES-UI-001):
- No UI work is complete without BOTH (a) agent-side automated
  review (Playwright e2e or equivalent) AND (b) operator user-
  side hand-inspection.
- All inline styles in `/ui/tabs/*.js` MUST use Catppuccin theme
  vars — NEVER hardcoded light-theme colors.
- Accordion/select UI keying: state MUST key on the truly-unique
  identifier (CUIDv2 `node_id`), NOT any human-mnemonic identifier
  that can duplicate.
- Rich error surface for flag-gated endpoints: fetch helpers MUST
  preserve `.status + .payload` on error, not just
  `${status} ${statusText}`.

**Data pipeline / privacy class**:
- Privacy-scrub patterns MUST be idempotent under multiple passes.
- Multi-branch CLI handlers walking independent per-table workloads
  MUST NOT early-return when an earlier branch is clean.
- Prefix predicates on data-pipeline queries MUST use `STARTS
  WITH`, not `CONTAINS`.
- Filesystem-walking ingesters MUST guard an operational-tree
  deny-set at directory entries (`.venv`, `__pycache__`,
  `node_modules`, `*.dist-info`, `*.egg-info`).
- When executing a corpus strip based on an audit-produced strip-
  list, preserve source files verbatim and SHA-verify pre + post.

**Pre-commit / drift-check class**:
- When adding a new DORMANT-CENSUS-* forcing function (new
  trigger-file → verifier → inventory triple), extend
  `_INVENTORY_CHECKS` in BOTH `.claude/hooks/pre-bash-check.py`
  AND `internal/cli/hook_templates/pre-bash-check.py` in the
  SAME PR.

**Reranking / LLM-call class**:
- Reranking LLM calls MUST set `Temperature: &rerankTemperature`
  (=0.0). Reranking is a relevance-scoring task, not a generation
  task.
- Score-array length-mismatch classes MUST be observable + retried,
  NEVER silently defaulted.

**Model-distribution class** (from HOMEBREW-INSTALLER-QWEN-UPDATE-001
Phase B):
- When a code change would break the "pulling a new model version"
  flow on customer machines, ship the safety guard BEFORE the
  operator publishes the new version.
- When a code change's live end-to-end verification requires
  external infrastructure not ready yet, ship the code with
  bounded live testing + operator runbook + a follow-up sprint
  scoped for wire-up after the external work lands.

---

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q4.md`
- `docs/development/roadmap/ROADMAP_2026Q3.md`
- `CLAUDE.md` (Architecture Notes §Q5-relevant, ~48 sprint notes)
- `docs/development/jiminy-ceiling-break-2/README.md`
- `docs/development/jiminy-metric-denominator-design-001/sprint_post.md`
- `docs/development/phase-4b-gate-decision-001/sprint_post.md`
- `docs/development/jiminy-metric-partition-001/sprint_post.md`
- `docs/development/jiminy-process-observer-{01..06}/sprint_post.md`
- `docs/development/jiminy-corpus-003/`
- `docs/development/jiminy-correction-corpus-001/`
- `docs/development/lever-c-tighten-002/`
- `docs/development/jiminy-classifier-context-002/`
- `docs/development/jiminy-hitl-velocity-001/`
- `docs/development/jiminy-rules-ui-001/`
- `docs/development/jiminy-corpus-audit-004/`
- `docs/development/sec-tranche-2-path-injection/`
- `docs/development/sec-tranche-3/`
- `docs/development/pre-commit-inventory-gate-001/`
- Repo PR merge history via `gh pr list --state merged` (PR #611 → #669)
