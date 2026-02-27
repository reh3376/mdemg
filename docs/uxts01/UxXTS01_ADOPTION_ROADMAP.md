# UxXTS01 Adoption Roadmap

Status: Draft
Date: 2026-02-27

## Goal

Deliver real value quickly, then harden progressively without unnecessary process overhead.

## Phase 0 - Baseline (1-3 days)

Deliverables:

1. Pick one framework (recommended `UATS` or `UOBS`).
2. Create schema + initial high-value specs.
3. Ensure runner emits canonical report format.

Exit criteria:

1. Specs validate.
2. One runner can execute specs locally.

## Phase 1 - Managed Reliability (1-2 weeks)

Deliverables:

1. CI job executes lint + runner for critical/smoke tags.
2. Hash integrity reporting enabled.
3. Parity checks for known/unknown fields.

Gate policy:

1. Start with `soft`.
2. Keep hash mismatch as report-only until review discipline stabilizes.

Exit criteria:

1. Machine-readable reports are stable.
2. Drift and parity issues are visible and triaged.

## Phase 2 - Brownfield Expansion (2-6 weeks)

Deliverables:

1. Deterministic discovery artifacts.
2. Scored opportunity backlog.
3. At least one high-priority remediation implemented or explicit waiver.

Exit criteria:

1. Verification coverage expands where risk is highest.
2. Manual/noisy checks are converted to declarative contracts where appropriate.

## Phase 3 - Multi-Framework Governance (ongoing)

Deliverables:

1. Additional frameworks as needed (`UPTS`, `USTS`, `UBTS`, etc.).
2. Aggregate reporting and governance dashboards.
3. Conformance checks for runner changes.

Exit criteria:

1. Cross-framework consistency remains intact.
2. Gate mode can promote toward `block` for stable critical scopes.

## Continuous Practices

1. Keep specs diffable and schema-valid.
2. Keep runner output schema-valid.
3. Keep integrity and assertion outcomes separate.
4. Keep discovery and decision artifacts current in brownfield mode.

