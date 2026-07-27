# NODE-DROP-CALIBRATION-001 — Sprint Plan

**Date:** 2026-07-27 | **Branch:** `reh3376_dev01`
**Parent:** ORPHAN-ALERT-001 (same defect class — per-space graph-health
alert with a hardcoded threshold, no min-node significance floor, and
CRITICAL severity that fires on routine maintenance).

## 1. Header & Metadata

Fix the chronic false-positive `graph_node_drop` alert
(`internal/alert/rules.go:96–127`) surfaced during the FULL-INGEST +
RECLUSTER maintenance pass on 2026-07-27: a deliberate operator-triggered
recluster tightened `mdemg-dev` L1 patterns 5210 → 4729, net −362 nodes
(0.4% of an 84k substrate). The rule fired **CRITICAL** on that drop and
re-fired every 12 min for the rolling hour window.

## 2. Problem Statement

Three structural defects, same class as ORPHAN-ALERT-001:

1. **Fixed absolute threshold `> 100 nodes`.** Calibrated when substrates
   were small; 100 nodes = 0.12% of a mature space but 20% of a 500-node
   scratch space. Every routine recluster (5–10% L1 tightening) trips it
   on the mature side; every tombstone burst on the tiny side trips it too.
2. **No minimum node-count floor.** A degenerate 1-node or 5-node
   `uats-*` / `global` / scratch space losing 3 nodes fires the alert —
   identical to the per-space graph-health defect ORPHAN-ALERT-001 fixed.
3. **Wrong severity: `CRITICAL`.** CRITICAL is reserved for data-loss
   emergencies (backup missing, live corruption). Identity-preserving
   pattern cleanup during an authorized maintenance pass is not a
   CRITICAL event — HIGH is the correct calibration.

Plus the standing "no hardcoded values" rule: the `100` threshold is a
magic literal in the rule struct, unreachable to operators.

## 3. Scope & Constraints

**In scope:**
- Extract inline `graph_node_drop` `AlertRule` → `alert.GraphNodeDropRule(
  minNodes, ratioThreshold, absoluteThreshold)` factory mirroring
  `alert.OrphanRules(...)` (`internal/alert/rules.go`).
- Add three config knobs:
  - `GRAPH_NODE_DROP_MIN_NODES` (default 50, matches
    `ORPHAN_RATIO_MIN_NODES`)
  - `GRAPH_NODE_DROP_RATIO_THRESHOLD` (default 0.10 = 10% drop, matches
    `ORPHAN_RATIO_THRESHOLD`)
  - `GRAPH_NODE_DROP_ABSOLUTE_THRESHOLD` (default 10000 — catches mass loss
    on huge substrates even when <10%; calibrated ~10× the largest
    operator-authorized recluster delta observed on mdemg-dev)
- Split into TWO rules with distinct Service labels (NOSILENT-001
  cooldown-key contract): `graph-node-drop-ratio` + `graph-node-drop-count`.
- Downgrade Severity `CRITICAL` → `SeverityHigh`.
- Preserve idle-safe aggregation contract (COALESCE(MAX(...),0), no
  `ORDER BY … LIMIT 1`) — TSDB-CONSUME-001.
- Wire the factory in `internal/cli/serve.go` alongside `OrphanRules(...)`.

**Out of scope:**
- The old inline `graph_node_drop` `DefaultRules` entry is deleted
  (extracted, not left in place — same as ORPHAN-ALERT-001 did for
  `high_orphan_ratio`/`_count`).
- The rolling comparison window (currently 60-min-ago vs now) is preserved.
- Additional graph-health rules — this sprint calibrates one rule.

## 4. Dependencies

- ORPHAN-ALERT-001 (shipped): pattern being mirrored.
- NOSILENT-001 (shipped): distinct-Service-per-rule cooldown-key contract.
- TSDB-CONSUME-001 (shipped): idle-safe alert-SQL contract.
- ALERT-TRUTH-001 (shipped): `TestAllRules_NoLimitOneAntiPattern` +
  `TestAllRules_DistinctServicePerSeverity` sweep tests auto-cover new
  rules — no test-suite edit needed for the sweep coverage.

## 5. Implementation Plan (sequential epics + gates)

**E1 — Extract `GraphNodeDropRule` factory** (`internal/alert/rules.go`)
- Delete the inline `graph_node_drop` entry from `DefaultRules`.
- Add `GraphNodeDropRule(minNodes int, ratioThreshold float64,
  absoluteThreshold int) []AlertRule` returning the two split rules.
- Preserve the current CTE shape (`current_val` vs `old_val` join by
  `space_id`); add the `nodes` significance floor CTE and gate both rules
  on `nodes.total_nodes >= minNodes`.
- Ratio rule: `MAX(CASE WHEN n.total_nodes >= minNodes AND o.value > 0
  THEN (o.value - c.value) / o.value ELSE 0 END)` compared to
  `ratioThreshold`.
- Absolute rule: same-CTE `MAX(CASE WHEN n.total_nodes >= minNodes
  THEN (o.value - c.value) ELSE 0 END)` compared to `absoluteThreshold`.
- **Gate:** file compiles.

**E2 — Config wire** (`internal/config/config.go`)
- Add three struct fields with pinned defaults matching ORPHAN-ALERT-001.
- Extend `FromEnv()` block with `atoi("GRAPH_NODE_DROP_MIN_NODES", 50)`,
  `atof("GRAPH_NODE_DROP_RATIO_THRESHOLD", 0.10)`,
  `atoi("GRAPH_NODE_DROP_ABSOLUTE_THRESHOLD", 10000)`.
- **Gate:** `go build ./...` green, defaults documented.

**E3 — Serve wire** (`internal/cli/serve.go`)
- Append the factory call alongside `OrphanRules(...)`.
- **Gate:** `go build ./...` green.

**E4 — Unit test** (`internal/alert/rules_test.go`)
- `TestGraphNodeDropRule_MinNodeFloor` — 5-node space loses 3, rule reads 0.
- `TestGraphNodeDropRule_RatioAbove/_Below` — 1000-node space drops 150
  (ratio 0.15 > 0.10) fires; drops 50 (ratio 0.05) does not.
- `TestGraphNodeDropRule_AbsoluteAbove` — 200k-node space drops 15000
  (ratio 0.075 < 0.10 but absolute 15000 > 10000) fires the count rule.
- **Gate:** `go test ./internal/alert/... -count=1` green.

**E5 — Sweep tests already cover us**
- `TestAllRules_NoLimitOneAntiPattern` re-reads the extended rule set via
  the existing walker + fails on any `ORDER BY … LIMIT 1`.
- `TestAllRules_DistinctServicePerSeverity` covers the new distinct
  Services (`graph-node-drop-ratio` / `graph-node-drop-count` — no
  collision with `graph-health-count`/`-ratio` from OrphanRules).
- **Gate:** `go test ./internal/alert/... -count=1` green.

**E6 — Lint + full-tree build**
- `golangci-lint run ./...` clean.
- `go test ./... -count=1` full suite green.

**E7 — Docs (mandatory Documentation Update epic)**
- CLAUDE.md architecture note: extend the ORPHAN-ALERT-001 min-node-floor
  rule to explicitly cover node-drop deltas.
- CHANGELOG entry under `### Changed`.
- Sprint `post.md`.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `TestGraphNodeDropRule_*` (E4).
- **Tier 2 (contract):** `TestAllRules_NoLimitOneAntiPattern` +
  `TestAllRules_DistinctServicePerSeverity` sweep coverage.
- **Tier 3 (live):** kickstart server with new binary; query
  `mdemg-dev`'s current graph gauge (the −362 real observed drop from the
  2026-07-27 recluster is still in the rolling window); confirm both new
  rules read 0 (drop 362 / 84047 = 0.43% < 10% ratio floor; 362 < 10000
  absolute floor); a small scratch space (5-node) loses 3 → the min-node
  floor excludes it → rule reads 0; force-fire test via `GRAPH_NODE_DROP_
  ABSOLUTE_THRESHOLD=100` env override + kickstart → new alert fires on the
  same real gauge; unset + kickstart to restore.

## 7. Commit Strategy

Single fix-commit under the `NODE-DROP-CALIBRATION-001` slug:
`fix(alert): calibrate graph_node_drop — ratio + min-node floor + severity
downgrade (NODE-DROP-CALIBRATION-001)`.

## 8. Verification Checklist

- [ ] `graph_node_drop` inline entry removed from `DefaultRules`.
- [ ] `GraphNodeDropRule` factory exported + mirrors `OrphanRules` shape.
- [ ] Three config knobs read via `FromEnv()` with pinned defaults.
- [ ] Serve wire matches the OrphanRules wire (same block).
- [ ] Severity is HIGH, distinct Services per rule.
- [ ] Unit tests + both sweep tests green.
- [ ] `go build`, `golangci-lint`, full `go test ./...` green.
- [ ] Live smoke: real −362 drop reads 0 in both new rules on mdemg-dev.
- [ ] Live force-fire test: `ABSOLUTE_THRESHOLD=100` env override reproduces
      the alert on the same gauge; unset restores.
- [ ] CLAUDE.md architecture note extended.
- [ ] CHANGELOG entry.
- [ ] Sprint `post.md` with Documents Accessed.

## 9. Rollback Procedures

Rule extraction is additive-in-shape (delete inline + add factory + wire):
revert commit fully removes both rules. Config knobs are default-safe;
absent env vars → shipped defaults (50 / 0.10 / 10000). No schema change.

## 10. Risks & Mitigations

- **Risk:** the new defaults (10% / 10k) mask a real substrate emergency
  that a lower threshold would have caught.
  - **Mitigation:** 10% is 2 orders of magnitude below the 5-min alert
    ForDuration × alert cooldown default; a real 10%+ substrate loss is
    catastrophic and should page. The absolute floor of 10000 catches mass
    loss on huge spaces where 10% would still be too large in absolute
    terms — but calibrated 10× above the largest operator-authorized
    recluster delta observed live (max delta 481 on 5210 L1 patterns).
    Operator can tighten via env if the mdemg-dev-observed norm differs.
- **Risk:** the extracted rule loses coverage on a truly small space
  losing everything.
  - **Mitigation:** the min-node floor is 50 (matches ORPHAN-ALERT-001);
    a 50-node space losing 5 nodes = 10% ratio, still fires. Below 50
    nodes is scratch-space territory (`uats-*`, `global`, drills) and
    correctly excluded — same as ORPHAN-ALERT-001's conclusion.

## 11. Documents Accessed

To be filled in `post.md`.

## 12. Documentation Update

**Final epic — never cut** (Sprint Plan Format v1.0). Covered by E7.
