# DOC-AUDIT-001 — Amended Charter (supersedes the as-written orchestrator prompt)

Operator approved 2026-06-11: "follow team's recommendations." Source
prompt: `PROMPT_doc_audit_orchestrator.md` (operator-provided), reviewed
by a 3-lens agent team (facts / design / execution-fit). The original
prompt's architecture (committed-ledger survival, single-writer,
non-uniform tiering, R-LT-4 compliance) is adopted; the following
amendments are BINDING and override it wherever they conflict.

## Phasing (lens-3 descoping — replaces the original's full-825 sweep)

- **001a — ledger + canon pass (~1–1.5d).** Bootstrap the three artifacts;
  audit T0 (13 canonical docs) + operator-facing T3
  (docs/operations, docs/user, guides, api, quickstart ≈40 files) +
  spot-check 10 pre-DH-004 feature docs. Runs after FT-CLASSIFY-002
  closes.
- **001b — stale-subtree disposition (~2–3d). AFTER DORMANT-CENSUS-001**
  (its endpoint↔consumer inventory is the verification oracle).
  Triage docs/architecture (116/121 untouched since pre-April) +
  docs/specs/uxts01/lang-parser/sidecar (≈250 files) into
  living-vs-design-history FIRST (banner the history — cheap);
  per-claim-verify only survivors.
- **001c — standing mechanism.** T4–T6 framing sweep folds into
  HYGIENE-SWEEP; the ledger's STALE_RECHECK re-baseline becomes a
  permanent post-merge step (forcing function > one-shot sweep).

## Binding amendments (lens 2, all 10)

1. **Survival = commit AND PUSH per wave** to the named audit branch.
   Resume = clone THAT branch, full history (never `--depth 1`, never
   main). Bootstrap only after confirming no prior state exists on any
   remote branch or open PR.
2. **Enumerator = `git ls-files '*.md'`** (the original `find` sweeps
   ~700 non-repo files incl. `.claude/` agent memory). Abort bootstrap
   if row count deviates >5% from ~825.
3. **Branch strategy**: audit runs on `reh3376_dev02` (reserved for
   secondary agents) so dev01 stays free for sprints. Phase A artifacts =
   ONE additive PR. Phase B fixes = sequential batches on the same
   branch, one auto-PR at a time, merged before the next. **CHANGELOG and
   CLAUDE.md edits are NEVER in batch PRs** — presented to the operator
   individually (CLAUDE.md edits mutate the auditing agent's own
   instructions).
4. **Convergence**: Phase A targets ONE pinned snapshot SHA; re-baseline
   exactly once at Phase A completion, scoping STALE_RECHECK to files
   whose recorded evidence paths intersect `git diff --name-only
   <snapshot>..HEAD`. Ledger rows record evidence paths for this purpose.
   Post-completion drift = 001c's standing mechanism, not this task.
5. **Context economy**: subagents return bounded summaries (verdict +
   ≤5-line digest/file); full evidence to disjoint per-agent scratch
   files in a non-repo scratch dir; findings sharded per-batch
   (`doc_audit_findings/<batch>.md`); checkpoint-commit-push BEFORE
   dispatching the next wave.
6. **Batching by claim budget**: T1/T2 lanes 5–10 files; CLAUDE.md,
   00_README_v2.md, README.md are single-file lanes; T4–T6 framing lanes
   30–50.
7. **Phase B CI discipline**: run the UxTS drift checker +
   `verify_config_consumers.py` locally pre-push; never change doc'd
   prompt text away from ULTS-pinned sources; never touch
   template-paired/generated files.
8. **Ledger = JSONL**, not hand-rolled CSV; notes newline-free.
9. Pin one checkout SHA per wave; `TIER_APPEAL` convention for
   misclassified files; subagent models haiku (T4–T6) / sonnet (T0–T3);
   push-rejection is the duplicate-orchestrator guard.
10. **DOC_AUDIT_PLAN.md follows the 12-section sprint-plan format**, with
    the doc-analog 3 tiers: T1 link/path lint over edits; T2 the
    merge-blocking CI gates run locally; T3 re-verify each corrected
    claim against code at the PR's HEAD.

## Drift-model corrections (lens 3)

- Drop `8101` as a stale marker (it is the LIVE sidecar port).
- Risk ranking is AGE-based: T2 + stale T3 subtrees > T1
  (docs/features has 0 stale files — the per-feature-doc rule works).
- The MoE/mlx-server marker class is already closed (DOC-TRUTH-001/002);
  do not re-sweep it.

## Status

- [x] Charter committed (this file)
- [ ] 001a execution — queued behind FT-CLASSIFY-002 close
- [ ] 001b — queued behind DORMANT-CENSUS-001
- [ ] 001c — fold into HYGIENE-SWEEP planning
