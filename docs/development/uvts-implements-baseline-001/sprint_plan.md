# UVTS-IMPLEMENTS-BASELINE-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint ID**: UVTS-IMPLEMENTS-BASELINE-001
- **Roadmap slot**: Q5 §3 #10 (formalized 2026-09-20 in ROADMAP_2026Q5.md)
- **Parent**: Q4 GO-IMPLEMENTS-001 + Q5 GO-IMPLEMENTS-002 disclosed follow-up
- **Date**: 2026-09-20
- **Effort**: 0.5-1 day (measurement, not capability)
- **Impact class**: measurement (validates a Q4 capability)

## 2. Problem Statement

Q4 GO-IMPLEMENTS-001 (2026-07-31) landed 188 Go IMPLEMENTS edges in `mdemg-dev` Neo4j via the `go/types` analyzer. Q5 GO-IMPLEMENTS-002 (2026-08-05) tightened the analyzer's ingest-filter parity (79 protobuf pairs correctly excluded → gap 83→3). Live state:

```
IMPLEMENTS edges on mdemg-dev: 190
- top interfaces by concrete count:
    LanguageParser (29 impls)
    NodeCreator (12)
    Embedder (7)
    Column (6)
    Authenticator (4)
```

Q5 §3 #10 asks: **do these IMPLEMENTS edges measurably improve retrieval quality for "who implements X?" queries?** Q4 disclosed the measurement as a follow-up but it never shipped.

RETRIEVAL-TYPED-EDGES-002 (2026-07-03) landed the plumbing: `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=true` default → the RRF graph column spreads activation through IMPLEMENTS + 6 other typed edge families via `SpreadingActivationWithAttention`. The A/B at ship time (against `lnl-demo-whk`, 16q) was `+0.001 mean` — flat. But that A/B ran BEFORE the shipped IMPLEMENTS edges existed on mdemg-dev's Go substrate; the delta was measured against edges that were still ~1 per space.

This sprint re-measures with real Go IMPLEMENTS coverage.

## 3. Scope & Constraints

**In-scope**:
- 8-10 real interface→implementer pairs sampled from live Neo4j on mdemg-dev (`LanguageParser`, `NodeCreator`, `Embedder`, `Column`, `Authenticator`, plus 3-5 smaller-fan-in cases)
- Purpose-built A/B script `scripts/uvts_implements_baseline.py`:
  - Read pairs from Neo4j
  - For each: run `POST /v1/memory/retrieve` with query `"types that implement <Interface> in this codebase"` at top_k=10
  - Grade: how many of the known-concrete names appear in top-K results (recall@K)
  - Run OFF (`RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=false`) + ON, one process per condition
  - Report per-pair recall + mean + delta
- Verdict doc `docs/development/uvts-implements-baseline-001/verdict.md`
- Zero code change to retrieval — this is a MEASUREMENT sprint

**Out-of-scope**:
- Full UVTS spec authoring (overkill for 8-10 targeted queries with discrete-set grading)
- LLM-graded semantic answer scoring (recall@K on a known implementer set is more honest than LLM grading here)
- Deciding whether to keep RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED default true/false (already shipped default-on per RETRIEVAL-TYPED-EDGES-002 verdict; this sprint measures the CURRENT lift, not re-litigates the default)
- Extending IMPLEMENTS coverage to other languages (Go-only per GO-IMPLEMENTS-001 design)

**Constraints**:
- `must-follow-12-section-format` — this doc ✓
- `never-hardcode-config` — the flag knob is env-tunable; script honors it
- `unit-integration-e2e-docs` — measurement scripts don't require unit tests; live-smoke = the measurement itself
- `live-testing-tier-required` — real binary against real Neo4j + TSDB
- `end-with-docs-accessed` — §12 populated
- No new alerts, no substrate mutation, no schema change

## 4. Dependencies

- **Upstream (live)**: GO-IMPLEMENTS-001/002 (edges present), RETRIEVAL-TYPED-EDGES-002 (RRF graph column reads IMPLEMENTS)
- **No downstream unblocked**: pure measurement

## 5. Implementation Plan

Single epic; measurement sprint.

### Epic 1 — Script + A/B run + verdict

- **E1.1**: `scripts/uvts_implements_baseline.py` — self-contained Python:
  - CLI: `--space-id mdemg-dev --top-k 10 --pairs-file pairs.json --base-url http://localhost:9999`
  - Reads pairs (or fetches from Neo4j if `--fetch-pairs` set)
  - For each pair, POSTs to `/v1/memory/retrieve` with a fixed query template
  - Grades: exact + fuzzy match of known-concrete names against result `name` + `content` fields
  - Emits JSON report + per-pair table
- **E1.2**: Run the script with flag OFF (via `launchctl setenv RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED false` + kickstart). Save `off.json`
- **E1.3**: Restore flag to ON (default), kickstart. Run again. Save `on.json`
- **E1.4**: Write `verdict.md` with per-pair table + aggregate: mean recall@K OFF vs ON, delta, honest interpretation
- **Gate**: verdict.md ships regardless of outcome (positive lift is nice-to-have; the measurement itself is the deliverable)

## 6. Testing Plan

**Tier 1 — Unit**: N/A (measurement script; no reusable library code)

**Tier 2 — Integration**: N/A

**Tier 3 — Live e2e**: THE measurement itself:
- 8-10 pairs sampled from real mdemg-dev Neo4j
- 2 real retrieval runs against real binary
- Grading is discrete-set membership on real result strings
- All measurements + numbers land in `verdict.md`

## 7. Commit Strategy

Two commits:
1. `feat(scripts): uvts_implements_baseline recall-at-K measurement (E1.1)`
2. `docs: UVTS-IMPLEMENTS-BASELINE-001 verdict + sprint post`

Each carries the Co-Authored-By trailer.

## 8. Verification Checklist

- [ ] Script runs cleanly against live mdemg-dev with flag OFF + ON
- [ ] Per-pair recall table published
- [ ] Mean recall + delta reported honestly (positive or negative)
- [ ] Sprint post + verdict.md land under `docs/development/uvts-implements-baseline-001/`
- [ ] ROADMAP_2026Q5.md §3 #10 checkbox update reflected

## 9. Documentation Update

- `docs/development/uvts-implements-baseline-001/verdict.md` — the measurement result
- `docs/development/uvts-implements-baseline-001/sprint_post.md` — narrative + interpretation
- CLAUDE.md pin ONLY if the finding produces a new arch rule (measurement sprints typically don't)
- CHANGELOG entry — small (measurement, not capability)

## 10. Risks & Mitigations

| Risk | Class | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| Delta is 0 or negative — validates ship-time A/B, IMPLEMENTS still not helping | measurement | MED | LOW | Ship the honest number. If lift is nil, verdict discloses; a follow-up considers amplification or panel-level surfacing. |
| Grading is too lax (result `name` fuzzy-matches an implementer that isn't actually the right node) | grading-quality | LOW-MED | MED | Grade against SymbolNode name matches (exact only, case-insensitive); fuzzy only for known synonyms. Report both strict + lenient modes. |
| Query wording biases the result | measurement | LOW | LOW | Fix one canonical query template `"types that implement <Interface> in this codebase"`; report the template verbatim. |
| Flag flip via launchctl doesn't propagate | operational | LOW | HIGH | Verify via boot log after kickstart; script logs the flag value it sees in `/v1/config` or via the audit call. |

## 11. Rollback Procedures

Zero substrate mutation. Zero code change to retrieval. `git revert` on the script + docs commits is a full rollback.

## 12. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q5.md` — §3 #10
- `docs/development/go-implements-001/` — parent
- `docs/development/go-implements-002/` — Q5 follow-up
- `docs/development/retrieval-typed-edges-002/` — plumbing (RRF graph column IMPLEMENTS support)
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — UVTS spec shape (informational; not using the heavy runner)
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — question record shape reference
- `internal/models/models.go::RetrieveRequest` — `/v1/memory/retrieve` field names
- `internal/retrieval/activation.go` — where IMPLEMENTS edge weight applies (line 329 case, line 206 weight)
- `internal/config/config.go` — `RetrievalGraphTypedEdgesEnabled` knob at line 809 / getBool at 3959
- Live Neo4j via `docker exec mdemg-neo4j-1 cypher-shell` — 190 IMPLEMENTS edges + top interfaces
