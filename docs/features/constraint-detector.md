# Constraint Detector

Hot-path pattern layer for `/v1/conversation/observe`. Regex-based scanner that flags observation content likely to be a durable rule so it can be promoted to a `role_type='constraint'` L1 node by the `CreateConstraintNodes` promoter.

## Where it fires

- Constructor: `internal/conversation/service.go::NewServiceWithConfig` builds it when `CONSTRAINT_DETECTION_ENABLED=true` (default).
- Call site: `Observe()` invokes `constraintDetector.Detect(content, obsType)` for every incoming observation and writes one `constraint:<type>` tag per emitted `DetectedConstraint`.
- Downstream: `internal/hidden/constraint_nodes.go::CreateConstraintNodes` reads those tags during consolidation and mints one L1 `role_type='constraint'` node per tag (subject to the `ConstraintPromotionGate` deny-set + regex backstops from JIMINY-CORPUS-001).

## Pattern classes (severity buckets)

Each bucket has a list of case-insensitive regex patterns with per-pattern base-confidence weights; the observation-type provides a confidence boost on top (`obs_type='constraint'` +0.25, `decision` +0.20, `correction` +0.15). See `initPatterns()` for the current pattern list.

| Severity | Example patterns | Base confidence range |
|---|---|---|
| `must` | `\bmust\b`, `\balways\b`, `\brequired\b`, `\bmandatory\b` | 0.65–0.80 |
| `must_not` | `\bnever\b`, `\bmust not\b`, `\bforbidden\b`, `\bprohibited\b` | 0.55–0.85 |
| `should` | `\bshould\b`, `\bprefer\b`, `\brecommended\b`, `\bbest practice\b` | 0.50–0.65 |
| `should_not` | `\bshould not\b`, `\btry to avoid\b`, `\bdiscouraged\b` | 0.55–0.65 |
| `deadline` | date-like patterns (`\bby\s+\d{4}[-/]\d{1,2}[-/]\d{1,2}\b`, `\bdeadline\b`) | 0.70–0.80 |

Weakest matches below the `CONSTRAINT_MIN_CONFIDENCE` floor (default 0.6) are dropped.

## Single-canonical-emit invariant (JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001, 2026-08-14)

**One L0 observation → ≤1 `DetectedConstraint` when `CONSTRAINT_DETECTOR_DEDUP_ENABLED=true` (default).**

Pre-sprint failure mode: an observation whose content triggered multiple severity buckets (e.g., "You must never commit to main" trips both `must` on `\bmust\b` AND `must_not` on `\bnever\b`) emitted one `DetectedConstraint` per bucket → the promoter minted one L1 node per emitted tag → two `role_type='constraint'` nodes sharing the same `constraint_code` but with different `constraint_type` axes. Fable's `JIMINY-CORPUS-AUDIT-004` found this class live on mdemg-dev in two twin pairs (`z5xgcm…`/`pwa2lm…` + `qi43sv83g136…` + its twin) before the AUDIT-004 tombstone batch cleaned them up.

The dedup gate closes the class at the source. When more than one severity bucket matches on the same observation, the detector collapses to the SINGLE canonical `DetectedConstraint` via a severity precedence, and the winner carries `SkippedSuppressed = N-1` (the count of same-observation sibling severities that were suppressed).

### Severity precedence

`must_not` > `must` > `should_not` > `should` > `deadline`

Rationale (from the AUDIT-004 evidence):
- `must_not` outranks `must` because a **prohibition is strictly more constraining** than an obligation over the same substrate. The `z5xgcm`/`pwa2lm` twin pair proved it: the `must_not` twin was semantically correct; the `must` twin was a spurious noun-phrase match on "must-constraint" prose in the ruling text.
- `should_not` outranks `should` for the same reason.
- `deadline` is temporally-oriented, not severity-oriented; it lands last only as a tiebreak — content that is ONLY a deadline isn't collapsed at all (single-severity match, no precedence needed).

Tiebreak within the same precedence bucket (structurally impossible today, since precedence is total-ordered by type) falls through to higher `Confidence`. Pin-tested (`severityPrecedence` map exported at package level).

### Known trade-off (documented)

The detector CANNOT distinguish "one rule with mixed language" (e.g., "You must always X and you must never Y" that IS one rule) from "two rules in one obs" (same string but genuinely two separate rules). The dedup gate treats both as one canonical emit.

**Operator guidance**: authoring genuinely-multi-rule observations? Submit them as **separate `observe` calls, one per rule.** The alternative (sentence-level splitting inside the detector) is a much larger change and out-of-scope for this sprint. The trade-off is regression-pinned by `TestDetectorDedup_LegitimatelyMultiSeverity_CollapsedWhenEnabled_DocumentsTradeOff`.

## Config

| Env var | Default | Meaning |
|---|---|---|
| `CONSTRAINT_DETECTION_ENABLED` | `true` | Master switch. `false` disables all detection. |
| `CONSTRAINT_MIN_CONFIDENCE` | `0.6` | Confidence floor for a pattern match to be emitted. |
| `CONSTRAINT_DETECTOR_DEDUP_ENABLED` | `true` | Severity-precedence collapse (this sprint). `false` reverts to pre-sprint dual-emit behavior. |
| `CONSTRAINT_PROTECT_FROM_DECAY` | `true` | Constraint-tagged observations survive tombstone sweeps. |

## Telemetry

- `mdemg_constraint_detector_multi_emit_suppressed_total{space_id}` — counter. Increments by `N-1` on each collapse (where `N` is the raw match count across severity buckets). Zero = no multi-severity content flowing on that space; sustained non-zero = the corpus is producing mixed-severity rules that operators may want to split into separate `observe` calls. Sourced from `internal/conversation/service.go::Observe`, keyed on the request's `space_id`.
- `slog.Debug("constraint detector: multi-emit collapsed to canonical", chosen_type, suppressed_types)` — one log line per collapse.

## Related sprints

- **CREATE-CORRECTION-DEDUP-001** (2026-08-12) — sibling defect at the L1 correction-promotion layer; uses vector-similarity dedup because corrections are 1:1 with obs (unlike constraints which are one-per-severity-tag). Both sprints attack the same "one L0 obs → multiple L1 nodes" class from different angles.
- **JIMINY-CORPUS-AUDIT-004** (2026-08-14) — the audit that surfaced this defect. AUDIT-004 already tombstoned the two live twin pairs on mdemg-dev; this sprint's forward-fix prevents new duplicates from forming.
- **JIMINY-CORPUS-001** — the `ConstraintPromotionGate` at `internal/hidden/constraint_gate.go` sits DOWNSTREAM of the detector. It filters out junk-shape content BEFORE promotion; the detector still emits, the promoter drops.

## Architectural rules pinned

1. **One L0 observation MUST mint ≤1 constraint node.** Enforced upstream at the detector (this sprint), not at the promoter. The promoter continues to be a pass-through of the detector's output.
2. **Severity precedence is a total order.** Adding a new severity bucket to `initPatterns` REQUIRES adding a slot in `severityPrecedence`. Missing entry means precedence value 0, which makes the type ineligible to win the collapse — the detector will emit it standalone (falling through to the pre-sprint path) but never win over any of the pinned severities.
3. **Genuinely-multi-rule content should be split into separate observe calls.** The detector's dedup trade-off is documented; operators authoring rules should submit them one-per-observe.

## Rollback

Set `CONSTRAINT_DETECTOR_DEDUP_ENABLED=false` in `.env` + restart. Byte-identical pre-sprint detector behavior returns immediately. No substrate mutation to undo (E4 was a no-op — AUDIT-004 had already tombstoned the live duplicates).

## Sprint

`docs/development/jiminy-corpus-constraint-detector-dedup-001/`
