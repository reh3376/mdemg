# UxXTS01 Supporting Files Index

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Main specification:

1. `UxXTS01_PORTABLE_AGENT_SPEC.md`
2. `UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md`

Governance and synthesis context:

1. `UxXTS01_MERGE_DECISIONS.md`
2. `SOURCE_PROVENANCE_MATRIX.md`
3. `DECISION_REGISTER.md`
4. `WORKING_CONTEXT_LOG.md`
5. `UxXTS01_ANALYSIS_TRIAGE_2026-02-28.md`
6. `UxXTS01_RUNNER_CORE_CONTRACT.md`

Rollout planning:

1. `UxXTS01_ADOPTION_ROADMAP.md`

Agent distribution package:

1. `agent-package/README.md`
2. `agent-package/START_HERE.md`
3. `agent-package/IMPLEMENTATION_GUIDE.md`
4. `agent-package/AGENT_BOOTSTRAP_PROMPT.md`
5. `agent-package/PACKAGE_MANIFEST.md`

Normative schemas:

1. `schemas/uxxts-common.schema.json`
2. `schemas/uxxts-report.schema.json`
3. `schemas/uxxts-report-aggregate.schema.json`

Conformance artifacts:

1. `conformance/conformance-suite.json`

Utilities:

1. `tools/uxxts_lint.py`
2. `tools/uxxts_init.py`

Reference examples:

1. `examples/frameworks/uats/schema.json`
2. `examples/frameworks/uats/_defaults.json`
3. `examples/frameworks/uats/specs/health.uats.json`
4. `examples/frameworks/uats/fixtures/health/expected.json`
5. `examples/sample_uxxts_report.json`

Usage order:

1. Read `UxXTS01_PORTABLE_AGENT_SPEC.md`.
2. Read `UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md` for intent-to-item automation contract.
3. Run examples/tools for bootstrap.
4. Apply roadmap phases.
5. Use conformance and schemas as enforcement contracts.
