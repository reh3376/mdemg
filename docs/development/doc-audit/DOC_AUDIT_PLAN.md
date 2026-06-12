# Sprint Plan — DOC-AUDIT-001a: Ledger Bootstrap + Canon Pass

## 1. Header & Metadata
DOC-AUDIT-001a · 2026-06-12 · branch `reh3376_dev02` (charter amendment 3)
· snapshot SHA **831eb0a** (all Phase-A verdicts are facts about this SHA)
· governed by `docs/development/doc-audit-001/CHARTER_amended.md`
(operator-approved; binding amendments 1–10) · effort ~1–1.5d.

## 2. Problem Statement
Docs drift continuously (~several merges/day); DOC-TRUTH-001/002 closed
the marker class but 821+ files have never had a systematic
claim-vs-source audit. 001a covers the highest-stakes slice: canonical
T0 + operator-facing T3 + a T1 spot-check.

## 3. Scope & Constraints
In: 8 T0 + 23 operator-facing T3 (ops/user/guides/api, excl. api-spec
test trees) + 10 T1 spot-checks (oldest feature docs). Single-writer:
ONLY the orchestrator writes ledger/findings/docs. Subagents read-only,
bounded summaries (≤5 lines/file), evidence to scratch outside repo.
Out: 001b subtrees (architecture/specs/uxts01/lang-parser/sidecar —
needs DORMANT-CENSUS oracle), T4–T6 framing sweep (001c), any doc EDIT
beyond verdicts this phase (fixes = follow-up batch after operator
reviews findings; CLAUDE.md/CHANGELOG never batch-edited).

## 4. Dependencies
Charter; ledger (`doc_audit_ledger.jsonl`, 827 rows, sanity-gated);
snapshot 831eb0a; read-only subagent lanes.

## 5. Implementation Plan
Wave 0 bootstrap (this commit) · Wave 1: T0 lanes (CLAUDE.md, README,
00_README_v2 single-file lanes; remaining T0 one lane) · Wave 2:
operator-facing T3 (3 lanes × ~8) · Wave 3: T1 spot-check (2 lanes × 5,
oldest by last-commit) · Wave 4: merge → findings shards → verdicts in
ledger → summary. Commit AND push after every wave (amendment 1).

## 6. Testing Plan
T1: ledger row-count sanity (±5% of 825) + JSONL parse. T2: every
verdict carries audited_commit=831eb0a + ≥1 evidence path; spot re-check
of 5 random subagent verdicts by the orchestrator. T3 (live): for docs
making runtime claims (ports, endpoints, env vars), verdicts verified
against the live system where cheap (healthz, env, CLI --help).

## 7. Commit Strategy
One additive PR from dev02 (Phase A artifacts only). Wave commits pushed
immediately. No repo doc edits in this PR.

## 8. Verification Checklist
- [x] Ledger 827 rows @ 831eb0a, JSONL, git ls-files enumerator
- [ ] All 31 001a rows + 10 spot-checks carry verdict + evidence
- [ ] Findings shards per wave committed
- [ ] Orchestrator spot re-check (5 verdicts) clean
- [ ] Summary: verdict histogram + fix-batch proposal for operator

## 9. Documentation Update — the artifacts ARE the docs; CHANGELOG at close.

## 10. Risks & Mitigations
Context reset → resume = read this plan + ledger from THIS branch (full
clone, never depth-1). Subagent error → orchestrator re-verifies all
DRIFT verdicts before recording. Drift during audit → verdicts pinned to
831eb0a; one re-baseline at close per amendment 4.

## 11. Documents Accessed
CHARTER_amended.md; 3-lens review reports (2026-06-11); ledger bootstrap
output.

## 12. Rollback Procedures
Additive-only this phase; revert commits. No doc edits to roll back.
