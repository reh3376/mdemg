# Jiminy Rules UI

**Status:** In development (JIMINY-RULES-UI-001, 2026-08-13) — Epic 1 shipping now; WRITE endpoints code-land during arc window but flag stays OFF through 2026-08-19.
**Sprint plan:** [docs/development/jiminy-rules-ui-001/sprint_plan.md](../development/jiminy-rules-ui-001/sprint_plan.md)

## Why

Jiminy enforces a live corpus of ~33 constraint + 3 correction nodes (post-JIMINY-CORPUS-003 + JIMINY-CORRECTION-CORPUS-001 purges). Operators need to see + curate this corpus without dropping to Cypher or CLI. Fable HITL bulk-grade findings surfaced two active pain points:

1. **Invisibility** — the shipped path to add a rule is `POST /v1/conversation/observe` → auto-classifier tag → consolidation-cycle promotion. Multi-hop, indirect, no user affordance for "I want Jiminy to enforce THIS rule right now."
2. **Corpus bloat** — JIMINY-CORRECTION-CORPUS-001 found 24 of 35 tombstoned corrections were duplicates. Without a dedup gate at create-time, this class recurs on every operator-authored rule.

The Jiminy Rules UI closes both gaps: a dedicated `/ui/rules` tab with list + add + tombstone flows, backed by 4 new endpoints, all bounded by the shipped tombstone-safety + dedup patterns.

## Choices (decided in 2-round design discussion, 2026-08-13)

### Round 1 — foundation

| Decision | Choice | Rationale |
|---|---|---|
| **Authorship** | Operator only | Same trust model as `mdemg jiminy constraint mark` CLI; simplest MVP |
| **Taxonomy** | Reuse existing 3-axis (type × severity × category) + optional scope | Maps 1:1 onto shipped `MemoryNode` fields; no schema change; scope reuses LEVER-C-TIGHTEN-002 9 families |
| **Lifecycle** | Immutable + tombstone-and-recreate | Matches JIMINY-CORPUS-001 pattern; no version-column migration; audit story is one boolean + one string |
| **Arc-safety** | READ-only during arc window, WRITE gated behind `JIMINY_RULES_UI_WRITE_ENABLED` flag (code default OFF) | Preserves JIMINY-CEILING-BREAK-2 T+72h / T+168h re-check integrity; operator flips 2026-08-19 |

### Round 2 — downstream

| Decision | Choice | Rationale |
|---|---|---|
| **UI placement** | New dedicated "Rules" tab | Jiminy tab already carries mode toggle + overrides + timeline; a new tab keeps rules discoverable + focused |
| **Default state on publish** | Active + actionable immediately | Publishing a rule that isn't enforced reads as "the UI doesn't work"; operator authored intentionally |
| **Dedup gate** | Warn ≥ 0.75 similarity + operator override | Mirrors CREATE-CORRECTION-DEDUP-001 threshold; hard-block on 0.75+ is too aggressive; below 0.75 is genuinely dissimilar |

## How it works

### Data model (unchanged from shipped substrate)

Rules are `MemoryNode` records in Neo4j with:
- `role_type ∈ {'constraint', 'correction'}` (the two shipped rule categories)
- `constraint_code` (CUIDv2 or operator-authored mnemonic; unique per space)
- `content` (the rule text)
- `constraint_type ∈ {'must', 'must_not', 'should', 'note'}` (severity)
- `is_informational bool` (category — actionable vs informational, from JIMINY-INFORMATIONAL-CATEGORY-001)
- `is_archived bool` + `archive_reason string` + `archived_at datetime` (tombstone, from JIMINY-CORPUS-001)
- `embedding` (vector; used by dedup gate)
- `scope string` (optional; one of the 9 LEVER-C-TIGHTEN-002 families or empty)
- `space_id` (always the request space; NEVER cross-space)
- Standard: `node_id` (CUIDv2), `created_at`, `updated_at`

The UI never mutates fields other than `is_archived` + `archive_reason` + `archived_at` (tombstone flow) or writes a new node (create flow). No in-place edit — immutable-and-recreate is the lifecycle.

### API

Four new endpoints under `/v1/jiminy/rules/*`:

#### `GET /v1/jiminy/rules?space_id=X&type=&severity=&category=&scope=&include_archived=false&limit=50&cursor=`

Lists rules in the given space. All filters optional; `include_archived=true` surfaces tombstoned rules. Cursor pagination.

Response: `{data: {items: [{node_id, constraint_code, role_type, constraint_type, is_informational, content, scope, is_archived, archive_reason, created_at, ...}], total, next_cursor}}`

#### `GET /v1/jiminy/rules/{code}?space_id=X`

Single-rule detail + recent enforcement outcomes (last 7d from `constraint_outcomes`).

Response: `{data: {rule: {...same fields as list...}, recent_outcomes: [{time, outcome_type, count}]}}`

#### `POST /v1/jiminy/rules?override_dedup=false`

Create a new rule. Runs dedup vector query against live corpus first; if any live node has cosine similarity ≥ `JIMINY_RULES_DEDUP_SIM_THRESHOLD` (default 0.75) → 409 `{data: {similar_rules: [{code, similarity, content}]}}`. Operator retries with `?override_dedup=true` to bypass. Otherwise mints CUIDv2 + writes MemoryNode.

Request body: `{space_id, role_type, constraint_type, is_informational, content, scope?}`

Response (happy): `{data: {node_id, constraint_code, similar_count}}`
Response (dedup): `409 {data: {similar_rules: [...]}, error: "similar rules exist; retry with ?override_dedup=true to bypass"}`
Response (flag off): `503 {error: "rule mutation is currently disabled (JIMINY_RULES_UI_WRITE_ENABLED=false)"}`

#### `POST /v1/jiminy/rules/{code}/tombstone`

Soft-delete: `is_archived=true` + `archive_reason='ui_tombstone_<timestamp>'` + `archived_at=datetime()`. Reversible via direct Cypher (see Rollback section of sprint plan).

Request body: `{space_id, reason?}` (reason optional; defaults to `ui_tombstone_<timestamp>`)

Response (happy): `{data: {node_id, code, previous_state: 'active' | 'already_archived'}}`
Response (unknown): `404`
Response (flag off): `503`

### UI

New tab at `/ui/rules` (path added to `internal/api/ui/nav.js`):

- **List view** (default): filterable table with columns [Code, Type, Severity, Category, Scope, Archived, Created, Surfaces(7d), Follow rate]. Click any row → detail side-panel opens.
- **Detail side-panel**: full content + metadata + recent outcomes chart (last 7d from `constraint_outcomes`) + "Show similar rules" button (runs dedup query for exploration) + "Tombstone" button (opens confirm dialog).
- **Add rule modal**: form with dropdowns (Type × Severity × Category × optional Scope) + Content textarea. On save → POST with `override_dedup=false`. On 409 dedup: inline "Similar rules exist: [chip: code · similarity%]... [Confirm anyway] [Cancel]". On 503: clean "Rule creation is disabled; flip `JIMINY_RULES_UI_WRITE_ENABLED=true` in `.env` + restart".
- **Tombstone confirm dialog**: shows the rule + reason input field + "Reversible via `mdemg cli ...` — see docs" note.

## How to use

### Operator flow (during arc window — READ-only)

1. Open `/ui/rules` — see the live corpus (~33 constraints + 3 corrections after JIMINY-CORPUS-003 + JIMINY-CORRECTION-CORPUS-001 purges).
2. Filter by type/severity/category/scope to drill in.
3. Click a rule → detail side-panel shows recent surfacing + follow rate.
4. Try the "Add rule" flow — the modal renders; on submit, hits 503 with the disabled-state message.

### Operator flow (post-arc, WRITE flag ON)

1. Same as above, but "Add rule" now works.
2. Fill the form. Save.
3. If a similar rule exists ≥ 0.75 sim: inline warning + "Confirm anyway" or "Cancel". Otherwise the rule lands immediately + becomes part of Jiminy's active surfacing pool.
4. To retire a rule: click "Tombstone" on the detail panel → confirm → rule flips `is_archived=true`, disappears from active surfacing (existing JIMINY-ARCHIVED-CODE-FILTER-001 filter honors this).
5. To un-tombstone: direct Cypher (deliberate: keeps the reverse path operator-gated + prevents accidental resurrection).

### Enabling WRITE endpoints on 2026-08-19

```bash
# In .env
JIMINY_RULES_UI_WRITE_ENABLED=true

# Restart to pick up
launchctl kickstart -k gui/$(id -u)/com.mdemg.server
```

Or set to `false` (or omit) to keep WRITE endpoints returning 503.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `JIMINY_RULES_UI_WRITE_ENABLED` | `false` | Master flag for POST endpoints; ships OFF during arc, flip on 2026-08-19 |
| `JIMINY_RULES_DEDUP_SIM_THRESHOLD` | `0.75` | Cosine similarity threshold for dedup warn on create; below = allow, at/above = 409 warn |
| `JIMINY_RULES_LIST_MAX_LIMIT` | `200` | Max page size for list endpoint |
| `JIMINY_RULES_OUTCOMES_LOOKBACK_HOURS` | `168` (7d) | Recent-outcomes window in the detail view |

## Follow-ups (disclosed, not shipped)

- **Rule import/export** — JSON round-trip for cross-workspace portability
- **Rule "supersede" arrow** — visual history link between tombstoned rule and its replacement (zero schema change; uses existing `supersedes` node property)
- **Auto-detection sprint** — RSIC-driven rule proposals from enforcement gaps (extends design discussion round 1 alternative)
- **HITL corrections-review tab** — dedicated review flow for `contradicted_correction_drafts` (separate future sprint)
- **CLI companion** — `mdemg jiminy rules list|create|tombstone` mirroring the UI for scriptable use

## Related shipped work

- **JIMINY-CORRECTION-PRODUCER-001** — the L1 correction promoter this UI's create endpoint mirrors
- **CREATE-CORRECTION-DEDUP-001** — the promotion-time dedup pattern this UI's create endpoint mirrors
- **JIMINY-INFORMATIONAL-CATEGORY-001** — the `is_informational` field this UI's category dropdown writes; also the CLI shape (`mdemg jiminy constraint mark`) this UI's tombstone flow mirrors
- **JIMINY-CORPUS-001/002/003** — the tombstone-safety pattern (`is_archived` + `archive_reason`) this UI's tombstone endpoint honors
- **JIMINY-CORRECTION-CORPUS-001** — the corpus-bloat evidence (24/35 dups) that motivates the dedup gate
- **JIMINY-ARCHIVED-CODE-FILTER-001** — the reader-side filter that ensures tombstoned rules don't leak into Jiminy's surfacing
- **JIMINY-MODE-001** — the sibling UI tab pattern (mode toggle in `/ui/`)
- **ENFORCE-UI-OVERRIDES** — the sibling table + timeline UI pattern
- **REVIEW-GRADE-NOTES-FIELD-001** — the readJSON improvement that gives clean error messages to the new endpoints for free
- **LEVER-C-TIGHTEN-002** — the 9 scope families the "scope" dropdown populates from
- **DORMANT-CENSUS-001** — the route inventory forcing function this sprint honors in Epic 6
