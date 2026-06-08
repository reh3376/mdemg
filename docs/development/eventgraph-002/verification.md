# EVENTGRAPH-002 — Live Verification (Tier 3)

**Date:** 2026-06-08
**Stack:** native `mdemg serve` (launchd `com.mdemg.server`, rebuilt from this branch) + Docker (Neo4j `mdemg_neo4j_data` + TimescaleDB `mdemg_metrics`) + llama-server :8102. Space `mdemg-dev`.

Per the standing directive — *standard code testing is not sufficient to find problems in the live running framework* — every acceptance item was exercised against the real running server with the real binary, and the federation output was **cross-checked against the source-of-truth SQL**.

## Acceptance bar: the CLI federates real guidance outcomes, joined on constraint_code, matching the DB exactly.

### Tier 1 (unit, `-race` clean)
- `internal/eventgraph/guidance_outcomes_test.go` — validation guards, empty-arrays-not-null, sortedKeys determinism, join resolution. **PASS.**
- `internal/cli/eventgraph_test.go` (guidance additions) — request-mapping omit-when-unset + conversion, `--query` seed resolution, surfaced-503, render (empty + followed/ignored table), truncStr. **PASS.**

### Tier 2 (integration + contract)
- `tests/integration/eventgraph_guidance_outcomes_test.go` (`-tags=integration`, live Neo4j+TSDB) — full round-trip: hops=1 (seed+related codes, off-neighborhood code excluded), hops=0 (seed code only), unknown-seed (empty non-nil). **PASS (0.43s).**
- `docs/api/api-spec/uats/specs/guidance_outcome_neighborhood.uats.json` — **6/6 live** against `http://localhost:9999`. sha256 verified, tagged `tsdb`.

### Tier 3 (live e2e — real binary, cross-checked against SQL)

**`--seed` form** (real constraint node for `no-direct-main-commits`, `--limit` enforced):
```
$ mdemg eventgraph guidance-outcome-neighborhood --seed myya3xf8kpk3wpbo0qonah99 --hops 1 --since 720h --limit 8
neighborhood: 1 nodes · 1 constraint codes · hops: 1
outcomes:     8 (followed: 8 · ignored: 0 · other: 0) · scanned: 8 · truncated: true   ← limit honored
CONSTRAINT_CODE             OUTCOME    sim g_type   guidance_id            recorded
no-direct-main-commits      followed  1.00 pattern  w2ewmlfsoiq8iq7r72n99t 06-08 11:15:19
... (8 rows, constraint_node_id resolved to the seed, in_neighborhood=true) ...
```

**Source-of-truth cross-check** (the key Tier-3 assertion):
```
$ mdemg eventgraph ... --json | jq '.outcomes | length'        → 11 (all followed)
$ psql -c "SELECT count(*), count(*) FILTER (WHERE outcome_type='followed')
           FROM constraint_outcomes
           WHERE space_id='mdemg-dev' AND constraint_code='no-direct-main-commits'
           AND time > NOW() - INTERVAL '720 hours';"            → 11 | 11
```
**The federation returns exactly what the DB holds — 11 outcomes, all followed.**

**`--query` form** (resolves an emergent-concept seed, walks 3 hops):
```
$ mdemg eventgraph guidance-outcome-neighborhood --query "never commit directly to main" --hops 3 --since 720h
resolved seed from query → ceb7c073-3703-4923-8f56-170d5bc8ec84
neighborhood: 15 nodes · 5 constraint codes · hops: 3
outcomes:     0 (followed: 0 · ignored: 0 · other: 0)
```
**Live-traced (did not assume):** the 5 codes found (`must-comment-sprint-summary-on-pr`, `must-complete-sprint-checklist`, `must-document-before-implementation`, `must-follow-12-section-format`, `must-not-skip-sprint-sections`) were SQL-checked — they genuinely have **0 outcomes** in the window. So "0 outcomes" is **correct behavior**, not a join failure: the federation correctly distinguishes "constraint code present in the neighborhood" from "constraint code has recorded feedback." (This is exactly the kind of apparent-anomaly the live-testing directive exists to catch — traced to ground truth rather than assumed.)

**Unknown seed** → graceful, non-nil empty slices:
```
neighborhood: 0 nodes · 0 constraint codes · hops: 2
No guidance outcomes for this neighborhood/window. (The graph walk succeeded — …)
```

**No seed/query** → `Error: a seed is required: pass --seed <node_id> or --query <text>`.

### Reinforcement endpoint — no regression from the shared-helper refactor
Epic 3 extracted `eventgraphGate` + `resolveFederationDefaults` and routed the **existing** reinforcement handler through them. Re-verified: reinforcement UATS **6/6 live**, 405/ceiling guards intact, happy-path unchanged.

## Acceptance criteria — met
1. ✅ `mdemg eventgraph guidance-outcome-neighborhood` federates live; `--seed` and `--query` both work.
2. ✅ Outcomes join on `constraint_code`; `constraint_node_id` resolves to the in-neighborhood constraint; `in_neighborhood=true`.
3. ✅ **CLI output matches direct `constraint_outcomes` SQL exactly** (11=11, all followed).
4. ✅ `--limit` truncation, `--json`, unknown-seed, no-arg error all correct live.
5. ✅ No hardcoded hops/since/limit defaults in CLI or handler (omit-when-unset; server config via the shared resolver).
6. ✅ V0023 index present; schema v23; reinforcement endpoint un-regressed by the shared-helper refactor.
7. ✅ UATS 6/6 live; sha256 verified; tagged `tsdb`.

## Conclusion
EVENTGRAPH-002's bar is met and live-verified. The guidance-outcome event stream is now federated (Pattern Y1, second event class) — "how well is this constraint and its graph-related constraints being followed?" — reusing the existing `constraint_outcomes` sink, joined on `constraint_code`, with output proven equal to the database. The federation API now has two consumers (reinforcement + guidance outcomes) sharing one gate/default helper.
