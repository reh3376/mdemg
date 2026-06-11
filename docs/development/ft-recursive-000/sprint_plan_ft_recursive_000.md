# Sprint Plan — FT-RECURSIVE-000: Recursive-Retraining Loop, As-Built Audit + Buildable Spec

## 1. Header & Metadata
Sprint: FT-RECURSIVE-000 · 2026-06-11 · branch `reh3376_dev01` ·
operator-prompted (`PROMPT_ft_recursive_loop_deep_dive_v2.md`, reviewed by
sub-agent 2026-06-11: 20/23 claims verified, 7 amendments adopted) ·
**doc-only — no production code** · effort 2–2.5d · risk low (audit +
specification; the build sprints it specs stay gated behind
FT-CLASSIFY-002 per Roadmap §4).

## 2. Problem Statement
The FT line's largest unfinished promise (Phases 6/7/9 — recursive
retraining, RSIC integration, monitoring) is NOT STARTED, yet a
partially-built skeleton already exists and is firing into a void **live,
today**: `dataset_builder` computes per-task readiness, RSIC insight #29
senses it and emits `RecommendedAction: trigger_training_pipeline`, and
the actuator is `executeAlertLog` — a no-op that has been spamming
"Training data threshold reached" alerts all session. The build sprints
need a code-verified as-built map, silent-failure + dead-seam
inventories, a target state machine honoring the no-silent substrate, and
an operator decision table — before anyone writes actuator code.

## 3. Scope & Constraints
**In**: governance handshake (done — disclosed: skill `jiminy-governance`
unregistered, guidance empty, instance reachable); anchor re-verification
on current HEAD; five-stage as-built map (capture → readiness/curation →
train → benchmark/gate → promote/rollback) + RSIC seam trace + the two
inventories, all `file:line` on HEAD; target design (state machine,
5-class auto-remediation taxonomy, compute lease/RSIC quiesce, auto-issue
escape hatch via `internal/gaps`); phased build plan (6a/6b/7/9) with exit
criteria; §7 decision verdict table; aligns-with note vs `00_README_v2.md`
+ `ROADMAP_2026Q3.md` §4. **The 7 review amendments are binding**:
overfitting-prevention policy as enforced gate parameters; promotion gate
= 16-task augmented eval (leak-audited) + valid_clean (never
valid_clean-only — the 11.5d 9-task-subset artifact); corrected resource
model (9h7m wall, 36 GB peak RAM, ~85 GB transient disk —
`phase_5_sft_post.md:81-82`); `rerank_collector.go` lives in
`internal/retrieval/`; anchors re-pinned on HEAD; the `DiagnosticActions`
re-classification question added to the decision forks; the readiness
threshold answer stated (global hardcoded const — a no-hardcoding
violation phase 6b must fix).
**Out**: any production code, config, or migration; advancing
`00_README_v2.md` STATUS phases (nothing ships — a pointer line only);
executing FT-CLASSIFY-002; resolving the §7 operator forks (recommended,
not decided).

## 4. Dependencies
The prompt doc (operator-provided); the sub-agent review report
(amendments source); HEAD `a4fc296` (anchors re-verified against it);
`internal/{tsdb/dataset_builder.go, ape/{self_assess,self_reflect,
task_dispatch,types_rsic}.go, jobhealth/, supervisor/, gaps/, llmclient/,
retrieval/, jiminy/, embeddings/, cli/data_*.go}`; `neural/training/` +
`neural/benchmarks/`; `docs/development/ft-lora/00_README_v2.md`;
`ROADMAP_2026Q3.md`; live system (alert stream + TSDB) for Tier 3.

## 5. Implementation Plan
Epic 0 this plan · **Epic 1** anchor re-verification on HEAD (every
`file:line` the prompt cites; drifted lines re-pinned) · **Epic 2**
as-built audit: stages A–E + RSIC seam + silent-failure & dead-seam
inventories (sub-agent fan-out; load-bearing claims re-verified directly —
last sprint's recon was wrong 3×) · **Epic 3** target design (state
machine, taxonomy, lease/quiesce, gaps-routed issue filer) · **Epic 4**
phased plan + decision verdict table + aligns-with note · **Epic 5**
documentation epic (never cut): CHANGELOG, `00_README_v2.md` pointer,
post.md; push.
Deliverable: `docs/development/ft-recursive-001/SPEC_recursive_retraining_loop.md`
(the buildable spec the FT-RECURSIVE-001..004 build sprints execute
from); sprint record in `docs/development/ft-recursive-000/`.

## 6. Testing Plan
Doc-only sprint — tiers map to verification classes. Tier 1 (citation
correctness): every `file:line` in the spec independently opened and
matched on HEAD; no invented paths/symbols (prompt §8 rule). Tier 2
(consistency): spec contradicts neither `00_README_v2.md`, the roadmap §4
deferral, nor the standing policies (no-tool-calling, overfitting
prevention, no-hardcoding, honest eval). Tier 3 (live): the dead-actuator
claim demonstrated on the live system — count today's
`trigger_training_pipeline` alert firings (TSDB `metric_samples`/alert
history) and capture the live `TrainingDataReadiness` snapshot
(readyCount, rows, projected days) so the spec's exhibit A is evidence,
not assertion.

## 7. Commit Strategy
Docs commits per epic-group (plan · spec(+evidence) · CHANGELOG/pointer/
post) · push once (auto-PR) · sprint summary comment · CI watcher.

## 8. Verification Checklist
- [ ] Governance handshake performed; skill-absence disclosed
- [ ] All prompt anchors re-verified or re-pinned on HEAD
- [ ] Five-stage map + RSIC seam, all file:line verified (Tier 1)
- [ ] Silent-failure + dead-seam inventories complete
- [ ] All 7 review amendments incorporated and marked
- [ ] State machine + 5-class taxonomy + lease/quiesce + filer specified
- [ ] 6a/6b/7/9 phased plan with exit criteria; FT-CLASSIFY-002 framed as
      the first vertical slice; no autonomy before the manual path proves
- [ ] §7 verdict table (now 7 forks incl. DiagnosticActions class) with
      recommendations, none silently resolved
- [ ] Aligns-with note vs 00_README_v2.md STATUS + roadmap §4
- [ ] Live evidence captured (actuator firing count + readiness snapshot)
- [ ] CHANGELOG + 00_README_v2.md pointer + post.md

## 9. Documentation Update — Epic 5 (the deliverable IS documentation;
CHANGELOG + the `00_README_v2.md` pointer are the ledger hooks).

## 10. Risks & Mitigations
Anchor drift between audit and build sprints → every anchor carries the
HEAD hash it was verified on; the spec instructs builders to re-verify.
Sub-agent audit errors → load-bearing claims (dead actuator, no Go
caller, readiness logic) re-verified directly by the orchestrator.
Spec front-runs the FT-CLASSIFY-002 trigger → explicit sequencing section;
deliverable is paper. Scope creep into code → hard "no production code"
constraint restated in the spec header.

## 11. Documents Accessed
`PROMPT_ft_recursive_loop_deep_dive_v2.md` (read in full); sub-agent
review report (2026-06-11); ROADMAP_2026Q3.md §§2,4; CLAUDE.md FT section;
`00_README_v2.md`; per-epic source files (listed in the spec's own
citations); live alert stream + TSDB.

## 12. Rollback Procedures
Doc-only; revert commits. No state, schema, or config changes.
