# JIMINY-CLASSIFY-ESCALATION-INSPECT-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** JIMINY-CLASSIFY-ESCALATION-INSPECT-001
- **Date:** 2026-08-10
- **Branch:** `reh3376_dev01`
- **Effort:** ~2 hours
- **Provenance:** disclosed follow-up from JIMINY-HEURISTIC-DEFAULT-001 post.md (2026-08-10). Live-caught during the earlier sprint when the pre-write hook blocked every Edit on `internal/config/config.go` and the operator override on constraint code `must` did not unblock — because `must` is a SEVERITY marker (must/should/info), NOT a constraint_code, and the classifier was extracting the wrong token.

## 2. Problem Statement

`internal/jiminy/strict_classifier.go::extractConstraintCode` extracts the leading `[TOKEN]` from the EvaluationItem's `Content` and treats it as the constraint_code. But `internal/jiminy/evaluator.go:415` builds Content as `fmt.Sprintf("[%s] %s (sim: %.2f)", cTypeStr, desc, simVal)` where `cTypeStr` is the `constraint_type` (`must` / `must_not` / `should` / `info`) — a SEVERITY axis, NOT a unique code.

Consequences:
1. **Operator override targets the wrong constraint.** `mdemg jiminy override apply --constraint must` overrides EVERY rule whose constraint_type=must (many). Not the specific offending rule.
2. **Block message doesn't name the actual code.** WARNED+ block messages surface `[must] <content>` — operator can't run `mdemg jiminy override apply` because the actual code is invisible.
3. **Escalation state is per-node, but overrides key on the (mis)extracted code.** So overrides + escalation can't correlate — the operator escape hatch is architecturally broken for constraints whose content doesn't happen to start with a well-formed `[CODE]` token.

The real `constraint_code` (a distinct mnemonic string like `no-hardcode-pool-sizes`, `always-use-cuidv2`, or generated `auto-<hash>`) lives on the Neo4j MemoryNode as the `constraint_code` property — the Cypher query already selects `role_type='constraint'` nodes; it just doesn't RETURN the code field.

## 3. Scope & Constraints

**In-scope:**
- Add `constraint_code` to the Cypher RETURN in `findMatchingConstraints` + `findMatchingCorrections`.
- Add `ConstraintCode string` field to `EvaluationItem`.
- Populate the new field when building items from the Cypher rows.
- Rewrite `strict_classifier.Classify` to read `item.ConstraintCode` directly (fall back to `extractConstraintCode` from content ONLY for legacy path — but grep-cleared it, no legacy caller remains).
- DELETE the buggy `extractConstraintCode` helper (it's an artifact of the wrong extraction path).
- Update the block DenialReason to prepend `code=<constraint_code>` so operators see the code they can override.
- Update `pre-write-check.py` stderr banner: display the code prominently so operators/agents don't need to inspect JSON to find it.
- Sync staged embed if hook template changes.
- Pin tests: (a) EvaluationItem carries constraint_code from Cypher, (b) StrictClassifier's ViolatedCodes has real codes not severity markers, (c) DenialReason includes `code=` annotation.

**Out-of-scope:**
- Changing the escalation-tracker's keying (already keys on session_id + node_id — correct).
- Changing the Neo4j vector index or query structure beyond adding the RETURN column.
- Backfilling the pre-write hook's message-history — going forward only.
- Fixing the classify escalation itself (that's separate — escalation-state was working correctly; the identification of WHICH escalated constraint blocked was broken).

## 4. Dependencies

- JIMINY-ENFORCE-003 — operator override CLI, whose contract is `--constraint <CODE>`. This sprint restores that contract's viability.
- Evaluator + Cypher paths in `internal/jiminy/evaluator.go` (constraint + correction node types both need the fix).
- Pre-write / pre-bash hooks that consume the classify response.

## 5. Implementation Plan (sequential)

**E1 — types + Cypher.**
- Add `ConstraintCode string \`json:"constraint_code,omitempty"\`` to `EvaluationItem`.
- Extend the Cypher in `findMatchingConstraints` to `RETURN … c.constraint_code AS constraint_code`.
- Same for `findMatchingCorrections`.
- Wire the field into the EvaluationItem construction at both call sites (~lines 413, 479).

**E2 — strict_classifier reads real code.**
- In `Classify`, replace `code := extractConstraintCode(item.Content)` with `code := item.ConstraintCode`.
- If `code == ""` (defensive — old Neo4j nodes might not have constraint_code), fall back to the SourceNode ID prefixed with `node:` so overrides can still target something specific (uniquely-identifying, unlike `must`).
- DELETE the `extractConstraintCode` function entirely (unused elsewhere per grep).

**E3 — DenialReason format.**
- Modify the reason string builder at strict_classifier.go:146 to prepend `[code=<CODE>]` for each violated constraint. Format: `Constraint violation (warned): [code=no-hardcode-pool-sizes] <content>; [code=<CODE2>] <content2>`.
- When the fallback `node:<id>` route fires, the DenialReason surface `[code=node:<id>]` so the operator sees they can override on that pseudo-code.

**E4 — hook display.**
- `internal/cli/hook_templates/pre-write-check.py` — parse the ClassifyResponse's `violated_codes` (already populated), display the codes prominently. Current stderr banner shows `[/strict] <reason>` — extend to `[/strict] BLOCKED (violated codes: c1, c2): <reason>\n[operator] to override: mdemg jiminy override apply --constraint <CODE> --reason ... --duration <window>`.
- Same treatment for `pre-bash-check.py` (has the same `violated_codes` field to work with).
- `make sync-grafana-embed` — actually this only touches hook templates, not Grafana. Just `go build` + run `make verify-hook-templates` if such target exists.

**E5 — pin tests.**
- `evaluator_test.go` (or new `evaluator_constraint_code_test.go`): assert EvaluationItem carries constraint_code from Cypher (mock-driver based, if that's the test shape here; otherwise skip Neo4j test + rely on end-to-end).
- `strict_classifier_test.go` (or a new `_code_test.go`): assert Classify's response's ViolatedCodes includes real codes NOT severity markers when the underlying EvaluationItem's ConstraintCode is set.
- Assert `extractConstraintCode` is DELETED (grep-based test / build failure if reintroduced).
- Assert DenialReason includes `[code=<CODE>]` annotations.

**E6 — live Tier-3 smoke.**
- Restart mdemg with rebuilt binary.
- Trigger a WARNED+ classify hit against a known constraint whose code IS `no-hardcode-pool-sizes` (a real code visible in the earlier CMS recall).
- Verify: response has `violated_codes: ["no-hardcode-pool-sizes"]`; DenialReason includes `[code=no-hardcode-pool-sizes]`; hook stderr shows the code + the override command.
- Apply `mdemg jiminy override apply --constraint no-hardcode-pool-sizes --duration 5m --reason "smoke test"`; re-trigger the classify; verify verdict now `pass` with override-suppressed annotation.

**E7 — docs.**
- Sprint dir: sprint_plan + post.md.
- CLAUDE.md pin.
- CHANGELOG entry.

## 6. Testing Plan

**Unit (T1):**
- `TestEvaluationItem_CarriesConstraintCode` — Cypher round-trip (against a mocked-record if the evaluator tests use one).
- `TestStrictClassifier_ViolatedCodesAreRealCodes` — a real code string, NOT `must`/`should`.
- `TestStrictClassifier_DenialReasonIncludesCodeAnnotation` — string-contains check.
- `TestExtractConstraintCode_Deleted` — pin the function is gone (compile-check via file-level grep).

**Integration (T2):**
- Existing evaluator + strict_classifier suites continue green.

**Live (T3):**
- E6 above: full round-trip on mdemg-dev with a known constraint code.

## 7. Commit Strategy

Single commit: `fix(jiminy): classify reports real constraint_code so overrides target the right rule (JIMINY-CLASSIFY-ESCALATION-INSPECT-001)`.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/jiminy/... ./internal/cli/hook_templates/...` = 0 issues
- [ ] `go test ./internal/jiminy/...` green including new pins
- [ ] Live smoke E6 passes: block message includes real code, override on that code unblocks
- [ ] hook_templates staged embed synced
- [ ] Pre-write + pre-bash hooks display code prominently
- [ ] CHANGELOG entry
- [ ] CLAUDE.md pin

## 9. Risks & Mitigations

**R1: Old constraint nodes in Neo4j lack `constraint_code` property.**
- Fallback to `node:<source_node_id>` as a pseudo-code. Overrides on `node:xxx` still work end-to-end via the same OverrideManager keying — it doesn't validate the code exists as a real constraint code, just uses the string as an opaque key.
- Live-verified: mdemg-dev constraint corpus per JIMINY-ARCHIVED-CODE-FILTER-001 has 128 archived + 103 live constraint nodes; earlier CORRECTION-CODE-GEN-001 backfilled 35 correction codes. Post-those-sprints, every ACTIVE (non-archived) constraint has a code. Fallback path is safety-net only.

**R2: Hook display gets too verbose.**
- Cap the codes displayed at 5 (elide with `+N more`); truncate reason to 300 chars for the banner (full reason in JSON already goes to the alert dispatcher).

**R3: DenialReason string grows past its 500-char sanitize cutoff, dropping useful content.**
- The prepended `[code=X]` per finding is ~30 chars; up to 5 findings = ~150 chars overhead. Reason content already truncated. Should fit; monitor.

**R4: Someone else was relying on `extractConstraintCode` for a legitimate purpose.**
- Grep-verified: no other caller. Function is DEAD after this sprint. Delete safely.

## 10. Rollback Procedures

Revert the 3-file commit. `EvaluationItem.ConstraintCode` is additive (omitempty JSON) so backward-compat is preserved. Cypher extension is additive. No schema/migration.

## 11. Documents Accessed

- `docs/development/jiminy-heuristic-default-001/post.md` — disclosed this follow-up
- `internal/jiminy/strict_classifier.go` — the buggy `extractConstraintCode` + Classify flow
- `internal/jiminy/evaluator.go` — `findMatchingConstraints` + `findMatchingCorrections` (Content construction)
- `internal/jiminy/types.go` — EvaluationItem struct
- `internal/jiminy/override.go` — override manager keying contract
- `internal/cli/hook_templates/pre-write-check.py` — hook consumer of ClassifyResponse
- Live: current alert `jiminy-block` message shape from `~/.mdemg/alerts/current.json`
- CLAUDE.md pins: JIMINY-ENFORCE-003, JIMINY-ARCHIVED-CODE-FILTER-001, CORRECTION-CODE-GEN-001
