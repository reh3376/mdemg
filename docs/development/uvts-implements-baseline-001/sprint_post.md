# UVTS-IMPLEMENTS-BASELINE-001 — Sprint Post

**Ship date**: 2026-09-20
**Wall-clock**: ~30 min (measurement sprint — no code beyond the script)
**Status**: SHIPPED (measurement; honest negative verdict)
**Roadmap slot**: Q5 §3 #10

## Goal delivered

Q5 §3 #10 asked: "Now that Go IMPLEMENTS edges exist (Q4 GO-IMPLEMENTS-001 + Q5 GO-IMPLEMENTS-002 fix), measure the before/after lift on interface-related queries."

Answer: **+0.0000 mean recall@10 delta on 10 real interface→implementer pairs.** The shipped RETRIEVAL-TYPED-EDGES-002 plumbing has zero measurable effect on this class of query at 190 live IMPLEMENTS edges.

## What shipped

| File | Purpose |
|---|---|
| `scripts/uvts_implements_baseline.py` | Self-contained recall@K measurement script (no third-party deps; stdlib urllib + docker exec cypher-shell) |
| `docs/development/uvts-implements-baseline-001/sprint_plan.md` | 12-section plan |
| `docs/development/uvts-implements-baseline-001/verdict.md` | The measurement result + honest interpretation |
| `docs/development/uvts-implements-baseline-001/report_off.json` | Raw OFF condition A/B data |
| `docs/development/uvts-implements-baseline-001/report_on.json` | Raw ON condition A/B data |
| `docs/development/uvts-implements-baseline-001/compare.txt` | Comparison table |

Zero substrate mutation. Zero code change to retrieval. Zero migration. This is a pure measurement sprint.

## Key numbers

- **10 pairs** (interface names) sampled from mdemg-dev Neo4j (top by concrete count)
- **76 expected implementers** across all pairs
- **`top_k=10`** per query
- **~3 implementers surfaced per condition** — 4% absolute recall
- **Mean strict recall@10 OFF vs ON**: `0.0212 ↔ 0.0212` (Δ = +0.0000)
- **1 pair changed** between conditions (Embedder-surprise.go went 0/7 → 1/7 in ON); 9 pairs identical
- **Scorer version confirmed distinct**: OFF `tge=off`, ON `tge=on|an=0.55|br=0.60|cw=0.50|ct=0.40|in=0.45|ds=0.70|th=0.65` — flag propagated

## Interpretation

The Q4 GO-IMPLEMENTS-001 A/B on `lnl-demo-whk` (with ~1 IMPLEMENTS edge) showed `+0.001 mean` — nearly flat. This sprint validates the finding at 190× the edge count: the graph column at weight 0.15 doesn't have the authority to promote SymbolNode-adjacent MemoryNodes above embedding+BM25's semantically-closer candidates, regardless of edge count.

The retrieval bottleneck for interface→implementer queries is architectural: **IMPLEMENTS edges live between SymbolNodes; retrieval surfaces MemoryNodes.** The RRF graph column walks IMPLEMENTS internally but the fused output pool is MemoryNode-typed, and the corpus's MemoryNodes are semantic summaries — not per-concrete-type file-level rows a "who implements X?" query resolves against.

## Deferred (future sprint options; none scoped here)

Per verdict.md §Honest interpretation:
- **A**: Raise graph column weight — REJECTED (RETRIEVAL-TYPED-EDGES-002 already tested this; regressed −0.016)
- **B**: SymbolNode→MemoryNode bridge — new capability sprint
- **C**: Ingest-time per-type MemoryNodes — substrate-growing sprint
- **D**: Remove IMPLEMENTS from graph column — reduces surface with no cost; deferred until audit shows it never helps any class

## Arch rule proposal (not unilaterally pinned)

Verdict §Arch rule proposal names a class worth watching:

> When a shipped capability's ship-time A/B was flat (like RETRIEVAL-TYPED-EDGES-002's `+0.001`), treat "shipped default-on" as a hypothesis under continuous measurement, not a settled outcome. The SHIP-DEFAULT-ON verdict deserves re-litigation after every capability-growth sprint that would EXERCISE the shipped plumbing.

Left as a proposal — the operator should decide whether to formalize.

## Verification (all green)

- ✅ Script runs cleanly against live mdemg-dev
- ✅ Both A/B conditions verified via scorer_version cache key (tge=off vs tge=on with weights visible)
- ✅ Per-pair recall table published (verdict.md)
- ✅ Aggregate mean + delta reported (Δ = +0.0000)
- ✅ Comparison artifact + raw JSON reports pinned in sprint dir
- ✅ Reproducibility recipe published (verdict.md §Reproducibility)
- ✅ launchctl setenv cleanup (`launchctl unsetenv RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED`) — future restarts read `.env` normally

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q5.md` — §3 #10
- `docs/development/go-implements-001/` — Q4 capability (188 edges)
- `docs/development/go-implements-002/` — Q5 fix (gap 83→3, filter parity)
- `docs/development/retrieval-typed-edges-001/` — Phase 1 (default-off ship, `+0.0000` A/B)
- `docs/development/retrieval-typed-edges-002/` — Phase 2 (default-on ship, `+0.001` A/B)
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — UVTS spec shape (informational)
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — question record shape reference
- `internal/models/models.go::RetrieveRequest` — `query_text` field
- `internal/retrieval/activation.go` — IMPLEMENTS edge weight (line 206 = 0.70; case at 329)
- `internal/config/config.go::RetrievalGraphTypedEdgesEnabled` — the A/B knob (line 809)
- Live Neo4j via `docker exec mdemg-neo4j-1 cypher-shell` — 190 IMPLEMENTS edges + pair sampling
- Live server log `~/.mdemg/logs/server.log` — scorer_version confirmation
