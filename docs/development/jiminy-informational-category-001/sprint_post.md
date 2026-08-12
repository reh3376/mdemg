# JIMINY-INFORMATIONAL-CATEGORY-001 — Sprint Plan + Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Arc-adjacent to:** `docs/development/jiminy-ceiling-break-2/README.md` (positions between Phase 1 and Phase 2 as a complementary corpus-curation lever)

## Problem

Operator flagged (mid-turn) that some canonical constraints in the post-JIMINY-CORPUS-003 corpus are **meta / epistemic directives** — not per-action rules. Grading them against every action produces false-ignored noise that drags actionable follow rate.

Examples in the current 33-node corpus:
- `trust-signal-must-be-persisted-never-ignore-honest` — reframing directive about the follow-rate itself
- `must-master-data-pipelines` — epistemic ("we need to know this")
- `must-enforce-jiminy-constraints` — meta about how Jiminy operates
- `markdown-mermaid-tables-and-charts` — operator preference (already discounted, but still counted)

Roughly 5-8 of the 33 canonical constraints are informational-shaped. If each accounts for ~4% of current outcomes, marking them mechanically lifts actionable follow rate by **~10-15pp** — an honest lift, because those items were never truly follow/ignore in the actionable sense.

## Shipped

**Data-model change:**
- Neo4j `MemoryNode` (role_type=constraint OR correction) gains an optional `is_informational` boolean property (default false / absent = false via `coalesce(n.is_informational, false)`)
- Also `informational_marked_at datetime()` audit-timestamp property, set on flip
- Fully reversible via `--unmark` or Cypher `SET n.is_informational = false`

**Server-side wiring** (`internal/jiminy/service.go`):
- `loadInformationalNodeSet(ctx, nodeIDs) map[string]bool` — batched Cypher query, one round-trip per `RecordOutcome` invocation, returns which source_node_ids are marked informational
- In `RecordOutcome`, immediately after the classifier verdict resolves, if any of the item's `SourceNodes` is in the informational set (and the outcome isn't already `NotApplicable`), the outcome is **overridden to `OutcomeNotApplicable`** — flowing through all existing NA gates (`service.go:1902,1927,1954,1986,2090`) unchanged. Reasoning field also updated to "source node marked informational (JIMINY-INFORMATIONAL-CATEGORY-001)" so the audit trail names why the verdict was overridden.
- Safe-default: nil driver / query error / missing property → treated as NOT informational (better to grade a real rule than silently skip)

**CLI** (`internal/cli/jiminy_constraint.go`):
- `mdemg jiminy constraint mark --code X --space-id Y [--informational=true|false] [--unmark] [--dry-run]` — flip the property; dry-run previews matched nodes without writing
- `mdemg jiminy constraint list-informational --space-id Y` — enumerate all marked constraints
- Talks to Neo4j directly (local operator-authorized property flip; not via HTTP API)

**Pin tests** (`internal/jiminy/informational_test.go`):
- `TestLoadInformationalNodeSet_NilDriverReturnsEmpty` — nil-driver returns empty (not nil) map; safe map-indexing default
- `TestLoadInformationalNodeSet_EmptyInputReturnsEmpty` — short-circuits for empty/nil/all-blank input
- `TestInformationalOverride_LogicMirror` — 9 subtests pinning the exact predicate + override logic from `RecordOutcome` (Followed→NA, Ignored→NA, PartialCompliance→NA, Contradicted→NA, NA→NA no double-override, non-informational doesn't override, empty set doesn't override, empty source doesn't override, multi-source ANY informational triggers)

## Live Tier-3 (mdemg-dev, 2026-08-12)

- `go build ./...` clean; `golangci-lint run ./internal/jiminy/... ./internal/cli/...` = 0 issues; `go test ./internal/jiminy/... ./internal/cli/...` = green (including 11 new pin subtests)
- Marked `trust-signal-must-be-persisted-never-ignore-honest` informational via the CLI dry-run then real; `mdemg jiminy constraint list-informational` returns 1 entry
- Direct Neo4j verify: `MATCH (n {node_id:'xbegkamy8lcaee0givcowkpg'}) RETURN n.is_informational, n.informational_marked_at` → `TRUE, 2026-08-12T15:00:39.702Z`
- Server restarted via `launchctl kickstart`; new `loadInformationalNodeSet` code path is live

**Deep behavioral confirmation** requires the next natural feedback event that references this source node — the override will fire in `RecordOutcome` and the `constraint_outcomes` write will be skipped. Observable via TSDB post-fact.

## Two arch rules pinned (CLAUDE.md)

1. **Informational-marked source nodes MUST be honored via the existing NotApplicable gates, not via new plumbing.** The clean design injects the override at the earliest point after classifier verdict resolution and lets the shipped NA-check gates (5 sites already checking `outcome != OutcomeNotApplicable`) handle downstream flow. This preserves single-source-of-truth for NA semantics and means any future NA-check gate automatically honors informational marking without additional code. NEVER add parallel `is_informational` checks in downstream gates — they'll drift.

2. **Operator-authorized substrate-property flips (like `is_informational`, `is_archived`) MUST require both `--space-id` AND a specific target selector (`--code`) — never operate on wildcards or unbounded scopes.** The CLI reads back the matched-node list before writing so the operator confirms intent; dry-run is one flag away. Follows the JIMINY-CORPUS-001/002/003 tombstone-safety pattern. Any future property-flip CLI MUST use the same shape.

## Suggested starter set (operator judgment)

Candidate constraints to mark informational (~7 nodes; each removes noise from the actionable denominator):

| Code | Rationale |
|---|---|
| `trust-signal-must-be-persisted-never-ignore-honest` | Reframing directive about the follow-rate itself — meta about how to think, not what to do (already marked) |
| `must-master-data-pipelines` | Epistemic "we need to understand this" — not action-level |
| `must-enforce-jiminy-constraints` | Meta directive about how Jiminy operates — not a rule agents follow per-action |
| `markdown-mermaid-tables-and-charts` | Preference for markdown authoring only — over-surfaces on non-doc actions |
| `must-follow-12-section-format` | Only applies to sprint-plan authoring — over-surfaces on non-planning actions |
| `must-comment-sprint-summary-on-pr` | Only applies at PR-create time — over-surfaces on all git operations |
| `mandatory-feature-docs` | Only applies at feature-completion time — over-surfaces on any doc edit |

Operator decides which to mark; each is one CLI invocation.

## Follow-ups disclosed

- **UI badge on the Review tab** — surface which items come from informational-marked sources so operators grading via HITL know to expect NA outcomes. Optional.
- **Grafana panel** — track `is_informational=true` outcome counts separately from actionable outcomes so the framing hygiene sweep (from JIMINY-CEILING-BREAK-2's cross-cutting measurement discipline) has a clean signal.
- **Auto-suggestion** — heuristic to identify additional candidates as new constraints get promoted; low-priority.

## Documents Accessed

- `docs/development/jiminy-ceiling-break-2/README.md` — arc position
- `docs/development/jiminy-corpus-003/tombstone_list.md` — post-purge KEEP list to audit for informational-shape
- `internal/jiminy/service.go` (loadSpaceConstraintCodes precedent + RecordOutcome NA-gate structure)
- `internal/jiminy/types.go` (GuidanceItem shape)
- `internal/cli/jiminy.go` (constraint-cmd wiring precedent)
- `internal/config/config.go` (Neo4j* field names)
- Live Cypher against `mdemg-dev` — pre/post mark verification
- CLAUDE.md pins: JIMINY-CORPUS-003, JIMINY-ARCHIVED-CODE-FILTER-001, JIMINY-CEILING-BREAK-2
