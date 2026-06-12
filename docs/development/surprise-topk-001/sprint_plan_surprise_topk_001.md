# Sprint Plan — SURPRISE-TOPK-001: Honest Novelty + a Multiplier That Can Fire

## 1. Header & Metadata
SURPRISE-TOPK-001 · 2026-06-12 · branch `reh3376_dev01` · Roadmap Q3
Phase 2–3 (next-in-line) · effort 2d · risk low-medium (hot-path Cypher
swap; learning-weight semantics).

## 2. Problem Statement
Surprise drives initial edge weights, decay protection, and the
federated `surprise_factor` — and the live evidence shows the whole
chain is **flat dead, not merely noisy**: all 221,504 reinforcement
events ever (both `apply_coactivation` and `coactivate_session`) carry
surprise_factor exactly 1.0; node-level `surprise_score` averages
**0.023** (max 0.503 across 5,808 scored nodes) against multiplier
thresholds of 0.4/0.7 (`learning/service.go:439-449`). Two compounding
causes: (1) `computeEmbeddingNovelty` compares against an **unordered
LIMIT 50 sample** (`surprise.go:252` — no ORDER BY, no vector index):
non-deterministic, similarity-decoupled; (2) the resulting scores sit
two orders of magnitude below the thresholds, so the HIGH/MEDIUM
branches are unreachable; additionally `CoactivateSession`'s Cypher must
be checked for whether it computes the surprise CASE at all (221k
events, all 1.0). HEBB-ETA-001 is explicitly gated behind this fix.

## 3. Scope & Constraints
**In**: (1) top-K nearest-neighbor novelty via
`db.index.vector.queryNodes` (`memNodeEmbedding`, 3072-dim; the
`matchConstraintCodeByEmbedding` pattern at `jiminy/service.go:2492`),
config `SURPRISE_EMBEDDING_NOVELTY_TOPK` (default 50) +
`SURPRISE_EMBEDDING_NOVELTY_SIM_FLOOR` (default 0.0 = off); space-scoped,
`is_archived` excluded, nil-embedding fallback preserved (0.8-novel).
(2) Trace + fix the score→threshold mismatch: audit the DetectSurprise
weighted formula's empirical output, make the multiplier thresholds
config-driven (`SURPRISE_FACTOR_HIGH/MEDIUM_THRESHOLD`, defaults
data-derived from the post-top-K distribution — the RRF-SCALE lesson:
never hardcode against a signal whose scale is changing this very
sprint) in BOTH ApplyCoactivation and CoactivateSession Cypher. (3) Fix
CoactivateSession if it skips the surprise CASE. (4) Tier 3: before/after
surprise_score + surprise_factor distribution shift in Neo4j +
`reinforcement_events`. **Out**: HEBB-ETA precision weighting; reweighting
the DetectSurprise component mix beyond what the threshold recalibration
requires; backfilling historical scores (forward-only).

## 4. Dependencies
`internal/conversation/surprise.go` (252–328); `internal/learning/
service.go` (431–449 + CoactivateSession Cypher); `internal/config/
config.go`; V0018 vector indexes (verified present); recon report
2026-06-12 (file:line map + gotchas A–G); live baselines captured:
events all-1.0; node scores avg 0.023/max 0.503/n=5,808.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** top-K novelty (Cypher swap + 2 config knobs +
unit tests; verify hot-path latency unchanged) · **Epic 2** multiplier
chain: audit CoactivateSession surprise handling; thresholds →
config-driven in both Cypher sites (shared-const discipline per
RSIC-STORM tombstone lesson if the CASE is duplicated); defaults derived
from the measured post-Epic-1 score distribution (e.g. p90/p99 — decided
from data, disclosed) · **Epic 3** live Tier 3: generate fresh
observations, verify scores shift + factors >1.0 appear in
`reinforcement_events`; distribution comparison recorded · **Epic 4**
docs (feature doc update, CHANGELOG, post, run evidence), push.

## 6. Testing Plan
Tier 1: novelty unit tests (sequenceEmbedder pattern; topK param
threading; sim-floor filter; nil-embedding fallback); threshold-config
tests (non-positive → defaults). Tier 2: full `go test ./internal/...`;
EXPLAIN-validate the new Cypher (vector index actually used). Tier 3
(live): restart → observe N fresh conversation observations → node
surprise_score distribution shifts off 0.023; co-activations produce
surprise_factor ∈ {1.0, MED, HIGH} with >0 non-1.0 rows in
`reinforcement_events`; before/after histograms in the post.

## 7. Commit Strategy
Per-epic commits · lint each · push once (auto-PR) · summary comment ·
CI watch. Live-smoke surprises get own fix commits.

## 8. Verification Checklist
- [ ] queryNodes top-K in place; EXPLAIN shows index seek
- [ ] SURPRISE_EMBEDDING_NOVELTY_TOPK/_SIM_FLOOR wired (scanner green)
- [ ] CoactivateSession surprise handling audited + fixed if dead
- [ ] Thresholds config-driven in BOTH Cypher sites; defaults data-derived
- [ ] Live: non-1.0 surprise_factor rows appear; distributions recorded
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 4 (never cut).

## 10. Risks & Mitigations
Top-K vs nearest semantics LOWERS embedding novelty for everything
(nearest-50 avgSim ≫ random-50 avgSim) → thresholds are recalibrated
from the new distribution in the same sprint, never against the old
scale. Hot path latency → index seek is O(K log N) vs O(N) scan;
verified live. Score-scale contract → all consumers of surprise_score /
surprise_factor re-audited (recon lists them); thresholds config-driven.

## 11. Documents Accessed
ROADMAP:42; recon report (a9c6654); live TSDB + Neo4j baselines
(captured above); surprise.go; learning/service.go; V0018 migration;
jiminy/service.go:2492 (pattern).

## 12. Rollback Procedures
Code-only; revert commits. Forward-only scores; no schema change.
