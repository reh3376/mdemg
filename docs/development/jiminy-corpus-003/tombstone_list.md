# JIMINY-CORPUS-003 — tombstone list

Audit of 64 live `role_type='constraint'` nodes on `mdemg-dev`. Each node evaluated against: **"Would a competent developer, reminded of this rule before an action, actually follow it (not because it's true, but because it's a durable, applicable, well-scoped rule)?"**

## KEEP (30 nodes — the canonical corpus)

| code | rationale |
|------|-----------|
| `auto-01288edd49b1` | Live-testing requirement (Tier 3) — durable process rule |
| `auto-250af3293675` | CUIDv2 canonical: "never use UUID v4" — durable identifier rule |
| `auto-29156377a1de` | Never `mdemg db start`, use `docker compose up -d neo4j` — real ops trap |
| `auto-a4a36173bff8` | MDEMG ingestion space_id=mdemg-dev — real ingest contract |
| `auto-build-restart-after-feature` | Rebuild + restart after feature — real feedback-loop rule |
| `auto-c0a62b1da979` | Do NOT parallelize epics — real workflow rule |
| `end-with-docs-accessed` | Every phase doc ends with Documents Accessed list |
| `iterate-break-fix-verify` | Pipeline fix ≠ compile-time success — real quality gate |
| `lint-before-commit` | Lint pass before commit |
| `mandatory-feature-docs` | Feature doc for every new feature |
| `markdown-mermaid-tables-and-charts` | Operator markdown preference |
| `memory-preservation-backup-integrity` | CMS backup constraint (JIMINY-CORPUS-001 spared) |
| `must-comment-sprint-summary-on-pr` | Sprint summary on PR |
| `must-enforce-jiminy-constraints` (must-type only) | THE 2026-08-01 architectural directive |
| `must-follow-12-section-format` | Sprint plan 12-section format |
| `must-master-data-pipelines` | Data-pipeline mastery imperative |
| `must-use-cuid2` | CUIDv2 canonical (single keeper; 5 dupes tombstoned) |
| `must-use-uxts-frameworks-consistently` | UxTS framework rule |
| `never-direct-alter-schema` | Schema alteration via migration only |
| `never-haiku-for-planning` | Advanced model for planning |
| `never-hardcode-config` | No hardcoded values in production code |
| `never-skip-discovered-issues` | Never leave discovered issues unresolved |
| `never-trust-unordered-samples` | Trust only ordered samples |
| `no-direct-main-commits` | Never commit to main (canonical; 5 dupes tombstoned) |
| `no-stash-for-release` | No git stash for goreleaser (canonical; 1 dup tombstoned) |
| `openai-max-completion-tokens` | gpt-5+ uses `max_completion_tokens` not `max_tokens` — real API contract |
| `own-test-failures-immediately` | Fix test failures now, not later |
| `plan-mode-before-change` | Enter plan mode before code changes |
| `rebase-dev-after-admin-merge` | Rebase after admin-merge |
| `trace-all-breaks` | Trace entire pipeline, don't stop at first break |
| `trust-signal-must-be-persisted-never-ignore-honest` | 2026-08-01 directive on trust reframe |
| `unit-integration-e2e-docs` | 3-tier testing + docs canonical (2 dupes tombstoned) |
| `must-e2e-live-data-verify` | Sibling to unit-integration-e2e-docs; keeps because scope differs |

Actual count: 33.

## TOMBSTONE_JUNK (13 nodes — not durable rules)

Reason: session-record, event-log, narrative, truncated-content, feature-spec, or implementation detail — not something an agent should be reminded of before every action.

| node_id | code | why junk |
|---------|------|----------|
| `mdjnl96zs16v2ec4wbbzg85c` | `ape-reflect-prompt-trim-first` | content stub ("ape") |
| `sqe0bca027ynqkx10cd9j90q` | `audit-before-prune` | session-record ("DATAPRUNE-AUDIT-001 audit complete") |
| `aegqqbtidh44w3qja6763xyt` | `auto-00f5f84ea9a3` | event log ("deleted 24 junk/test/demo spaces") |
| `pt415lj5yyxfqwy6sk0df743` | `auto-9c8e5bef7da1` | truncated content ("Never commit") |
| `pbcmknqz7rjbgjlvctgb3d72` | `auto-a77a769e417a` | narrative ("planning protocol is not overhead") |
| `gm0kx5x9lkpe4a0phzawbwsu` | `auto-aa4903b877dd` | event log with cost_estimate ("$1,429") |
| `jotv1iseky2aa6hda1mxafsw` | `auto-b9f8c9400464` | decision record (VSCode ext dropped) — not a rule |
| `ftdfhm6hjy1hvcndlyui9bpc` | `dynamic-registry-consolidation` | one-off feature spec |
| `w3git8mqgw547o60179gojy7` | `incremental-ancestry-reaggregation` | narrow impl-detail |
| `ykqvbobzs7bz5zzq8q1tcocr` | `live-validation-against-guardrails` | session finding, not a rule |
| `i9e6xjl542a6qxjmbd2tz6c0` | `pinned-e2e-test-permanent` | session observation |
| `udzpmtsiw04sptyodrv7mot2` | `prune-guardrail-compliance-check` | event log ("1898 rows deleted") |
| `h9c0gruub80rtn9o82udhd97` | `timestamp-format-normalization` | implementation record |
| `rktgw4xwpyn6b84kdjsto6q4` | `uuid-precedent-divergence` | incident report; the RULE is `must-use-cuid2` |
| `u9n0ww7f3b6uy2k06ay56whh` | `always-use-cuidv2-by-default` | wrong header ("Phase 13 live verification test observation") |

Count: 15 (I miscounted above).

## TOMBSTONE_STALE (2 nodes)

| node_id | code | why stale |
|---------|------|-----------|
| `must-pin-mlx-8101` (find by code) | `must-pin-mlx-8101` | Superseded by Phase 13.5 llama-server migration to :8102 |
| `w00hrcdsjfpwzvciby4aqmjs` | `rename-before-finetune` | Superseded by 2026-04-22 MoE→dense pivot |

## TOMBSTONE_DUPLICATE (14 nodes — non-canonical variants)

Keep canonical instance (marked in KEEP list); tombstone the rest.

**Branch-protection cluster** (canonical: `no-direct-main-commits`):
- `pj2vok699kb98zwkgbz9y908` (`auto-07051612cf24`)
- `z5fadt0ptvf8cp554es9dona` (`direct-main-commits-never-allowed`)
- `lq3gxsgu7sbj6xnom4d372tv` (`must-follow-branch-protection-policy` — must)
- `soegva1qvpp3gqnfwjipwezi` (`must-follow-branch-protection-policy` — should)

**CUIDv2 cluster** (canonical: `must-use-cuid2`):
- `gwc3twp6cpe1tdrfs7zcet5w` (`jiminy-uuid-cuid2-enforce`)
- `jnv3zpp4rog0t5g37r1mldzk` (`use-cuidv2-by-default`)
- `ry5tes3xrw90llxdp4nye820` (`must-use-cuidv2-by-default` — wrong content anyway)

**MDEMG db start cluster** (canonical: `auto-29156377a1de`):
- `iq9wylbhu16eecug2tqtvqm4` (`never-start-mdemg-dbs`)

**Goreleaser cluster** (canonical: `no-stash-for-release`):
- `wm4h4u2e5ne8y6g7layn3i4w` (`commit-before-goreleaser`)

**CMS usage cluster** (canonical: `must-master-data-pipelines`):
- `uek7gk8h4dvuhwspth3ndh1x` (`auto-be9db2924276`)

**E2E test cluster** (canonical: `unit-integration-e2e-docs`):
- `l55bfgpweamadwdys68ak0s6` (`auto-08fc9fd25e3b`)
- `yeacdnr9e0ql91drmqckd127` (`mandatory-e2e-docs-before-commit`)

**Test-failure ownership cluster** (canonical: `own-test-failures-immediately`):
- `r7xt3abpn0qb0aebdbo7r6iw` (`auto-1a5fc1e18b88`)

**Jiminy-enforce cluster** (canonical: `must-enforce-jiminy-constraints` must-type = `xkvytpf7e4m3xjy5aj40m9q4`):
- `xgun8uz3hlvnujoq9amb0zf6` (`must-enforce-jiminy-constraints` should-type — spurious enum-level dup)

Count: 14.

## Totals

- KEEP: 33
- TOMBSTONE_JUNK: 15
- TOMBSTONE_STALE: 2
- TOMBSTONE_DUPLICATE: 14
- **Total to tombstone: 31 of 64 (48%)**
- **Post-purge live: 33 constraints**

Reversible via the rollback cypher in sprint_plan.md §10.

## The stronger reframe (post >80% target directive)

Corpus purge is Phase 1 of what needs to be a broader arc to hit >80% follow rate. Written up in `docs/development/jiminy-ceiling-break-2/README.md` as the master arc.
