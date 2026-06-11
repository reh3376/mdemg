# Sprint Plan — JIMINY-OUTCOME-002: Tier-2 `not_applicable` + Verdict Provenance

## 1. Header & Metadata
Sprint: JIMINY-OUTCOME-002 · 2026-06-11 · branch `reh3376_dev01` ·
operator-approved insert before MCP-REVIVE-001, sourced from the live
effectiveness-decline investigation (2026-06-11) · effort ~1–1.5d ·
risk low (one prompt surface + one additive migration).

## 2. Problem Statement
The post-SUPERVISOR-002 effectiveness "decline" (0.45–0.50 → 0.07–0.20
from 19:00Z) is a measurement correction — the revived Tier-2 LLM
classifier replaced the dead-call heuristic that half-credited everything
as `partial_compliance`. But the corrected number is biased LOW: the
Tier-2 enum (`outcome_classifier.go` prompts + `ollamaClassifySchema`)
offers only followed/partial_compliance/ignored/contradicted, so guidance
that simply doesn't apply to the action (hooks deliver up to 10 items per
action; most can't apply) is scored `ignored`. The type system, the
parser (`mapOutcomeString`), AND the persistence path already handle
`not_applicable` (service.go skips it at all four sinks) — only the LLM
was never offered the option. Secondary: heuristic-fallback verdicts are
indistinguishable from LLM verdicts in `constraint_outcomes` (today only
inferable from the confidence value's shape), so the artifact class can't
be filtered historically.

## 3. Scope & Constraints
**In**: (1) add `not_applicable` to `classifySystemPrompt`,
`classifySystemPromptCompact`, and the `ollamaClassifySchema` enum — and
re-pin `system_prompt_hash` in `jiminy_evaluate_llm.ults.json` in the
same PR (merge-blocking ULTS gate). (2) Verdict provenance:
`ClassificationResult.Source` (`tier1|llm|heuristic|explicit`) set at
every decision point; V0026 adds `classifier_source` to
`constraint_outcomes` (additive, default ''); writer + RecordOutcome
extended; TSDB schema 25→26. (3) Re-baseline annotation: Grafana
effectiveness panel descriptions + feature-doc + CHANGELOG note that
pre-2026-06-11T19:00Z history is heuristic-dominated. **Out**: stats.go
denominator changes (not needed — not_applicable never reaches the sinks);
backfilling/reclassifying historical rows (forward-only, the EVENTGRAPH
precedent); the independent `jiminy.synthesize` error-rate and NLI-bias
follow-ups (recorded, not actioned).

## 4. Dependencies
`internal/jiminy/{outcome_classifier,service,types}.go`;
`internal/tsdb/{constraint_outcomes_writer.go, migrations/}`;
`docs/tests/ults/specs/jiminy_evaluate_llm.ults.json` (+ runner re-pin);
Grafana `mdemg-jiminy.json`; config.go schema-version bump; live stack
for Tier 3.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** enum/prompt/schema + ULTS hash re-pin ·
**Epic 2** provenance (Source field, V0026, writer) · **Epic 3**
re-baseline annotations + docs · **Epic 4** tests + live Tier 3 + push.

## 6. Testing Plan
Tier 1: classifier tests (prompts/schema contain not_applicable;
mapOutcomeString round-trip; Source set per decision path); writer test.
Tier 2: full `go test ./internal/...`; V0026 applies idempotently on the
live TSDB; ULTS `--verify-hashes` green locally. Tier 3 (live):
(a) `/v1/jiminy/feedback` with a deliberately-unrelated guidance/action
pair → Tier-2 verdict `not_applicable`, NO `constraint_outcomes` row, NO
`GUIDANCE_OUTCOME` edge; (b) a related pair → row lands WITH
`classifier_source` populated; (c) effectiveness queries observed
post-fix.

## 7. Commit Strategy
Epic-grouped commits · lint before each · push once (auto-PR) · summary
comment · CI watch.

## 8. Verification Checklist
- [ ] not_applicable in both prompts + ollama schema; parser already maps it
- [ ] ULTS jiminy_evaluate_llm hash re-pinned same PR; verify-hashes green
- [ ] ClassificationResult.Source set at tier1/llm/heuristic/explicit paths
- [ ] V0026 applied live + idempotent; schema 25→26 everywhere CI checks
- [ ] constraint_outcomes rows carry classifier_source (live-verified)
- [ ] live: unrelated pair → not_applicable, zero rows/edges
- [ ] Grafana panel descriptions + feature doc + CHANGELOG re-baseline note
- [ ] follow-ups recorded (synthesize error rate, NLI bias)

## 9. Documentation Update — Epic 3/4 (feature-doc note, CHANGELOG, post).

## 10. Risks & Mitigations
LLM over-uses not_applicable (effectiveness inflates) → the option is
constrained in the prompt ("topic unrelated to the action taken", with
the ignored-vs-not_applicable distinction spelled out) and Tier 3 checks
a related pair still classifies; the provenance column makes any future
drift analyzable. Prompt edit destabilizes classification quality → ULTS
hash discipline + the existing grammar-constrained schema bound the
output space. Migration risk → additive column with default, idempotent.

## 11. Documents Accessed
Investigation report (2026-06-11); internal/jiminy/{outcome_classifier,
service,types,stats}.go (read); service.go:1480-1545 sink-skip
verification; mdemg-jiminy.json panels 216/591/822; ULTS spec dir.

## 12. Rollback Procedures
Code: revert commits. V0026: additive column — `ALTER TABLE ... DROP
COLUMN classifier_source` + schema_meta reset documented in the migration
header (standard convention).
