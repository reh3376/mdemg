# Sprint Plan — Phase 11.6.x Operational Hygiene

**Sprint ID:** FT-LORA-PHASE11.6.x (operational hygiene)
**Date:** 2026-05-01
**Branch:** `reh3376_dev01`
**Status:** APPROVED — execution starting Epic 0
**Predecessor:** Phase 11.6 (production cutover, PR #364 + commit `144918a`)
**Successor:** Phase 12 — UVTS Activation

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11.6.x |
| Title | Close 4 follow-ups from Phase 11.6 production cutover + start conflicting-guidance tracker |
| Type | Operational hygiene; small Go changes + Grafana JSON + TSDB migration + 1 Python instrumentation script |
| Risk | LOW (rate-limit is additive, Grafana additive, jiminy swap fix is verified by content-routing audit) |
| Budget | $0 OpenAI; ~5 hr local compute (TSDB relabel migration is the heaviest piece) |
| Output adapter | None (no model training) |
| New TSDB migration | V0014 — backfill `task_name` for jiminy.evaluate ↔ jiminy.evaluate_llm rows via content-hash routing |
| Post-sprint artifacts | `internal/ape/cycle.go` semaphore patch; `internal/jiminy/{eval_prompt,outcome_classifier}.go` swap; V0014 migration; `dashboards/mdemg-overview.json` panel additions; `internal/conversation/conflict_tracker.go` (new); sprint post-doc |

---

## 2. Problem Statement

Phase 11.6 production cutover surfaced four operational issues that should close before Workstream B (research extensions) begins. None is a blocker for using the system today, but all represent ongoing tech-debt accruing during normal operation:

1. **RSIC concurrent fan-out → Metal OOM trigger** (11.6.2). `internal/api/handlers_conversation.go:106` spawns one `go func() { rsicCycle.RunCycle(...) }()` per conversation observation. With ~150 observations recorded and an unbounded-concurrency policy, multiple `ape.reflect` calls fire within milliseconds, queue at `mlx_lm.server`, and exceeded the 180s timeout. At higher mlx concurrency the multi-prompt allocation can crash Metal entirely. The cutover smoke required us to run mlx with `--prompt-concurrency 4` (manageable) and ape.reflect success rate climbed from 7% → 39%, but unbounded fan-out still caps the achievable success rate.

2. **Grafana dashboards lack `model_name` visibility** (11.6.3). The `model_name` column in `llm_interactions` now reflects the cutover. No dashboard panel surfaces this, so operators can't see at-a-glance whether traffic shifted to local model or whether per-task latency regressed.

3. **Jiminy production task_name swap** (11.6.4). Discovered Phase 11.5e Epic 1: `WithContext("jiminy.evaluate", ...)` and `WithContext("jiminy.evaluate_llm", ...)` are crossed at production call sites. TSDB rows tagged `jiminy.evaluate` actually contain `outcome_classifier` content (which is `evaluate_llm`'s prompt). Affects all rows logged through 2026-04-29.

4. **mlx prompt-cache not configured** (11.6.5). `mlx_lm.server` supports `--prompt-cache-size N` to amortize repeat-prefix calls. `ape.reflect` prompts share the 20-action enum prefix; cache could cut second-and-onward latency 20-30%.

Plus one cross-cutting always-on item:

5. **Conflicting-guidance tracker** (Action 1 / RD-8 from research-eval). Logs divergent recommendations from Jiminy/RSIC/Consulting to TSDB. 3-month observation window determines whether Note 09 (FEP capstone, 9-12 month program) is empirically justified. **Cost: ~1 day. Start now so we have data before Note 09 ever runs.**

---

## 3. Scope & Constraints

**In scope:**

| # | Deliverable | Path |
|---|---|---|
| 1 | Per-space LLM-action semaphore in CycleOrchestrator | `internal/ape/cycle.go` |
| 2 | `RSIC_LLM_CONCURRENCY_LIMIT` config knob (default 2) | `internal/config/config.go` |
| 3 | Jiminy `WithContext` swap fix | `internal/jiminy/eval_prompt.go`, `internal/jiminy/outcome_classifier.go` |
| 4 | TSDB V0014 migration: content-hash backfill of `task_name` | `internal/tsdb/migrations/014_jiminy_task_name_backfill.sql` |
| 5 | Grafana panel additions (model_name distribution, LLM latency by task×model, error rate, breaker state) | `dashboards/mdemg-overview.json` (or new `mdemg-llm-routing.json`) |
| 6 | Prompt-cache config flag for mlx_lm.server invocation | `CLAUDE.md` Testing section + production runbook |
| 7 | Conflicting-guidance detector | `internal/conversation/conflict_tracker.go` (new); writes to existing TSDB `llm_interactions` or new `guidance_conflicts` table |
| 8 | Sprint post-doc | `docs/development/ft-lora/phase_11_6_x_post.md` |

**Out of scope:**
- Anything in Workstream B (research extensions) — gated on UVTS-Activation
- Action 2 (UAITS governance discussion): scheduled but not executed in code
- Actions 5, 6 (collaboration-brief outreach): outside engineering scope
- Container redeploy: gated on next CI image build (separate operator task)

**Hard constraints (MEMORY):**
- **No hardcoded values** — concurrency limit, conflict detection thresholds all in config
- **CUIDv2** for any new run_ids
- **Sequential epics** — semaphore + jiminy fix before Grafana before tracker
- **3-tier testing** — unit (semaphore math, content routing), integration (cycle orchestrator with mocked reflector + concurrent triggers), e2e (real production-shape scenario)
- **Min `max_tokens` ≥ 3000, `latency_budget_ms` ≥ 15000** (no LLM calls in this sprint anyway)
- **Plan-options pattern** — concurrency limit value (default 2 vs 1 vs 4) disclosed at PR
- **Single batched commit** at sprint close
- **Sprint summary on PR comments** immediately after push
- **TSDB additive** — V0014 only updates `task_name` of existing rows; no schema changes
- **Container redeploy NOT in scope** — patches go to `bin/mdemg` and ship in next CI image
- **MLX single-instance** — sprint doesn't restart mlx_lm.server beyond verification

---

## 4. Dependencies

**Consumed (code, pre-existing):**
- `internal/ape/cycle.go` — `CycleOrchestrator.RunCycle()` (target of concurrency limit)
- `internal/api/handlers_conversation.go:106` — spawn site
- `internal/jiminy/{eval_prompt,outcome_classifier}.go` — call sites with swapped task names
- `scripts/x11_jiminy_evaluate_rescue.py` (Phase 11.5e Epic 1) — content-routing logic to port to V0014 SQL
- `internal/tsdb/migrations/` — migration framework
- TSDB `llm_interactions` table (~56K rows; ~437 affected by jiminy swap)

**Consumed (data):** TSDB sample of jiminy.evaluate / jiminy.evaluate_llm rows for V0014 backfill verification.

**Consumed (compute):**
- Local MLX inference (verification only; no training)
- TSDB write workload for V0014 (~437 rows; instant)

---

## 5. Implementation Plan (Sequential Epics + Gates)

### Epic 0 — Preflight (≈30 min)

1. Verify mdemg native binary running with patches from `144918a` (PID 11696 at start of sprint).
2. Verify mlx_lm.server alive (PID 11140, concurrency=4, no Metal OOM since restart).
3. Verify TSDB `llm_interactions` count ~56K + per-task hash distribution audit (re-run from 11.5e Epic 0).
4. 167 unit tests still green.
5. Branch from `reh3376_dev01`; verify clean working tree (apart from .env which is gitignored).

**Gate:** all 5 checks green.

### Epic 1 — RSIC Concurrency Limit (11.6.2, ~4 hr)

1. Add `RSICLLMConcurrencyLimit int` field to `Config` struct (default 2; min 1; max 8).
2. Add `getInt("RSIC_LLM_CONCURRENCY_LIMIT", 2)` reader.
3. Add `llmSem chan struct{}` to `CycleOrchestrator` struct, initialized in `NewCycleOrchestrator` to `make(chan struct{}, cfg.RSICLLMConcurrencyLimit)`.
4. In `RunCycle()`, around line 132 (`reflector.Reflect()` call): acquire `llmSem` before, release in defer.
5. Add `mdemg_rsic_llm_semaphore_blocked_total` counter — increments when goroutine waits.
6. Test: 8 concurrent `RunCycle()` invocations should serialize through 2 in-flight slots; the other 6 wait. Counter increments by 6 once.

**Gate:** semaphore acquired+released cleanly under stress test (Tier 2).

### Epic 2 — Jiminy Task-Name Swap Fix (11.6.4, ~3 hr)

1. **Audit**: confirm the swap. Currently `internal/jiminy/eval_prompt.go` calls `WithContext("jiminy.evaluate", ...)` but emits the outcome-classifier system prompt. Same crossed wiring at `outcome_classifier.go`.
2. **Patch**: swap the two task names in code so `task_name` matches the actual prompt content.
3. **TSDB V0014 migration**: SQL function that, for each row in `llm_interactions` with `task_name IN ('jiminy.evaluate', 'jiminy.evaluate_llm')`, computes `system_prompt_hash` and looks up the correct task_name in a static map (using the spec hashes from Phase 11.5e). Atomic update.
4. **Verification**: post-migration audit confirms all jiminy.* rows have `task_name` matching `system_prompt_hash` per spec.

**Gate:** content-routing match rate 100% on jiminy.* rows; tests in Tier 1 + Tier 2.

### Epic 3 — Grafana Panels (11.6.3, ~2 hr)

1. Add 4 panels to existing `dashboards/mdemg-overview.json`:
   - `LLM call distribution by model_name` (stacked time series, last 24h, group by `model_name`)
   - `LLM latency p50/p95/p99 by task_name × model_name` (heatmap)
   - `LLM error rate % by task_name` (single-stat per task)
   - `Open circuit-breaker count` (single-stat; queries `mdemg_circuit_breaker_state{state="open"}`)
2. Validate: `mdemg dashboard validate` (or equivalent).
3. Reload Grafana via Docker; verify panels render.

**Gate:** all 4 panels visible in Grafana with live data.

### Epic 4 — Prompt-Cache Configuration (11.6.5, ~1 hr)

1. Update CLAUDE.md production-use command to add `--prompt-cache-size 4096`.
2. Restart mlx_lm.server with the flag; observe `ape.reflect` latency over next RSIC cycle.
3. If 20%+ improvement: document; otherwise note no-effect and revert.

**Gate:** measured before/after latency of `ape.reflect`; decision recorded.

### Epic 5 — Conflicting-Guidance Tracker (Action 1, ~5 hr)

1. New module `internal/conversation/conflict_tracker.go`:
   - Hooks into Jiminy + RSIC + Consulting decision callbacks
   - When 2+ subsystems produce divergent recommendations on the same context, log row to TSDB with: `space_id`, `context_hash`, `jiminy_recommendation`, `rsic_recommendation`, `consulting_recommendation`, `divergence_kind` (textual/numeric/ordinal), `time`
2. New TSDB table `guidance_conflicts` (V0015 migration).
3. Add Grafana panel: "Conflicting-guidance frequency over time" — feeds the 3-month observation window.

**Gate:** 1 synthetic divergence test produces 1 row; counter increments.

### Epic 6 — Testing (3 Tiers)

**Tier 1 (Unit):**
- `internal/ape/cycle_test.go` — semaphore acquire/release, blocked counter
- `internal/jiminy/*_test.go` — swap fix
- `internal/conversation/conflict_tracker_test.go` — divergence detection logic
- `tests/scripts/test_v0014_migration.py` — content-hash routing math (using Phase 11.5e spec hashes)

**Tier 2 (Integration):**
- 8 concurrent `RunCycle()` invocations, mocked reflector — semaphore should serialize through 2 slots
- TSDB V0014 forward + reverse on test DB; verify row counts + correct `task_name` distribution
- Conflict tracker against canned divergence fixtures

**Tier 3 (E2E):**
- Run mdemg native binary; observe RSIC cycles fire + ape.reflect concurrency cap holds
- Verify Grafana panels render with real production traffic
- Verify V0014 applied to dev TSDB; sample of relabeled rows passes `system_prompt_hash` ↔ `task_name` consistency check

**Gate:** all 3 tiers green; 167+ unit tests still green.

### Epic 7 — Documentation + Commit + PR (Final Epic — Never Cut)

1. `docs/development/ft-lora/phase_11_6_x_post.md` — executed-truth post-doc
2. `00_README_v2.md` v5.12 → v5.13 with 11.6.x changelog
3. `CHANGELOG.md` `[Unreleased] ### Added` entry
4. `AGENT_HANDOFF.md` top entry
5. `CLAUDE.md` Testing section — `--prompt-cache-size 4096` recommendation
6. Single batched commit; auto-PR; sprint summary on PR

**Gate:** all docs committed; cross-refs valid.

---

## 6. Testing Plan (Three Tiers)

Covered in Epic 6 above.

---

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ops): Phase 11.6.x — operational hygiene (RSIC rate-limit + jiminy swap fix + Grafana panels + conflict tracker)`
- Body: each of the 4 follow-ups + Action 1 summarized; before/after metrics for prompt cache; V0014 + V0015 migrations summarized; policy compliance checklist.
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push to `reh3376_dev01` → auto-PR opens → sprint summary comment on PR.

---

## 8. Verification Checklist

- [ ] Epic 0: preflight green
- [ ] Epic 1: RSIC concurrency limit holds at 2 in-flight under 8-goroutine stress; counter increments by 6
- [ ] Epic 2: V0014 migration applied + reversed cleanly; jiminy task_name 100% content-matched
- [ ] Epic 3: 4 Grafana panels live with traffic
- [ ] Epic 4: prompt-cache effect measured; documented either way
- [ ] Epic 5: conflict tracker fires on synthetic divergence; new TSDB table populated
- [ ] Epic 6: 3 tiers green; 167+ tests
- [ ] Epic 7: single commit pushed; auto-PR; sprint summary on PR

---

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | Semaphore default of 2 still too tight; ape.reflect success rate doesn't improve materially | Medium | `RSIC_LLM_CONCURRENCY_LIMIT` is config-driven; bump to 3 or 4 if 2 underperforms | Document at PR |
| 2 | V0014 migration accidentally reassigns task_name on rows that AREN'T affected by the swap | Medium | Migration WHERE clause restricts to `system_prompt_hash` matching one of the 2 known swap-pattern hashes; dry-run first | Reverse via V0014_down + investigate |
| 3 | Jiminy swap fix breaks downstream consumers expecting old labels | Low | TSDB historical data was already mislabeled; swapping production labels matches actual content | Add `task_name_legacy` column for grace period if a consumer breaks |
| 4 | Conflict tracker generates too many rows under high traffic (TSDB cost) | Low | Insert only on actual divergence (not parallel agreement); rate-limit at 1 row/space/minute | Disable + drop table if cost balloons |
| 5 | Grafana panels show stale data | Low | Panels query directly from `llm_interactions` (live) | Force dashboard reload |
| 6 | Prompt-cache flag causes mlx OOM | Low | Cache is configurable size; start at 4GB | Drop the flag |
| 7 | mdemg binary rebuild + restart loses in-flight RSIC cycles | Low | Rebuild binary then SIGTERM (graceful) the old; let cycle complete; start new | Standard production restart pattern |

---

## 11. Documents Accessed (during planning)

- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_6_post.md`
- `/Users/reh3376/mdemg/docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` (this sprint's parent roadmap)
- `/Users/reh3376/Downloads/mdemg-future-sprint-assessments/{mdemg-collaboration-brief.md,mdemg-research-evaluation.md}`
- `/Users/reh3376/mdemg/internal/ape/cycle.go`
- `/Users/reh3376/mdemg/internal/api/handlers_conversation.go`
- `/Users/reh3376/mdemg/internal/api/server.go`
- `/Users/reh3376/mdemg/scripts/x11_jiminy_evaluate_rescue.py` (content-routing logic)
- `/Users/reh3376/mdemg/dashboards/` (Grafana JSON)
- TSDB schema migrations directory
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `feedback_mlx_set_wired_limit_footgun.md`, `project_mdemg_purpose.md`

---

## 12. Rollback

All changes additive or reversible.

1. `git revert <final commit SHA>`
2. TSDB: `mdemg tsdb migrate --target V0013` (reverses V0014 + V0015)
3. `.env`: revert `RSIC_LLM_CONCURRENCY_LIMIT` default
4. Grafana: revert dashboard JSON via git
5. mlx restart: drop `--prompt-cache-size` flag if used

Phase 11 + 11.5 + 11.6 production state untouched.
