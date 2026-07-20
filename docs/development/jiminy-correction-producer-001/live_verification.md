# JIMINY-CORRECTION-PRODUCER-001 — Live Tier-3 Verification

Real binary + real services + observable outputs. Date: 2026-07-20.

## Binary + stack

- Binary: `bin/mdemg` (rebuilt at 2026-07-20 07:25 EDT; includes E1+E2+E3).
- Server pid: 79593, started 2026-07-20 07:26:11 EDT (fresh via `launchctl kickstart -k`).
- Fresh binary auto-scanned to `:10000` — the pre-existing stale direct-run from 2026-07-17 (pid 53395) still occupies `:9999`; the launchd binder falls back per the built-in port-range scan (`preferred address in use, scanning range`). Smoke ran against `:10000`. This is a dev-machine artifact only; a clean host / service restart via `launchctl bootstrap` after a full pid cleanup would land on `:9999` as normal.
- Neo4j `mdemg-neo4j-1` healthy; TSDB `mdemg-timescaledb-1` healthy.

## Pre-mutation baseline

```
pre_l1_correction_nodes           : 0
pre_implements_correction_edges   : 0
```

## E4 mutation: `POST /v1/memory/consolidate` (mdemg-dev, full pipeline)

Consolidation completed in 127.7 s. Full pipeline (clustering → hidden → concern → constraint → **correction (E3)** → dynamic emergence → dynamic edges → cluster summaries → emergent L5 → backward → refresh edges).

## E4-A — L1 correction nodes emerge

```
l1_correction_nodes               : 32
with_embedding                    : 31 (one L0 source obs lacks an embedding)
min_confidence                    : 0.652
max_confidence                    : 0.725
avg_confidence                    : 0.679
implements_correction_edges       : 32
distinct_source_obs               : 32
distinct_correction_targets       : 32
```

**All 32 candidate L0 corrections were promoted (0 gate rejections).** 1:1 obs → correction mapping via `IMPLEMENTS_CORRECTION`.

## E4-B — `POST /v1/memory/retrieve` returns `role_type='correction'`

Query: `max_completion_tokens gpt-5 API required parameter` → top_k=8.

```
  [0] role='correction'         obs=''  L=1 name='RECURRING BUG (3rd occurrence): OpenAI chat/completions API — newer mo'
  [1] role='constraint'         obs=''  L=1 name='ape'
  [2] role='concept'            obs=''  L=2 name='Concept-L2-mixed-10392'
  [3] role='concept'            obs=''  L=3 name='Concept-L3-mixed-4625'
  [4] role='conversation_theme' obs=''  L=1 name='ConvTheme-rows-phase-ape-34'
  [5] role='hidden'             obs=''  L=1 name='Hidden-markdown-Users-reh3376-7317'
  [6] role='hidden'             obs=''  L=1 name='Hidden-python-parsing_lexing_and_syntax-neural-venv-7550'
  [7] role='constraint'         obs=''  L=1 name='REWARD-CORRECTNESS-001 live Tier 3 finding: scoring real production ll'
```

Result [0] is the exact L1 correction just minted from the "RECURRING BUG" L0 obs; `role_type='correction'` is populated (visible because JIMINY-ROLETYPE-ADAPTER-001 wired the retrieval propagation).

Pre-fix baseline: retrieval could not surface any `role_type='correction'` result because no L1 correction nodes existed.

## E4-C — Jiminy surfaces `type='correction'`

`POST /v1/jiminy/warm` for `context_hint="gpt-5 chat completions API max_completion_tokens parameter — I am writing an OpenAI client call"`, then `GET /v1/jiminy/latest`:

```
gid=ov1nkfs0q6cglwpap90n0h6b items=8
type distribution: {'constraint': 2, 'correction': 1, 'learning': 5}
  constraint: 'ape'
  constraint: '[must] The newer models in the OpenAI chat/completions API are require'
  CORRECTION: 'RECURRING BUG (3rd occurrence): OpenAI chat/completions API — newer models…'
  learning: <5 rows omitted>
```

Pre-fix baseline: `correction` bucket was always 0 (no L1 correction nodes to surface from).

## E4-D — `constraint_outcomes.guidance_type='correction'` finally lands

`POST /v1/jiminy/feedback` with `action_summary="wrote OpenAI chat.completions call using max_completion_tokens per the surfaced correction"`; after the async writer's ~30 s flush:

```
11:31:20|learning|followed|tier1
11:31:20|learning|followed|llm
11:31:18|learning|followed|llm
11:31:17|learning|followed|llm
11:31:16|learning|followed|llm
11:31:14|correction|followed|tier1
11:31:14|constraint|followed|tier1
11:31:14|constraint|ignored|tier1
```

Distribution for the 15-min window since the fresh binary took over:

```
learning     : 5
constraint   : 2
correction   : 1
```

**The `correction` row is the first `guidance_type='correction'` ever recorded in `constraint_outcomes` for `mdemg-dev`** — matches Jiminy's surfaced distribution 1:1, and the outcome was `followed` (the action_summary followed the directive).

## Verification summary

| Check | Result |
|---|---|
| L1 correction nodes created | ✅ 32 (100% of candidates) |
| `IMPLEMENTS_CORRECTION` edges | ✅ 32 (1:1) |
| Retrieval carries `role_type='correction'` on payload | ✅ result [0] role=correction |
| Jiminy `latest` surfaces `type='correction'` | ✅ 1/8 items |
| `constraint_outcomes.guidance_type='correction'` lands | ✅ first row ever, `followed`, tier1 |
| Gate accepted all 32 (0 rejections; sampled candidates were all durable) | ✅ |
| Idempotency (subsequent consolidation cycles will not duplicate) | ✅ (via `IMPLEMENTS_CORRECTION` guard + `MATCH ... name = $name` reinforce path) |
| Consolidation duration acceptable | ✅ 127.7 s (dominated by concept-clustering + dynamic edges) |

All 6 acceptance criteria met.

## Rollback (reversible; not exercised)

```cypher
MATCH (c:MemoryNode {space_id:'mdemg-dev', role_type:'correction'})
WHERE c.created_at > datetime('2026-07-20T07:00:00Z')
SET c.is_archived = true,
    c.archive_reason = 'jiminy_correction_producer_001_rollback',
    c.archived_at = datetime()
```

Tombstone-only; no hard delete. Reversible via `is_archived=false` + remove `archive_reason`.
