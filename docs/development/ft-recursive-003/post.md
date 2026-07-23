# FT-RECURSIVE-003 — Sprint Post

**Dates:** 2026-07-23 (single-day, 8 epics) | **Branch:** `reh3376_dev01`
**Parent:** SPEC_recursive_retraining_loop.md §3/§4 Phase 7 +
FTLOOP-DRILL-001's two pre-work findings.

## Verdict

**The loop can promote, defend, and escalate autonomously — every layer
proven live the day it was built.** The spec's exit criterion ("a bad
candidate auto-rolls-back in a live drill") was met three independent ways:

| Layer | Drill | Result |
|---|---|---|
| Pre-swap canary (E4) | 1MB garbage GGUF confirmed for promotion | Blocked at the side-port — **prod pid unchanged (0 restarts)**; ledger `canary_failed` |
| Fail-closed swap (E3) | same garbage, canary off | Swap → 2m unhealthy → **auto-revert** → production healthy; ledger `rolled_back\|promote_failed\|swap_reverted=true` |
| Post-swap tripwire (E5) | real window + injected 15.2% error rate | **Autonomous rollback** + HIGH alert; statuses corrected |

Plus the autonomy proof (E6): a `promote_pending` cycle promoted with **no
human in the loop** (policy 2/2 operator confirms → canary 8/8 → swap →
`promoted|promote_auto|auto` → MEDIUM alert), and the class-5 proof (E7):
issue **#538** filed 20s after two failures clustered, third recurrence
**commented not duplicated**, jobhealth green.

## What shipped per epic

- **E1** — lease-aware `training_readiness_stale` (gauge
  `mdemg_ftloop_lease_held`, 60s republish ticker; live both-branch proof
  2.49min→0) + `fuse` split from `convert` in the ledger.
- **E2** — serving indirection: the as-built plist pointed at the REAL GGUF
  (the spec's rollback presumed a symlink that didn't exist); established
  `.local-models/serving/current.gguf`, one-time live cutover (~90s,
  watchdog-covered; first bootstrap EIO = the bootout race), `SwapServing`
  fail-closed (5 pin tests), `ft_model_versions` first writer,
  `mdemg model swap/rollback`, installer prefers the symlink.
- **E3** — `ft-loop promote` performs real promotion; drill-caught fix:
  post-swap ledger writes need a FRESH context (the 15s command ctx is dead
  after a multi-minute swap — the rolled_back event silently failed until
  fixed).
- **E4** — pre-swap canary: deterministic first-per-task probe slice of the
  `[AMD-2]`-pinned eval, STRUCTURAL divergence only, production-failed
  probes never held against the candidate.
- **E5** — post-swap tripwire: supervised loop, caller-cancellation-filtered
  error rate (LLM-HEALTH contract) with volume floor; field lessons:
  `llm_interactions.trace_id` NOT NULL; organic traffic dilutes the
  denominator (validates rate+floor over raw counts).
- **E6** — `FT_LOOP_AUTO_PROMOTE_AFTER` (default 3; 0=never) on the
  controller TICK (restart-safe — end-of-run-only would wedge a pending
  cycle across restarts); `PromoteCycle` extracted as the ONE flow;
  `[AMD-6]` resolved single-actor (no second RSIC executor; taxonomy
  classifies `promote_candidate` reversible; stays out of
  `AllowedLLMActions` per RSIC-LLM-ALERT-GUARD-001).
- **E7** — class-5 filer: volatile-token-normalized fingerprints
  (paths/hex/digit runs collapsed — pin-tested that recurrences cluster);
  CapabilityGap + fingerprint-labeled `gh` issue; recurrence = comment;
  jobhealth `ft-issue-filer` per filing.
- **E8** — this post + feature-doc §Phase-7 + CHANGELOG + CLAUDE.md.

## End-state honesty

- Serving: canonical `mdemg-llm-v1` active + healthy (every drill restored).
- Version ledger: 1 active, all drill versions `rolled_back`.
- Cycle ledger latest-status: 1 failed (the FTLOOP-DRILL-001 record),
  3 promoted (drill promotions incl. the auto one), 5 rolled_back
  (incl. the 3 E7 synthetic cycles neutralized by superseding events).
- Issue #538 closed as a drill artifact; drill fingerprint label deleted.
- All flags code-default OFF; `.env` carries only the pre-sprint state
  (canary flag from E4 remains, harmless while `FT_LOOP_ENABLED` is off).

## Follow-ups (disclosed)

- FT-RECURSIVE-004: `ft_production_drift` (score-trend monitoring beyond
  the tripwire window) + scheduled UBENCH re-runs.
- Filer per-process dedupe resets on restart (an already-commented
  recurrence may re-comment once after a restart) — acceptable noise,
  noted.
- The E7 sweep currently keys on `status='failed'` latest-event rows only;
  a repeated `rolled_back|canary_failed` pattern is visible in the ledger
  but not yet clustered — candidate extension for 004.

## Documents Accessed

SPEC §2E/§3/§3a/§4/§5; FTLOOP-DRILL-001 drill_record;
`internal/ftloop/*` (controller, stages, gate, lease, promote,
promote_cycle, canary, tripwire, issue_filer);
`internal/tsdb/{ft_cycle_ledger,ft_model_versions}.go`;
`internal/cli/{ft_loop,model_swap}.go`; `internal/ape/{task_dispatch,cycle}.go`;
`internal/gaps/{detector,store}.go`; live plist/ledger/alerts/log evidence.
