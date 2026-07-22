# BENCH-SIDECAR-APPLY-001 — Sprint Post

**Shipped:** 2026-07-21 | closes the twice-disclosed manual-psql follow-up (FT-BENCH-REFRESH-001 + APE-REFLECT-EVAL-REFRESH-001)

## What shipped
- `persist.apply_to_tsdb(report, dsn, _connect)` — executes the V0012 INSERTs in one transaction; psycopg v3 preferred with psycopg2 fallback (deferred imports — soft dep); `_connect` test seam.
- `run_benchmark --apply-tsdb` — writes the sidecar first (audit artifact), then applies directly; failure is NON-FATAL (WARN + sidecar remains the recovery path, mirroring the UVTS best-effort pattern).
- `psycopg[binary]>=3.1` pinned in `neural/pyproject.toml` [training] extra + installed into `neural/.venv` via uv.
- 3 unit tests via the seam (executed+committed+closed; connect-failure raises; mid-tx failure still finally-closes). Full neural/benchmarks suite green.

## Live Tier-3 (E4)
1. First smoke run surfaced a REAL env gap: neither psycopg nor psycopg2 existed in system python OR neural/.venv — meaning the UVTS runner's `--persist-tsdb` had ALSO been silently skipping in this environment (its HAS_TSDB guard). The graceful-failure path fired exactly as designed (`WARN: tsdb apply failed (No module named 'psycopg') — apply the sidecar manually`).
2. Installed the driver (pinned) → re-smoke under `neural/.venv`: **`tsdb applied: 2 statements (run_id vse8lx4y…)`** — run + result rows landed with zero manual steps.
3. Restore-state cleanup (operator-confirmed; the pre-bash guard correctly required explicit confirmation): both smoke rows deleted by run_id in one transaction; `runs_total` back to 3; Latest Run panel back on `ed062ea8` (0.9188).

## Learning pinned
The env gap means historical `--persist-tsdb` invocations outside a psycopg-equipped environment silently produced sidecar-only output. `--apply-tsdb` + the pinned dep close both halves.

## Rollback
Flag is additive/default-off; revert per-commit.
