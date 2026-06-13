# Sprint Plan — CONTEXT-LIVE-001: Context Fingerprinting Goes Live

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · Roadmap Q3 stretch tier, first
pick (ROADMAP:71) · effort 5d est · risk medium (retrieval-scoring
changes — gated by 120q UVTS A/B per the roadmap's own requirement).

## 2. Problem Statement
Context fingerprinting (Phase 14.2.x, default-on) is benchmark-only in
practice. Recon (2 lanes + live queries, all roadmap claims CONFIRMED):
(1) **the 5th column is dormant on live traffic** — server-side query
fingerprint derivation fires only on `?context=auto`, which no hook,
MCP, or CLI caller passes (handlers.go:496-503); (2) **version skew is
structural** — mdemg-dev: 76,906 nodes on fingerprint v1 / 22 v2 / 425
v3 vs active catalog v3; every weekly stage-6 rebuild bumps the catalog
WITHOUT re-fingerprinting nodes because `RefineWithCoactivations` has
ZERO callers (the Phase-B walk was never wired; CycleOrchestrator lacks
a driver); ContextColumn Jaccard-compares across versions with NO guard
— bit positions reallocate per build, so cross-version similarity is
noise; (3) **per-category protections never fire live** — `req.Category`
comes only from the body field/`?category=` param (UVTS runners);
QueryClassifier output (`code|architecture|relationship|data_flow|
symbol_lookup|generic`) never feeds it, and its vocabulary ≠ the UVTS
override keys; (4) **consensus is systematically deflated** — the
always-empty live ContextColumn counts in `colsAttempted`, hard-capping
live consensus at 0.8 (feeds DH-005 retrieval confidence + the
consensus_shift tripwire); disabled columns count inconsistently
(structural is excluded, the rest count); column_context.go:8-9 comment
contradicts the code.

## 3. Scope & Constraints
**In**: (1) ContextColumn version guard (stale-version node fingerprint
⇒ treated as no-fingerprint); (2) consensus denominator semantics —
columns structurally unable to vote (disabled, or context-with-no-
query-fingerprint) excluded from `colsAttempted`; errored/timed-out
columns still count (documented intent preserved); comment fixed;
`scorerVersion()` gains hashes of the per-category weight + sparse
override maps (pre-existing cache gap, live-relevant once categories
flow); (3) Phase-B wire — driver into CycleOrchestrator;
`maybeRefreshContextCatalog` streams stale-version observations post-
build through `RefineWithCoactivations` under the existing 60s budget
(resumable, partial-work-fine per the existing comment); (4) one-shot
heal: `mdemg migrate context-fingerprint` on mdemg-dev (~77k nodes,
dry-run first per small-batch rule); (5) classifier→category dispatch:
config-driven `QUERY_CLASSIFY_CATEGORY_MAP` (default
`{"data_flow":"data_flow_integration","architecture":
"architecture_structure","relationship":"relationship"}`), explicit
body/param category always wins, deterministic first-mapped type for
multi-label, set BEFORE CacheKey (cache-safe; Category already in key);
(6) default server-side query-fingerprint derivation — config-gated
(`CONTEXT_QUERY_AUTO_DEFAULT`, default true; per-call opt-out
`?context=off`) — sequenced AFTER skew heal; (7) **120q UVTS full A/B
gate** (Note 02: candidate mean ≥ baseline, no per-question regression
> 0.10) before push. **Out**: growing the classifier vocabulary
(service_relationships / business_logic_constraints overrides stay
UVTS-only — documented); Phase-B symbol-bit enrichment beyond what
RefineWithCoactivations already does; consensus_shift tripwire
recalibration beyond disclosure (the rule is sample-gated and the
denominator change will register — expected, disclosed).

## 4. Dependencies
Recon lanes (fingerprint-refresh, dispatch+consensus — in sprint dir);
internal/{conversation/fingerprint.go:146, ape/cycle.go:370-416,
retrieval/{column_context.go,consensus.go:162-240,scoring_rrf.go:57-175,
service.go:416-444,768, gate.go:51,140, query_classifier.go,
scoring.go:454-471, cache.go:91}, api/handlers.go:472-503,
cli/migrate_context_fingerprint.go}; UVTS 120q corpus + A/B harness.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** guards (version guard + consensus semantics +
scorerVersion completeness; pin tests) · **Epic 2** Phase-B wire +
mdemg-dev heal (driver into orchestrator; stale-stream refine; CLI
backfill dry-run → live) · **Epic 3** classifier dispatch (mapping
config + plumbing classification out of ComputeRetrievalHintsWithLLM +
pre-CacheKey assignment; tests incl. multi-label determinism + explicit-
wins) · **Epic 4** default derivation (config + opt-out param) ·
**Epic 5** UVTS 120q A/B gate + live Tier 3 · **Epic 6** docs
(feature doc update, CHANGELOG, post), push.

## 6. Testing Plan
T1: version-guard unit (stale fp scores 0 / current scores normally);
consensus tests (no-op columns excluded; errored counted; live-default
denominator = voting columns); mapping tests (multi-label, explicit
wins, unmapped→empty); scorerVersion flips on map changes. T2: full
`go test ./internal/...`; scanner + route gate green. T3 (live):
backfill heals mdemg-dev to v-current (Neo4j count check); a hook-path
retrieve produces non-empty context column + consensus no longer capped
(debug fields); stage-6 cycle refines stale nodes (logs + version
counts move); UVTS 120q A/B passes the Note 02 gate.

## 7. Commit Strategy
Per-epic · lint each · UVTS gate before push · push once (auto-PR) ·
summary comment · CI watch.

## 8. Verification Checklist
- [ ] Cross-version Jaccard impossible (guard + pin test)
- [ ] Live consensus denominator = columns able to vote; cap-at-0.8 gone
- [ ] scorerVersion includes category-weight + sparse-override maps
- [ ] RefineWithCoactivations wired into stage-6; stale counts shrink across cycles
- [ ] mdemg-dev healed (≥99% nodes on active version)
- [ ] Classifier-derived category fires sparse override + context weights live
- [ ] Explicit ?category= still wins; cache key safe
- [ ] Default derivation on; ?context=off opts out
- [ ] UVTS 120q A/B: mean ≥ baseline, no regression > 0.10
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 6 (never cut).

## 10. Risks & Mitigations
Scoring change regresses retrieval → the 120q A/B IS the merge gate;
fail = don't flip defaults (ship guard-only). Consensus semantics shift
trips consensus_shift → expected one-time step, disclosed in PR; rule is
sample-gated. 77k-node backfill load → batched (batch-size 500),
dry-run first, off-peak. Default-on derivation cost → reuses the query
embedding already computed; ~256 dot products/call; opt-out param +
config kill-switch. Cache: category set pre-CacheKey; namespace flips
via scorerVersion on map/semantics changes (intended).

## 11. Documents Accessed
ROADMAP:71; recon lane reports (committed alongside);
docs/features/context-fingerprinting.md; Phase 14.2.x CLAUDE.md notes;
Note 02 merge gate; RRF-SCALE-001 score-scale contract.

## 12. Rollback Procedures
All behavior config-gated: `CONTEXT_QUERY_AUTO_DEFAULT=false`,
`QUERY_CLASSIFY_CATEGORY_MAP={}` restore pre-sprint behavior without
code revert. Backfill is forward-only but harmless (fingerprints are
derived data; re-runnable). Code: revert commits.
