# FT-RECURSIVE-000 — Sprint Post

**Status: COMPLETE** · 2026-06-11 · branch `reh3376_dev01` · doc-only

## Deliverable

`docs/development/ft-recursive-001/SPEC_recursive_retraining_loop.md` —
the buildable spec for FT Phases 6/7/9 (FT-RECURSIVE-001..004 execute
from it, gated behind FT-CLASSIFY-002 per Roadmap §4).

## Verification (doc-sprint tier mapping)

- **Tier 1 (citations):** every prompt anchor re-verified on HEAD
  `17c283a` (drift re-pinned: `task_dispatch.go:307→369`); every
  agent-sourced citation in the spec independently opened
  (`data_curate.go:45`, `config.go:2506/3878`, `self_reflect.go:531-548`,
  `task_dispatch.go:1154`, `types_rsic.go:55-62`, `evaluate_ft.py:952`,
  both regression gates, `002_ft_schema.sql:71/92`, plist).
- **Tier 2 (consistency):** spec contradicts neither `00_README_v2.md`
  (STATUS untouched, pointer added), the roadmap §4 deferral
  (FT-CLASSIFY-002 remains the trigger and first vertical slice), nor the
  standing policies (overfitting prevention now a binding gate-parameter
  requirement; honest-eval rule extended with the 16-task-augmented
  mandate; no-hardcoding violations inventoried, not excused).
- **Tier 3 (live):** `mdemg data status` snapshot (98,006 interactions,
  3 tasks Ready=YES → `readyCount=3` re-fires insight #29 every cycle);
  ≥9 no-op actuator alert firings observed in one session (20:52–22:57Z);
  `ape.reflect` 70,880-rows-but-NOT-ready captured as SF-7 (per-gate
  reasons unsurfaced).

## Audit quality notes

Three sub-agents fanned out over stages A–E; the orchestrator re-verified
every load-bearing claim directly (the TSDB-CONSUME-001 lesson). One
agent error caught and corrected: "`evaluate_ft.py` hardcodes :8101" —
false; those are stale docstring examples, `--base-url` defaults to None
(`:952`). One agent couldn't see `002_ft_schema.sql` (looked for
`002_llm_interactions.sql`) — the ft_* tables (`ft_training_cycles:71`,
`ft_model_versions:92`) were located and pinned directly.

## Governance

J17/Jiminy handshake performed; instance reachable; disclosed: the
`jiminy-governance` skill named by the prompt has **no pinned
observations** (recall → 0 results) and scoped guidance returned 0 items.
Executed under standing hook enforcement. Follow-up candidate: register
the skill or remove the reference from operator prompt templates.

## All 7 review amendments incorporated (marked `[AMD-n]` in the spec)

1. Overfitting-prevention as machine-enforced gate parameters.
2. Promotion gate = 16-task augmented (leak-audited) + valid_clean.
3. Resource model corrected: 9h7m / 36 GB RAM / ~85 GB disk.
4. `rerank_collector.go` → `internal/retrieval/`.
5. Anchors re-pinned on current HEAD.
6. DiagnosticActions re-classification added as decision fork 7.
7. Readiness threshold answered: `DefaultReadinessThreshold = 500`,
   global, hardcoded const, zero env override — phase 6b requirement.

## Documents Accessed

See the spec's §7 (the authoritative list); plus the sprint plan §11.
