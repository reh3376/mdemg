# Memory Lifecycle — Weakening, Graduation, and Honest Co-activation

**Sprint**: NEGFEED-001 + COOLER-001 (2026-06-13) · **Status**: shipped

## Why

The Hebbian memory lifecycle was half-open at both ends, and its
dominant producer had wrong semantics (live-verified at HEAD):

- **The graph could only strengthen.** `ApplyNegativeFeedback` (weaken
  CO_ACTIVATED_WITH / record CONTRADICTS) had no automated caller —
  live, 2 weaken rows ever (test rows) and **1 CONTRADICTS edge in the
  entire graph**.
- **Nothing graduated.** Two divergent graduation paths (the Context
  Cooler at threshold 0.8 with a pinned-guard and `graduated_at`; an
  RSIC action at a hardcoded 0.7 with neither), the cooler default-off
  and pinned to one space, `mdemg maintenance` never graduating, and
  retrieval reading no graduation signal at all.
- **CoactivateSession had wrong semantics at scale.** Live it was 73%
  of all reinforcement writes (244k rows/14d). It ran synchronously on
  the Observe request path and regenerated the full unbounded C(n,2)
  session clique every call — O(n²) per observe, and `evidence_count`
  measured session length, not co-activation count.

## What shipped

### 1. CoactivateSession: off-request + delta emission
The Observe call site now fires CoactivateSession in a panic-recovered
background goroutine on a detached context — observe latency is
decoupled from session size. CoactivateSession co-activates only the
**new** observation against the most-recent prior session members,
bounded by `LEARNING_SESSION_CLIQUE_WINDOW` (default 50; 0 = all
priors). Per-call work is O(window), not O(n²); a pair's edge is
created/strengthened only on genuine co-activation, so `evidence_count`
counts co-activation events. The cumulative graph is unchanged (sum of
per-call deltas = C(n,2)). The DH-004 stability raise
(`reinforceSessionObservations`) is preserved. Forward-only: existing
inflated `evidence_count` values are left as-is.

### 2. Anti-Hebbian producer (two bridges)
- **Bridge A (automatic)**: a Jiminy `contradicted` guidance outcome
  weakens the co-activations *among that guidance's own source nodes*
  (they jointly produced guidance that was contradicted). Uses
  `ApplyNegativeFeedback`'s `q <> r` guard, so single-source guidance
  is a safe no-op and unrelated memory is never touched; weight floors
  at 0. Gated `JIMINY_CONTRADICTED_WEAKEN_ENABLED` (default true).
- **Bridge B (explicit)**: the MCP `memory_reject` tool — the agent
  rejects a memory that surfaced wrongly for a context; resolves both
  sides via retrieve and POSTs `/v1/learning/negative-feedback`. The
  opposite of `memory_associate`. (24 MCP tools now.)

Both feed the existing EVENTGRAPH-003/004 federation
(`reinforcement_events`, `apply_negative_feedback` /
`apply_negative_feedback_contradict` trigger_paths) — no new sink.

### 3. Graduation unified onto the Context Cooler
RSIC's `executeGraduateVolatile` delegates to
`ContextCooler.ProcessGraduations` (config threshold 0.8 + pinned-guard
+ `graduated_at`) instead of its hardcoded-0.7 inline Cypher. The
rollback snapshot predicate is aligned to the cooler's via
`SnapshotStore.SetGraduationThreshold` so executor and snapshot can't
drift (RSIC-STORM-001 discipline). The cooler also runs per-space from
`mdemg maintenance` (new Step 2 — Graduation, between decay and prune;
skipped in dry-run since it mutates), so graduation no longer depends
on the default-off background loop.

### 4. "Graduated" means something to retrieval: decay protection
CO_ACTIVATED_WITH edges incident to a graduated (`volatile=false`) node
decay at a reduced rate — stable memory's associations resist time
decay. `GRADUATED_DECAY_PROTECTION_FACTOR` (default 0.5; 1.0 = off, 0 =
no decay) multiplies the effective decay rate in the live decay job.
This is a learning-side effect, not a retrieval-scorer change — no RRF
score-scale risk, no UVTS gate.

## How to use

- Reject a wrong memory from an agent: MCP `memory_reject(query,
  rejected_query)`.
- Graduate + maintain a space: `mdemg maintenance --space-id <id>
  --dry-run=false` (decay → graduate → prune).
- Tune: `LEARNING_SESSION_CLIQUE_WINDOW`,
  `JIMINY_CONTRADICTED_WEAKEN_ENABLED`, `COOLER_GRADUATION_THRESHOLD`,
  `GRADUATED_DECAY_PROTECTION_FACTOR`.

## Limitations / follow-ups
- Bridge A fires only for multi-source contradicted guidance
  (single-source is a no-op by design); the explicit `memory_reject`
  is the primary producer.
- Forward-only: historical inflated `evidence_count` values are not
  backfilled.
- The decay-protection semantic is learning-side; a retrieval-scorer
  graduation boost was deferred (would need a 120q UVTS A/B).
