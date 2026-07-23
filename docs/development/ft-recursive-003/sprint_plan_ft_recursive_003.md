# Sprint FT-RECURSIVE-003 — Phase 7: promotion executor, canary, auto-rollback, RSIC action class

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-RECURSIVE-003 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~4 dev-days (spec §4 Phase 7); epics land individually shippable |
| Parent | `docs/development/ft-recursive-001/SPEC_recursive_retraining_loop.md` §3/§4 Phase 7 + FTLOOP-DRILL-001's two recorded pre-work items |

## 2. Problem Statement

The loop is drill-proven through `promote_pending`, but promotion itself is
ledger-theater: `mdemg ft-loop promote --confirm` records a decision and
nothing else — no serving mutation, no `ft_model_versions` row (still a
zero-writer table), no canary, no rollback primitive. Worse, the spec's
rollback mechanic ("restore symlink + kickstart") presumes a serving
indirection that DOES NOT EXIST: the llama-server plist points at the real
file `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf`. And two
drill findings await: the readiness-staleness rule false-alarms during a
legitimate quiesce, and the ledger hides fuse inside `train`'s window.

## 3. Scope & Constraints

**In scope (sequential epics, each shippable alone):**
- **E1 — drill pre-work pair**: (a) `training_readiness_stale` becomes
  lease-aware (suppressed while a cycle holds the compute lease — the
  heartbeat SHOULD pause during a retrain); (b) new `converting` ledger
  stage posted before fuse (sharper timeline).
- **E2 — serving indirection + rollback primitive + model-versions writer**:
  establish `.local-models/serving/current.gguf` symlink → production GGUF;
  plist `--model` retargeted to the symlink (one-time live cutover,
  verified byte-identical serving); `internal/ftloop/promote.go` gains
  `SwapServing(target)` (atomic symlink retarget + kickstart + health-wait
  + fail-closed revert on unhealthy) and `Rollback()`;
  `ft_model_versions` writer revived (V0002 table, first writer) — one row
  per swap with SHAs/scores/cycle id; new `mdemg model rollback` CLI.
- **E3 — promotion executor**: `ft-loop promote --confirm` performs the real
  flow: pre-swap canary (E4 gate if built, else skip-with-log) → SwapServing
  → post-swap health verify → ledger `promoted` + model_versions row;
  failure at any point = auto-revert + ledger `rolled_back` with cause.
- **E4 — pre-swap canary (held-call replay)**: replay `FT_LOOP_CANARY_PROBES`
  (default: the UBENCH min_rows_per_task slice) against the candidate on the
  side-port; structural-divergence checks (parse/shape/finish_reason, not
  exact-match) → divergence blocks promotion.
- **E5 — post-swap tripwire**: `FT_LOOP_CANARY_WINDOW_MIN` (60) elevated
  LLM-error tripwire (caller-cancellation-filtered, per the
  LLM-HEALTH contract) → auto `Rollback()` + high alert.
- **E6 — RSIC action class + auto-promote policy**: the mutating-long-running
  action class validated fail-closed (RSIC-VALIDATE-001 semantics; snapshot
  = the pre-swap model_versions row) `[AMD-6]`;
  `FT_LOOP_AUTO_PROMOTE_AFTER` (default 3, 0 = never-auto) — auto path only
  after N operator-confirmed promotions.
- **E7 — class-5 issue filer** via `internal/gaps` + `gh issue create`
  (fingerprint-idempotent, jobhealth-reported).
- **E8 — docs** (feature doc, CHANGELOG, CLAUDE.md, post).
**Out of scope:** Phase 9 drift monitoring (`ft_production_drift` beyond the
E5 window tripwire); multi-host serving.
**Constraints:** production serving mutations ONLY in verified, reversible
steps with health-wait + fail-closed revert; every new env var config-driven
with defaults; UATS/contract coverage for any HTTP-surface change in the
same epic (the 2026-07-23 UxTS reminder); no promotion of the drill's
archived candidate (it remains a non-candidate).

## 4. Dependencies

✅ Drilled loop (FTLOOP-DRILL-001); ✅ `resolveTool` (launchd PATH);
✅ ledger + jobhealth + alert plumbing; ✅ watchdog state machine (post-swap
health); ✅ `ft_model_versions` DDL exists (zero-writer); ✅ archived drill
adapter + regenerable candidate for the bad-candidate rollback drill;
✅ `internal/gaps` CapabilityGap store.

## 5. Implementation Plan

Sequential E1→E8 as above. The sprint's exit criterion (spec): **a bad
candidate auto-rolls-back in a live drill; the action class validates
fail-closed.** Each epic ends with its own live Tier-3 before the next
begins. Work proceeds until the JIMINY A/B re-measure fires (2026-07-24
09:41); whatever epic is mid-flight at that point completes before the
sprint pauses.

## 6. Testing Plan

Tier 1: unit per epic (symlink swap semantics with temp dirs; canary
divergence classifier; policy counter; rule SQL pins). Tier 2:
`go test ./...` + UATS. Tier 3 per epic: E2 live cutover byte-identical
serving + live rollback drill on the PRODUCTION model (swap to itself);
E3/E4/E5 live bad-candidate drill (a deliberately-degraded GGUF must be
refused pre-swap, and if forced past the canary, auto-roll-back within the
window); E6 fail-closed validation drill.

## 7. Commit Strategy

`docs(E0)` → one `feat`/`fix` commit per epic with its Tier-3 evidence →
`docs(E8)`. Surprise defects: own fix-commits.

## 8. Verification Checklist

E1 rule suppressed under a held lease + fires without one · `converting`
stage visible in a ledger timeline · E2 cutover: serving byte-identical
(same model list + a probe completion), rollback drill restores in <60s ·
model_versions row lands per swap · E3 promote executes the full flow ·
E4 divergent candidate blocked pre-swap · E5 forced-bad swap auto-rolls-back
in-window · E6 action class fail-closed · unit+lint+UATS green · docs.

## 9. Documentation Update

`docs/features/ft-recursive-loop.md` §Phase-7; CHANGELOG; CLAUDE.md
FT open-work note; post.md.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Serving cutover breaks production llama-server | High | Symlink established pointing at the CURRENT file first; plist edit + kickstart verified with health-wait + model probe; revert path is retarget-back; performed as its own supervised live step |
| Auto-rollback flaps (tripwire too sensitive) | Med | Caller-cancellation-filtered error signal (LLM-HEALTH contract); window + threshold config-driven; rollback is idempotent |
| Canary false-blocks good candidates | Med | Structural divergence only (shape/parse), not score-exact; probes pinned + versioned |
| RSIC action class mutates outside its snapshot | High | RSIC-VALIDATE-001 fail-closed pattern; snapshot = pre-swap model_versions row; drill validates refusal paths |

## 11. Rollback

Each epic independently revertible; serving indirection reverts by
retargeting the plist to the direct file path. Config kill-switches:
`FT_LOOP_AUTO_PROMOTE_AFTER=0`, `FT_LOOP_ENABLED=false`.

## 12. Documents Accessed

SPEC_recursive_retraining_loop.md §2E/§3/§3a/§4/§5; FTLOOP-DRILL-001
drill_record; `internal/cli/ft_loop.go` (promote as-built);
`internal/tsdb/migrations/002_ft_schema.sql` (ft_model_versions DDL);
`~/Library/LaunchAgents/com.mdemg.llama-server.plist` (direct-path finding);
`internal/ftloop/*`; LLM-HEALTH-INVESTIGATION-001 (error-signal contract).
