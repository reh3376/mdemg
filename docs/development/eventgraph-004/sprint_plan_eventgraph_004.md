# Sprint Plan EVENTGRAPH-004 — `CONTRADICTS` Event Federation

## 1. Header & Metadata

- **Sprint ID:** EVENTGRAPH-004
- **Sprint line:** `docs/development/eventgraph-004/`
- **Date opened:** 2026-06-10
- **Branch:** `reh3376_dev01`
- **Target version:** v0.10.x (additive — no schema, endpoint, or config change)
- **Estimated effort:** ~0.5 dev-day
- **OpenAI spend:** $0
- **Risk level:** Low

## 2. Problem Statement

`ApplyNegativeFeedback` (`internal/learning/service.go`) has two actions per
(query, rejected) pair: **weaken** (an existing `CO_ACTIVATED_WITH` edge —
federated into `reinforcement_events` by EVENTGRAPH-003) and **contradict**
(no co-activation edge exists → `MERGE` a `CONTRADICTS` edge). The contradict
action emits no telemetry — it is the last unfederated Hebbian write path in
the codebase.

**Live findings that reshaped the scope (data-decided, disclosed):**

1. **Zero `CONTRADICTS` edges exist in any space** — the path has never fired
   in real usage (verified live 2026-06-10: `MATCH ()-[c:CONTRADICTS]->()`
   returns no rows across the whole graph).
2. **`/v1/learning/negative-feedback` has no production caller** — no hook,
   MCP tool, CLI command, or internal service invokes it (repo-wide grep).
   The endpoint is producer-dormant: the same silent-gap class as the
   `CoactivateSession` dormancy found in EVENTGRAPH-003, but one level up —
   nothing calls the endpoint at all.

This sprint therefore **instruments the path before the producer arrives** —
the inverse of the dormancy pattern that has bitten this project four times
(EVENTGRAPH-001 RRF Activation-drop, RRF-SCALE-001 guidance gates,
NOSILENT-001 backup scheduler, EVENTGRAPH-003 CoactivateSession). When a
producer is wired later, telemetry exists from day one and the stream's
health is observable immediately.

## 3. Scope & Constraints

**In scope:**
- Emit one `reinforcement_events` row per contradict action with
  `trigger_path='apply_negative_feedback_contradict'`.
- Behavior-preserving Cypher restructure (the `CONTRADICTS` MERGE currently
  lives inside a `FOREACH`, where the edge variable is invisible to `RETURN`).
- 3-tier testing incl. live Tier 3; documentation; disclose the producer gap
  as a roadmap follow-up.

**Out of scope:**
- A new hypertable / federation endpoint / CLI subcommand for contradictions
  (see decision 1 below — empty stream).
- Extending the federation walk to traverse `CONTRADICTS` edges (zero edges
  to traverse — deferred until data exists; would silently change the
  existing API's neighborhood semantics).
- Wiring a producer for negative feedback (own sprint — needs design: MCP
  `memory_reject` tool vs Jiminy contradicted-outcome bridge; it decides
  *what counts as rejection signal*).

**Constraints:**
- Sequential epics; live Tier 3 required; no hardcoded values; weight-update
  behavior must be unchanged by construction.

**Three data-decided design decisions (plan-options pattern — disclosed):**

1. **Reuse `reinforcement_events`; no new hypertable.** EVENTGRAPH-002's
   lesson ("don't duplicate a populated sink") has a corollary: *don't build
   a new sink for an empty stream*. The V0022 schema fits fully — src/dst,
   prev/new/delta weight, evidence_count, `created_new_edge`, and the
   `trigger_path` discriminator. A dedicated event class (hypertable +
   endpoint + CLI) would be infrastructure for zero rows; escalate only if
   contradiction volume + query shapes ever demand it.
2. **`delta_weight` = the CONTRADICTS edge's own weight delta** (+`negWeight`
   on create, `0` on evidence-increment re-match). The *negative* semantics
   live in `trigger_path`, not the sign — documented prominently so consumers
   summing deltas over a node don't misread `+0.15` as Hebbian strengthening.
3. **Federation walk untouched** (`CO_ACTIVATED_WITH|GENERALIZES`).
   Contradict events still surface in the federation read whenever either
   endpoint is in the neighborhood — the TSDB join in
   `internal/eventgraph/query.go::queryEvents` is by node-id, not edge type.

## 4. Dependencies

- EVENTGRAPH-001: V0022 `reinforcement_events` hypertable + buffered writer
  (shipped).
- EVENTGRAPH-003: per-pair `RETURN` pattern + `reinforcement_parser.go`
  (shipped); the writer is already injected into the learning service.
- Existing UATS spec `learning_negative_feedback.uats.json`.
- Live stack (native `mdemg serve` + Docker Neo4j/TimescaleDB) for EXPLAIN
  validation and Tier 3.

## 5. Implementation Plan

**Epic 0 — Sprint plan + carry-over findings (~0.1d)**
Commit this plan + `coactivate_session_health_review.md` (the 2026-06-10
post-fix graph-health measurements that closed the EVENTGRAPH-003 follow-up
"investigate the revived CoactivateSession at scale": 399 rows/30h, clean
15-node session clique, healthy weight dynamics, decision = no tuning, leave
pre-fix orphans as historical record).

**Epic 1 — Cypher restructure + contradict emission (~0.25d)**
Restructure `ApplyNegativeFeedback` into **two statements in the same
`ExecuteWrite` transaction** (atomicity preserved):
(a) the weaken statement — current Cypher minus the contradict `FOREACH`;
    per-pair `RETURN` unchanged;
(b) a contradict statement — `MATCH … WHERE NOT EXISTS {co-activation} MERGE
    (q)-[c:CONTRADICTS]->(r) ON CREATE … ON MATCH … RETURN` per-pair rows.
`created_new_edge` detected via `c.updated_at IS NULL` (the `ON MATCH` branch
always sets `updated_at`; the `ON CREATE` branch never does — invariant
pinned by comment at both SET sites). Emit rows with
`trigger_path='apply_negative_feedback_contradict'`. Both statements
`EXPLAIN`-validated against live Neo4j before commit. The
`NegativeFeedbackResult` counts (`processed/weakened/contradicted`) are
unchanged.

**Epic 2 — Tier 3 live verification (~0.1d)**
Synthetic probe nodes (`eg004-*` session, EVENTGRAPH-003 pattern) → POST
`/v1/learning/negative-feedback` on a pair with no co-activation → observe:
the `CONTRADICTS` edge in Neo4j; the contradict row in `reinforcement_events`
(correct `trigger_path`, `created_new_edge=true`, `delta_weight=+negWeight`);
re-POST → `evidence_count_after=2`, `delta_weight=0`,
`created_new_edge=false`. Federation read with seed=probe node surfaces the
event (hops=0 suffices — the TSDB join matches src/dst). Weaken path
re-verified unchanged. Findings → `verification.md`.

**Epic 3 — Documentation Update (final epic — never cut)**
`docs/features/event-graph-federation.md` trigger_path table + delta-
semantics warning; `CHANGELOG.md`; `CLAUDE.md` architecture note; UATS spec
extended to pin the `contradicted` count; `post.md` sprint close with the
producer-gap follow-up.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** parser/emission tests for contradict rows (create vs
  re-match branches); existing `internal/learning` tests pass;
  `golangci-lint run ./...` clean.
- **Tier 2 (integration):** UATS `learning_negative_feedback.uats.json`
  passes (response contract unchanged) + extended assertion pinning
  `contradicted`; `make test-uvts-lint` unaffected (no retrieval change).
- **Tier 3 (live e2e):** Epic 2 — the real binary against real Neo4j + TSDB,
  rows observed via SQL + the federation CLI. Live smoke item: *run
  negative-feedback against the real system, observe the CONTRADICTS edge in
  Neo4j + the event row in TSDB + the event in `mdemg eventgraph
  reinforcement-neighborhood` output, confirm the weaken path unchanged.*
- **UVTS / UBENCH:** N/A — no retrieval or LLM surface changes.

## 7. Commit Strategy

One commit per epic on `reh3376_dev01`; surprise live-smoke bugs get their
own fix-commit (Phase 11.6.2 precedent); push → auto-PR; sprint summary
comment on the PR.

## 8. Verification Checklist

- [ ] Both restructured statements EXPLAIN-validate; weaken RETURN/behavior
      equivalent to pre-sprint
- [ ] Contradict create: row lands with
      `trigger_path='apply_negative_feedback_contradict'`,
      `created_new_edge=true`, `delta_weight=+negWeight`
- [ ] Contradict re-match: `evidence_count_after=2`, `delta_weight=0`,
      `created_new_edge=false`
- [ ] Live smoke: negative-feedback against the real system → CONTRADICTS
      edge in Neo4j + event row in TSDB + event in federation CLI output;
      weaken path unchanged
- [ ] Tier 1 green; UATS green; lint clean
- [ ] Feature doc, CHANGELOG, CLAUDE.md, post.md updated

## 9. Documentation Update

Epic 3 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Two-statement split changes weaken/contradict classification at the margin | Low | High | Same transaction; the contradict statement's `WHERE NOT EXISTS{co-activation}` is evaluated after weaken (which never deletes edges) — classification identical. Tier 3 re-verifies weaken. |
| `updated_at IS NULL` new-edge detection breaks if a future edit sets `updated_at` on create | Low | Medium | Comment pinning the invariant at both SET sites; Tier 1 test covers both branches. |
| Consumers misread `+delta_weight` on contradict rows as strengthening | Medium | Low | Decision 2 documented in the feature doc; the `trigger_path` filter is the contract. |

## 11. Documents Accessed

- `internal/learning/service.go` (`ApplyNegativeFeedback`)
- `internal/learning/reinforcement_parser.go`
- `internal/eventgraph/query.go` (federation walk + TSDB join)
- `internal/config/config.go` (EVENTGRAPH config block)
- `docs/api/api-spec/uats/specs/learning_negative_feedback.uats.json`
- Live Neo4j/TSDB measurements (2026-06-10 session)
- EVENTGRAPH-001 / -002 / -003 sprint lines

## 12. Rollback Procedures

Revert the Epic 1 commit — restores the single-statement Cypher. Recorded
contradict rows are inert telemetry (no consumer assumes the new
trigger_path). Probe nodes/edges remain as disclosed test data in the
`eg004-*` probe session.
