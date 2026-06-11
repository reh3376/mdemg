# Sprint Plan DOC-TRUTH-001 — Documentation Matches Reality

## 1. Header & Metadata
- **Sprint ID:** DOC-TRUTH-001 — Q3 Phase 1, rank #6 (final Phase-1 sprint)
- **Line:** `docs/development/doc-truth-001/` · **Date:** 2026-06-11 · **Branch:** `reh3376_dev01`
- **Target:** v0.10.x · **Effort:** ~2d budgeted (docs-only) · **Spend:** $0 · **Risk:** Low

## 2. Problem Statement
Stale documentation seeded the Q3 roadmap audit with a dead architecture:
CLAUDE.md's FT-LORA section still presents the Qwen3.6-35B-A3B MoE target,
two-tier MoE-Sieve strategy, and Sprint A→E sequence as CURRENT — all
superseded by the 2026-04-22 MoE→dense pivot (00_README v5.6: Metal 499K
ceiling; dense Qwen3-14B shipped through Phase 13.5). Verified-stale items:
1. CLAUDE.md FT section (dead MoE plan presented as current).
2. CLAUDE.md guardrail note ("bypasses llmclient … queued for FT-LORA-B")
   — the migration SHIPPED (`internal/guardrail/llm_evaluator.go` imports
   and dispatches through llmclient).
3. `.env` pins `J17_SIDECAR_TIMEOUT_MS=200` — the exact value DH-004
   remediated (default now 1000, floor 100); the local override re-breaks it.
4. CLAUDE.md cites memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` as canonical
   — the file does not exist in the repo (provenance break).
5. CLAUDE.md env list reads `MDEMG_MODEL_ADAPTER_BASE`; code uses
   `MDEMG_ADAPTER_BASE`. `model.go` help text still says the adapter path is
   "deferred to MODEL-DIST-002" — which shipped 2026-05-25.
6. `AGENT_HANDOFF.md` (repo root) is a month-stale handoff (2026-05-06)
   presented as live.
7. `preflight_mlx.go` user-facing error text says "mlx probe" though the
   probe is backend-agnostic (llama-server) — minor text correction
   (package/file names stay per the Phase 13.6 operator-invisible rule).
8. Spec §6.6 / R-LT-4 adjudication + FT-2/FT-3 rationale: the spec file is
   UNTRACKED (`docs/research/mdemg_sprint_ideas/`) — notes land in the
   tracked `00_README_v2.md` (canonical FT doc) with the untracked status
   disclosed; full spec-tracking disposition stays with FG-2 (deferred).

## 3. Scope & Constraints
Docs + comments + one .env line + user-facing strings only — NO functional
code changes. Tier 3 for a docs sprint = grep-verification that no stale
assertion survives + the .env change observed live (sidecar timeout log).

## 4. Dependencies
00_README_v2.md v5.12 + phase_5_sft_post.md (ground truth); deep-dive
verification (claims re-verified tonight against code).

## 5. Implementation Plan
- **Epic 0** — plan + verification sweep (done; all 8 items confirmed).
- **Epic 1** — CLAUDE.md: rewrite the FT-LORA section to post-pivot
  reality (dense Qwen3-14B shipped; MoE plan marked superseded with the
  pivot rationale + pointer); fix the guardrail note; fix the adapter env
  name; replace the memo-07 citation with 00_README v5.6 + the absence
  disclosure.
- **Epic 2** — `.env`: drop the stale `J17_SIDECAR_TIMEOUT_MS=200`
  override (default 1000 applies); `model.go` stale "deferred" help text;
  `preflight_mlx.go` user-facing "mlx" → "LLM endpoint" wording.
- **Epic 3** — `00_README_v2.md`: top-of-file STATUS block (line complete
  through Phase 13.5; recursive-retraining Phases 6/7/9 NOT STARTED with
  the FT-CLASSIFY-002 trigger; R-LT-4 adjudication note; FT-2 skip / FT-3
  supersession rationale; spec-untracked disclosure). `AGENT_HANDOFF.md`
  → superseded stub pointing at CLAUDE.md + the roadmap.
- **Epic 4** — close: CHANGELOG; grep-sweep proves no stale assertions
  remain; live sidecar-timeout observation; post.md; push.

## 6. Testing Plan
Tier 1: n/a (no code). Tier 2: grep-sweep for every stale phrase
(Qwen3.6-35B as current, "queued for Sprint FT-LORA-B", memo-07 citation,
MDEMG_MODEL_ADAPTER_BASE, "deferred to MODEL-DIST-002"). Tier 3: restart
sidecar-consuming path NOT needed — the .env var is read by the SERVER:
restart server, observe the J17 sidecar timeout in effect (log/config) at
1000ms.

## 7. Commit Strategy
One commit per epic; push → auto-PR → summary.

## 8. Verification Checklist
- [ ] Zero grep hits for the 5 stale phrases in tracked docs
- [ ] Guardrail note states the migration shipped
- [ ] .env override gone; server runs with the 1000ms default (live)
- [ ] 00_README STATUS block present; recursive-loop trigger named
- [ ] AGENT_HANDOFF superseded stub
- [ ] CHANGELOG updated; post.md complete

## 9. Documentation Update — the sprint IS the documentation update.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Rewrite erases true history | L | M | Supersede-with-pointer pattern: the MoE plan stays documented as superseded, never deleted |
| .env change shifts live behavior unexpectedly | L | L | DH-004 default (1000ms) is the TESTED value; 200ms was the bug; observe live after restart |

## 11. Documents Accessed
CLAUDE.md (FT section, guardrail note, env list); `internal/guardrail/
llm_evaluator.go`; `.env`; `internal/cli/model.go`, `preflight_mlx.go`;
`docs/development/ft-lora/00_README_v2.md` (v5.6/v5.12), `phase_5_sft_post.md`;
`AGENT_HANDOFF.md`; roadmap.

## 12. Rollback
Revert commits (docs only).
