# Sprint POST-FT-LORA-PHASE13.1 — Column-Weight Ablation

> **DRAFT — pending operator approval.** Direct follow-up to Phase 13's FAILED A/B (`phase_13_ab_verdict_quick.json`). The infrastructure shipped; the equal-weight v1 configuration regressed retrieval quality on 2 of 16 questions. Phase 13.1 finds a configuration that beats baseline.

## Context

Phase 13 (commit `1b53d1f`, post `phase_13_column_voting_post.md`) shipped the 4-column RRF retrieval infrastructure (Embedding + BM25 + Graph + Structural) but its A/B merge gate **failed**:

| Run | Branch label | Mean | Result |
|---|---|---|---|
| Baseline (legacy linear scorer) | `phase13-baseline-linear` | **0.396** | — |
| Candidate (RRF v1-rrf4, equal weights `1.0/N`) | `phase13-candidate-rrf4` | **0.358** | **−0.038** |

Two catastrophic per-question regressions to 0.000:
- **q `69`** architecture_structure: 0.354 → 0.000 (−0.354)
- **q `hard_sym_4`** computed_value: 0.350 → 0.000 (−0.350)

One improvement (q `hard_sym_20` computed_value, +0.100). The other 13 of 16 questions produced *bit-identical* candidate sets between scorers — divergence concentrates entirely on 3 hard-symbol queries.

`RetrievalColumnVotingEnabled` default stayed `false` at Phase 13 close. Phase 13.5 (commit `d292d9c`) migrated production to llama.cpp + GGUF Q5_K_M, which removed the mid-A/B watchdog crash interruptions that polluted Phase 13's first attempt — Phase 13.1 can now run repeated A/B cycles cleanly.

**Why now.** Phase 14 (Notes 05+06: sparse fingerprints + percentile activation gate) consumes the per-query `consensus_strength` signal Phase 13 emits. Phase 14 is gated on Phase 13.x passing. Phase 13.1 closes that gate.

**Why this is small enough to ship in one sprint.** All 4 columns + the RRF aggregator are already in production code, behind `RetrievalColumnVotingEnabled`. The cache-version namespacing prevents A/B contamination. The UVTS runner + `uvts_ab_compare.py` harness is mature (used 4× in Phase 13/13.5). The work is configuration + diagnostic forensics + a sweep runner — no new framework code.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE13.1 |
| Title | Column-Weight Ablation — find a Phase 13 RRF configuration that beats baseline |
| Date | 2026-05-03 (plan) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 13.5 (commit `d292d9c`), Phase 13 (commit `1b53d1f`) |
| Successor | Phase 14 — Notes 05+06 (gated on this sprint passing) |
| Type | Code-small (~150 LOC ablation runner + ~20 LOC config) + UVTS-driven research |
| Risk | MEDIUM (touches a flag that, when flipped, changes production retrieval-ranker behavior on every retrieve call. Mitigated by A/B merge gate + scorer-version cache namespace + per-column suppression knobs from Phase 13) |
| Budget | $10–25 OpenAI (multiple 16-question A/B cycles + 1× full 120-question A/B for the winner). ~3 hr local compute |
| Effort estimate | 5–7 dev-days |
| New TSDB migration | None expected — V0017 `retrieval_audit` already captures per-call consensus_strength and per-column latency; V0016 `uvts_runs` already captures A/B cycles |
| Post-sprint artifacts | `scripts/phase13_1_ablation_runner.py` (new), `internal/config/config.go` weight-default tweaks (conditional, only on winning preset), `RetrievalColumnVotingEnabled` default flip (conditional, only if A/B passes), Phase 13.1 post-doc with full ablation table |

## 2. Problem Statement

**Symptom (concrete, from Phase 13 verdict):**
- 13 of 16 questions produced bit-identical scores between legacy linear and v1-rrf4 (the scorers converge on most queries)
- 2 catastrophic regressions: q `69` and q `hard_sym_4` both hit the floor (0.000) under RRF, were healthy (~0.35) under linear
- 1 improvement: q `hard_sym_20` rose +0.100 under RRF
- Net mean: 0.358 (RRF) vs 0.396 (linear) = **−0.038, fails the A/B mean gate** (B ≥ A required)
- Per-question regression threshold > 0.10 = also fails (2 regressions at −0.35)

**Three open hypotheses (Phase 13 post-doc §"Why the 2 catastrophic regressions"):**

| # | Hypothesis | What experiment falsifies it |
|---|---|---|
| H1 | **Equal-weights pathology.** Structural's hop-decayed score pulls weight from Embedding's high-quality vector ranking, displacing precise-symbol candidates from top-K | Lower Structural weight or zero it; observe q 69 + q hard_sym_4 recover |
| H2 | **Structural over-aggressive at 2 hops.** `RETRIEVAL_STRUCTURAL_HOPS=2` pulls in siblings/parents that crowd the top-K for precise-symbol queries | Reduce hops to 1; observe regressions recover without touching weights |
| H3 | **Graph column re-seeding rotates right answer below top-K.** Graph runs its own vector recall + spreading activation; activation-ranking may demote correct answers | Zero Graph weight; observe q 69 + q hard_sym_4 recover |

These aren't mutually exclusive — could be 1 dominant + 2 contributing.

**The decision space (Phase 13.1 must answer):**
1. Which hypothesis is correct (forensic diagnosis on q `69` + q `hard_sym_4`)
2. What weight preset (and hop depth) yields A/B pass on the quick profile
3. Does the same preset hold on the full 120-question profile
4. Is the resulting consensus_strength signal still meaningful for Phase 14 consumption

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Forensic diagnostic — actual top-K candidates for q `69` + q `hard_sym_4` under each scorer | `docs/development/post-ft-lora/phase_13_1_forensic_diagnosis.md` |
| 2 | Ablation runner — automates "set env, restart mdemg, run UVTS, compare" loop | `scripts/phase13_1_ablation_runner.py` (new, ~150 LOC) |
| 3 | Weight-preset sweep (3–5 presets) on quick profile (16q × 1 run) | `/tmp/phase13_1_runs/<preset_name>/grades.json` per preset |
| 4 | Hop-depth sweep (1, 2, 3) on quick profile | Same |
| 5 | Per-category weight tuning experiment (if generic presets don't pass) | One additional preset |
| 6 | Winner verification on full 120-question profile | `/tmp/phase13_1_full_winner/` + verdict |
| 7 | Conditional default flip — `RetrievalColumnVotingEnabled` `false → true` IF AND ONLY IF winner passes A/B | `internal/config/config.go` (~5 LOC) |
| 8 | Conditional weight defaults — if winner uses non-equal weights, update config defaults | `internal/config/config.go` (~10 LOC) |
| 9 | Sprint plan + post + roadmap update + AGENT_HANDOFF + CHANGELOG | Standard 7-doc footprint |

**Out of scope (deferred):**
- Adding new columns (Temporal, RoleScoped) — Phase 13 Epic 0 deferred them per data audit
- Changing the RRF formula itself (`k=60`, `score(node) = Σ weight_c / (k + rank_c(node))`)
- Touching the upstream `vectorRecall` / BM25 / spreading-activation code paths
- Phase 14 design or Note 05/06 implementation
- Cross-space ablation (the spec measures `whk-wms` only; ablation results may not transfer to other spaces)
- Reranker (`internal/retrieval/rerank.go`) consumption of `consensus_strength` — flag `RetrievalRerankConsumeConsensus` stays default-off

**Constraints (hard, MEMORY):**
- **No hardcoded values** — every weight preset goes through env vars (`RETRIEVAL_COLUMN_WEIGHT_{EMBEDDING,BM25,GRAPH,STRUCTURAL}` already exist in config; ablation runner mutates `.env` per preset)
- **Sequential epics** — diagnosis before sweep; sweep before winner-verification; winner before flip
- **Plan-options pattern** — at least one weight-design fork at §13 (manual presets vs grid search vs Bayesian)
- **Single batched commit at sprint close** — ablation runner + winner config + docs
- **Sprint summary on PR comment**
- **CUIDv2** for any new IDs (none expected)
- **max_tokens ≥ 3000, latency_budget_ms ≥ 15000** — no LLM call sites added; if any reranker prompt is touched, observe floor
- **Live-testing required** — Tier 3 = real mdemg + real Neo4j + real UVTS grader against `whk-wms` space, observed via `uvts_runs` rows in TSDB
- **A/B merge gate (canonical)**: B mean ≥ A mean AND no per-question regression > 10% (matches `lnl_demo_validation.uvts.json::ab_mode.regression_threshold_per_question`)
- **Cache hygiene**: every preset change bumps the scorer_version hash so namespaces are isolated. Phase 13 already implemented this via `Service.scorerVersion()`. Phase 13.1 must verify the hash actually changes when weights change — if it doesn't, cache pollution will silently invalidate A/B results

## 4. Dependencies

**Consumed (code, pre-existing — reuse):**
- `internal/retrieval/consensus.Aggregate` — RRF aggregator (Phase 13 Epic 3)
- `internal/retrieval/scoring_rrf.Service.ScoreAndRankRRF` — scorer entry point with weight-config plumbing (Phase 13 Epic 4)
- `internal/retrieval/cache.go::CacheKey(req, scorerVersion)` — scorer-version namespace (Phase 13 Epic 4)
- `internal/retrieval/column_{embedding,bm25,graph,structural}.go` — 4 column adapters (Phase 13 Epic 1+2)
- `internal/config/config.go` — `RetrievalColumnVotingEnabled`, `RetrievalRRFK`, `RetrievalStructuralHops`, `RetrievalColumnWeight*`, `RetrievalColumn*Enabled` knobs (Phase 13 Epic 6)
- `docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` — A/B harness (Phase 12)
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — 16q quick + 120q full profiles, `ab_mode.regression_threshold_per_question=0.10`

**Consumed (data):**
- Neo4j `whk-wms` space (used by Phase 13 baseline + candidate runs)
- Phase 13 baseline grades at `/tmp/uvts-baseline/grades.json` (16q, mean 0.396) — the canonical A reference
- TSDB V0016 `uvts_runs` + `uvts_results` for historical ablation tracking
- TSDB V0017 `retrieval_audit` for per-call consensus_strength + per-column latency forensics
- Production model `mdemg-llm-v1.Q5_K_M.gguf` via `llama-server` on port 8102 (Phase 13.5 cutover)

**Consumed (compute):**
- mdemg HTTP API at `localhost:9999` with `RetrievalColumnVotingEnabled=true` per ablation preset
- llama-server at `127.0.0.1:8102` (Phase 13.5 substrate, stable since cutover)
- TimescaleDB at `localhost:5433` for `uvts_runs` writes
- OpenAI API for UVTS grader (`gpt-5.4-mini`). Cost per A/B cycle: ~$0.60 quick (16q), ~$5–8 full (120q). Plan: 4–6 quick cycles + 1 full = ~$10–25.

**External services:**
- mdemg HTTP API for retrieve calls
- TimescaleDB for V0016 + V0017
- OpenAI API (UVTS grader)

No new infrastructure. No code generation, no model training.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean post-PR-365 merge (or rebased onto whatever lands first); mdemg + llama-server healthy via launchd; TSDB schema_version=18; `RetrievalColumnVotingEnabled=false` baseline confirmed.

### Epic 0 — Forensic diagnosis (q 69 + q hard_sym_4)

> **The single highest-leverage step in the sprint.** Until we know *why* these 2 questions go to 0.000, every weight preset is a guess. The diagnosis tells us which hypothesis is correct and points at the surgical fix.

1. Pull q `69` and q `hard_sym_4` text from `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` + `docs/architecture/benchmarks/whk-wms/test_questions_120.json`
2. Run the **legacy linear** scorer manually: `RetrievalColumnVotingEnabled=false`, hit `/v1/memory/retrieve` with each query, capture top-K (default 5) candidates with their scores + node identities
3. Run the **v1-rrf4** scorer manually: same but with the flag `true`, default equal weights. Capture per-column candidate sets (`per_column_latency` + `top_k_node_ids` are already persisted to V0017 `retrieval_audit`)
4. Diff the candidate sets:
   - Are the right answers (per the spec's `expected_evidence`) in any column's results?
   - If yes, which column ranked them top-3? What rank did RRF aggregate them to?
   - If no (RRF returned zero matches because no column produced them), it's a Structural-overshadows-Embedding problem (H1)
5. Run column-suppression isolation:
   - Disable Structural (`RETRIEVAL_COLUMN_STRUCTURAL_ENABLED=false`), re-run both queries → if they recover, H1 or H2 confirmed
   - Disable Graph (`RETRIEVAL_COLUMN_GRAPH_ENABLED=false`) → if they recover, H3 confirmed
   - Disable BM25 → control case (don't expect change)
6. Reduce Structural hops to 1 (`RETRIEVAL_STRUCTURAL_HOPS=1`), re-run with all columns active → if regressions recover, H2 isolated
7. Output diagnosis doc with cited per-column candidate sets and a verdict on H1/H2/H3 ranking

**Gate:** diagnosis doc committed; the dominant hypothesis is identified with a per-column trace; ablation Epic 1+2 know whether to focus on weights, hop depth, or column suppression.

### Epic 1 — Ablation runner

1. New script `scripts/phase13_1_ablation_runner.py` (~150 LOC, no Go changes):
   - Argparse: `--presets <preset_set_name>`, `--profile <quick|full>`, `--baseline-grades <path>`, `--out-dir <path>`
   - Per preset:
     - Edit `.env` to set the preset's env vars (weights, hops, column-enable flags)
     - `launchctl bootout` + `bootstrap` mdemg (forces fresh boot of new config)
     - Wait for `/healthz` ready
     - Run `uvts_runner.py --profile $profile --space-id whk-wms --persist-tsdb --branch-label phase13_1-$preset --output-dir $out/$preset`
     - Run `uvts_ab_compare.py` against the supplied baseline → preset verdict JSON
   - At end: aggregate preset verdicts into a single comparison table written to `$out/ablation_summary.md`
2. Idempotent + resumable: if `$out/$preset/grades.json` already exists, skip that preset
3. Restores `.env` to original state at exit (defer-style)
4. Tier-1 unit test: small fake-presets dict + mocked uvts_runner — verify the script's preset loop + restore-on-exit semantics

**Gate:** runner ships; smoke pass on a single fake-preset dry-run; `.env` cleanly restored on script exit.

### Epic 2 — Weight-preset sweep (informed by Epic 0)

> Preset list is **conditional on Epic 0 outcome**. The defaults below assume H1 (equal-weights pathology) is dominant; if H2 or H3 wins, preset list pivots to hop-depth or column-suppression sweeps.

Default preset set (4 presets, ~$2.40 in OpenAI):

| Preset | Embedding | BM25 | Graph | Structural | Hops | Hypothesis tested |
|---|---|---|---|---|---|---|
| `equal-baseline` | 0.25 | 0.25 | 0.25 | 0.25 | 2 | Reproduce Phase 13 v1-rrf4 (sanity check the runner itself) |
| `embedding-heavy` | 0.50 | 0.20 | 0.15 | 0.15 | 2 | Tests whether boosting Embedding alone recovers the 2 regressions |
| `lexical-priority` | 0.30 | 0.40 | 0.15 | 0.15 | 2 | Tests whether BM25 dominance helps hard_sym queries (they're literal symbol names) |
| `structural-suppress` | 0.40 | 0.30 | 0.25 | 0.05 | 2 | Tests whether nearly-zeroing Structural fixes both regressions (H1) |

Acceptance (per preset): A/B passes the 16-question quick profile (B mean ≥ A mean AND no per-question regression > 10%).

**Gate:** at least one preset passes A/B quick. If none pass, escalate to Epic 3 (hop depth sweep) before adding more weight presets.

### Epic 3 — Hop-depth + per-category sweep (escalation if Epic 2 yields no winner)

1. Hop-depth sweep on the best Epic-2 preset (lowest mean delta, even if still failing):
   - `RETRIEVAL_STRUCTURAL_HOPS=1` — minimum, tests H2
   - `RETRIEVAL_STRUCTURAL_HOPS=3` — control (Note 04 doc range upper)
2. Per-category weight tuning if needed: weight Structural lower for `architecture_structure` + `computed_value` categories specifically. This requires a small mdemg config addition to support per-category weights — **deferred to Phase 13.2 if needed** (out-of-scope for 13.1; if necessary the sprint stops at Epic 2 and Phase 13.2 becomes a code-required sprint)

**Gate (success path):** an Epic 2 or Epic 3 preset passes the quick A/B.
**Gate (no-pass path):** sprint terminates at Epic 5 with default still `false`; Phase 13.2 spec drafted with per-category weight requirement.

### Epic 4 — Full-profile verification of winner

1. Run the winning preset on the **full 120-question profile** (`uvts_runner.py --profile full`). ~10 min wall-clock + ~$5–8 OpenAI.
2. Compare against the existing 120-question baseline (capture if not already in TSDB)
3. Acceptance: same A/B gate (B mean ≥ A mean AND no per-question regression > 10%)
4. If full-profile fails after quick passed, the regressions are concentrated in untested-on-quick categories — record findings + scope Phase 13.2

**Gate:** winner's 120-question A/B verdict captured. Pass = proceed to Epic 5. Fail = doc the gap and stop at default `false`.

### Epic 5 — Conditional default flip + commit

1. **Pass path:**
   - Edit `internal/config/config.go`: `RetrievalColumnVotingEnabled` default `false → true`
   - If winner uses non-equal weights: update `RetrievalColumnWeight{Embedding,BM25,Graph,Structural}` defaults
   - If winner uses non-default hops: update `RetrievalStructuralHops` default
   - Update CLAUDE.md "Column-Voting Retrieval" subsection — record the new default-on state + winning preset weights
2. **Fail path:**
   - No code change. Defaults stay as-is.
   - Update CLAUDE.md to record Phase 13.1 sweep results + Phase 13.2 scope (per-category weights or alternative hypothesis)

### Epic 6 — Documentation (Final Epic — Never Cut)

- `docs/development/post-ft-lora/sprint_plan_phase_13_1_column_weight_ablation.md` — frozen plan (this file)
- `docs/development/post-ft-lora/phase_13_1_post.md` — executed-truth: forensic diagnosis verdict, full ablation table, winner spec, full-profile verdict, OpenAI spend actual, decision-fork outcomes, follow-up Phase 13.2 scope (if any)
- `docs/development/post-ft-lora/phase_13_1_forensic_diagnosis.md` — Epic 0 output
- `docs/development/post-ft-lora/phase_13_1_ablation_summary.md` — Epic 2/3 output (auto-generated by runner, hand-augmented)
- `SPRINT_ROADMAP_POST_FT_LORA.md` — mark Phase 13.1 EXECUTED; flag Phase 14 unblocked (if pass) or Phase 13.2 queued (if fail)
- `AGENT_HANDOFF.md` top entry
- `CHANGELOG.md [Unreleased] ### Changed` (if pass: default-flag flip + weight defaults)
- `CLAUDE.md` Architecture Notes "Column-Voting Retrieval" subsection update

**Gate:** all docs landed; cross-refs valid.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- No new Go code expected (config-defaults only). Existing `internal/retrieval/consensus_test.go` covers the RRF math; existing `cache_scorer_version_test.go` covers namespace isolation. Verify nothing broke after default flip with `go test -race ./internal/retrieval/... ./internal/config/...`.
- New: `scripts/phase13_1_ablation_runner_test.py` — pytest covering preset loop + restore-on-exit.

**Tier 2 (Integration) — `go test -tags=integration`:**
- `tests/integration/column_voting_pipeline_test.go` (Phase 13) — verify still green after default flip.
- New (if winner uses non-equal weights): `tests/integration/phase13_1_default_weights_test.go` — confirms the new defaults round-trip through `FromEnv` and `Service.ScoreAndRankRRF` uses them.

**Tier 3 (Live E2E) — MANDATORY:**
- **Forensic diagnosis** (Epic 0) — real mdemg + real Neo4j + real UVTS spec questions, candidate top-K observed and recorded.
- **Ablation runner sweep** (Epic 2) — real mdemg + real `whk-wms` space; preset rotation via `.env` + launchd kickstart; UVTS grader hits OpenAI; verdicts persisted to TSDB `uvts_runs`/`uvts_results`.
- **Winner full-profile verification** (Epic 4) — real 120-question A/B; verdict in `phase_13_1_post.md`.
- **Post-flip soak** (if Pass path): 30 retrieves through real mdemg with the new default; observe `mdemg_retrieval_consensus_strength` histogram + `retrieval_audit` rows are healthy (median ≥ 0.5, no zero-strength rows on the fixed forensic queries).

**State restoration (MEMORY):** all changes additive or config-default. Rollback = `git revert <commit>` + `mdemg restart`. The flag remains operator-controllable via `.env`.

**Gate:** all 3 tiers green; A/B verdicts captured (quick + full); if Pass, post-flip soak shows healthy consensus distribution.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):
- Title (Pass): `feat(retrieval): Sprint POST-FT-LORA-PHASE13.1 — column-weight ablation winner: <preset_name>, default flipped to true`
- Title (Fail): `chore(retrieval): Sprint POST-FT-LORA-PHASE13.1 — ablation complete; default stays false; Phase 13.2 scoped`
- Body: forensic diagnosis verdict, full ablation table, winning preset (or no-winner notes), full-profile verdict, OpenAI spend actual, decision-fork outcomes, conditional config-default change citation
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`
- Push → PR auto-update → sprint summary comment posted to PR (MEMORY)

## 8. Verification Checklist

- [ ] Epic 0: forensic diagnosis doc committed; dominant hypothesis identified with per-column trace
- [ ] Epic 1: ablation runner ships; Tier-1 test passes; `.env` cleanly restored on exit
- [ ] Epic 2: 4 preset sweep complete; verdicts persisted to TSDB `uvts_runs`; ablation summary auto-generated
- [ ] Epic 3 (if needed): hop-depth sweep complete OR Phase 13.2 scoped if no preset reaches A/B pass
- [ ] Epic 4 (if Epic 2/3 passes): full 120-question A/B captured for winner
- [ ] Epic 5: conditional default flip applied IF AND ONLY IF A/B quick + full both passed
- [ ] Epic 6: sprint plan + post + ablation summary + ROADMAP + AGENT_HANDOFF + CHANGELOG + CLAUDE.md
- [ ] Commit pushed; auto-PR updated; sprint summary posted to PR
- [ ] All decision-fork outcomes disclosed in commit body + PR comment
- [ ] `golangci-lint run ./...` clean
- [ ] CI green
- [ ] OpenAI spend logged (target $10–25)

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 6. Specifically: `phase_13_1_post.md` adds the "what we learned about the regressions" section (forensic findings); CLAUDE.md "Column-Voting Retrieval (Phase 13)" subsection gets updated with new default + weight values + a one-line "default flipped 2026-XX-XX after Phase 13.1 ablation showed <preset_name> beats baseline" entry.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **No preset passes the quick A/B** — even after weight + hop sweeps, the 2 catastrophic regressions persist | Medium | Epic 0 diagnosis identifies the structural cause; Epic 3 hop-depth sweep + per-column suppression escalates; if still no pass, Phase 13.2 with per-category weights is the right answer (out-of-scope here) | Stop at Epic 5 fail-path; default stays `false`; Phase 13.2 spec drafted |
| 2 | **Quick passes but full 120q fails** — winner regresses on categories not in quick | Medium | Capture full A/B verdict; analyze which categories regress; either tune those category weights (Phase 13.2) or accept the gap and ship with a known limitation flag | Stop at Epic 5 fail-path |
| 3 | **Cache pollution between presets** — two presets produce identical scorer_version hash, A/B sees stale cache hits | Low | Phase 13.1 verifies `Service.scorerVersion()` includes weight values in the hash; if not, fix it (small code change) before Epic 2 starts; ablation runner does a `mdemg cache flush` between presets as belt-and-suspenders | Add `--no-cache` flag to UVTS runner OR delete cache rows manually between runs |
| 4 | **Ablation runner mutates `.env` and crashes mid-run** | Low | Runner uses `try/finally` (or Python context manager) to restore `.env` on any exit path; `.env.bak` written before first preset | Manual restore from `.env.bak` |
| 5 | **OpenAI API rate-limit during sweep** | Low | Quick profile is 16 calls × 4 presets = 64 calls. Well under any rate limit. Full profile (120q) also under | Retry with exponential backoff; UVTS runner already has retry built-in |
| 6 | **Operator's interactive workload contaminates ablation** — they're using mdemg while sweep runs and their queries go through the candidate scorer | Low | Sweep takes ~40 min for 4 presets; operator agrees not to use mdemg during sweep, OR sweep targets a different space (`mdemg-dev`) than operator's working space (`mdemg-mdemg`) | Re-run polluted preset |
| 7 | **Forensic diagnosis is inconclusive** — q 69 + q hard_sym_4 fail for *different* reasons | Medium | Doc both root causes; Epic 2 presets test both hypotheses; if dual-hypothesis, the winning preset must address both | Phase 13.2 surface-area expands |
| 8 | **Phase 13.5 watchdog mid-sweep firing** — if MLX_WATCHDOG_ENABLED=true and llama-server hits a transient blip during an A/B run | Low | Stable post-cutover. If it fires, retry the affected preset; V0018 `llm_endpoint_health_events` will record any transition | Disable watchdog for sweep duration if necessary |

## 11. Documents Accessed (during planning)

**Internal:**
- `docs/development/post-ft-lora/phase_13_column_voting_post.md` — Phase 13 post-doc with the failed A/B verdict, hypothesis enumeration, per-question delta table
- `docs/development/post-ft-lora/phase_13_ab_verdict_quick.json` — canonical machine-readable verdict
- `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` — Phase 13 frozen plan (for column architecture + RRF formula references)
- `docs/development/post-ft-lora/phase_13_5_bakeoff_results.md` — Phase 13.5 results (proves the substrate is now stable for repeated A/B cycles)
- `internal/retrieval/consensus.go` — RRF aggregator + consensus_strength formula
- `internal/retrieval/scoring_rrf.go` — `Service.ScoreAndRankRRF` + scorer-version namespace integration
- `internal/retrieval/cache.go::CacheKey(req, scorerVersion)` — cache namespace isolation
- `internal/config/config.go` — `RetrievalColumnWeight*`, `RetrievalStructuralHops`, `RetrievalColumn*Enabled` knobs
- `docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` — A/B harness
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — quick + full profile specs, A/B threshold
- `/tmp/uvts-baseline/grades.json` — Phase 13 quick-profile baseline (mean 0.396, the canonical A reference)
- Memory: `feedback_no_short_term_mlx_patches.md`, `feedback_data_decides_not_operator.md`, `feedback_sequential_epics.md`, `feedback_no_hardcoded_values.md`, `feedback_plan_options_pattern.md`, `feedback_mandatory_testing_tiers.md`, `feedback_live_testing_required.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_sprint_summary_on_pr.md`

**External (none expected — Phase 13.1 is internal data analysis + sweep)**

## 12. Rollback

All changes additive or config-default-flip — no destructive ops on Neo4j or TSDB rows.

1. `git revert <final commit SHA>` — removes ablation runner + config-default changes + docs
2. **Runtime emergency disable** (no rebuild): `RetrievalColumnVotingEnabled=false` in `.env` + `mdemg restart`. Reverts to legacy linear scorer immediately. Cache becomes cold (different scorer_version namespace) — first 5 minutes of post-disable retrieves are uncached, then warm
3. **Per-column suppress** (no rebuild): if a specific column is the regression source, set `RETRIEVAL_COLUMN_<NAME>_ENABLED=false` per the Phase 13 escape hatch
4. **Cache flush** if cross-preset pollution suspected: `mdemg cache flush --space whk-wms` (or whichever target space)

Phase 11 + 11.6.x + 11.6.2 + 11.6.3 + 11.6.3.1 + 12 + 13 + 13.5 artifacts untouched. Production model + llama-server config untouched. No Neo4j writes from this sprint. No TSDB schema changes (V0018 is the latest; V0017 retrieval_audit is the data sink, no migration required).

---

## 13. Plan-Options (decision forks — pick at execution, disclose in PR)

Per MEMORY `feedback_plan_options_pattern.md`. Some forks are data-decided post-Epic 0; others are operator-input on values:

| Fork | Data-decided after | Recommendation (provisional, may shift on Epic 0) | Rationale |
|---|---|---|---|
| **Hypothesis ranking** | Epic 0 forensics | Whichever H1/H2/H3 has the strongest per-column-trace evidence | Diagnosis-driven |
| **Preset count for sweep** | Epic 0 outcome | 4 default; expand to 6 if Epic 0 says dual-hypothesis | OpenAI cost is cheap; better data > saved $2 |
| **Quick profile sample size for sweep** | — | 16 (current spec); already sufficient for the 2 known catastrophic regressions | Phase 13's failure was identified at 16q; matching profile is honest |
| **Full-profile run when** | Epic 2/3 outcome | Run only after winner identified on quick (don't burn $5–8 on losing presets) | Cost discipline |
| **Default flip vs operator-toggle-only** | Epic 4 verdict | Flip default to `true` IFF quick + full both pass | Matches Phase 11.6.3.1 / Phase 13 commit-with-conditional-flip pattern |
| **Per-category weight escalation** | Epic 2/3 outcome | Defer to Phase 13.2 (out-of-scope for 13.1) | Sprint discipline; per-category weights need code surface, not just config |
| **Sweep target space** | — | `whk-wms` (matches Phase 13 baseline) | A/B comparison must hold space constant |
| **Watchdog state during sweep** | — | Leave enabled (Phase 13.5 substrate is stable; no need to disable) | Production-equivalent test conditions |

The remaining open question — "which preset wins" — is Epic 2 + Epic 4's empirical answer, not a planning-time decision.

---

## Acceptance bar (top-level)

A successful Phase 13.1 ships when:
1. **Forensic diagnosis** identifies the dominant hypothesis (H1/H2/H3) with cited per-column candidate traces
2. **Ablation sweep** finds at least one preset that passes the 16-question quick A/B (B mean ≥ A mean AND no per-question regression > 10%)
3. **Winner verification** on full 120-question profile passes the same A/B gate
4. **Default flip** applied IFF (2) and (3) both pass
5. **Documentation complete** — operator can read post-doc and reproduce the sweep

A "failed" Phase 13.1 (no preset reaches A/B pass) still ships value: the forensic diagnosis + ablation table is the input to Phase 13.2 (per-category weights or alternative hypothesis). Default stays `false`; framework is unchanged.

Anything beyond either outcome is Phase 13.2.
