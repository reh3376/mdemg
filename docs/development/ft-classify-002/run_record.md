# FT-CLASSIFY-002 — Run Record (6a instrumentation input)

Stage-by-stage record of the manual vertical slice, per the
FT-RECURSIVE-001 spec's phased plan (this is the evidence base for
phase 6a's instrumentation design).

| Stage | Started (ET) | Wall | Outcome | Failures/surprises |
|---|---|---|---|---|
| capture v1 | 2026-06-11 ~20:55 | 2.4 min (200 teacher calls) | REJECTED — 37/200 kept, ZERO none-class | **Root cause found**: `summary_quality` reward zero-credits correct-`none` (empty summary is correct BY SPEC) → the ≥0.8 filter rejected the entire dominant class. This, not sampling, was 11.5d's real failure mechanism. |
| reward fix | ~21:00 | — | `{type:none, summary:""}` → 1.0; 98 reward tests green | Invalidates ALL stored classify baselines (computed under biased reward) — gate must recompute fresh. |
| capture v2 | ~21:05 | 2.3 min | 200/200 kept, reward mean 0.981; train dist none 82/must 12/must_not 3/should 3/should_not 1 (±2pp of production) | toolchain gap: `psycopg` missing from neural venv (installed) |
| training env | ~21:15 | — | mlx-lm 0.31.3 reinstalled into neural/.venv | **`mlx_lm` was absent from every Python env on the machine** — the Phase 5/11.x training toolchain did not survive the Phase 13.5 serving decommission. Any retrain (incl. the future recursive loop) would have hard-blocked here. 6a must pin the training env (requirements file or dedicated venv + preflight check). |
| train | 21:30–21:43 | ~13 min (90 iters, 0.118 it/s) | val_loss 0.449→0.268 monotonic, train 0.200, peak mem 13.1 GB | far faster than 11.5d (109 min) — short classify prompts; resource model is task-dependent, not fixed |
| baseline (fresh) | 21:50–~22:30 | ~35-40 min | full 17-task sweep vs llama-server :8102, fixed reward | **`benchmark_phase10.yaml` still pointed at decommissioned port 8101** — un-overridden runs make zero calls and report aggregate 0.0000 (fixed + UBENCH sha re-pinned, commit 826cfb4). A real silent-failure: a plausible-looking 0.0 report with `specs_with_matched_rows 17/17`. |
| guardrail accumulation (Epic 4) | — | — | NOT a flag gap: `GUARDRAIL_ENABLED=true` live, 3 rows ever (2026-04-22) | The only producer is the MCP `validate_changes` tool, which no live workflow invokes. Follow-up: **GUARDRAIL-PRODUCER-001** (hook-side or workflow producer). No synthetic traffic faked. |
| candidate eval | pending | | mlx side-port (eval-only, 11.5d precedent) → same sweep | |
| gate + operator decision | pending | | | |

## 6a lessons (for FT-RECURSIVE-001)

1. Reward functions are gate infrastructure — class-conditional bugs in
   them silently shape every corpus and every baseline. The loop's gate
   must pin reward-function versions alongside prompt hashes.
2. Training-environment presence is a preflight check, not an assumption.
3. Endpoint configs rot at runtime cutovers — the loop's benchmark stage
   must hard-fail on 0 successful calls, never report 0.0000 as a score.
4. Stage timings vary by task ~10× (13 min vs 109 min training) — the
   compute lease should size from corpus stats, not a constant.
