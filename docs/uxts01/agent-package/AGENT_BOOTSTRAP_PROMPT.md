# Agent Bootstrap Prompt

Use this prompt with any coding agent:

```text
You are implementing UxXTS governance from this package.

End goal:
Implement intent-driven, deterministic creation of governed framework items with required verification and deterministic CI gate outcomes.

Read in this order:
1) START_HERE.md
2) source/original/UXTS_PORTABLE_AGENT_SPEC.md
3) source/original/UXTS_PORTABLE_AGENT_SPEC02.md
4) source/uxts01/UxXTS01_PORTABLE_AGENT_SPEC.md
5) source/uxts01/UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md
6) source/live-frameworks/uats/README.md
7) source/live-frameworks/upts/README.md
8) IMPLEMENTATION_GUIDE.md

Non-negotiable constraints:
1) Preserve core UxXTS philosophy and boundaries.
2) Enforce schema-runner parity; unimplemented required behavior must hard-fail.
3) Keep hash integrity and assertion outcomes independent.
4) Brownfield mode is extension-first.

Implementation target for first increment:
1) UATS intent-to-item generation.
2) Stage execution T0-T4.
3) Manifest/report emission under .uxxts/generated/<framework>/<item>/.
4) Deterministic CI gate decision (observe/soft/block).

Deliverables:
1) code changes
2) tests (unit + integration where required)
3) sample intent packet
4) sample manifest/report
5) CI gate output evidence
6) concise residual risk list
```
