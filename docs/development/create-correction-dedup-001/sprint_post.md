# CREATE-CORRECTION-DEDUP-001 — Sprint Plan + Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Follow-up from:** JIMINY-CORRECTION-CORPUS-001 (same day)

## Problem

JIMINY-CORRECTION-CORPUS-001 tombstoned 35 of 39 live corrections; 24 of those tombstones were DUPLICATES of shipped constraints (e.g. `always-commit-before-goreleaser` = `no-stash-for-release`; `never-commit-to-main-directly` = `no-direct-main-commits`; `auto-b48d40e79848` = `auto-build-restart-after-feature`; `must-master-bash-sed-edit` = `plan-mode-before-change`).

Without a promotion-time dedup check, the promoter (`CreateCorrectionNodes`) will re-mint these on every consolidation cycle as fresh L0 correction observations arrive with similar semantics — the 24-DUP class will re-accumulate over days, dragging the actionable follow rate back down.

Root cause: `CreateCorrectionNodes`'s existing idempotency check (line 176) matches by `name` only — catches exact-name reruns of the same obs but doesn't dedupe against **cross-role** (constraint) or **similar-content** matches.

## Shipped

**Config knobs** (`internal/config/config.go`):
- `JiminyCorrectionDedupEnabled` bool, default **true** — safe default because dedup only fires when a specific high-similarity live node exists; never blocks a truly-novel correction
- `JiminyCorrectionDedupSimThreshold` float64, default **0.75** — conservative "same rule" cutoff (the constraint-code match threshold 0.55 is looser; too-low threshold here would over-suppress). 0 disables regardless of enabled flag.

**Dedup logic** (`internal/hidden/correction_nodes.go`):
- Added `SkippedDup int` field to `CorrectionNodeResult` for live-smoke telemetry
- Inside the CREATE branch (after name-based idempotency check fails), if dedup is enabled + threshold > 0 + obs has embedding, run a vector-index query:
  ```cypher
  CALL db.index.vector.queryNodes('memNodeEmbedding', 5, $embedding)
  YIELD node AS n, score AS sim
  WHERE n.space_id = $spaceId
    AND n.role_type IN ['constraint','correction']
    AND NOT coalesce(n.is_archived, false)
    AND sim >= $threshold
  RETURN n.node_id AS nid, ...
  ORDER BY sim DESC LIMIT 1
  ```
- If a hit → `slog.Info("correction promotion skipped: duplicate of existing live node", obs_id, dup_node_id, dup_code, dup_role, similarity, threshold)`, increment `res.SkippedDup`, `continue` to next obs (no create)
- If query error → non-fatal, WARN + fall through to create (better to mint a possible dup than fail the whole consolidation)
- Summary log at end reports `SkippedDup` count when > 0

**Pin tests** (`internal/hidden/correction_dedup_test.go`):
- `TestCorrectionDedupConfig_DefaultsAreSafe` — enabled=true, threshold=0.75
- `TestCorrectionDedupConfig_ThresholdZeroDisables` — escape hatch
- `TestCorrectionDedupConfig_EnabledFalseDisables` — primary disable
- `TestCorrectionNodeResult_SkippedDupFieldExists` — telemetry field contract

## Live Tier-3

- `go build ./...` clean; `golangci-lint run ./internal/hidden/... ./internal/config/...` = 0 issues; 4 new pins green + existing hidden/config suites green
- **Behavioral verification requires next natural consolidation cycle** — when a fresh L0 correction obs is promoted (during `RunConsolidation` phase-20 correctionStep), the dedup check will fire against the current live 33 constraints + 3 corrections. Observable via server log grep `correction promotion skipped: duplicate` OR post-consolidation `CorrectionNodeResult.SkippedDup > 0`.
- Not exercised inline because forcing consolidation would burn 30+ min real time.

## Two arch rules pinned (CLAUDE.md)

1. **Promoter-time dedup MUST be cross-role.** `CreateCorrectionNodes` and `CreateConstraintNodes` are separate promoters, but the dedup check MUST query BOTH role types (`n.role_type IN ['constraint','correction']`). Otherwise 24 out of every 39 promoted corrections will be re-mints of shipped constraints (the concrete class we just observed). Same logic applies in reverse to `CreateConstraintNodes` — if a similar-content correction already exists, don't mint a new constraint. Follow-up: add symmetric dedup to `CreateConstraintNodes` (deferred; low-priority since constraint promotion is stricter-gated).

2. **Substrate-mutation gates that CAN fail on query error MUST fall through, not fail-closed.** `CreateCorrectionNodes` is called in the consolidation critical path; failing the whole consolidation because a single dedup query returned an error would be a bigger operational cost than the occasional mint-of-dup. The fall-through path WARN-logs so operators know to check. This is the same fail-open pattern used by the JIMINY-CORPUS-001 ConstraintPromotionGate on backstop-regex parse errors.

## Follow-ups disclosed

- **Symmetric constraint-side dedup** — `CreateConstraintNodes` should also check against live corrections. Deferred; low-priority since constraint promotion is already stricter-gated (F6a LLM classifier gate + regex tag requirement). Only ship if telemetry shows constraints being re-minted as dups of corrections.
- **Link vs skip** — current policy is SKIP the correction promotion. Alternative: LINK the L0 obs to the existing L1 node via `IMPLEMENTS_CORRECTION` edge so provenance is preserved. Ship if operator wants dedup metadata retained.
- **Threshold recalibration** — 0.75 is a first-guess default; if `SkippedDup` counts drift toward zero over 30 days on real traffic, lower to 0.70. If false-positive skips of legitimate novel corrections happen, raise to 0.80. Passive re-check.

## Documents Accessed

- `docs/development/jiminy-correction-corpus-001/sprint_post.md` — why this sprint exists
- `internal/hidden/correction_nodes.go` (CreateCorrectionNodes structure)
- `internal/hidden/service.go` (Service struct + cfg access)
- `internal/config/config.go` (JiminyConstraintCodeSimThreshold precedent for the parse+wire shape)
- CLAUDE.md pins: JIMINY-CORPUS-001, JIMINY-CORRECTION-PRODUCER-001, JIMINY-CORRECTION-CORPUS-001, JIMINY-CEILING-BREAK-2
