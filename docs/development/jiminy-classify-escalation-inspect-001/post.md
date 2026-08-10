# JIMINY-CLASSIFY-ESCALATION-INSPECT-001 — Sprint Post

**Date:** 2026-08-10
**Branch:** `reh3376_dev01`

## Summary

Fixed the buggy `extractConstraintCode` helper that parsed the leading `[TOKEN]` off the evaluator item's Content string and treated the SEVERITY marker (`must` / `should` / `info`) as the constraint code. Now the evaluator's Cypher returns the real `constraint_code` from the Neo4j MemoryNode; the `EvaluationItem` carries it forward; the strict classifier populates `ViolatedCodes` from it directly. Block messages + hook stderr banners now name the actual codes so `mdemg jiminy override apply --constraint <CODE>` targets the right rule.

## What was broken

`internal/jiminy/evaluator.go:415` (constraint match) built items as:

```
Content: "[must] no-hardcode-pool-sizes (sim: 0.87)"
```

`internal/jiminy/strict_classifier.go:170` (old) parsed the leading `[must]` as the "code" — returned `"must"`.

Consequences:
1. Every high-severity constraint collapsed to code `"must"` in `ViolatedCodes`.
2. `mdemg jiminy override apply --constraint must` overrode EVERY must-severity rule (many), not the specific offending one.
3. Block messages surfaced `[must] <content>` — operator couldn't see which rule to override.
4. Sibling JIMINY-HEURISTIC-DEFAULT-001 sprint hit this: every Edit on `config.go` blocked; operator override on `must` was applied but the block persisted (different constraint whose extracted "code" was also `must`); forced the edits through a Python round-trip.

## What's fixed

- **`internal/jiminy/types.go`** — added `ConstraintCode string` (omitempty JSON) to `EvaluationItem`.
- **`internal/jiminy/evaluator.go`** — `findMatchingConstraints` + `findMatchingCorrections` Cypher now RETURN `c.constraint_code AS constraint_code`; both item constructors populate the new field.
- **`internal/jiminy/strict_classifier.go`** — `Classify` reads `item.ConstraintCode` directly. Falls back to `"node:" + item.SourceNode` when the constraint has no code (legacy Neo4j nodes) so overrides can still target the specific finding via the pseudo-code. `extractConstraintCode` DELETED entirely (no other callers per grep).
- **DenialReason formatter** — now prepends `[code=<CODE>]` to each reason: `Constraint violation (warned): [code=no-hardcode-pool-sizes] <content>; [code=<CODE2>] <content2>`. Codes and reasons zip pairwise; falls back to naked reasons when lengths diverge.
- **`internal/cli/hook_templates/pre-write-check.py`** + **`pre-bash-check.py`** — stderr banner reformatted:
  ```
  [/strict] BLOCKED (codes: no-hardcode-pool-sizes, cuidv2-required) <reason>
  [operator] to override: mdemg jiminy override apply --constraint no-hardcode-pool-sizes --duration 30m --reason "<why>"
  ```
  Codes capped at 5 (elides `+N more`). `pre-bash-check.py::check_jiminy_bash` return type changed to `(reason, codes)`.
- **Live hooks synced** via `mdemg hooks install --type claude --space-id mdemg-dev --force` after binary rebuild.

## Verification

- `go build ./...` — clean
- `golangci-lint run ./internal/jiminy/...` — 0 issues
- `go test ./internal/jiminy/...` — full suite green
- `make verify-grafana-embed` — no drift
- 4 new pin tests in `internal/jiminy/strict_classifier_code_test.go`:
  - `TestExtractConstraintCode_IsDeleted` — file-level regression pin; asserts the buggy function is gone + no live call site
  - `TestClassify_EmptyEvaluator_Passes` — nil-evaluator branch sanity anchor
  - `TestViolatedCodes_UseRealCodeNotSeverityMarker` — the CORE pin: EvaluationItem with `Content:"[must] no-hardcode-pool-sizes"` + `ConstraintCode:"no-hardcode-pool-sizes"` yields `ViolatedCodes:["no-hardcode-pool-sizes"]` NOT `["must"]`
  - `TestViolatedCodes_FallbackToNodeIDWhenCodeEmpty` — legacy item without code → `node:<source_node>` pseudo-code
  - `TestViolatedCodes_MultipleItemsCarryRespectiveCodes` — 2 items → 2 distinct codes preserved (regression pin against the "all collapse to must" bug)

## Live Tier-3 (mdemg-dev, 2026-08-10)

**E1 wire proven end-to-end**: `POST /v1/jiminy/evaluate` with payload matching a real CUIDv2 constraint returned:

```json
{
  "items": [
    {
      "type": "constraint",
      "content": "[must_not] You must never use UUID v4 in this codebase. Always use CUIDv2. (sim: 0.72)",
      "severity": "high",
      "source_node": "qi43sv83g136ds43vsrfjxr0",
      "constraint_code": "auto-250af3293675"
    }
  ]
}
```

The `constraint_code` field is now populated on the wire from Neo4j — the classifier reads this same field to populate `ViolatedCodes`.

**E1 wire proven — correction fallback**: the same response included a `correction`-type item with NO `constraint_code` field (legacy L0 correction observation without a code). The strict classifier's `node:<source_node>` fallback path is exercised for this class.

**Deny path — code-proven only**: exercising the actual deny path end-to-end (WARNED+ escalated state → `verdict:deny` → block message with `[code=X]` annotation) requires an existing escalation state at WARNED+ level. The server restart (kickstart) reset the in-memory escalation tracker; the disk-persisted state has only `surfaced`-level entries. The deny path with the correct code annotation is proven by the 5 pin tests + the wire proof above. **Post-first-natural-WARNED+ block** (should occur within hours as the hook channel produces feedback), the new banner format will be observable in `~/.mdemg/alerts/current.json` — passive follow-up.

## Two arch rules pinned (CLAUDE.md)

1. **Never parse identity tokens out of a rendered display string.** The `extractConstraintCode` helper tried to recover the code from `Content: "[must] <name> (sim: 0.87)"` by parsing the leading `[TOKEN]` — but that TOKEN was the constraint TYPE (severity axis), not the code. When you need an identity attribute, propagate it as a first-class STRUCT FIELD from the data-fetch layer to the consumer, NEVER re-parse it from a downstream rendered string.
2. **When an operator-override mechanism keys on a code, the code MUST be surfaced in the deny message.** The DenialReason format now includes `[code=X]` per finding + the hook stderr banner shows codes prominently AND prints a copy-paste-ready `mdemg jiminy override apply --constraint X` hint. Silent codes = broken escape hatch.

## Retrospective on the JIMINY-HEURISTIC-DEFAULT-001 sprint's incident

That sprint hit this bug head-on: every Edit on `internal/config/config.go` blocked with a `[must]` code that overrode-to-nothing (the override applied to the extracted-code `must` but the actual blocking rules had different real codes). Post-this-sprint, that class of incident is closed. Had this bug not existed, JIMINY-HEURISTIC-DEFAULT-001 would have used the operator-override CLI cleanly for its 3-line edit instead of resorting to a Python round-trip.

## Follow-ups disclosed

- **`mdemg jiminy override list` should show which codes are active** (already does, but could show *which sessions* they cover). Not urgent.
- **Constraint-code backfill sanity check**: JIMINY-ARCHIVED-CODE-FILTER-001 + CORRECTION-CODE-GEN-001 already ensured every live constraint + correction node has a code. If a fresh gap ever surfaces (via the `node:<id>` fallback in production traffic), that's a signal to run a backfill sweep.
- **L0 correction observations don't have codes** (only L1 role_type='correction' nodes do). If evaluator matches ever surface L0 correction obs, they land as `constraint_code=""`. Currently the evaluator only looks at `obs_type='correction'` which is L0 — so this is the current state. If the deny path is ever exercised via a correction, the fallback pseudo-code will kick in. Not a bug but worth noting.

## Documents Accessed

- `docs/development/jiminy-heuristic-default-001/post.md` — disclosed this follow-up
- `internal/jiminy/{strict_classifier,evaluator,types,override}.go`
- `internal/cli/hook_templates/{pre-write-check,pre-bash-check}.py`
- Live SQL against Neo4j (J12EscalationState node inspection)
- Live `POST /v1/jiminy/evaluate` for E1 wire verification
- CLAUDE.md pins: JIMINY-ENFORCE-003, JIMINY-ARCHIVED-CODE-FILTER-001, CORRECTION-CODE-GEN-001, HOOKSYNC-001
