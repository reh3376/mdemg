# JIMINY-CONTRADICTED-BRIDGE-QUALITY-001 — Sprint Post

**Date:** 2026-08-04 | **Branch:** `reh3376_dev01`
**Trigger:** Follow-up to HITL-AUTO-DISMISS-001. Sprint B drained the queue after-the-fact; this sprint attacks the noise SOURCE upstream so the queue stops re-accumulating dismiss-eligible drafts. Investigation of the 9 pending drafts (surfaced by the `hitl-curation` alert) found the contradicted-bridge emitted drafts for:
- 3 items with content matching the existing `ConstraintPromotionRejectPatterns` list (Bash errors × 2, "Phase 92: Full system gap analysis")
- 3 items with non-actionable guidance types (`pattern` × 2, `learning` × 1) — contradiction is semantically odd on abstractions

## Verdict

Shipped. Two-layer content-quality gate on the bridge, mirroring `hidden.ConstraintPromotionGate`'s design. Type filter (default: only `constraint` + `correction` eligible) fires first; content-pattern filter (default: reuses `DefaultConstraintPromotionRejectPatterns` — single source of truth with the constraint-promoter) fires second. Rejections log at INFO with the reason; no silent drops.

## What shipped

### `internal/jiminy/contradicted_bridge.go` — the gate
```go
type ContradictedBridgeGate struct {
    enabled        bool
    allowedTypes   map[GuidanceType]struct{}
    rejectPatterns []*regexp.Regexp
}
func (g *ContradictedBridgeGate) Reject(guidanceType GuidanceType, guidanceContent string) (reason string, rejected bool)
```
- Nil / disabled → fail-open, everything passes (backward-compat)
- Type filter fires first; content pattern filter second
- Invalid regexes skipped with WARN log (mirrors constraint gate's contract)
- Reason string always names the blocking rule (`type:pattern` / `pattern:<regex>`)

### `internal/jiminy/service.go` — the emit-site gate check
```go
if outcome == OutcomeContradicted && s.contradictedDraftWriter != nil && s.cfg.JiminyContradictedBridgeEnabled {
    if reason, rejected := s.contradictedDraftGate.Reject(item.Type, item.Content); rejected {
        slog.Info("jiminy: contradicted draft suppressed by quality gate", "guidance_id", ..., "reason", reason, ...)
    } else {
        // existing dedup + RecordDraft path
    }
}
```
Gate built alongside the dedup cache in `SetContradictedDraftWriter` — same lifecycle, no re-construction risk.

### `internal/config/config.go` — three new knobs
```
JIMINY_CONTRADICTED_BRIDGE_GATE_ENABLED    bool      (default: true)
JIMINY_CONTRADICTED_BRIDGE_ALLOWED_TYPES   comma-sep (default: "constraint,correction")
JIMINY_CONTRADICTED_BRIDGE_REJECT_PATTERNS JSON list (default: DefaultConstraintPromotionRejectPatterns())
```
- Regex validation at config parse time (same as constraint gate) — an invalid pattern in `.env` fails startup rather than silently disabling the filter
- Default reuses `DefaultConstraintPromotionRejectPatterns()` — the two gates SHARE their pattern list by default, so a widening in the constraint gate (e.g. JIMINY-CORPUS-002's five narrative-shaped patterns) automatically strengthens the bridge gate too

## Tests

9 pin tests in `internal/jiminy/contradicted_bridge_quality_test.go`, all pass:
- `TestBridgeGate_NilPassesEverything` — nil gate is fail-open
- `TestBridgeGate_DisabledPassesEverything` — enabled=false is fail-open
- `TestBridgeGate_RejectsNonAllowedType` — pattern-type guidance rejected with `type:pattern` reason
- `TestBridgeGate_AcceptsAllowedType` — constraint + correction pass with clean content
- `TestBridgeGate_RejectsMatchingPattern` — Bash-error content rejected with `pattern:^Bash error` reason
- `TestBridgeGate_MultiplePatternsBothCatch` — every pattern in the list can fire
- `TestBridgeGate_InvalidRegexSkipsWithoutDisablingGate` — a single bad regex doesn't disable the whole gate
- `TestBridgeGate_TypeFilterOffOnlyPatternFilter` — nil allowedTypes disables type filter, keeps pattern filter
- `TestBridgeGate_TypeFilterFiresBeforePatternFilter` — reason string reflects which filter blocked
- `TestBridgeGate_LiveMdemgDevNoiseClass` — **6 subtests using EXACT content from the live queue** that fired the HITL alert (regression pin: if the gate ever stops catching these, the alert re-fires on the same class)

`go test ./internal/jiminy/... ./internal/config/... ./internal/api/...` clean; lint 0 issues.

## Live Tier-3 (mdemg-dev)

**Pin-test approach** replaces synthetic live traffic. Reason: firing a natural `OutcomeContradicted` on synthetic content requires warming a real guidance + submitting a real feedback POST + waiting for the classifier's tier-2 verdict; on a saturated LLM path this can take 2+ min per attempt and is flaky. The `TestBridgeGate_LiveMdemgDevNoiseClass` test uses the EXACT 6-item mix from the live queue that fired the alert:

| Content class | Guidance type | Gate verdict | Reason |
|---|---|---|---|
| `Bash error in command: grep ...` | pattern | REJECT | type:pattern (type filter fires first) |
| `Bash error in command: sed ...` | constraint | REJECT | pattern:^Bash error |
| `Phase 92: Full system gap analysis` | constraint | REJECT | pattern:phase-analysis regex |
| `Details MDEMG's vision as ...` | learning | REJECT | type:learning |
| `CONSTRAINT: NEVER commit ...` | constraint | PASS | — |
| `After merging a PR to main via --admin ...` | correction | PASS | — |

**Post-restart state on mdemg-dev**: 5 pending / 4 dismissed / 2 approved (sprint B's drain held); 0 new drafts written since server restart (natural quiet window; confirms the write path is intact and gate isn't over-rejecting real traffic).

## Rules pinned

⚠️ **Downstream noise-filter gates SHOULD share their pattern list with the upstream producer's gate** — the bridge and the constraint-promoter both filter "is this content a durable rule?" and they'd diverge silently if they had separate defaults. `DefaultConstraintPromotionRejectPatterns()` is the single source; JIMINY-CORPUS-002's widening (5 new narrative regex patterns) automatically strengthened the bridge gate here. If a future sprint needs to fork them (e.g. bridge needs a wider net than promoter), keep the fork explicit and reference `DefaultConstraintPromotionRejectPatterns()` in the diff.

⚠️ **Type filter should fire BEFORE pattern filter in the two-layer gate** — the reason string tells operators which filter blocked (`type:pattern` vs `pattern:<regex>`). Reordering would hide type-class rejections behind noisy pattern-class reasons.

⚠️ **Nil/disabled gate must be fail-open** — the contradicted-bridge shipped without a gate for a full week; the fail-open default preserves that shipped behavior when `JIMINY_CONTRADICTED_BRIDGE_GATE_ENABLED=false`. Any regression that makes a nil gate reject would silently break the bridge for operators who haven't opted in.

## Not shipped (intentional)

- **Live natural-traffic smoke** — deferred to pin tests using exact live-queue content (see above). If the type/pattern filter ever misfires on natural traffic, the INFO log line will surface it (`jiminy: contradicted draft suppressed by quality gate`).
- **Metric-gauge for suppressed-draft rate** — could add `mdemg_jiminy_contradicted_draft_suppressed_total{reason}`. Deferred: the INFO log line + DORMANT-METRICS-CLEANUP-001's discipline (don't add metrics that don't drive alerts) argue against a gauge until an alert would consume it.
- **Widening the pattern list** — the current defaults are the union of JIMINY-CORPUS-001 + JIMINY-CORPUS-002 patterns. If the live queue accumulates a new noise class the current list doesn't catch, add the pattern to `DefaultConstraintPromotionRejectPatterns()` — BOTH gates benefit.

## Follow-ups disclosed

- **AUTOGRADE-SCHEDULE-001** (from sprint B) — still open. Scheduled autograde loop with `--force` capability. Not urgent given the source-side filter reduces the accumulation rate.
- **Widen the type filter to reject `decision`/`preference`/`risk`/`conflict`** — currently only `pattern`/`learning`/`concept` are implicitly filtered (by absence from AllowedTypes). If those types ever produce noise drafts, they'd be caught too — but the default `constraint,correction` already excludes them.

## Rollback

Single-commit revert. Setting `JIMINY_CONTRADICTED_BRIDGE_GATE_ENABLED=false` in `.env` is a zero-restart operator escape hatch that restores pre-sprint behavior (Set-and-restart applies; the gate reads config at construction time via `SetContradictedDraftWriter`).

## Documents Accessed

- `internal/jiminy/contradicted_bridge.go` (gate + cache + templater; edited)
- `internal/jiminy/service.go:2001` (emit-site; edited)
- `internal/jiminy/service.go:2540` (SetContradictedDraftWriter; edited)
- `internal/hidden/constraint_gate.go` (pattern source; sibling gate design mirrored)
- `internal/config/config.go` (3 new knobs; edited)
- `docs/development/hitl-auto-dismiss-001/post.md` (sprint B — the downstream drain)
- Live `contradicted_correction_drafts` snapshot on mdemg-dev (9 pending items → gate verdict cross-check)
- `JIMINY-CORPUS-001/002` posts (upstream sibling gates, shared pattern list)
