# Sprint Plan — APE-PROMPT-BUDGET-001: Bound the ape.reflect Prompt So Output Can't Truncate

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · training-integrity remediation
(operator-sequenced after REWARD-CORRECTNESS-001, whose live Tier 3 surfaced
this) · target v0.10.x patch · effort ~1.5–2d · risk **medium** (changes a
production LLM hot-path prompt + possibly serving config; getting the trim
wrong drops signal the reflection needs, but the gate/validation catches
output correctness, and the change is reversible config/templating).

## 2. Problem Statement
ape.reflect — the LARGEST training target (54k rows) — produces ~87% truncated,
invalid-JSON responses. Live-measured root cause (REWARD-CORRECTNESS-001
`live_findings.md` + this sprint's recon):
- The user prompt is **7489 tokens** (system 628 + user 6861). Real stored-row
  breakdown: **Current Assessment (report JSON) ~3895 tok** (inflated by the
  TSDB dataset fields — `LLMPerformance` ×≤17 tasks, `RetrievalDataset`,
  `EmbeddingDataset`, `TrainingReadiness`), **Recent Cycle History (5 cycles)
  ~2693 tok**, Calibration ~274.
- The llama-server per-slot KV bound is `--ctx-size 32768 / --parallel 4 =
  **8192** tokens`. Prompt 7489 leaves only **~700 tokens** for output.
- Real `tokens_out`: 191/200 invalid responses cluster at **490–520 tokens**,
  truncating mid-JSON exactly at the KV ceiling. The 9 valid rows had shorter
  prompts (down to 5719 tok) → room to finish (744–981 tokens_out).
- NOT a max_tokens cap (`MaxTokens` floors at 2000, llm_reflector.go:126-131;
  valid rows exceed 500). NOT fixable by compression (`RSIC_LLM_REFLECT_COMPRESS`
  already defaults true and IS on). The prompt is **structurally unbounded** —
  it grows with task count + history + dataset richness, so it will keep
  re-crossing any fixed slot.

This corrupts the largest FT corpus: EVAL-INTEGRITY-001 "recovered" 71k
ape.reflect rows by fixing hash-drift, but ~87% are truncated garbage that
would poison a retrain. The distill gate's `json_valid` correctly rejects them
(0.133 mean) — so today the corpus is mostly unusable, not wrong.

## 3. Scope & Constraints
**In:** make the ape.reflect prompt **bounded by a token budget** so output
always has guaranteed headroom, durably (not a one-off trim). Two complementary
levers, **both proposed, picked/combined at execution** (memory:
`feedback_plan_options_pattern`):

- **Lever A (primary, structural — prompt budget):** introduce a configurable
  prompt-token budget for ape.reflect (`RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS`,
  default ~3500) enforced in `buildUserPrompt`:
  1. **Trim the Current Assessment** — omit or aggregate the verbose TSDB
     dataset fields for the reflection prompt (keep the scalar health/edge/
     orphan metrics the pattern-detection rules actually use; the dataset
     slices are the bloat and are rarely referenced in real reflections).
     Make inclusion configurable (`RSIC_LLM_REFLECT_INCLUDE_DATASETS`, default
     false) so it's reversible.
  2. **Bound history** — `RSIC_LLM_REFLECT_HISTORY_CYCLES` (default 3, was
     hardcoded 5) + keep the existing per-cycle `CriteriaDetail` truncation,
     tightened under compression.
  3. A final **budget guard**: if the assembled prompt still exceeds the
     budget, drop history cycles oldest-first, then truncate the assessment
     tail, logging loudly what was dropped (no silent truncation — memory
     rule). Target assembled prompt ≤ budget so output headroom ≥ 8192−628−
     budget ≈ 4000 tokens.

- **Lever B (complementary, serving headroom):** raise the per-slot KV bound so
  even a large prompt has room. Either `--parallel 4→2` (slot 16384) or
  `--ctx-size 32768→49152` (slot 12288) in the llama-server launchd plist
  (`~/Library/LaunchAgents/com.mdemg.llama-server.plist`). Trade-off:
  `--parallel 2` halves concurrency (4 slots → 2; mdemg's hot paths are mostly
  serial per call, so likely fine); `--ctx-size` raise costs more KV RAM. This
  is a config-only change, instantly reversible. **Decision at execution**
  based on measured concurrency need + RAM headroom; document in the post.

**Out:** re-capturing / backfilling the historical truncated corpus (separate —
forward-only fix here; the prune phase handles the bad rows); the reward/schema
mismatch (jiminy.evaluate) + keyword-bag (jiminy.synthesize) follow-ups; the
baseline recompute (still gated behind a clean corpus). Changing what
ape.reflect *detects* (only what's in its prompt, bounded).

**Constraints:** no-hardcoding (every new bound is a config field with a sane
default — memory `feedback_no_hardcoded_values`); sequential epics; Tier 3 live
required; durable structural fix, not a patch (memory
`feedback_no_short_term_mlx_patches`) — Lever A is the structural piece, B is
the safety margin.

## 4. Dependencies
`internal/ape/llm_reflector.go` (`buildUserPrompt`, `Reflect`);
`internal/ape/types_rsic.go` (`SelfAssessmentReport`); `internal/config/config.go`
(new fields + FromEnv); `internal/api/server.go:828` (wiring);
`internal/encoding` (CompactJSON/TruncateAtWord); llama-server launchd plist;
the llm token counter (estimate via chars or a tokenizer helper if present);
real TSDB ape.reflect rows for before/after measurement.

## 5. Implementation Plan
- **Epic 0** — this plan + recon (committed).
- **Epic 1** — config: add `RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS` (3500),
  `RSIC_LLM_REFLECT_HISTORY_CYCLES` (3), `RSIC_LLM_REFLECT_INCLUDE_DATASETS`
  (false) to config.go + FromEnv (range-validated) + wire through
  LLMReflectorConfig + server.go. Bump nothing schema-side.
- **Epic 2** — `buildUserPrompt` budget enforcement: dataset-field gating,
  history-cycle cap, final budget guard with loud drop-logging. Unit tests
  (Tier 1): a synthetic oversized report assembles to ≤ budget; datasets
  excluded by default; history capped; drop-log emitted; a small report is
  unchanged.
- **Epic 3** — (if Lever B chosen) update the llama-server plist + document the
  slot math; reload the agent. Measure new per-slot bound.
- **Epic 4 (Tier 3 LIVE)** — rebuild mdemg, restart, trigger real ape.reflect
  cycles (or wait for the RSIC micro cadence), then **measure on the real
  wire**: pull fresh ape.reflect rows from TSDB, confirm `tokens_in` dropped to
  ~budget, `json_valid` rate recovers from 0.13 → near-1.0, and `tokens_out`
  reflects complete arrays. Cross-check a sample parses + has ≥1 valid insight.
- **Epic 5** — docs: feature doc `docs/features/rsic-reflection-prompt-budget.md`
  (or extend an existing RSIC feature doc), CHANGELOG, post.md, CLAUDE.md
  Architecture-note update (correct the "~5800 tokens" figure to the measured
  ~7489 + the budget mechanism). Push → auto-PR → summary → CI.

## 6. Testing Plan (3 tiers)
**T1 (unit):** budget enforcement in `buildUserPrompt` — oversized report →
assembled ≤ budget; datasets gated; history cap honored; drop-events logged;
under-budget report untouched; config range validation. **T2 (integration):**
go build + `golangci-lint`; config scanner (`verify_config_consumers.py`) sees
the new fields consumed; a constructed `SelfAssessmentReport` with populated
TSDB datasets assembles within budget. **T3 (LIVE — required):** real
ape.reflect cycles on the running stack; TSDB before/after: `tokens_in` ≈
budget, `json_valid` recovery (0.13 → ~1.0 on fresh rows), responses parse with
valid insights. This is the proof the corpus is fixed.

## 7. Commit Strategy
Per-epic on dev01; lint each; push once at sprint end; PR summary; CI watch.
Live-surprise bugs get their own fix-commit (memory `feedback_live_testing`).

## 8. Verification Checklist
- [ ] New config fields, range-validated, no hardcoded bounds, scanner-clean
- [ ] `buildUserPrompt` assembles ≤ budget; datasets gated; history capped
- [ ] Budget guard drops oldest-first and logs loudly (no silent truncation)
- [ ] (If Lever B) plist slot math documented + agent reloaded
- [ ] LIVE: fresh ape.reflect `tokens_in` ≈ budget; `json_valid` recovered to ~1.0
- [ ] LIVE: sampled fresh responses parse + carry ≥1 valid insight
- [ ] Feature doc + CHANGELOG + post + CLAUDE.md figure correction
- [ ] go build + golangci-lint clean

## 9. Documentation Update — Epic 5 (never cut).

## 10. Risks & Mitigations
Trimming drops signal the reflection needs → keep all scalar health metrics
(the rule-based detectors' inputs); only gate the verbose dataset slices +
oldest history; `INCLUDE_DATASETS` reversible. Budget too tight → default 3500
leaves ~4000 output headroom (5× the failing ~700); tunable. Lever B
(`--parallel 2`) starves concurrency → measure real concurrent-call load first;
mdemg LLM hot paths are mostly serial; reversible instantly. Prompt still
grows later → the budget guard is the structural backstop (bounded by
construction), not a one-time trim.

## 11. Documents Accessed
`internal/ape/llm_reflector.go`, `internal/ape/types_rsic.go`,
`internal/api/server.go`, `internal/config/config.go` (compress + max-tokens
defaults); REWARD-CORRECTNESS-001 `live_findings.md`; live TSDB ape.reflect rows
(tokens_in/out distribution, section sizing); CLAUDE.md Local-LLM-Runtime note
(KV-slot math); llama-server launchd plist.

## 12. Rollback Procedures
All changes are config defaults + a reversible prompt-assembly path: set
`RSIC_LLM_REFLECT_INCLUDE_DATASETS=true` + `HISTORY_CYCLES=5` +
`PROMPT_BUDGET_TOKENS=0` (disable guard) to restore prior behavior. Lever B:
restore the prior plist `--parallel`/`--ctx-size` and reload. No schema, no
data migration; forward-only on new rows.
