# Source Provenance Matrix

Status: Active
Purpose: Trace major UxTS01 sections to origin materials.

| Target Section | Primary Source | Secondary Source | Rationale | Decision |
|---|---|---|---|---|
| Problem framing + architecture | uxts-v3.2.0/UXTS_PORTABLE_AGENT_SPEC.md | FINAL_SPEC.../SPEC_UxTS_v3_1_FINAL.md | v3.2 depth + v3.1 clarity | Merged, concise wording |
| Core normative principles | uxts-v3.2.0/UXTS_PORTABLE_AGENT_SPEC.md | — | Most complete and hardened | Adopted with light editorial tightening |
| Report schema contract | uxts-v3.2.0/schemas/uxts-report.schema.json | FINAL_SPEC.../schemas/uxts-report.schema.json | v3.2 schema is stricter and more complete | Adopt v3.2 schema |
| Aggregate schema contract | uxts-v3.2.0/schemas/uxts-report-aggregate.schema.json | — | Only bundle with normative aggregate schema | Adopted |
| Conformance tests | uxts-v3.2.0/conformance/conformance-suite.json | — | Strongest implementation-level compliance artifact | Adopted |
| Practical bootstrap scripts | FINAL_SPEC.../tools/uxts_lint.py, uxts_init.py | uxts-v3.2.0 tooling section | Real executable seed tools in lean bundle | Adopted and aligned with final spec wording |
| Adoption roadmap | FINAL_SPEC.../ROADMAP.md | uxts-v3.2.0 lifecycle/governance sections | Lean phased execution + robust controls | Merged roadmap |
| Naming grammar | user suggestion + v3.2 naming convention | v3.1 naming examples | Preserve spirit + support 1-2 char domain token | Adopt `U[A-Z0-9]{1,2}TS` |
