# Source Provenance Matrix

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Status: Active
Purpose: Trace major UxXTS01 sections to origin materials.

| Target Section | Primary Source | Secondary Source | Rationale | Decision |
|---|---|---|---|---|
| Problem framing + architecture | Bundle A (`v3.2.0` portable spec) | Bundle B (`v3.1` final spec) | v3.2 depth + v3.1 clarity | Merged, concise wording |
| Core normative principles | Bundle A (`v3.2.0` portable spec) | — | Most complete and hardened | Adopted with light editorial tightening |
| Report schema contract | Bundle A (`v3.2.0` report schema) | Bundle B (`v3.1` report schema) | v3.2 schema is stricter and more complete | Adopt v3.2 schema |
| Aggregate schema contract | Bundle A (`v3.2.0` aggregate schema) | — | Only bundle with normative aggregate schema | Adopted |
| Conformance tests | Bundle A (`v3.2.0` conformance suite) | — | Strongest implementation-level compliance artifact | Adopted |
| Practical bootstrap scripts | Bundle B (`v3.1` lint/init tools) | Bundle A tooling guidance | Real executable seed tools in lean bundle | Adopted and aligned with final spec wording |
| Adoption roadmap | Bundle B (`v3.1` roadmap) | Bundle A lifecycle/governance sections | Lean phased execution + robust controls | Merged roadmap |
| Naming grammar | user suggestion + v3.2 naming convention | v3.1 naming examples | Preserve spirit + support 1-2 char domain token | Adopt `U[A-Z0-9]{1,2}TS` |
