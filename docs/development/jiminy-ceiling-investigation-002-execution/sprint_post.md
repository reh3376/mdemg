# JIMINY-CEILING-INVESTIGATION-002 — Path 4 + Path 1 Execution

**Tasks**: #154 (Path 4) + #155 (Path 1)
**Executed**: 2026-09-06 (~40min wall-clock)
**Predecessor**: JIMINY-CEILING-INVESTIGATION-002 (#153) — established the two paths
**Verdict**: ✅ SHIPPED — META-SCOPE flag flipped + 9 process-class rules marked informational; passive re-measure window opens now (168h to 2026-09-13).

## What shipped

### Path 4 — META-SCOPE flag flip (task #154)

`.env`:
```
+ JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED=true
```

Kickstarted `com.mdemg.server` via launchd; new pid 80747 picked up `.env` via `loadConfig()` → `godotenv.Load()` (`serve.go:83`, verified path).

**Ship-dormant since 2026-08-14** (JIMINY-CLASSIFIER-META-SCOPE-001) — the flip condition was "when JIMINY-CEILING-BREAK-2 T+168h shows CONTEXT-002 underdelivers." The investigation confirmed that condition. Passive re-measure per the shipping sprint's own recommendation.

### Path 1 — mark 9 process-class rules `is_informational=true` (task #155)

Applied via `mdemg jiminy constraint mark --code X --space-id mdemg-dev` (shipped in JIMINY-INFORMATIONAL-CATEGORY-001, #99). Each dry-run confirmed exactly 1 matching non-informational live node before applying.

| # | Code | Node ID | Class |
|---|---|---|---|
| 1 | `plan-mode-before-change` | johzbd6pbcyvpejc2omshmlt | process |
| 2 | `iterate-break-fix-verify` | jil0yqis8jl6qdrbounf6ysp | process |
| 3 | `must-validate-all-claims-before-commit` | rtyx9qcql5os1jyo2mmxlolq | process |
| 4 | `end-with-docs-accessed` | r1fkni8hy0ocwap09ixb5f5z | process |
| 5 | `memory-preservation-backup-integrity` | edippshe56qmtgf7v8u4xadu | meta |
| 6 | `auto-build-restart-after-feature` | a6nxulr95pa7i6m5xia2nmcl | workflow |
| 7 | `agent-handoff-requirement-guardrail` | qx883msd30v8vrlgzrk1ymp3 | meta (UI-completion) |
| 8 | `live-testing-tier-required` | mpbbazh3k46rkj37n9ghanbr | process |
| 9 | `never-skip-discovered-issues` | a5ax6i2owzuwlry7emx0pqkh | process (behavioral) |

Post-apply: `mdemg jiminy constraint list-informational --space-id mdemg-dev` shows **18 total** (9 pre-existing + 9 new).

### Rules deliberately NOT marked (3 of the top-12 from investigation)

Excluded because they ARE structurally classifier-verifiable from action-text (not process-verifiable-only):

| Code | Rationale for keeping graded |
|---|---|
| `openai-max-completion-tokens` | Code-content rule — verifiable from diffs; grep-able for `max_completion_tokens` vs `max_tokens` in gpt-5.x call sites |
| `no-direct-main-commits` | Bash-content rule — verifiable from `git push origin main` in bash commands; ALSO has independent pre-bash-check.py enforcement (defense-in-depth is real, not just grading) |
| `project-planning-docs-in-repo-only` | Path-verifiable — file writes to `~/Downloads/` vs `docs/development/<sprint-line>/` are directly observable in action-text |

These 3 rules retain grading. If they persist as top-ignored codes, the classifier IS failing at a task the LLM could learn — that's the honest signal Path 5 (retrain) could address later.

## Verification

| Check | Result |
|---|---|
| `.env` diff | ✅ single-line add + comment |
| launchd kickstart | ✅ new pid 80747 (Path 4), then 81191 (Path 1 kickstart to reload informational set) |
| `/healthz` post-kickstart | ✅ HTTP 200 (both restarts) |
| Boot log | ✅ jiminy config re-emitted with strict mode ON |
| `mdemg jiminy constraint list-informational` | ✅ **18 total** (was 9) |
| Dry-runs before apply | ✅ each dry-run matched exactly 1 non-informational live node |
| Production llama-server on 8102 | ✅ untouched (both restarts are the mdemg server on 9999) |
| Reversibility | ✅ every mark is `mdemg jiminy constraint mark --code X --space-id mdemg-dev --unmark`; META-SCOPE reversal is `--no-`-prefix flag or `.env` edit + kickstart |

## Substrate mutation record

- 1 `.env` edit (1 flag added)
- 9 Neo4j MemoryNode property writes (`is_informational=true`, `informational_marked_at=now()` on each)
- 2 launchd kickstarts (Path 4 and Path 1 respectively — the Path 1 kickstart reloads the informational-node set into `RecordOutcome`'s in-memory cache per JIMINY-INFORMATIONAL-CATEGORY-001's implementation)

## Expected trajectory

- **Path 4** (META-SCOPE flag): per JIMINY-CLASSIFIER-META-SCOPE-001 live-smoke verdict (flip counts 1/0/0), expected **+1-3pp** effect on the classifier-verifiable class. Won't move the 97% majority; won't move the 9-rule informational class (which now routes to `not_applicable` and drops out of the metric denominator entirely).
- **Path 1** (9 informational marks): the 9 codes accounted for **~215 ignored rows in the last 168h** (extrapolating from the top-12 that included 253 ignored; -3 excluded codes ≈ -38). Post-flip these rows will route to `not_applicable` via `RecordOutcome`'s override, which the shipped `constraint_outcomes` writer gates out of both Neo4j `GUIDANCE_OUTCOME` and TSDB. **Expected metric move: 16.73% → likely 30-50% variance-dependent** (small remaining denominator = high variance).
- **Combined honest interpretation**: the metric will spike upward as the denominator collapses. This is a MEASUREMENT REFRAME, not a quality lift — the underlying agent-follows-durable-rules behavior didn't change. The purpose is to give the metric denominator TO the class the classifier can actually verify, so future arc phases (like Phase 4b retrain) can be evaluated on a meaningful scoreboard.

## Follow-ups

1. **T+168h passive re-measure** (2026-09-13): re-run the investigation's D1 query to compare post-Path-1 stable-window follow rate vs the pre-Path-1 15.62% baseline. Expected 30-50%.
2. **Path 2 or Path 3 design sprint** — pre-retrain metric-denominator design. Either build process-verification observers (Path 2, 2-4 sprints) or redefine the metric to grade classifier/process/human classes separately (Path 3, 1 sprint).
3. **Correction-sink 99.6% drop investigation** — noted in #153 D6; separate sprint.
4. **Reconsider the 3 excluded codes** post-Path-1 re-measure — if they persist as top-ignored, they're the honest baseline for what a retrain COULD improve on the classifier-verifiable class.

## Documents Accessed

- `docs/development/jiminy-ceiling-investigation-002/verdict.md` — the paths this sprint executes
- `docs/development/jiminy-classifier-meta-scope-001/` — Path 4 shipping sprint, established the flip condition
- `docs/development/jiminy-informational-category-001/` — Path 1 shipping sprint, established the CLI mechanism
- `.env` — Path 4 target
- Live Neo4j `MemoryNode {space_id: 'mdemg-dev', constraint_code: <top-9>}` — Path 1 target
- launchd plist `com.mdemg.server` — kickstart target
- `~/.mdemg/logs/server.log` — boot verification
- CLI `mdemg jiminy constraint {mark,list-informational}` — shipped substrate-mutation tool
- CLAUDE.md pins
- Operator directive: "run path 4 then path 1" (2026-09-06)
