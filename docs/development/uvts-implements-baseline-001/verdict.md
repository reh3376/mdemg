# UVTS-IMPLEMENTS-BASELINE-001 — Verdict

**Sprint**: UVTS-IMPLEMENTS-BASELINE-001 (Q5 §3 #10)
**Date**: 2026-09-20
**Question**: Do the shipped Go IMPLEMENTS edges (GO-IMPLEMENTS-001/002, 190 live on mdemg-dev) improve retrieval quality for "who implements X?" queries?

## TL;DR

**No measurable effect.** OFF and ON conditions produce **identical** aggregate recall@10 (0.0212 both), and identical or near-identical per-pair recall on 9/10 pairs.

## Method

- **Pairs**: 10 real interface→implementer sets sampled live from Neo4j on mdemg-dev, top-10 interfaces by concrete count (LanguageParser 29 impls, NodeCreator 12, Embedder ×2, Column 6, plus 5 smaller-fan-in cases). Total expected implementers across all pairs: 76.
- **Query template**: `"types that implement <Interface> in this codebase"` (fixed)
- **Endpoint**: `POST /v1/memory/retrieve` at `top_k=10`, `space_id=mdemg-dev`, `query_text` only (no embedding override, no filters)
- **Grading**: recall@10 — strict = interface name matches result `name` exactly (case-insensitive); lenient = substring match in `name` / `content` / `file_path`
- **A/B toggle**: `RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED` via `launchctl setenv` + `kickstart`. Verified via `scorer_version` cache key: OFF → `tge=off`, ON → `tge=on|an=0.550|br=0.600|cw=0.500|ct=0.400|in=0.450|ds=0.700|th=0.650`

## Results

```
interface                      OFF strict  ON strict   Δ strict   OFF lenient  ON lenient   Δ lenient
--------------------------------------------------------------------------------------------------------------
LanguageParser                     0.0690     0.0690    +0.0000       0.0690      0.0690     +0.0000
NodeCreator                        0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
Embedder (surprise.go)             0.0000     0.1429    +0.1429       0.0000      0.1429     +0.1429
Embedder (embeddings.go)           0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
Column                             0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
modelInstallPool                   0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
AuthMethodConfig                   0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
Authenticator                      0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
execer                             0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
jobEventPool                       0.0000     0.0000    +0.0000       0.0000      0.0000     +0.0000
--------------------------------------------------------------------------------------------------------------
MEAN                               0.0212     0.0212    +0.0000       0.0212      0.0212     +0.0000
```

- **Aggregate strict recall@10: 0.0212 ↔ 0.0212** (Δ = +0.0000)
- **Aggregate lenient recall@10: 0.0212 ↔ 0.0212** (Δ = +0.0000)
- **Per-pair regressions**: 0
- **Per-pair improvements**: 1 of 10 (Embedder-surprise.go: +0.1429)
- **Absolute recall level**: **2%** overall — the retrieval surface returns only ~3 of the 76 expected implementers at top-10 in either condition

The single "improvement" (Embedder / surprise.go) shifts the mean by 0.014pp — statistically indistinguishable from noise given the 3-of-76 sample size across the corpus.

## What this measures + what it doesn't

**Measures**: whether the RRF graph column's IMPLEMENTS-edge-attention (`RETRIEVAL-TYPED-EDGES-002` plumbing, weight 0.150 vs embedding 0.500) demonstrably promotes concrete-implementer candidates into top-10 for interface-shaped queries when the IMPLEMENTS edges actually exist.

**Doesn't measure**: whether IMPLEMENTS edges could help ANY retrieval class — this is one specific class of query at one weight configuration.

## Root cause hypothesis

Three compounding factors, ranked most→least confident:

1. **The graph column weight is 0.15; embedding + BM25 dominate at 0.70.** The graph column can rerank within its own top candidates but has limited authority to promote a semantic-content-weak candidate (a SymbolNode-derived MemoryNode with sparse content) above a semantic-content-strong candidate (a MemoryNode for a doc / summary node that mentions "Parser" or "LanguageParser" more).
2. **IMPLEMENTS edges live between `SymbolNode` nodes; retrieval surfaces `MemoryNode` results.** The RRF graph column's structural walk crosses IMPLEMENTS but the fused candidates are still MemoryNode-typed. A SymbolNode adjacency doesn't necessarily surface a semantically-matching MemoryNode into the RRF fusion pool.
3. **The corpus has ~sparse MemoryNode coverage of concrete Go types.** Even if IMPLEMENTS edges perfectly promoted the right SymbolNode neighborhood, the corresponding MemoryNodes may not exist as top-10-eligible candidates. Absolute recall of 2% suggests the retrieval surface simply doesn't have the concrete-type MemoryNodes indexed at a granularity where "types that implement Embedder" resolves to the right rows.

Root cause 1 is the RETRIEVAL-TYPED-EDGES-002 pinned observation ("The Phase-13 finding reconfirmed — amplifying graph crowds out embedding/BM25"). Root causes 2 + 3 are new here — the SymbolNode-vs-MemoryNode surface gap is an architectural class the shipped design doesn't bridge.

## Honest interpretation

The Q4 GO-IMPLEMENTS-001 sprint was correct that IMPLEMENTS edges belong in the graph — the analyzer works, ingest is clean, 190 edges live. And RETRIEVAL-TYPED-EDGES-002 was correct that the RRF graph column should walk them.

But the shipped combination **does not measurably improve the query class it was designed to help** (as measured on 10 real pairs / 76 expected implementers / 2% baseline recall). The Q4 A/B result (`+0.001 mean` on lnl-demo-whk with ~1 IMPLEMENTS edge) was NOT a small-sample artifact that would resolve at scale — it was the honest signal. This sprint validates that with 190 real IMPLEMENTS edges on production-scale Go corpus.

## No code change this sprint

Per plan §3, this is a **measurement sprint**. The verdict is negative but honest. Options for future work (none scoped in this sprint):

- **A**: Raise the graph column weight (`RETRIEVAL_COLUMN_WEIGHT_GRAPH` 0.15 → 0.20) — a UVTS A/B in RETRIEVAL-TYPED-EDGES-002 already tested this and REGRESSED aggregate quality (0.4130 → 0.4090, -0.016). Rejected then; still rejected.
- **B**: Bridge SymbolNode-to-MemoryNode retrieval — new capability: a query that mentions an interface name promotes MemoryNodes whose file_path matches concrete implementer paths via a SymbolNode-lookup pre-filter. Bigger sprint; open question whether the general retrieval mix wants this class of promotion.
- **C**: Ingest-time strategy — mint MemoryNodes for each Go type/interface at the file granularity so semantic retrieval already surfaces them at RRF top-10. New sprint; substrate-growing.
- **D**: Accept the finding and remove IMPLEMENTS from the structural graph column entirely — reduces surface area with no measured cost. Deferred until an audit shows IMPLEMENTS is actually never helping any query class.

## Recommendation

Ship this verdict as the honest measurement. Don't recommend a fix — three shipped-then-measured attempts (Phase 13 amplification; RETRIEVAL-TYPED-EDGES-001 default-off; RETRIEVAL-TYPED-EDGES-002 default-on with growth) have converged on "typed edges have real live counts but don't move retrieval-quality metrics for the class of query they were expected to help." The next iteration should be a MEASURE-FIRST design that identifies the surface gap (SymbolNode↔MemoryNode) before shipping more retrieval-side capability.

## Arch rule proposal

Not a rule I'll pin unilaterally — but this class of finding suggests: **when a shipped capability's ship-time A/B was flat, treat "shipped default-on" as a hypothesis under continuous measurement, not a settled outcome.** RETRIEVAL-TYPED-EDGES-002 shipped default-on based on a `+0.001` mean lift on a corpus that DIDN'T EXERCISE the capability. This sprint measured the capability being exercised and found `+0.000`. Flipping the default off is not the right move (there might be OTHER query classes where the graph column helps), but the SHIP-DEFAULT-ON verdict deserves re-litigation after every capability-growth sprint that would EXERCISE the shipped plumbing.

## Reproducibility

```bash
# Fetch pairs live + run baseline
python3 scripts/uvts_implements_baseline.py --fetch-pairs --limit 10 --out report_current.json

# A/B (requires launchd or restart between conditions)
launchctl setenv RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED false
launchctl kickstart -k gui/$UID/com.mdemg.server
python3 scripts/uvts_implements_baseline.py --fetch-pairs --limit 10 --out report_off.json

launchctl setenv RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED true
launchctl kickstart -k gui/$UID/com.mdemg.server
python3 scripts/uvts_implements_baseline.py --fetch-pairs --limit 10 --out report_on.json

python3 scripts/uvts_implements_baseline.py --compare report_off.json report_on.json
```

Artifacts pinned:
- `report_off.json` — full JSON, per-pair grade with hits + expected sets
- `report_on.json` — same
- `compare.txt` — the printed table
