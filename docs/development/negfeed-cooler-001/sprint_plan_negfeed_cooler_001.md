# Sprint Plan — NEGFEED-001 + COOLER-001: Complete the Memory Lifecycle

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · Roadmap Q3 Phase 2 (committed core,
ranked #9) · effort 5.5d est · risk **high** (graph-mutating: an
anti-Hebbian weaken producer + a change to the path carrying 99% of
Hebbian writes). Destructive-test → restore discipline mandatory.

## 2. Problem Statement
The memory lifecycle is half-open at both ends, and its dominant
producer has wrong semantics (recon CONFIRMED, live-verified):
- **No weakening**: `ApplyNegativeFeedback` (weaken CO_ACTIVATED_WITH /
  MERGE CONTRADICTS — both with EVENTGRAPH-003/004 federation already
  wired) has ZERO automated callers; the graph can only strengthen.
  Live: 2 `apply_negative_feedback` rows ever (test rows), 1 CONTRADICTS
  edge in the entire graph. A Jiminy `OutcomeContradicted` records an
  outcome edge but never weakens the source nodes.
- **Nothing graduates**: two divergent graduation paths (cooler @ 0.8 +
  pinned-guard + graduated_at; RSIC `executeGraduateVolatile` @ hardcoded
  0.7, neither guard nor timestamp); the cooler is default-OFF and
  hardcoded to mdemg-dev; `maintenance` never graduates; and retrieval
  reads no graduation signal at all — "graduated" is write-only.
- **CoactivateSession** (live 244k rows / 73% of reinforcement_events,
  99%+ with symbol co-activation) runs SYNCHRONOUSLY on the Observe
  request path and regenerates the FULL unbounded session clique every
  call: O(n²) per observe, and `evidence_count` measures session length
  not co-activation count. (Small-n weight health is fine — the defect
  is scaling + metric semantics; HEBB-ETA-001 would inherit both.)

## 3. Scope & Constraints
**In**:
1. **Anti-Hebbian producer (NEGFEED)** — (a) Jiminy bridge: inject a
   learning adapter into the jiminy service (mirror `confidenceUpdater`);
   on `OutcomeContradicted` with `len(item.SourceNodes)>0`, call
   `ApplyNegativeFeedback(ctx, spaceID, queryNodeIDs, item.SourceNodes)`
   (query nodes = guidance constraint node / retrieval seeds). (b) MCP
   `memory_reject` tool → POST /v1/learning/negative-feedback + a
   `mcp_contract_test.go` contract (MCP-REVIVE-001 rule). Downstream
   (weaken + CONTRADICTS + V0022 federation) is untouched — wiring only.
2. **CoactivateSession semantics (NEGFEED)** — (a) move the call OFF the
   Observe request path into a supervised background goroutine
   (fire-and-forget; already error-non-fatal); (b) emit DELTAS — new
   observation × existing session members (O(n) directed pairs), not the
   full C(n,2) clique — so `evidence_count` counts genuine co-activations;
   (c) bound the session window via `LEARNING_SESSION_CLIQUE_WINDOW` (last
   N members, default 50; 0 = unbounded legacy) as a safety belt.
   PRESERVE the DH-004 `UpdateStabilityOnReinforcement` stability raise.
   Forward-only: existing inflated `evidence_count` values are left as-is
   (disclosed; a backfill is out of scope).
3. **Graduation unification (COOLER)** — RSIC `executeGraduateVolatile`
   delegates to `ContextCooler.ProcessGraduations` (config threshold,
   pinned-guard, graduated_at); remove the hardcoded 0.7 at the executor
   AND its snapshot predicate. Run the cooler per-space from `mdemg
   maintenance` (it already takes `--space-id`), gated so it executes the
   decay→graduate cycle for the requested space (not only the background
   loop's hardcoded mdemg-dev).
4. **"Graduated" means something to retrieval (COOLER)** — minimal,
   lowest-risk interpretation: **decay protection**. Graduated/stable
   nodes resist learning-edge decay (a learning-side multiplier in the
   decay path), NOT a retrieval-scorer change — so no scoring-scale risk
   and no 120q UVTS gate required. (Options weighed in §10; scorer-side
   boost rejected for this sprint as it would need a full UVTS A/B and
   re-opens the RRF-SCALE contract.)
**Out**: retrieval-scorer graduation boost (deferred — needs UVTS A/B);
evidence_count historical backfill (forward-only); new CONTRADICTS sink
(reuse V0022); fixing the dormant surprise-path CONTRADICTS producer
(separate concern); growing classifier vocab.

## 4. Dependencies
Recon (this dir); `internal/learning/service.go:{797,841,918,1029}`,
`internal/conversation/{service.go:489,cooler.go:252,contradiction.go}`,
`internal/jiminy/{service.go:1487,1831,types.go:62,persistence.go}`,
`internal/ape/{task_dispatch.go:444,action_snapshot.go:226}`,
`internal/cli/{mcp.go,maintenance.go,serve.go:604}`,
`internal/api/{handlers_learning.go,server.go:2386}`, config.go:{515,708}.
EVENTGRAPH-003/004 federation (downstream, built).

## 5. Implementation Plan
Epic 0 plan+recon · **Epic 1** CoactivateSession fix (bg goroutine +
delta-emission + bounded window + config; preserve DH-004 stability;
pin tests for delta semantics + window bound) · **Epic 2** anti-Hebbian
producer (Jiminy contradicted→weaken adapter bridge + MCP memory_reject
+ contract) · **Epic 3** graduation unification (RSIC delegates to
cooler; maintenance per-space cooler invocation; de-hardcode 0.7×2) ·
**Epic 4** graduation decay-protection (learning decay multiplier reads
stability/graduated; config-gated) · **Epic 5** live Tier 3 + restore ·
**Epic 6** docs (feature doc, CHANGELOG, post), push.

## 6. Testing Plan
**T1**: delta-emission produces n-1 pairs not C(n,2) (count assertion);
window bound caps participants; evidence_count increments only on genuine
new co-activation; graduation predicate parity (RSIC path == cooler
path); decay-protection multiplier math; memory_reject contract.
**T2**: full `go test ./internal/...`; config scanner; route gate; MCP
contract suite.
**T3 (live, on mdemg-dev — destructive, restore after)**: (a) **backup
first** (`mdemg` backup or pg/neo4j snapshot); (b) observe a multi-obs
session → confirm CoactivateSession runs off-request (observe latency
flat as session grows; reinforcement_events still written; evidence_count
reflects deltas); (c) drive a Jiminy contradicted outcome → confirm an
`apply_negative_feedback` row lands + the source edge weakens / a
CONTRADICTS edge appears (federation read surfaces it); (d) MCP
memory_reject end-to-end; (e) run `mdemg maintenance --space-id mdemg-dev`
→ confirm graduations occur via the cooler path (graduated_at set,
pinned respected); (f) confirm graduated nodes resist decay vs volatile;
(g) **restore/verify** graph state is consistent (no unintended
weakening of production edges — scope the contradicted-bridge test to a
throwaway guidance/session).

## 7. Commit Strategy
Per-epic · lint each · push once after Epic 6 (auto-PR) · summary · CI
watch. Live-smoke surprises get own fix-commits (precedent).

## 8. Verification Checklist
- [ ] CoactivateSession off the request path; observe latency flat at large n
- [ ] Delta emission (O(n)); evidence_count = co-activation count, not session length
- [ ] Session window bound configurable; DH-004 stability raise preserved
- [ ] Jiminy contradicted → ApplyNegativeFeedback (live row + edge effect)
- [ ] MCP memory_reject + contract
- [ ] RSIC graduation delegates to cooler; 0.7 hardcode gone (executor + snapshot)
- [ ] `mdemg maintenance --space-id` graduates via cooler
- [ ] Graduated nodes resist decay (config-gated, no retrieval-scorer change)
- [ ] Tier 3 destructive tests + state restore verified
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 6 (never cut).

## 10. Risks & Mitigations
**Weaken corrupts production memory** → the Jiminy bridge fires only on
genuine `OutcomeContradicted` with SourceNodes; Tier-3 uses a throwaway
session; ApplyNegativeFeedback already floors weight at 0 (no negative
weights); backup before live test. **CoactivateSession change drops
edges that retrieval/Hebbian depend on** → delta emission is purely
additive to NEW pairs (never deletes existing edges); window bound
defaults generous (50) with 0=legacy escape; forward-only evidence_count
(no rewrite of history). **"Graduated→retrieval" scope creep** → chose
the decay-protection interpretation precisely to avoid a scorer change +
UVTS gate; scorer boost explicitly deferred. **Graduation unification
changes RSIC behavior** → cooler threshold (0.8) is STRICTER than RSIC's
0.7, so fewer/safer graduations, not more; pinned-guard ADDS safety.
**Background CoactivateSession loses error visibility** → route through
the supervisor / log loudly (SUPERVISOR-002 pattern), don't bare-go.

## 11. Documents Accessed
ROADMAP:41; recon_findings.md (this dir); EVENTGRAPH-003/004 records +
coactivate_session_health_review.md; DH-004 (CoactivateSession stability)
+ RSIC-STORM-001 (cooler tombstone cap) CLAUDE.md notes; score-scale
contract (RRF-SCALE-001, for the deferred scorer option).

## 12. Rollback Procedures
All new behavior config-gated: `LEARNING_SESSION_CLIQUE_WINDOW=0` restores
full-clique; the Jiminy bridge behind a `JIMINY_CONTRADICTED_WEAKEN_ENABLED`
flag (default true, 0=off); graduation decay-protection behind a flag.
Background-goroutine move + RSIC→cooler delegation are code reverts.
Forward-only data changes; backup taken before Tier 3.
