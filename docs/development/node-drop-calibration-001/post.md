# NODE-DROP-CALIBRATION-001 — Sprint Post

**Date:** 2026-07-27 | **Branch:** `reh3376_dev01`
**Parent:** ORPHAN-ALERT-001 (same defect class).

## Verdict

**Shipped.** The chronic-false-positive `graph_node_drop` CRITICAL alert
that fired on every routine recluster is replaced by a split
`graph_node_drop_ratio` + `graph_node_drop_count` pair with a min-node
significance floor, config-driven thresholds, and severity downgrade
CRITICAL → HIGH.

## Trigger

The 2026-07-27 FULL-INGEST + RECLUSTER maintenance pass produced a −362
node drop on `mdemg-dev` (identity-preserving L1 pattern cleanup during
the deliberate `mdemg concepts recluster` operator authorized). The old
rule fired CRITICAL and re-fired every 12 min against the 60-min rolling
comparison window — 0.4% of an 84k substrate should not be a data-loss
emergency.

## What was fixed (by defect)

- **Fixed absolute threshold `> 100 nodes`.** Extracted to
  `alert.GraphNodeDropRule(minNodes, ratioThreshold, absoluteThreshold)`
  factory mirroring ORPHAN-ALERT-001's `alert.OrphanRules(...)`. New
  defaults: min-nodes 50 / ratio 0.10 (10%) / absolute 10,000 (~10× the
  largest operator-authorized recluster delta observed on mdemg-dev).
- **No min-node significance floor.** SQL now gates both rules on
  `c.value >= minNodes` — the degenerate 2-node `global` /
  `rune-smoke-001` scratch spaces are excluded (proven live below).
- **Wrong severity: `CRITICAL`.** Downgraded to `SeverityHigh`. CRITICAL
  stays reserved for the actual data-loss emergencies it was designed for.
- **Distinct Services per rule** (NOSILENT-001 cooldown-key contract):
  `graph-node-drop-ratio` + `graph-node-drop-count`. The old rule used
  `graph-health`, which now collides with nothing.
- **Three config knobs** via `FromEnv`: `GRAPH_NODE_DROP_MIN_NODES`,
  `GRAPH_NODE_DROP_RATIO_THRESHOLD`, `GRAPH_NODE_DROP_ABSOLUTE_THRESHOLD`.

## Testing

- **Tier 1 (unit):** `TestGraphNodeDropRule_Defaults` +
  `TestGraphNodeDropRule_CustomParams` in `internal/alert/rules_test.go` —
  pin defaults, distinct Services, min-node floor visible in SQL,
  COALESCE(MAX(...)) idle-safe contract, param substitution.
- **Tier 1 pin update:** `TestDefaultRules_Count` bumped 6→5;
  `TestDefaultRules_RemovedRulesStayRemoved` catches any regression that
  reintroduces the inline rule.
- **Tier 2 (contract):** `TestAllRules_NoLimitOneAntiPattern` +
  `TestAllRules_DistinctServicePerSeverity` sweep both new rules through
  the same walker `allRules()` uses (added the factory call — new rules
  are auto-covered).
- **Tier 2 (build/lint):** `go build ./...`, `golangci-lint run ./...`
  clean (0 issues in edited packages), `go test ./...` full suite green.
- **Tier 3 (live):** simulated both new rules against real historical
  `metric_samples` at the exact moment the 2026-07-27 recluster drop was
  fully in-window — `mdemg-dev` old=84,408 → cur=84,331, drop=77
  (0.09%): **ratio verdict SUPPRESSED (0.0009 < 0.10)**, **absolute
  verdict SUPPRESSED (77 < 10,000)**. Even the full −362 drop = 0.43%
  ratio / 362 absolute, both far under thresholds. The OLD rule would
  have fired CRITICAL (77 > 100). Min-node floor query on the current
  substrate shows `global` (2) and `rune-smoke-001` (2) correctly
  EXCLUDED, `lnl-demo-whk` (9812) and `mdemg-dev` (84334) ELIGIBLE.

## Rules pinned

1. **Any new per-space graph-health alert rule MUST gate on a min-node
   significance floor** (matches the shipped ORPHAN-ALERT-001 pattern +
   the CLAUDE.md architectural rule) — a fixed absolute threshold on a
   per-space metric misbehaves across the ~4-order-of-magnitude space-size
   range MDEMG actually runs against.
2. **CRITICAL is for data-loss emergencies**, not for routine cleanup.
   Identity-preserving pattern churn during an authorized maintenance
   pass is HIGH at most.

## Cleanup / state

No scratch spaces created. The prior maintenance pass's real −362 drop
event remains in TSDB (`metric_samples`) as historical record; it will
age past the 60-min comparison window on its own within minutes of the
new rule taking effect.

## Documents Accessed

- `internal/alert/rules.go` (edited: extraction + factory)
- `internal/alert/rules_test.go` (edited: sweep walker + new unit tests
  + extraction pin)
- `internal/alert/evaluator_test.go` (edited: `TestDefaultRules_Count`
  bumped 6→5)
- `internal/config/config.go` (edited: 3 field defs, FromEnv reads,
  struct literal assignments)
- `internal/cli/serve.go` (edited: factory wire alongside `OrphanRules`)
- `docs/development/orphan-alert-001/` (parent-pattern reference)
- `CLAUDE.md` (updated architectural note — pinned rule extends to
  node-drop deltas)
- `CHANGELOG.md` (Changed entry)
- Live TSDB queries against `metric_samples` for Tier 3 evidence
