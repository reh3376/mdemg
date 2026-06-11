# DOC-TRUTH-001 — Sprint Close

**Date:** 2026-06-11 · **Branch:** `reh3376_dev01` · **Roadmap:** Q3 Phase 1, rank #6 — **PHASE 1 COMPLETE**

## What shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Plan + 8-item verification sweep (every claim re-verified against code/files before editing) | `616d0c2` |
| 1 | CLAUDE.md FT section → post-pivot reality; guardrail exception CLOSED; memo-07 provenance break disclosed; adapter env drift fixed | `af92057` |
| 2 | Preflight errors → llama-server guidance (were directing operators to the decommissioned mlx stack); model.go stale text; `.env` J17 200ms override removed (operational, live-verified restart) | `4d60865` |
| 3 | 00_README STATUS block (shipped/superseded/NOT-STARTED + R-LT-4 adjudication + provenance); AGENT_HANDOFF retired | `75d9400` |
| 4 | Grep-sweep proof + CHANGELOG + close | (this) |

## Verification

Grep-sweep across living docs + CLI strings: 0 hits for "queued for
Sprint FT-LORA-B", "MDEMG_MODEL_ADAPTER_BASE", "deferred to
MODEL-DIST-002", the MoE base-model-as-current text, and the memo-07
canonical citation. Server restarted clean with the .env fix.
The supersede-with-pointer pattern preserved all true history.

## Why it mattered

The Q3 deep-dive audit consumed CLAUDE.md's FT section and initially
reasoned about a dead architecture — stale docs are not cosmetic debt,
they contaminate planning. The worst instance was operator-facing:
preflight errors instructed starting the crash-looping stack Phase 13.5
decommissioned.

## Documents Accessed

CLAUDE.md; `internal/guardrail/llm_evaluator.go`; `.env`;
`internal/cli/{model,preflight_mlx}.go`;
`docs/development/ft-lora/00_README_v2.md` + `phase_5_sft_post.md`;
`AGENT_HANDOFF.md`; roadmap.
