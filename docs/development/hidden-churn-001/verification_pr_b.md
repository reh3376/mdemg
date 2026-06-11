# HIDDEN-CHURN-001 — PR-B Verification (Coverage Retune + Grounding Tools)

Date: 2026-06-11 · Branch: `reh3376_dev01` · Space: `mdemg-dev` (live)

PR-A shipped stable theme identity. PR-B closes the coverage gap that churn
left behind and ships the grounding audit/repair tools.

## Epics delivered

| Epic | Deliverable | Commit |
|---|---|---|
| B1 | Config-driven theme ratio + density assignment + coverage gauge + alert rule | `7f5c511` |
| B2 | `mdemg concepts repair` + `mdemg concepts trace` | (B2 commit) |
| B3 | `themes_updated` / `noise_assigned` in consolidate API + periodic log | `46c3718` |
| B4 fix | Live-smoke fixes (noise pool, archived filter, min-obs gate) | (fix commit) |

## Tier 1 — unit

- `TestCoverageRules` + `TestMetricSamplesRules_UseTimeColumn` (alert) — the
  second pins the `recorded_at` → `time` column fix (see Bonus below).
- `TestConceptsRepair_DefaultsToDryRun`, `TestConceptChildEdges_IncludesAbstractsTo`,
  `TestConceptsCmd_HasSubcommands` (cli).
- Existing hidden/api/metrics/config suites green.

## Tier 3 — live (real binary, real services, real outputs)

### Live-smoke catches (the Tier 3 forcing function, 3 defects — fix commit)

1. **Noise pool was structurally empty.** KMeans never emits label −1, so
   `assignNoiseToThemes` always received `[]` — `noise_assigned` could never
   be nonzero. Cross-check that exposed it: 110/122 sampled unthemed
   observations had best-theme cosine ≥ 0.70, yet 0 assigned. The
   min-samples / max-themes / nil-centroid drops now feed members into the
   noise pool.
2. **Clustering included archived debris.** The observation fetch had no
   `is_archived` filter — it clustered 4,838 observations of which only
   183 were live (MAINT-LIVE-001 tombstones). Themes were being built on
   archived data. After the filter: clusterable count 4,838 → 65.
3. **Born-firing alert hazard.** Tiny scratch/test spaces (2–13 obs) emitted
   coverage 0.000 and would have alarmed forever. Gauge now gated on
   `CONVERSATION_COVERAGE_MIN_OBS` (default 50, DH-005 confidence pattern).

### Consolidation (after fixes)

| Cycle | themes_created | themes_updated | noise_assigned | Notes |
|---|---|---|---|---|
| pre-fix | 0 | 24 | 0–1 | themes matched, but built on archived debris |
| post-fix 1 | 4 | 1 | 0 | debris themes swept 24 → 5 clean |
| post-fix 2 | 0 | 5 | 0 | **stable identity on clean data** |

`noise_assigned: 0` post-fix is legitimate — all 5 clusters passed
min-samples, so no members were dropped that cycle.

### Coverage gauge + alert

- `metric_samples`: `mdemg_neo4j_conversation_coverage_ratio{mdemg-dev}` =
  0.318 → **0.331** across cycles; tiny spaces emit nothing (gate verified).
- Evaluator loaded 19 rules incl. `low_conversation_coverage`
  (floor 0.2, ForDuration 6h). 0.331 > 0.2 → silent, correctly.

### `concepts repair` (LIMIT-5-first per the destructive-ops rule)

- Dry-run preview: 10,395 childless layer≥2 nodes (all layer 2; matches the
  roadmap audit number exactly once the predicate uses the real grounding
  edge — see Predicate note).
- `--limit 5 --dry-run=false`: 5 tombstoned, verified by query
  (`archived_reason='childless_concept_repair'`, recoverable).
- Full run: **10,390 further tombstoned; re-preview shows 0 childless**.

### `concepts trace`

- Grounded-to-L1 concept: members listed with edge types, census per layer,
  honest `UNGROUNDED` verdict (chain stops at L1 — see Hierarchy state).
- Childless node: `(none — candidate for concepts repair)`.

## Predicate note (pin-tested)

Grounding edges into layer≥2 MemoryNodes are `ABSTRACTS_TO` (37k live) —
NOT `GENERALIZES` (the obs→theme edge). A GENERALIZES-only childless
predicate over-counts 19,147 vs the true 10,395.

## Bonus fix — `metric_samples` rules used the wrong time column

Pre-flight audit of the new rule's SQL caught `WeightIntegrityRules`
(HIDDEN-WEIGHT-001) querying `metric_samples` with `recorded_at`; the column
is `time`. The null-weight rule had been silently erroring on every
evaluation since it shipped (evaluator logs query failures at Debug only —
the open SUPERVISOR-002 roadmap item demonstrated). Both rules fixed;
`TestMetricSamplesRules_UseTimeColumn` bans the pattern.

## Hierarchy state after PR-B (expected, not a defect)

The L2+ concept layer was built from churn-era themes that no longer exist;
repair archived the childless majority, and the survivors are grounded only
to L1 nodes whose own L0 children were pruned. No L2+ node currently chains
to L0. This is the post-churn reset: future consolidation cycles (emergence
now reachable per PR-A) rebuild concepts from the 5 clean themes, and
`concepts trace` will show the chains as they form.

## New config

| Env | Default | Meaning |
|---|---|---|
| `HIDDEN_THEME_TARGET_RATIO` | 0.1 | themes-per-observation clustering ratio |
| `HIDDEN_THEME_ASSIGN_SIM_THRESHOLD` | 0.70 | density-assignment cosine floor (0 disables) |
| `CONVERSATION_COVERAGE_ALERT_FLOOR` | 0.2 | `low_conversation_coverage` rule floor |
| `CONVERSATION_COVERAGE_MIN_OBS` | 50 | min live obs for a space to emit the gauge |

## Documents Accessed

- `docs/development/hidden-churn-001/sprint_plan_hidden_churn_001.md`
- `docs/development/hidden-churn-001/verification_pr_a.md`
- `internal/hidden/service.go`, `internal/hidden/theme_identity.go`
- `internal/alert/rules.go`, `internal/cli/serve.go`, `internal/cli/graph_repair.go`
- `internal/api/server.go`, `internal/api/handlers_conversation.go`
- `internal/metrics/collectors.go`, `internal/config/config.go`
- Live Neo4j (`mdemg-dev`) + TimescaleDB `metric_samples` + `~/.mdemg/logs/server.log`
