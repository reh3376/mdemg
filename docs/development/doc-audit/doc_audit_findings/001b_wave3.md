# DOC-AUDIT-001b Findings — Stale-Subtree Disposition

Audited @ dev02 `1ff56a3` (2026-06-12). Subtrees: docs/architecture,
docs/specs, docs/uxts01, docs/lang-parser, docs/sidecar — 251 files.

## Verdict histogram

| Verdict | Count |
|---|---|
| DESIGN_HISTORY (bannered, content preserved) | 205 |
| CURRENT | 18 |
| DRIFT_MINOR | 19 |
| DRIFT_MAJOR | 9 |

Triage: 4 read-only lanes; orchestrator re-verified every LIVING verdict
and spot-checked DESIGN_HISTORY. **16 lane verdicts reversed** (12+2 at
triage, 2 more at Wave 3 — frozen work plans presented as current). The
route-consumer inventory (DORMANT-CENSUS-001) served as the endpoint
oracle throughout.

## DRIFT_MAJOR (9) — the fix-batch core

1. **docs/architecture/06_Retrieval_API_and_Scoring.md** — presents the
   legacy linear `α·vector + β·activation…` formula as THE ranking; RRF
   column-voting (default-on since Phase 13.1) entirely absent (no
   columns, weights, consensus_strength). Response example shows a
   `{data:{…}}` envelope; the real retrieve response is unwrapped
   (models.go:117). BATCH_INGEST_MAX_ITEMS 100 vs 500 default. All 13
   endpoint-table routes verified ACTIVE in the inventory; legacy-path
   parameter values all still exact.
2. **docs/architecture/LEARNING_EDGES.md** — scoring weights stated
   55/30/10/5 vs actual 0.60/0.20/0.15/0.05; linear no longer default
   scorer at all; score-range tables (0.3–0.9, >0.85=HIGH) invalid on
   the RRF scale (~0.49–0.58 top); endpoints `/v1/memory/learning/*` →
   real `/v1/learning/*`; `/v1/learning/freeze` exists ("when
   implemented" stale); both referenced docs moved. Correct: η/μ,
   surprise factors, decay, percentile formula.
3. **docs/architecture/maps/dict_pkg_codes.md** — "27 internal packages"
   vs 50; tsdb/alert/supervisor/eventgraph/jobhealth/etc. (the whole
   post-v0.7 telemetry+supervision plane) absent; nearly every file/LOC
   count off ≥2× (jim 34→72 files, cli 50→115, ret 22→62).
4. **docs/architecture/maps/dist_channels.md** — 5 claimed packaging
   submodules (mdemg_linux, apt-mdemg, mdemg-windows, mdemg-menubar,
   mdemg-linux-sidebar) do not exist (.gitmodules: homebrew-mdemg +
   autoresearch only); apt-publish.yml absent; Linux = scripts/install.sh.
5. **docs/architecture/maps/flow_retrieval.md** — legacy linear scorer
   as THE pipeline; RRF columns, sparse gate, rerank stage absent;
   `SpreadActivation` misattributed to lrn (real:
   retrieval/activation.go); `hid.BoundedExpand` does not exist.
6. **docs/architecture/maps/schema_neo4j.md** — `SymbolNode` (9 files,
   V0023 constraint) missing; `ABSTRACTS_TO` + `GENERALIZES` — the
   abstraction-hierarchy backbone — missing from the edge map. Listed
   labels/edges all real.
7. **docs/architecture/maps/svc_external.md** — TimescaleDB (:5433),
   llama-server (:8102, mandatory runtime), Grafana all missing;
   neural-sidecar ":8100" stale (live NLI sidecar 127.0.0.1:8101).
   gRPC service inventory verified exact.
8. **docs/architecture/maps/uxts_frameworks.md** — 12 vs 16 frameworks
   (ULTS/UTDS/UAITS/UBENCH absent); ~half the rows carry stale
   counts/status; post-UXTS-CI-001 merge-blocking gates unrepresented.
9. **docs/lang-parser/PARSER_SPEC.md** — "3 complete / 15 planned" vs
   28 shipped parsers; `cmd/ingest-codebase/languages/` paths dead
   (real: internal/languages/); symbol output contract still current.

## DRIFT_MINOR (19) — single-claim batch fixes

Architecture: 01 (aci-claude-go narrative, envelope claim, 6s→90s,
RRF omission), 02 (constraint/index name drift, SignalState shape,
CUIDv2), 08 (SCORING_* names, η 0.1→0.02, retry defaults, RRF), 11
("24 migrations"→26), 13 (/v1/maintenance/* phantom routes, query_text
embedding), 14 (/metrics→/v1/prometheus, deleted neo4j_pool gauges,
native evaluator). Maps: dep_pkg_graph (grd→llmclient edge, ape deps),
flow_jiminy_guide (6s), flow_observe_learn_consolidate (function-name
drift), flow_rsic_cycle (13→16 actions). Specs: FRAMEWORK_GOVERNANCE
(124→214 UATS, UVTS/USTS status, UBENCH row). Lang-parser: UPTS_README
(3→28, CI snippet), upts/CHANGELOG (PHP unlogged), upts/README (dead
cmd/ links, evidence-validation set), UPTS_SUMMARY (27→28, Rust claim).
Sidecar: configuration (codex args, .codex claim), friction-log
(linux/amd64 ships, Go 1.26), installation (1536-dim comment, Go 1.26),
troubleshooting (phantom sidecar.log).

## Cross-cutting patterns

- **The RRF blind spot**: 5 of 9 majors are the same root drift — the
  2026-05-03 RRF default-on never propagated to the architecture set.
- **`cmd/ingest-codebase/languages/` ghost**: 4 lang-parser docs point
  at a directory that no longer exists (real: internal/languages/).
- **The maps/ set drifts fastest** (0 of 10 fully CURRENT) — generated-
  looking content with no generator; candidate for regeneration tooling
  or a STALE_RECHECK cadence (001c).

## Fix-batch proposal (operator review)

Batch B1 (high value, ~0.5d): rewrite the scoring sections of 06 +
LEARNING_EDGES + flow_retrieval around RRF (with legacy path noted as
fallback); add missing services to svc_external; add SymbolNode +
hierarchy edges to schema_neo4j; regenerate uxts_frameworks from the
matrix; fix dist_channels to the real channel set; PARSER_SPEC status
table → 28 shipped + path fixes.
Batch B2 (mechanical, ~0.5d): the 19 DRIFT_MINOR single-claim fixes.
Defer: dict_pkg_codes + dep_pkg_graph regeneration (needs tooling
decision — hand-maintained counts will drift again immediately).
