# U5 · RSIC, Recursive Self-Improvement (8 slides)

**Intent**: RSIC is MDEMG's self-improvement engine. It reflects on operational metrics, generates insights, triggers improvement cycles, and (where safe) auto-executes mutations. This section covers mental model, admission mechanics, insight generation, action taxonomy, /ui/ RSIC tab, current trajectory, gotchas.
**Target slides**: 8
**Mermaid**: 1 (U5.2, RSIC cycle)
**Natural Q&A anchor**: after U5

---

## U5.1 · Section Landing

**Title**: `U5 · RSIC, The Self-Improvement Engine`

**Body**:
- Intent: "RSIC asks 'how well is MDEMG working?' every few minutes, and when it finds a problem WITH a known action, it triggers improvement, sometimes just a diagnostic alert, sometimes a real substrate mutation."
- Preview: cycle → admission → insight → action → UI → trajectory → gotchas
- Points back to U1.5 "How does it continuously improve?", this is loop #1 (the top-level improvement orchestrator)

**Provenance**:
- CLAUDE.md "RSIC admission + archival attribution (RSIC-STORM-001)"
- CLAUDE.md "RSIC auto-execute for enforcement patterns (ENFORCE-AUTO-EXECUTE)"
- `docs/features/rsic-orchestration.md`

**note**:
- Operator cue: "RSIC is the subsystem that MOST makes MDEMG unique. Everything else is machinery; RSIC is how the machinery learns."

---

## U5.2 · Mental Model: The RSIC Cycle

**Title**: `How RSIC Thinks`

**Body**:
- **Mermaid #5** (cycle):
  1. **Assess**: read current substrate health across 7 dimensions (retrieval, memory, edge, task, guidance, protocol, synergy) → `SelfAssessmentReport`
  2. **Reflect**: pattern-match against 32+ rule-based patterns (SQL-queryable signals) + LLM reflector (novel-pattern discovery) → list of `Insight`s
  3. **Admit**: gate: single-flight cooldown, protected-space check, rate limits, dry-run, RSIC-STORM-001 reserves the slot at admission (not completion)
  4. **Act**: dispatch the recommended action: diagnostic alert / HITL review item / substrate mutation (archive, adjust confidence)
  5. **Record**: write cycle event to TSDB `ft_training_cycles` + emit metric
  6. **Reflect again**: next cycle sees the outcome
- One-sentence summary: "RSIC is the loop that reads MDEMG's own telemetry and decides what to improve."

**Provenance**:
- CLAUDE.md "RSIC-STORM-001" admission + archival pattern
- `internal/ape/` package

**note**:
- PM-fallback: "Think of RSIC as the system's internal post-mortem engineer, writes a review after every shift and files one actionable fix."

---

## U5.3 · Admission Mechanics (Why This Matters)

**Title**: `Why RSIC Doesn't Storm`

**Body**:
- Pre-RSIC-STORM-001: cycles were CREATED on completion, not admission → race condition ran ~20-30k micro cycles/day, saturating llama-server (the recurring LLM-timeout cascade class)
- Post-RSIC-STORM-001: `EvaluateTrigger` RESERVES the slot at admission under the checking lock; `RecordTrigger` updates with real cycle id; `CompleteCycle` frees; failed cycles still cool down
- Live result on `mdemg-dev` (post-fix): ~1 admitted / 132 rejected in 4-min window
- Protected spaces: `RSICProtectedSpaces` (default `mdemg-dev`), RSIC can't mutate; it can only diagnose
- Rate limits per-action: `ENFORCEMENT_AUTO_EXECUTE_MAX_PER_HOUR` (default 3), per-code cooldown (`_COOLDOWN_HOURS` default 24)

**Provenance**:
- CLAUDE.md "RSIC admission + archival attribution (RSIC-STORM-001)", the detailed invariant
- `internal/ape/scheduler.go`

**note**:
- Operator cue: "If you see RSIC rejecting many cycles in a short window, that's by design. The admit gate protects the LLM endpoint."

---

## U5.4 · Insight Generation

**Title**: `What RSIC Watches`

**Body**:
- 32+ **rule-based patterns** (`internal/ape/self_reflect.go`), SQL-queryable signals:
  - Memory bloat, orphan ratio, graph health floors
  - Jiminy follow-rate regression (classifier-verifiable, now floor-config-driven DASHBOARD-TRUTH-003)
  - LLM error-rate spike (with recency gate RSIC_LLM_ERROR_RECENCY_MIN)
  - FT benchmark staleness (FT-BENCH-REFRESH-001 added via `benchmark_runs` direct read)
  - Enforcement outcome patterns (ENFORCE-004-FOLLOWUP): `enforcement_false_positive_high`, `enforcement_missed_violation_high`
  - Production drift (`ft_production_drift` → `trigger_training_pipeline` via DRIFT-TRIGGER-001)
- 1 **LLM reflector**: novel-pattern discovery via GPT-5.4-mini on the assessment JSON (deterministic alert emission REMOVED per RSIC-LLM-ALERT-GUARD-001, the LLM reflector can't mint hallucinated deterministic alerts)
- `AllowedLLMActions` whitelist: ~18 actions; new actions require explicit addition

**Provenance**:
- CLAUDE.md "RSIC-LLM-ALERT-GUARD-001" + "ENFORCE-004-FOLLOWUP" + "DRIFT-TRIGGER-001"
- `internal/ape/self_reflect.go`

**note**:
- Operator cue: "When adding a new metric or signal, consider whether RSIC should react to it. If yes, add a pattern in `self_reflect.go`."

---

## U5.5 · Action Taxonomy

**Title**: `What RSIC Does`

**Body**:
- Three dispatch classes:
  1. **Diagnostic alerts**: informational; routed via alert dispatcher (`service=rsic-<action>` per NOSILENT-001); no substrate mutation
  2. **HITL queue items**: generate contradicted drafts (JIMINY-CONTRADICTED-BRIDGE-001) or review candidates for operator grading
  3. **Substrate mutations** (auto-execute), ONLY for safe, reversible, rate-limited actions
     - `archive_constraint_by_code` (ENFORCE-AUTO-EXECUTE), ships default-off + dry-run-true behind 5 guards
     - `trigger_training_pipeline` (FT-RECURSIVE-002), gated by `FT_LOOP_ENABLED` default false
- Reversibility check: `archive_*` boolean is auto-executable; `adjust_*` scalar mutations require HITL (not auto-executable)
- Default auto-execute: OFF for all mutation actions on fresh install

**Provenance**:
- CLAUDE.md "RSIC auto-execute for enforcement patterns (ENFORCE-AUTO-EXECUTE)" (3/3 of JIMINY-ENFORCE arc)
- CLAUDE.md "Recursive-retrain loop Phase 6b (FT-RECURSIVE-002)"

**note**:
- Operator cue: "The system CAN mutate itself. By default, it won't. Flip knobs deliberately when you want autonomy."

---

## U5.6 · /ui/ RSIC Tab

**Title**: `The RSIC Tab`

**Body**:
- Where: `/ui/` → **RSIC**
- Shows: current health dimensions (DH-005 reweighting) with per-dimension confidence (new in DH-005), recent insight stream, admission success/rejection ratio, cycle event timeline
- Interaction: read-only + a Health gauge summary; drill into any insight to see its evidence
- Companion to Grafana `mdemg-rsic` dashboard (U13) which carries the historical view
- Panel titles embed design-intent numbers per DASHBOARD-TRUTH-002 lesson (e.g. "Cycle Admission Ratio (expected 1-10% by design)")

**Provenance**:
- CLAUDE.md "DH-005 Health Formula Reweighting"
- `internal/api/ui/tabs/rsic.js`
- `deploy/docker/grafana/dashboards/mdemg-rsic.json`

**note**:
- Operator cue: "The health gauge is the TL;DR; the insight stream is the DETAIL. Both update every ~5 min."

---

## U5.7 · Current State (Honest Trajectory)

**Title**: `RSIC Today, Honest`

**Body**:
- Current state (`mdemg-dev`, 2026-10):
  - Overall health: ~0.80 (dimension-weighted; DH-005)
  - LLM reflector: on, with deterministic-alert guardrail (RSIC-LLM-ALERT-GUARD-001)
  - Auto-execute: OFF for every mutation action (operator opt-in per action)
  - Jiminy follow-rate drop pattern (JIMINY-FOLLOW-RATE-REMEASURE-001) feeding into JIMINY-CEILING-BREAK-2 arc
- Known trajectory items:
  - Follow-rate reflector pattern 9+15 floors are config-driven (DASHBOARD-TRUTH-003), no silent threshold drift
  - FT benchmark staleness pattern fires HIGH when `benchmark_runs` row > 7d old (currently FIRING, see current pending alerts)
  - Scheduled job staleness patterns (NOSILENT-001) fire on silent backup failures

**Provenance**:
- CLAUDE.md DH-005 + "DASHBOARD-TRUTH-003" + "ALERT-NOISE-REDUCTION-001"

**note**:
- Operator cue: "Pending HIGH alerts you see on session start are RSIC-authored patterns. They're not noise; they're honest signals of named arcs."

---

## U5.8 · Gotchas + Where To Go Deeper

**Title**: `RSIC, Gotchas + More`

**Body**:
- Gotchas:
  - RSIC cycles reset on server restart (in-memory admission state); first-cycle-after-restart may feel aggressive
  - Protected-space check is HARDCODED for `mdemg-dev`, never remove without operator authorization
  - `AllowedLLMActions` doesn't prevent HALLUCINATION, the LLM might emit an unknown action name; dispatcher IGNORES it loudly
  - Deterministic alerts (threshold-gated) MUST stay in `deterministicAlertActions` not `AllowedLLMActions` (RSIC-LLM-ALERT-GUARD-001)
- Where to go deeper:
  - `docs/features/rsic-orchestration.md`
  - `docs/features/rsic-feedback-loop.md`
  - `docs/features/rsic-guidance-health-floors.md`
  - CLAUDE.md "RSIC admission + archival attribution"
  - The Grafana `mdemg-rsic` dashboard (U13)

**note**:
- Operator cue: "If a new RSIC action feels like it should auto-mutate, verify reversibility + guards + rate limit before adding. See ENFORCE-AUTO-EXECUTE invariants."
- Transition: "U6 next, UxTS, the forcing-function family that prevents DRIFT at merge time."
