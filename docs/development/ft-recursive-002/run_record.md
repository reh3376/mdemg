# FT-RECURSIVE-002 Epic 6 — Run Record (Option A: orchestration + FAIL-path)

2026-06-29. The first time the recursive-retrain controller ran a **real cycle
end-to-end** against the live system. Scope = validate the controller's live
machinery + the FAIL path (the full successful-cycle pipeline automation is the
6b continuation — see `epic6_issues.md` E6-3/4/5).

## Preflight (all green)
`mlx_lm 0.31.3` in `neural/.venv`; 862 GB free; llama-server `:8102` → 200;
`consulting.classify` Ready (4782 rows).

## Stages observed
| t (ET) | Event | Notes |
|---|---|---|
| 09:30:27 | Gate opened cycle `h1pb0cnya605…` | RSIC insight #29 (`consulting.classify` Ready) → `RSIC trigger gate: retrain cycle opened` |
| 09:30:41 | Controller → `curating` | picked up within the 15s poll; lease acquired, RSIC quiesced |
| 09:30:41 | `curate` ran, exit 2 → `failed` | `paradigm_router` usage error: missing `--spec/--input-dir/--output-dir` — **the module resolved + executed** (E6-1 venv + E6-2 cwd validated) |
| 09:30:41 | FAIL handling | ledger `failed`; `ft-loop:curate` jobhealth failure row; 2 distinct-`ft-loop` alerts (job high + cycle medium); lease released |
| ~09:31+ | Re-trigger suppressed | next RSIC cycle → interval gate → no new cycle (1 cycle total; SF-2 holds) |

## Exit criteria
- [x] A real cycle runs end-to-end through the controller (trigger→open→pick-up→stage)
- [x] **FAIL path** verified live (ledger `failed` + one-per-concern `ft-loop` alert, no promotion)
- [x] **Zero alert spam** — `rsic-trigger_training_pipeline` suppressed; interval gate bars re-triggers
- [x] Lease acquire + **release** (lockfile gone post-failure)
- [x] E6-1 (venv python) + E6-2 (per-stage cwd) — module resolved + ran
- [~] Quiesce-under-contention — code ran (set+clear) but not exercised (sub-second fail; E6-6)
- [ ] **Successful cycle → `promote_pending`** — needs the real arg-sets + train→convert→gate chain (E6-3/4/5, 6b continuation)

## Disposition
Controller restored to **dormant** (`FT_LOOP_ENABLED` unset); the test cycle +
jobhealth rows cleaned (a failed cycle's start-time would otherwise suppress
real triggers for 168h). Issues E6-1/E6-2 fixed + validated; E6-3..E6-7 ledgered
for the 6b continuation / future.
