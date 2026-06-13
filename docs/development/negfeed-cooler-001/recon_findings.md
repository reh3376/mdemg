# NEGFEED-001 + COOLER-001 Recon (2026-06-13, HEAD post-#456)

3 lanes + orchestrator live verification. All roadmap claims CONFIRMED;
two refinements noted.

## Lane 1 — anti-Hebbian producer (NEGFEED)
- `ApplyNegativeFeedback(ctx, spaceID, queryNodeIDs, rejectedNodeIDs)`
  at `internal/learning/service.go:1029` — weakens CO_ACTIVATED_WITH
  (trigger_path `apply_negative_feedback`, floor 0) OR MERGEs
  CONTRADICTS when no co-activation edge (trigger_path
  `apply_negative_feedback_contradict`). **Only caller**:
  `handlers_learning.go:42` (POST /v1/learning/negative-feedback,
  route server.go:2386). Zero hook/MCP/CLI/internal callers. CONFIRMED.
- Jiminy `OutcomeContradicted` produced at `jiminy/service.go:1831`,
  surfaces in the feedback loop (~:1487–1554: escalation, comprehension
  1.0, PersistGuidanceOutcome, constraint_outcomes row) — **nothing
  bridges it to the weaken path**. CONFIRMED absent.
- No MCP `memory_reject` (23 tools; 7 memory tools, none reject).
- `GuidanceItem.SourceNodes` (`jiminy/types.go:62`) populated and
  in-scope at the contradicted site → a bridge has its inputs.
- **Refinement**: a 2nd CONTRADICTS producer exists in code — the
  observe-time surprise/NLI path (`conversation/contradiction.go:209`
  `upsertContradictionEdge`, via `DetectSurprise→checkContradictions`,
  gated `ContradictionEnabled` default true) — but **live it has made
  1 CONTRADICTS edge in the entire graph**. Effectively dormant; the
  roadmap's "zero ever" is effectively correct. NOT federated to
  reinforcement_events (no trigger_path).

## Lane 2 — graduation + cooler (COOLER)
- TWO graduation paths, divergent: (A) `conversation/cooler.go:252`
  `ProcessGraduations` — volatile & !pinned & stability ≥ `COOLER_
  GRADUATION_THRESHOLD` (0.8) → sets volatile=false + graduated_at;
  (B) RSIC `task_dispatch.go:444` `executeGraduateVolatile` —
  stability ≥ **hardcoded 0.7**, no pinned-guard, no graduated_at
  (snapshot predicate `action_snapshot.go:226` mirrors 0.7). CONFIRMED.
- Cooler config `config.go:708-714`; master gate `CONTEXT_COOLER_
  ENABLED` **default false** (`config.go:515`); loop supervised
  (`server.go:2062`) but started only if enabled AND **hardcoded to
  mdemg-dev** (`serve.go:604`). Nothing graduates by default. CONFIRMED.
- Retrieval reads `stability_score`/`volatile`/`graduated_at`:
  **ZERO** (only `is_archived`, 5 sites). "Graduated" is a no-op
  signal to retrieval. CONFIRMED — greenfield.
- `mdemg maintenance` runs decay+prune only; no cooler/graduation
  (0 hits). CONFIRMED.
- Unification target: `executeGraduateVolatile` → delegate to
  `ContextCooler.ProcessGraduations`; maintenance calls same per-space.

## Lane 3 — CoactivateSession semantics
- SYNCHRONOUS on the Observe request path (`conversation/service.go:489`,
  blocks the HTTP response; error non-fatal). CONFIRMED.
- Full session clique every call: `learning/service.go:797-805` MATCH
  ALL conversation_observation WHERE session_id=$sid → C(n,2) pairs,
  both directions MERGE'd. **N UNBOUNDED** (no LIMIT / window cap; the
  1h temporalProximity only weights, >1h still gets 0.1). CONFIRMED.
- `evidence_count` increments on ALL clique edges every call
  (`:841-843`) → tracks session length, not co-activation count.
  CONFIRMED.
- **Live volume (14d reinforcement_events)**: coactivate_session
  244,030 (73%), apply_symbol_coactivation 88,513 (26%),
  apply_coactivation 1,324, apply_negative_feedback 2,
  apply_negative_feedback_contradict 2. Session paths = 99%+.
- Health-review ("healthy", avg w 0.116, n=15) vs roadmap ("wrong
  semantics") **COMPATIBLE**: review measured small-n weight health
  (real); roadmap flags large-n O(n²) latency + evidence_count
  mis-definition (also real). DH-004 stability raise
  (`learning/service.go:918`) lives on this path — must preserve.
- Fix shape: (1) off request path (bg goroutine); (2) emit deltas
  (new obs × existing members, O(n)) not full clique — fixes both
  latency and evidence_count semantics; (3) bound window (safety belt).

## Orchestrator verdict
Producer-wiring only (downstream built EVENTGRAPH-003/004); graduation
unification is real (0.7 vs 0.8, missing graduated_at/pinned-guard);
CoactivateSession fix is the highest-volume + highest-risk change.
Effectively-empty CONTRADICTS baseline confirmed live (1 edge).
