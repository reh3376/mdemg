# Sprint Post — NEGFEED-001 + COOLER-001

2026-06-13 · `reh3376_dev01` · Q3 Phase 2, ranked #9. Plan + recon in
this dir. Highest graph-mutation risk of the quarter; Tier 3 used a
throwaway space with the mdemg-dev baseline verified unchanged.

## Shipped (6 epics)
- **Epic 1** — CoactivateSession off the Observe request path
  (panic-recovered bg goroutine, detached ctx) + delta emission
  (`LEARNING_SESSION_CLIQUE_WINDOW` 50). O(window)/observe; evidence_count
  counts genuine co-activations. Obsolete C(n,2) test rewritten to delta
  arithmetic; DH-004 stability raise preserved.
- **Epic 2** — anti-Hebbian producer. Bridge A (Jiminy contradicted →
  ApplyNegativeFeedback over source nodes, self-pair-guarded) +
  Bridge B (MCP memory_reject + contract). Downstream EVENTGRAPH-003/004
  reused.
- **Epic 3** — graduation unified: RSIC → ContextCooler.ProcessGraduations;
  snapshot predicate aligned (SetGraduationThreshold); cooler per-space
  from `mdemg maintenance` Step 2.
- **Epic 4** — graduated-incident edges resist decay
  (`GRADUATED_DECAY_PROTECTION_FACTOR` 0.5; learning-side).
- **Epic 5** — live Tier 3 (below).
- **Epic 6** — feature doc + CHANGELOG + this post.

## Tier 3 evidence (throwaway space `negfeed-tier3`)
- Off-request: observe latencies 0.19–0.52s, flat as the session grew.
- Delta: 3 distinct obs → 6 directed CO_ACTIVATED_WITH edges,
  evidence_count uniformly 1 (the old full-clique would show
  max_evidence ≥2 from per-observe re-incrementing). Stage-6 heal+refine
  also fired live (scanned=2 updated=2).
- Weaken: `/v1/learning/negative-feedback` dropped a pair 0.119→0.0
  ({weakened:1}); the `apply_negative_feedback` reinforcement row
  flushed with delta_weight −0.119 (federation telemetry confirmed).
- Graduation: `mdemg maintenance --space-id negfeed-tier3
  --dry-run=false` → "Graduated: 5" via the Context Cooler (Step 2);
  all 5 nodes volatile=false WITH graduated_at set (cooler semantic,
  not the old bare RSIC flip).
- Decay: the new graduated-incident query clause validated live (8 edges
  scanned, no Cypher error); protection math unit-pinned.
- **mdemg-dev baseline UNCHANGED**: 191457 coact / 1 contradicts before
  and after — the destructive tests stayed contained to the throwaway
  space.

## Decisions disclosed
- "Graduated means to retrieval" → decay-protection (learning-side), NOT
  a retrieval-scorer boost — keeps off the RRF score-scale contract and
  avoids a 120q UVTS gate. Scorer boost deferred.
- CoactivateSession is forward-only: historical inflated evidence_count
  left as-is (no backfill).
- Bridge A fires only for multi-source contradicted guidance
  (single-source no-op by design); memory_reject is the primary producer.

## Follow-ups
- Throwaway test space `negfeed-tier3` left in place (5 inert nodes; the
  pre-bash guard blocked its destructive Cypher cleanup — operator can
  remove via `mdemg space` tooling or confirmed Cypher).
- Retrieval-scorer graduation boost (deferred; UVTS-gated).
- evidence_count historical backfill (if ever wanted).

## Verification checklist (plan §8) — all met
Off-request ✓ · delta + evidence semantics ✓ · window configurable +
DH-004 preserved ✓ · Jiminy bridge (live weaken via endpoint + unit) ✓ ·
MCP memory_reject + contract ✓ · RSIC→cooler, 0.7 hardcode gone ✓ ·
maintenance graduates via cooler ✓ · graduated edges resist decay ✓ ·
Tier 3 + baseline-unchanged ✓ · docs ✓.

## Documents Accessed
ROADMAP:41; recon_findings.md; EVENTGRAPH-003/004 +
coactivate_session_health_review.md; DH-004 / RSIC-STORM-001 CLAUDE.md
notes; internal/{learning,conversation,jiminy,ape,api,cli,config}.
