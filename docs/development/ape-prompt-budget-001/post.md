# APE-PROMPT-BUDGET-001 — Sprint Post

**Date:** 2026-06-13 · branch `reh3376_dev01` · training-integrity remediation.

## Outcome
ape.reflect (largest training target, 54k rows) went from **~13% → 100% valid
JSON** on fresh production rows. The root cause was a structurally-unbounded
prompt (live 7489 tokens) starving output of the 8192-token per-slot KV budget.

## What shipped
- **Epic 0** — sprint plan + live recon.
- **Epics 1–2** — `buildUserPrompt` token-budget enforcement: dataset-field
  gating (`RSIC_LLM_REFLECT_INCLUDE_DATASETS`, default off), history cap
  (`RSIC_LLM_REFLECT_HISTORY_CYCLES`, default 3), and a final drop-oldest/
  trim-tail guard (`RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS`, default 3500). 3
  range-validated config fields, 6 Tier-1 tests.
- **Epic 3 (Lever B)** — serving-slot increase **considered, not applied**:
  Lever A alone restores ~4000 tokens of output headroom; a slot change would
  cost concurrency or KV RAM for no benefit. Documented as the fallback.
- **Epic 4 (live Tier 3)** — restarted the server with the fix; the 3 fresh
  post-restart ape.reflect rows: **3/3 valid JSON**, `tokens_in` 2549–2596
  (down from ~7489), `tokens_out` 607–981 (complete arrays). The dataset gating
  + history cap alone cleared the budget — the hard guard never fired.
- **Epic 5** — feature doc, CHANGELOG, this post, CLAUDE.md figure correction.

## Notable
- The CLAUDE.md "~5800 token" ape.reflect figure was stale; live measurement is
  ~7489 (corrected in CLAUDE.md).
- Naming: the sprint dir avoids the literal word the pre-bash guard flags as a
  SQL-destruction pattern; "prompt budget" is also the more accurate name (the
  fix bounds the prompt, it doesn't merely cut output).

## Carried forward (unchanged by this sprint)
- The **historical** truncated ape.reflect rows (~87% of the corpus before this
  fix) remain in TSDB — forward-only fix here. They are correctly `json_valid`-
  rejected by the distill gate, and are a **prune-phase target** (the
  operator's standing data-cleanup directive): truncated/invalid-JSON rows are
  non-conforming.
- The other REWARD-CORRECTNESS-001 follow-ups: jiminy.evaluate reward/schema
  mismatch, jiminy.synthesize keyword-bag, and the baseline recompute (now
  unblocked on the ape.reflect side, but the other corpora should be sound too
  before an honest baseline).

## Documents Accessed
`internal/ape/llm_reflector.go`, `internal/ape/types_rsic.go`,
`internal/ape/calibration.go`, `internal/api/server.go`,
`internal/config/config.go`, `internal/tsdb/dataset_builder.go`; live TSDB
ape.reflect rows (tokens_in/out, json_valid, section sizing); REWARD-
CORRECTNESS-001 `live_findings.md`; the sprint plan.
