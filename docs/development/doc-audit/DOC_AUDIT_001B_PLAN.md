# Sprint Plan — DOC-AUDIT-001b: Stale-Subtree Disposition

## 1. Header & Metadata
DOC-AUDIT-001b · 2026-06-12 · branch `reh3376_dev02` (charter amendment 3)
· governed by `docs/development/doc-audit-001/CHARTER_amended.md`
(operator-approved; binding amendments 1–10) · unblocked by
DORMANT-CENSUS-001 (PR #452): `docs/api/route_consumer_inventory.json`
is the endpoint-claim verification oracle · effort ~2–3d.

## 2. Problem Statement
251 files across docs/architecture (121), docs/specs (46), and
docs/uxts01 + lang-parser + sidecar (84) — 241 untouched since
pre-April (129 from Feb, 112 from Mar; newest 2026-04-20). They predate
the MoE→dense pivot, the llama-server cutover, RRF default-on, and the
entire Q3 core. Auditing all 251 per-claim is wasteful and was lens-3
rejected; the charter's answer: triage living-vs-design-history FIRST
(banner the history — cheap), per-claim-verify only survivors.

## 3. Scope & Constraints
In: (1) triage all 251 into LIVING (claims to describe the current
system) vs DESIGN_HISTORY (plans/proposals/point-in-time analyses for
since-shipped or superseded work); (2) banner every DESIGN_HISTORY file
with a standard header (content preserved — R-LT-4); (3) per-claim
verification of LIVING survivors against code + the route inventory +
live system; verdicts to the ledger; (4) findings shard + summary.
Single-writer: only the orchestrator edits ledger/findings/docs;
subagent lanes are read-only with bounded output. Out: T4–T6 framing
sweep (001c → HYGIENE-SWEEP); doc fixes beyond banners (fix batch after
operator reviews findings, 001a precedent); CLAUDE.md/CHANGELOG edits
in batch PRs (amendment 3 — presented individually at close).

## 4. Dependencies
CHARTER_amended.md; ledger `doc_audit_ledger.jsonl` (827 rows);
route_consumer_inventory.json (census oracle); /tmp/001b_files.txt
enumeration (251 rows, last-commit-dated); dev02 synced with main
@7eaf4dd.

## 5. Implementation Plan
Wave 0: this plan, committed+pushed (amendment 1) · Wave 1: 4 read-only
triage lanes (architecture×2, specs×1, uxts01+lang-parser+sidecar×1),
verdict CSV LIVING|DESIGN_HISTORY + 1-line reason/file; orchestrator
spot re-checks ≥10 and ALL LIVING verdicts · Wave 2: banner
DESIGN_HISTORY files (single mechanical pass, standard banner naming
the superseding reality + audit date) · Wave 3: per-claim-verify LIVING
survivors (lanes sized 5–10 files, amendment 6); endpoint claims
checked against the route inventory; runtime claims live-verified where
cheap · Wave 4: ledger verdicts (all 251 rows), findings shard,
summary + fix-batch proposal, push, PR, summary comment, CI watch.

## 6. Testing Plan
T1: ledger JSONL parse + row-count sanity after update; banner
idempotency (re-run adds nothing). T2: every verdict carries
audited_commit + ≥1 evidence path; orchestrator re-verifies all
DRIFT/LIVING verdicts. T3 (live): LIVING docs' runtime claims (ports,
endpoints, env vars, CLI) verified against the live stack + inventory;
UxTS drift checker + verify_config_consumers.py green pre-push
(amendment 7).

## 7. Commit Strategy
Commit AND push per wave on dev02 (amendment 1). One auto-PR at close
(banners + ledger + findings). CHANGELOG presented individually.

## 8. Verification Checklist
- [ ] All 251 files triaged; every LIVING verdict orchestrator-verified
- [ ] DESIGN_HISTORY files bannered (content preserved), idempotent
- [ ] LIVING survivors per-claim verified; verdicts + evidence in ledger
- [ ] Endpoint claims cross-checked against route_consumer_inventory.json
- [ ] Findings shard + verdict histogram + fix-batch proposal
- [ ] Drift checker + config-consumer scanner green; CI green on PR

## 9. Documentation Update — the artifacts ARE the docs; CHANGELOG at close (individually).

## 10. Risks & Mitigations
Lane misclassification buries a living doc under a history banner →
orchestrator re-verifies every LIVING verdict AND spot-checks
DESIGN_HISTORY (10+); banners are reversible one-line reverts. Context
reset → resume = this plan + ledger on dev02. Subtree contains
generated/template-paired files → banner pass excludes them
(amendment 7); none known in these subtrees.

## 11. Documents Accessed
CHARTER_amended.md; DOC_AUDIT_PLAN.md (001a precedent);
doc_audit_ledger.jsonl; route_consumer_inventory.json;
/tmp/001b_files.txt enumeration.

## 12. Rollback Procedures
Banners: git revert (single mechanical commit). Ledger/findings:
additive. No other doc edits this phase.
