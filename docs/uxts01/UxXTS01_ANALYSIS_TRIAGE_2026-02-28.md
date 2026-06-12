# UxXTS01 Analysis Triage (2026-02-28)

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Status: Active  
Source analysis: `/Users/reh3376/Downloads/UxXTS_FRAMEWORK_ANALYSIS (1).md`  
Intent: Keep only actionable framework issues, discard non-actionable commentary.

---

## Triage Rules

1. Keep findings that create real implementation risk, governance risk, or agent-assumption risk.
2. Discard findings that are stylistic, duplicate, or snapshot-only without operational impact.
3. Track accepted findings with priority, status, and owner path.

---

## Accepted Findings (Actionable)

| ID | Finding | Priority | Status | Evidence | Planned/Applied Action |
|---|---|---|---|---|---|
| `A-001` | Example report shape drifted from normative report schema | P0 | Resolved | `examples/sample_uxxts_report.json` vs `schemas/uxxts-report.schema.json` | Replaced sample with schema-conformant canonical envelope |
| `A-002` | Assertion dialect mismatch between canonical grammar and UATS schema/runner grammar | P0 | Resolved | `UxXTS01_PORTABLE_AGENT_SPEC.md` Section 4.3; `source/live-frameworks/uats/uats.schema.json` `$defs/BodyAssertion`; runner parser behavior | Added canonical `op` grammar + legacy compatibility policy in portable spec; updated UATS schema to explicit canonical/legacy assertion dialects; upgraded runner to normalize/enforce operators with parity-fail on unsupported assertions; updated conformance suite to canonical syntax with legacy compatibility case |
| `A-003` | `sha256-jcs` declared while current lint canonicalization is not strict RFC8785 JCS | P1 | Resolved | `schemas/uxxts-common.schema.json` + `tools/uxxts_lint.py` canonicalization note | Adopted explicit method contract `sha256-c14n-v1` with `sha256-jcs` as legacy alias; updated schemas/examples/spec text; added lint validation/warning for declared hash method |
| `A-004` | Runner-core duplication creates linear cost for each new framework runner | P2 | Resolved | UATS/UPTS runners duplicate env/hash/parity/report orchestration | Added shared runner-core module `docs/tests/uxts_runner_core.py`; migrated UATS/UPTS runners to shared hash/status primitives; documented extraction contract in `UxXTS01_RUNNER_CORE_CONTRACT.md` |
| `A-005` | Package refresh script is layout-coupled to this repo | P2 | Resolved | `agent-package/BUILD_PACKAGE.sh` absolute layout assumptions | Added portable script mode with `--repo-root` and `--output`, auto-discovery fallback, source validation checks, and external-output packaging support |

---

## Discarded Findings (Non-Actionable for Framework)

| ID | Finding | Reason Discarded |
|---|---|---|
| `D-001` | Static file count/line-count values drift over time | Snapshot-only metadata; does not affect framework correctness |
| `D-002` | Broad commentary on conceptual load without concrete remediation path | Retained as background risk, not tracked as blocking issue |
| `D-003` | Naming variations in narrative examples when canonical filenames are already specified in manifests | Editorial only; monitor but do not prioritize |

---

## Next Implementation Sequence

1. Run full regression and conformance execution against live runners.
2. Promote compatibility warnings into deprecation timelines for legacy assertion and hash aliases.
3. Expand runner-core extraction to shared env-token/parity helpers.

---

## Notes

1. This triage document is authoritative for accepted vs discarded findings from R2 analysis.
2. Updates must be reflected in `DECISION_REGISTER.md` and `WORKING_CONTEXT_LOG.md`.
