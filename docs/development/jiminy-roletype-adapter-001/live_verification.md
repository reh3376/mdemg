# JIMINY-ROLETYPE-ADAPTER-001 — Live Tier-3 Verification

Real binary + real services + observable outputs. Date: 2026-07-17.

## Binary + stack

- Binary: `bin/mdemg` (rebuilt at 2026-07-17 17:07 EDT, includes E1–E3 + reasoning fix-commit).
- Server pid: 53395, started 2026-07-17 17:13:38 EDT (post-restart).
- Neo4j: `mdemg-neo4j-1` healthy.
- TSDB: `mdemg-timescaledb-1` healthy.
- Note: launchd path rejected the ad-hoc-signed binary with `OS_REASON_CODESIGNING`
  (macOS 26 launchd hardening). Direct `nohup ./bin/mdemg serve` bypasses the
  check; this is only a dev-loop artifact (production install uses a signed
  Homebrew binary).

## E4-A — role_type propagates through `/v1/memory/retrieve`

Query: `MUST use UxTS framework for JSON schema contract test spec`  → top_k=10.

Response now carries `role_type` and `obs_type` per result:

```
  [0] role='constraint'   obs=''             L=1 name='When creating a JSON schema, contract, or test spec that will be used '
  [1] role='conversation_observation' obs='constraint'   L=0 name='<nil>'
  [2] role='leaf'         obs=''             L=0 name='uxts_frameworks.md'
  [3] role='emergent_concept' obs=''             L=3 name='EmergentConcept-L3-uxts-framework-2860'
  [4] role='emergent_concept' obs=''             L=3 name='EmergentConcept-L3-json-uxts-3135'
  [5] role='emergent_concept' obs=''             L=3 name='EmergentConcept-L3-framework-error-2792'
  [6] role='emergent_concept' obs=''             L=3 name='EmergentConcept-L3-json-uxts-3477'
  [7] role='emergent_concept' obs=''             L=3 name='EmergentConcept-L3-error-json-2804'
  [8] role='conversation_theme' obs=''             L=1 name='ConvTheme-uats-json-specs-26'
  [9] role='emergent_concept' obs=''             L=3 name='EmergentConcept-L3-json-framework-4211'
```

Pre-fix (E0 baseline): the same query returned the same result set but with
**`role_type=''` and `obs_type=''` on every row** — the payload didn't include
the two keys because the fields weren't in `models.RetrieveResult` at all.

## E4-B — Jiminy surfaces `type='constraint'` in guidance

Fresh `/v1/jiminy/warm` for the same context; `GET /v1/jiminy/latest` returns:

```
gid=qd3f7rsny13pelruuck4lw5u items=5
  'constraint': 'MANDATORY PROCESS: After a new feature is created in the development plan, a doc...'
  'constraint': '[must] When creating a JSON schema, contract, or test spec that will be used rep...'
  'constraint': '[must] All development plans MUST include three testing tiers: unit tests, integ...'
  'constraint': 'MANDATORY WORKFLOW: ALWAYS run e2e testing once a new section of development is ...'
type distribution: {'constraint': 4, 'learning': 1}
```

**Pre-fix behavior**: all 5 items would have been `type='learning'` (the
default fallback in `classifyRetrievalItem` when `ObsType` is empty). The
4/5 shift to `constraint` is the direct end-to-end fix.

## E4-C — `constraint_outcomes.guidance_type='constraint'` finally lands

`POST /v1/jiminy/feedback` with an action_summary that follows the surfaced
directive; then `SELECT` from `constraint_outcomes` since binary restart:

```
21:17:40|constraint|followed|tier1|auto-01288edd49b1
21:17:40|constraint|followed|tier1|must-use-uxts-frameworks-consistently
21:17:39|constraint|ignored|llm|mandatory-feature-docs
```

Distribution since restart: `constraint: 3`.

**Pre-fix baseline in the same table** for the earlier 10-day post-JIMINY-CORPUS-001
window: only `learning` and (heuristically-assigned) `constraint` rows — never
via the retrieval-classifier path. The `constraint_code` column now populates
too (via `matchConstraintCodeByEmbedding` and the LLM tier-2 path), letting the
Neo4j `GUIDANCE_OUTCOME` sink attribute back to real constraint nodes.

## E4-D — Surprise: E1's reasoning-module rerank drop

Live smoke initially returned empty `role_type` on the response payload. Root
cause: `internal/retrieval/reasoning.go` rebuilds `models.RetrieveResult` from
the reasoning-module proto response and copies only 7 fields, dropping the two
new ones. The existing `originalByID` restore-hook was already used to preserve
`Layer` through the proto boundary; extending it to `RoleType` + `ObsType`
closes the loop. Shipped as a **separate fix-commit** per the
CLAUDE.md "surprise bugs during live smoke get their own commit" rule.

## Verification summary

| Check | Result |
|---|---|
| Response payload carries `role_type` on constraint hits | ✅ `[0] role='constraint'` |
| Jiminy surface types items as `constraint` (not `learning`) | ✅ 4/5 surfaced items typed `constraint` |
| `constraint_outcomes.guidance_type='constraint'` rows appear | ✅ 3 rows |
| `constraint_code` populates on rows | ✅ 3/3 rows have codes |
| Both classifier sources exercised | ✅ `tier1` + `llm` |
| `go test ./...` green | ✅ full-suite pass |
| `golangci-lint run` on touched packages | ✅ 0 issues |

All 5 acceptance criteria met.
