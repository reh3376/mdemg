# UxTS01 Merge Decisions

Status: Draft
Date: 2026-02-27

## Objective

Produce a best-in-class consolidated UxTS specification by selectively combining:

1. `/Users/reh3376/Downloads/uxts-v3.2.0`
2. `/Users/reh3376/Downloads/UXTS_FINAL_SPEC_AND_ROADMAP_BUNDLE`

## Selection Principles

1. Preserve original UxTS intent: declarative, schema-governed, runner-portable verification.
2. Prefer normative precision over broad prose when behavior could diverge.
3. Keep adoption path practical (low initial overhead, progressive hardening).
4. Keep machine-readability first-class (schemas, report contracts, conformance).

## Section-by-Section Merge Outcomes

| Topic | Chosen Baseline | Kept From Other Source | Decision |
|---|---|---|---|
| Core architecture and problem framing | v3.2.0 | v3.1 lean wording style | Keep v3.2.0 scope, compress prose where possible |
| Hash integrity model | v3.2.0 | v3.1 pragmatic framing | Keep v3.2.0 canonical procedure + independence from assertions |
| Assertion grammar | v3.2.0 | — | Keep expanded operator set including `schema_match`, `items_all_match` |
| Defaults merge semantics | v3.2.0 | v3.1 deterministic merge implementation | Keep RFC7396 array-replace semantics |
| Variant specs | v3.2.0 | — | Keep formalized variant behavior |
| Setup/teardown hooks | v3.2.0 | — | Keep bounded lifecycle hooks with parity fail behavior |
| Human-readable intent | v3.2.0 | — | Keep advisory `metadata.intent` |
| Governance tiers | v3.2.0 | v3.1 lean rollout language | Keep lite/full tiers and emphasize progressive adoption |
| Canonical report schemas | v3.2.0 | v3.1 simpler examples | Keep normative schemas from v3.2.0, retain concise examples |
| Conformance suite | v3.2.0 | — | Keep conformance test cases as required compliance artifact |
| Tooling starter scripts | v3.1 bundle | v3.2.0 CLI/MCP vision | Keep practical scripts and align naming/contracts with v3.2 norms |
| Adoption roadmap | v3.1 bundle | v3.2 requirements | Keep lean phased roadmap, add hardening gates |
| Naming grammar | New synthesis | v3.2 `U<X>TS` + user proposal | Adopt `U<XX>TS` grammar: `U[A-Z0-9]{1,2}TS` with one-char preferred |

## Explicit Non-Adoptions

1. No attempt to replace OpenAPI/Pact/pytest; only complementarity guidance.
2. No mandatory full-governance artifact burden for small projects.
3. No imperative workflow engine beyond bounded hooks and `depends_on` ordering.

## Open Questions Resolved

1. Naming wildcard width: resolved to `U<XX>TS` with compatibility for legacy `U<X>TS` names.
2. Report contract source: resolved to normative JSON schemas in `schemas/`.
3. Integrity policy semantics: resolved to runner always-report + CI gate decides block/soft/observe.
