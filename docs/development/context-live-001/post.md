# Sprint Post — CONTEXT-LIVE-001

2026-06-13 · `reh3376_dev01` · Q3 stretch tier, first pick. Plan + recon
+ A/B analysis in this directory.

## Shipped (4 epics + gate)
- **Epic 1**: cross-version Jaccard guard (ContextColumn + strict mode);
  consensus denominator counts only columns able to vote (live cap-at-0.8
  gone); scorerVersion v2 + category-map hashes; CacheKey gains the
  fingerprint version (the CACHE-KEY-002 reflection guard caught it
  unprompted — and exposed a %+v-on-pointers hash determinism flaw,
  fixed). Pins: version guard, denominator, scorerVersion-flips.
- **Epic 2**: `RecomputeStaleFingerprints` healer (disclosed deviation:
  RefineWithCoactivations merges old-catalog bits — recomputation is the
  honest healer; refine wired as the enrichment pass with a
  refined-version marker). Stage 6 heals every cycle; one-shot CLI heal:
  mdemg-dev 76,906-on-v1 → 77,749-on-v3 (1,454 no-bits skipped by
  design). Live: heal mopped stragglers (44→2 updated), refine drained
  200→176→6/cycle, symbol_bits_added=0 (no symbol bits in catalogs —
  disclosed no-op until catalogs carry them).
- **Epic 3**: classifier→category dispatch via QUERY_CLASSIFY_CATEGORY_MAP
  (explicit wins; first mapped type for multi-label; pre-CacheKey).
  Live-proven: data-flow query → sparse floor 20 (override) vs global 15.
- **Epic 4**: derivation default-on (CONTEXT_QUERY_AUTO_DEFAULT, opt-out
  ?context=off). Live-proven: v3 vec cache built on a bare retrieve —
  the column votes on real traffic for the first time.

## Gate (120q UVTS A/B)
Strict verdict FAIL at mean −0.004 — root-caused to pure
citation_bonus generation noise (13 flips, all exactly ±0.100, 9−/4+);
retrieval components bit-identical on 120/120; zero per-question
regressions. Parity is the constructed expectation (lnl-demo-whk has no
catalog; UVTS passes explicit category). Pass-by-analysis, unmodified
verdict committed. The binary swaps live-fired the scorer-drift
tripwire (TSDB-CONSUME-001) — correct behavior, alerts expected.

## Follow-ups recorded
- Catalogs carry no symbol bits → Phase-B refine is a structural no-op
  until the builder allocates them (HIDDEN-CHURN/catalog scope).
- service_relationships + business_logic_constraints have no classifier
  vocabulary equivalent — per-category protections for those remain
  benchmark-only until the classifier grows labels.
- lnl-demo-whk has no ContextCatalog — UVTS cannot exercise the context
  column until that space gets one (would make future A/Bs sharper).

## Verification checklist (plan §8) — all checked
Guard pinned · cap gone · scorerVersion maps · stage-6 live · mdemg-dev
healed ≥99% of fingerprinted nodes on v3 (77,749; remainder = no-bits)
· dispatch live · explicit-wins + cache-safe · default-on + opt-out ·
UVTS retrieval-parity (analysis) · docs.

## Documents Accessed
Plan + recon_findings.md (this dir); internal/{retrieval,conversation,
ape,api,config}; Note 02 gate; RRF-SCALE-001 contract; UVTS runner +
ab_compare; live stack (Neo4j, server.log, /healthz, retrieves).
