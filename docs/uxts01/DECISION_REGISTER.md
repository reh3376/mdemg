# UxXTS01 Decision Register

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Status: Active

| ID | Topic | Options | Selected | Rationale | Timestamp |
|---|---|---|---|---|---|
| DR-001 | Naming grammar (`U<X>TS` vs `U<XX>TS`) | Keep single-char wildcard / allow 1-2 alnum | `U<XX>TS` where `XX` = 1..2 alphanumeric chars (`U[A-Z0-9]{1,2}TS`) | Preserves original spirit while enabling finer domain specificity; backward compatible with legacy names | 2026-02-27T12:22:00Z |
| DR-002 | Baseline source selection | Lean v3.1 / Full v3.2 | Full v3.2 normative baseline + lean v3.1 usability layer | Maximizes rigor without sacrificing adoption practicality | 2026-02-27T12:22:00Z |
| DR-003 | Governance burden at start | Full-only / Progressive tiers | Progressive tiers (Core -> Managed -> Full) | Reduces adoption friction while preserving hardening path | 2026-02-27T12:22:00Z |
| DR-004 | Intent-to-item automation contract | Keep framework-only scaffolding / add explicit item pipeline contract | Additive companion spec `UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md` | Enables deterministic generation and gating of new items without changing core UxXTS philosophy | 2026-02-28T01:20:18Z |
| DR-005 | Packaging model for external/new agents | Single-file monolith / multi-file directory package | Multi-file directory package at `docs/uxts01/agent-package/` | Preserves executable tools, schemas, and provenance while reducing ambiguity for implementation workflows | 2026-02-28T01:20:18Z |
| DR-006 | External analysis handling policy | Keep all comments / triage actionable issues only | Adopt triage document `UxXTS01_ANALYSIS_TRIAGE_2026-02-28.md` with accepted/discarded findings | Focuses roadmap on real implementation risk and reduces noise from snapshot-only commentary | 2026-02-28T14:02:37Z |
| DR-007 | A-002 to A-005 remediation strategy | Defer changes / implement compatibility-first hardening now | Implement compatibility-first remediation across assertion dialect, hash method naming, runner-core extraction, and package portability | Closes actionable framework risks without breaking existing specs or delivery workflows | 2026-02-28T16:55:00Z |
