# LEVER-C-TIGHTEN-002 — Sprint Plan + Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Phase 2 of:** `docs/development/jiminy-ceiling-break-2/`
**Prior:** JIMINY-CORPUS-003 (Phase 1) — 64 → 33 canonical constraints; expected +2-5pp; passive re-check 2026-08-18

## Problem statement

Data (mdemg-dev, 7d ending 2026-08-12):
- Top-25 canonical constraints have follow rates in **10-20% band across the board**
- Similarity distribution: followed p50=0.900, ignored p50=0.800 — **similarity is NOT a discriminator**; raising sim_floor won't help
- Example: `never-direct-main-commits` correction has 75 events / 62 ignored (14.7%); constraint variant has 57 events / 48 ignored (15.8%)
- Root cause per JIMINY-CEILING-BREAK-2 diagnosis: constraints get surfaced on retrieval queries that AREN'T the mechanism they govern (`never-direct-main-commits` fires on docs edits mentioning "commit" — not actual git operations)

Knob-tuning is exhausted (LEVER-C-TIGHTEN-001 already tightened TOPK 5→4 + sim_floor 0.30→0.55). The remaining leverage is **action-context matching** — a git-scoped rule shouldn't surface when the agent is editing a doc.

## Shipped

**Path A — knob tightening (mechanical, small delta):**
- `.env`: `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK` **4 → 3**
- `.env`: `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR` **0.55 → 0.60**
- Code defaults unchanged (in-code stay at 4 / 0.45 — conservative for new operators)

**Path B — scope-gate filter (structural, real delta):**
- New `internal/jiminy/scope_gate.go` — heuristic action-verb match. Derives 9 scope families (git, file_mutation, bash, schema, identifier, testing, process_docs, llm_config, cms) from characteristic tokens in each item's Content; derives same families from request Context+AgentOutput+FilePath+Query; suppresses items whose family set doesn't intersect the request's.
- Safe defaults: items with no derived scope surface anywhere; requests with no derived scope receive all items. Bounds the failure mode to "suppress items with specific scope on requests with unrelated scope" — exactly the class causing the ~14% ceiling.
- New `applyScopeGate(items, req, enabled)` — filter helper returning surviving items + drop count.
- Wired at `service.go:1231` between Lever C merge and content normalization; drops recorded in `debug.scope_gate_drops` for observability.
- New config `JIMINY_SCOPE_GATE_ENABLED` — default false in code, `true` in `.env`.
- 9 pin tests covering: git-scoped item suppressed on doc-edit request; git-scoped item surfaced on commit request; no-scope item always surfaces; empty request surfaces all items; identifier constraint on bash-restart request suppressed; identifier constraint on codegen request surfaces; disabled gate is no-op; multi-item drop-count matches expectation; family classification for representative canonical corpus.

## Live Tier-3 (mdemg-dev, 2026-08-12 08:25 UTC post-restart)

**Verified boot log:** `jiminy: lever c actionable bias enabled=true topk=3 sim_floor=0.6` — new knobs loaded from `.env`.

**Doc-edit query smoke** (`/v1/jiminy/guide` with context "editing sprint plan documentation" + file_path "docs/development/plan.md"):
- `guidance_items: 10`
- `debug.leverc_actionable_merged: 2` (was expected 3 → 2 with TOPK=3)
- **`debug.scope_gate_drops: 3`** ← the scope gate actively suppressed 3 out-of-scope items
- Surfaced items are ALL doc/sprint-related: `project-planning-docs-in-repo-only`, `mandatory-feature-docs`, `must-follow-12-section-format`, `unit-integration-e2e-docs`, plus concept clusters about sprint planning
- Zero git-scoped, zero identifier-scoped, zero schema-scoped items surfaced (they'd have appeared pre-fix on this query per the ceiling data)

Live confirms the mechanism works on production traffic shape.

## Expected delta

Per the JIMINY-CEILING-BREAK-2 arc plan: **+5-10pp actionable follow rate** over the 7d window. Passive re-check: **2026-08-19** (window rollover point for signals under the new gate).

Composite Phase 1+2 expected: 12% → 17-25% by 2026-08-19.

## Two arch rules pinned (CLAUDE.md)

1. **When similarity is not a discriminator between follow and ignore, knob-tuning the similarity floor cannot improve the ceiling — it can only reduce volume.** The discriminator that DOES matter is action-context match: does the constraint's governed mechanism apply to what the agent is doing right now? If yes, surface. If no, suppress regardless of similarity. Direct-fetch retrieval strategies (Lever C shape) MUST apply a scope-gate filter — pure embedding similarity picks up rules on tangential topic queries, and those tangential surfaces are the primary training signal for the agent to ignore constraints as a class.

2. **Scope-gating heuristics MUST use safe-defaults on both sides.** Items with no derived scope surface anywhere (a rule with no discernible mechanism-verb is probably a preference or universal directive; suppress-by-default here would silence legitimate guidance). Requests with no derived scope receive all items (a caller that didn't pass tool-context hints must not be worse off than pre-gate; suppress-by-default here would wholesale silence guidance). Both sides safe-default → failure mode bounded to "specific scope × unrelated scope" — exactly the class we're trying to fix.

## Follow-ups disclosed

- **Scope-family expansion**: the current 9 families are a starting set. Real production traffic may reveal a common miss class (e.g. `network_config`, `crypto`, `env_var`). Extend `scopeVerbFamily` in `scope_gate.go` when a class of legitimate-but-suppressed guidance shows up in the passive re-check.
- **Structured `applies_to` metadata (deferred)**: eventually the heuristic verb-match should be superseded by explicit per-constraint scope tags. Backfill effort ~30 min for the 33 canonical constraints, then reads at scope-gate time replace the derivation logic. Ship if the heuristic proves brittle across families.
- **Scope-gate telemetry**: `debug.scope_gate_drops` is per-request; consider aggregating over 7d to see the drop-rate distribution + which constraints get suppressed most often. Informs family expansion.

## Documents Accessed

- `docs/development/jiminy-ceiling-break-2/README.md` — Phase 2 spec
- `docs/development/jiminy-corpus-003/post.md` — Phase 1 precedent + arc position
- `docs/development/lever-c-tighten-001/` — knob-tuning precedent + audit
- Live SQL over `constraint_outcomes` (7d similarity distribution + top-25 follow rates)
- `internal/jiminy/service.go::Guide()` (insertion point at line 1231)
- `internal/config/config.go` (JiminyGuidanceConstraint* + JiminyScopeGateEnabled)
- Live `/v1/jiminy/guide` smoke on mdemg-dev post-restart
- CLAUDE.md pins: LEVER-C-TIGHTEN-001, JIMINY-CORPUS-003, JIMINY-CEILING-BREAK-2
