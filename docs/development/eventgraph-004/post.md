# EVENTGRAPH-004 — Sprint Close

**Date:** 2026-06-10 · **Branch:** `reh3376_dev01` · **Target:** v0.10.x (additive — no schema/endpoint/config change)

## What shipped

The `ApplyNegativeFeedback` **contradict** action (no co-activation edge →
`MERGE CONTRADICTS`) now emits `reinforcement_events` rows with
`trigger_path='apply_negative_feedback_contradict'` — the **last Hebbian
write** in the codebase is federated. Every Hebbian mutation now lands in
TSDB with its own discriminator.

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan + `CoactivateSession` post-revival health review | `31c9022` |
| 1 | Two-statement split + contradict emission + Tier 1 tests | `8c238d2` |
| 2 | Tier 3 live verification | `af2c496` |
| 3 | Feature doc, CHANGELOG, CLAUDE.md, UATS pin, close | (this) |

## How it works

The `CONTRADICTS` MERGE lived inside a Cypher `FOREACH`, where the edge
variable is invisible to `RETURN` — so the original single statement is split
into **two statements in the same `ExecuteWrite` transaction**: (a) weaken
(EVENTGRAPH-003 telemetry, behavior unchanged) and (b) contradict with a
per-pair `RETURN`. Classification is identical: weaken never deletes edges,
so contradict's `NOT EXISTS` sees the same edge set the original
`OPTIONAL MATCH` did. `created_new_edge` is detected via the
`c.updated_at IS NULL` invariant (ON MATCH always sets it; ON CREATE never
does). Both statements EXPLAIN-validated live before commit.

**⚠️ Delta semantics:** on contradict rows, `delta_weight` is the CONTRADICTS
edge's *own* weight delta (+`negWeight` on create, 0 on re-match). The
negative-feedback meaning is carried by `trigger_path`, not the sign.

## Data-decided scope (disclosed)

- **Zero `CONTRADICTS` edges existed in any space at ship time**, and
  `/v1/learning/negative-feedback` has **no automated producer** (no hook,
  MCP tool, CLI, or internal caller). So: reuse the existing V0022 sink
  (don't build a new event class for an empty stream — the corollary of
  EVENTGRAPH-002's "don't duplicate a populated sink"), and leave the
  federation walk untouched (contradict events surface via the node-id join,
  live-confirmed). This is **telemetry-before-the-producer** — the inverse of
  the dormancy pattern that's bitten this project four times. When a producer
  arrives, the stream is observable from day one.

## Carry-over closed: CoactivateSession health review

Epic 0 also closed the EVENTGRAPH-003 follow-up with 30h of post-revival live
measurement (`coactivate_session_health_review.md`): ~13 rows/hr, textbook
C(n,2) session-clique formation, healthy weight dynamics (avg 0.116, max
0.193, no saturation), whole-space anomaly sweep clean. **Decisions:** no
runtime tuning; the 957 pre-fix `claude-core` orphan observations stay as
historical record (operator decision — synthetic backfill rejected). Notably,
in this observation-heavy workload the revived path is now the
**highest-volume Hebbian path** (~95% of event volume).

## Verification

Tier 1: 2 new parser tests (create/re-match), learning suite green, lint
clean. Tier 2: UATS `learning_negative_feedback` **5/5** live (spec extended:
zero-count `equals` assertions pin the contract; hash refreshed). Tier 3: see
`verification.md` — contradict create (+0.15, `new_edge=t`), re-match
(delta 0, evid 2), weaken byte-equivalent, federation CLI surfaces the new
trigger_path. UVTS/UBENCH N/A (no retrieval/LLM surface).

## Follow-ups

- **Negative-feedback producer** (roadmap) — decide what counts as rejection
  signal: MCP `memory_reject` tool vs Jiminy contradicted-outcome bridge.
  Own sprint; telemetry is already in place.
- **`CONTRADICTS` in the federation walk** — revisit only when contradiction
  data exists at meaningful volume (would change neighborhood semantics).
- **UOTS for the EVENTGRAPH Grafana panels** (carried over from -001/-002/-003).

## Documents Accessed

- `internal/learning/service.go` (`ApplyNegativeFeedback`), `internal/learning/reinforcement_parser.go` (+tests)
- `internal/tsdb/reinforcement_writer.go` (nullableFloat/nullableString mapping)
- `internal/eventgraph/query.go` (walk rel-types + node-id TSDB join)
- `docs/api/api-spec/uats/specs/learning_negative_feedback.uats.json` + `uats_runner.py`
- `docs/features/event-graph-federation.md`; EVENTGRAPH-001/CLI-001/002/003 sprint lines
- Live Neo4j/TSDB (mdemg-dev) — CONTRADICTS census, producer grep, Tier 3 runs
