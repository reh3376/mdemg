# JIMINY-CONTRADICTED-BRIDGE-001 — Live Tier-3 Verification

Real binary + real services + observable outputs. Date: 2026-07-20.

## Binary + stack

- Binary: `bin/mdemg` rebuilt 2026-07-20 08:18 EDT (includes E1+E2+E3+E4).
- Server pid: 84822, started 2026-07-20 08:18:52 EDT via `launchctl kickstart -k`.
- Fresh binary landed on `:10000` (dev-machine artifact: stale JIMINY-ROLETYPE-ADAPTER-001 pid 53395 still holds `:9999`; documented cross-sprint).
- Auto-migrate applied V0030 on startup: `tsdb_schema_meta.schema_version=30`, `contradicted_correction_drafts` hypertable present with the expected columns.
- Flag: `JIMINY_CONTRADICTED_BRIDGE_ENABLED=true` in `.env`.

## E5-A — Bridge fires on live `contradicted` verdict

Trigger: `POST /v1/jiminy/warm` with a context targeting the `never-commit-to-main` constraint, then `POST /v1/jiminy/feedback` with an explicit contradiction:

```
context_hint: "never commit directly to main branch — I am about to push a commit"
action_summary: "I committed directly to main branch, bypassing the dev-branch
                 workflow (git checkout main; git commit -m fix; git push
                 origin main). This directly opposes the never-commit-to-main
                 rule."
```

Jiminy `latest` surfaced 10 items; the LLM classifier verdicts on `POST /v1/jiminy/feedback` included:

```json
{"type":"correction","content":"After merging a PR to main via --admin…",
 "outcome":"contradicted","similarity":0.85,
 "reasoning":"The agent action directly violates the guidance by committing to
              the main branch instead of following the required workflow of
              using a dev branch."}
```

Draft row landed within the writer's flush window (~30s):

```
12:20:13 | c8jvgnmkl8zlmr4m58nl7rj3 | correction | pending
guidance: "After merging a PR to main via --admin, if the dev branch ha…"
action:   "I committed directly to main branch, bypassing the dev-branc…"
```

## E5-B — HITL surface exposes the draft

`GET /v1/review/datasets?space_id=mdemg-dev` returned the new dataset with `candidate_count: 1`:

```json
{"id":"contradicted_drafts",
 "display_name":"Contradicted-outcome correction drafts",
 "description":"Draft corrections auto-generated from Jiminy contradicted-outcome verdicts...",
 "rubric_version":"gr-v1", "rubric_kind":"rated",
 "candidate_count":1}
```

Guidance + LLM datasets remain registered alongside — the new one composes cleanly.

## E5-C — Approve grade mints an L0 correction obs + flips draft to `approved`

`POST /v1/review/grade` with `durable_rule=4, phrasing_quality=3`:

```json
{"data":{"gold_score":0.875, "grade_id":"tvlenr83vdmiyp1vjnujbpgj",
         "reinforcement_applied":true}}
```

Draft row 30s later:

```
id: c8jvgnmkl8zlmr4m58nl7rj3
status: approved
applied_at: 2026-07-20 12:21:31.750011+00
applied_obs_id: oqqi977em6x29w8tx6n521j0
```

Neo4j `mdemg-dev` L0 obs freshly minted by `conversation.Service.Correct`:

```
id:      po2zahas8mh10ahwe0iimmoz
content: "CORRECTION: Incorrect: I committed directly to main branch,
          bypassing the dev-branch workflow (git c…"
```

⚠️ Note: the sink recorded `resp.ObsID` in `applied_obs_id`; `ObserveResponse` carries both `ObsID` and `NodeID` and they can differ. For forensic queries, the operator can grep either — but the Neo4j-side lookup uses `node_id`. E6 documents this and captures the follow-up to also persist `NodeID` in a dedicated column (additive; deferrable).

## E5-D — Consolidation promotes the L0 obs to a fresh L1 correction node

Pre-consolidation L1 correction count on `mdemg-dev`: **32**.

`POST /v1/memory/consolidate` on `mdemg-dev` completed the full pipeline. Post-consolidation count: **33** (+1 = the fresh promotion).

The new L1 correction:

```
id:      ymehdkihmj2yiu7t3bywsgxc
content: "CORRECTION: Incorrect: I committed directly to main branch,
          bypassing the dev-branch workflow (git c…"
```

Content matches the drafted correction exactly. `CreateCorrectionNodes`
(JIMINY-CORRECTION-PRODUCER-001, shipped v0.11.2) picked up the freshly-
created L0 obs and promoted it — no bridge-specific code in the promotion
path. Clean pipeline composition.

## Verification summary

| Check | Result |
|---|---|
| V0030 migration applied on startup | ✅ schema_version=30, table + indexes present |
| Bridge fires on live `contradicted` verdict | ✅ draft `c8jvgnmkl8zlmr4m58nl7rj3` persisted |
| HITL `/v1/review/datasets` exposes the new dataset | ✅ candidate_count: 1 |
| `POST /v1/review/grade` (durable_rule=4) → approve → L0 obs created | ✅ obs `po2zahas8mh10ahwe0iimmoz` |
| Draft flipped to `approved` with `applied_obs_id` captured | ✅ 12:21:31 UTC |
| Consolidation promotes to L1 role_type='correction' | ✅ 32 → 33 |
| Test suite green + lint clean | ✅ (pre-restart) |

All 6 acceptance criteria met.

## E6 default-flip decision

Recommend flipping `JIMINY_CONTRADICTED_BRIDGE_ENABLED` default `false → true` in E6. Rationale:
- Live smoke clean; end-to-end flow works.
- HITL gate remains the operator-approval requirement — the flag flip only enables *draft emission*, not *substrate mutation*.
- Low fire rate (~3/mo baseline) — HITL flooding risk is negligible.
- Bridge is idempotent (LRU + DB dedup); a bad emission is a no-cost dismissal.

If you prefer keeping default-off (operator explicit opt-in per install), the writer + HITL dataset still ship enabled so a manual flag flip in `.env` is a one-line change with immediate effect.

## Rollback (not exercised)

- `JIMINY_CONTRADICTED_BRIDGE_ENABLED=false` stops new draft emission immediately.
- `REVIEW_CONTRADICTED_DATASET_ENABLED=false` hides the dataset from the reviewer surface.
- Approved L0 obs (`po2zahas8mh10ahwe0iimmoz`) and promoted L1 node (`ymehdkihmj2yiu7t3bywsgxc`) are legitimate additions to the substrate — not rollback targets. They can be tombstoned via `mdemg concepts tombstone` if the E5 smoke evidence is deemed test-only.
