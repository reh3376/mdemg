# JIMINY-CORPUS-AUDIT-004 — Sprint Post

**Date:** 2026-08-14
**Branch:** `reh3376_dev01`
**Trigger:** Operator, after using the just-shipped `/ui/rules` tab to discover 2 same-content dual-severity twins in the Jiminy corpus, directed a full deep-dive audit of the entire live rule corpus for duplicates, unnecessary rules, and needs-modification rules.

## Summary

37 live rules → 31 live rules via 6 tombstones + 1 metadata fix + 7 content rewrites (2 of them include a code rename from opaque `auto-*` mnemonics to human-readable codes). All mutations reversible. All performed via operator-authorized direct-Cypher (Option B for tombstones + metadata) OR the shipped `/v1/jiminy/rules` Save flow (Option 2 for content rewrites — preserves round-1 immutable-tombstone lock).

The audit itself was executed by a Fable 5 sub-agent in 2.7 minutes wall clock (177K tokens), read-only against the real corpus; the report was operator-adjudicated per verdict and applied in this session.

## Trigger + first dispositions (pre-audit)

Same-session UI-review discovery: expanding rule `z5xgcm…` also expanded near-dup `pwa2lm…`. Both had identical content (my earlier session's `never-classify-policy-docs-as-constraint` taxonomy ruling recorded as an L0 observation) but opposite severity — `must_not` vs `must`. Root cause: constraint_detector regex tagger picked BOTH "MUST NOT" and "must" from the ruling text (the second "must" was the noun-phrase "must-constraint"). `CreateConstraintNodes` promoted BOTH matches as separate L1 nodes with the same code.

Operator dispositions:
- Tombstone `pwa2lm…` (`must` twin, semantically wrong)
- Mark `z5xgcm…` (`must_not` twin, semantically correct) `is_informational=true`

Both applied via direct Cypher, then the audit was spun up to check for the same class elsewhere.

## Audit (Fable read-only investigation)

Agent brief: adjudicate all 37 live rules with one of 7 verdicts (KEEP_ACTIONABLE / KEEP_INFORMATIONAL / TOMBSTONE_DUPLICATE / TOMBSTONE_JUNK / TOMBSTONE_OBSOLETE / NEEDS_MODIFICATION / REVIEW_UNCERTAIN). Do NOT mutate anything; report to operator.

Verdict distribution:
- 15 KEEP_ACTIONABLE
- 6 KEEP_INFORMATIONAL (all correctly marked)
- 4 TOMBSTONE_DUPLICATE (incl. 2nd instance of the dual-severity dual-mint class + 3 semantic merges)
- 1 TOMBSTONE_OBSOLETE (`rebase-dev-after-admin-merge` — superseded by shipped `sync-dev-after-merge.yml`)
- 1 TOMBSTONE_JUNK (`never-trust-unordered-samples` — contextless session artifact)
- 8 NEEDS_MODIFICATION (with specific per-item rewrites suggested)
- 2 REVIEW_UNCERTAIN

## Dispositions applied (Option B + 7 rewrites)

### Option B (safe subset): 6 tombstones + 1 metadata fix

Applied via direct Cypher (arc-safe substrate mutation with operator authorization). Uniform archive_reason prefix `jiminy_corpus_audit_004_operator_option_B_2026-08-14__<class>`.

Full list + class + rollback Cypher in `batch_record.md`.

### 7 content rewrites (Option 2: tombstone-and-recreate)

Executed via the shipped `POST /v1/jiminy/rules/{code}/tombstone` + `POST /v1/jiminy/rules?override_dedup=true` flow — matches the round-1 immutable-tombstone lock and validates the just-shipped JIMINY-RULES-UI-001 Save semantic in production.

The `JIMINY_RULES_UI_WRITE_ENABLED` flag was temporarily flipped ON via `launchctl setenv` + kickstart, and reverted at batch end (probe-verified: post-revert POST returns 503 with the JIMINY-CEILING-BREAK-2 arc-window language).

Per-item table + rationale in `batch_record.md §Follow-up: 7 content rewrites`.

## Systemic patterns Fable surfaced (5, all worth acting on)

1. **Dual-severity dual-mint class confirmed as recurring** — 2 instances found today (pre-audit `pwa2lm/z5xgcm` twins + Option B target `qi43sv83g136 auto-250af3293675` twin). Follow-up: `JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001` (pending file).
2. **All dup clusters + all `auto-*` codes trace to the 2026-04-04 bootstrap batch** — batch-minted rules deserve priority re-audit
3. **`constraint_type` label unreliable** — found ≥2 `must_not` labels on MUST content; interacts badly with `nonViolationCreditClause` (must_not-only credit) → classifier under-credits real MUST rules. Metadata fix on `must-follow-12-section-format` addresses one instance
4. Session narratives embedded in durable rules dilute mechanism-verbs → weaken scope/mechanism gates. 3 of the 7 content rewrites (#3 evidence narrative, #6 stale sprint reference, #7 J17 case study) trimmed this class
5. Highest-volume-ignored rules drive ignored counts via **over-surfacing not falsity** → scope-gating + phrasing fixes beat tombstoning. Rewrite #1 (`query-mdemg-cms-file-paths` — worst correction at 53 ignored/7d) tests this thesis; passive re-measurement over next 7d

## Live cross-refs

The 2 code renames (`auto-01288edd49b1 → live-testing-tier-required`, `auto-c0a62b1da979 → sequential-epics`) triggered a grep-check that surfaced 4 doc references across the repo:
- 2 in the live in-progress `docs/development/jiminy-rules-ui-001/sprint_plan.md` — updated in-place to the new codes
- 2 in historical audit records (`jiminy-corpus-003/tombstone_list.md`, `jiminy-roletype-adapter-001/live_verification.md`) + this sprint's own `pre_batch_snapshot.json` — left verbatim per the framing-hygiene rule (historical records reference codes as-they-were)

## Follow-ups disclosed

- **`JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001`** — fix `constraint_detector.go` so one L0 observation mints ONE canonical L1 constraint node, not one per detected pattern-variant. Sibling of CREATE-CORRECTION-DEDUP-001 which operates at vector-similarity; this fix operates at the pattern-detection layer. Deferred post-arc.
- **Retrain-corpus consumer feed** — the 7 content-rewrite pairs (old + new node with same code) are a natural DPO/preference-tuning signal source ("operator prefers new phrasing over old for this rule"). Ship if the retrain harness gets a "corpus-improvement pairs" channel.
- **Passive re-measurement of `query-mdemg-cms-file-paths` follow rate** — rewrite #1's rephrase-not-tombstone thesis predicts a large ignored-count drop over 7d. Track via `constraint_outcomes` in the JIMINY-CEILING-BREAK-2 T+168h re-check (2026-08-19).
- **2 REVIEW_UNCERTAIN rules kept informational per operator ruling** — `uctrlnyxd9af must-comment-sprint-summary-on-pr` and `lq3omb4tx7wm mandatory-feature-docs`. Both have clear mechanisms (`gh pr comment`, create `docs/features/*.md`). Ship-to-actionable is a future toggle if the operator wants stricter enforcement.

## Documents Accessed

- `docs/development/jiminy-corpus-001/` + `jiminy-corpus-002/` + `jiminy-corpus-003/` — the tombstone-safety pattern this audit mirrors
- `docs/development/jiminy-correction-corpus-001/` — the audit shape (constraint + correction parity)
- `docs/development/create-correction-dedup-001/` — the vector-similarity dedup that pairs with the pattern-layer fix disclosed here
- `docs/development/jiminy-rules-ui-001/sprint_plan.md` + `sprint_post.md` + `../features/jiminy-rules-ui.md` — the shipped surface that made this audit executable via the UI Save flow
- Fable audit report (177K tokens, 2.7 min wall clock) — read-only agent
- Live: 6 tombstones + 1 metadata fix via direct Cypher; 7 content rewrites via `POST /v1/jiminy/rules` Save flow with WRITE flag temporarily flipped; post-batch flag revert confirmed via probe
- CLAUDE.md pins: JIMINY-INFORMATIONAL-CATEGORY-001, JIMINY-ARCHIVED-CODE-FILTER-001, BACKUP-RESTORE-VERIFY-001 (rewrite #5 rationale), sequential-epics + live-testing-tier-required (renamed by this sprint)
