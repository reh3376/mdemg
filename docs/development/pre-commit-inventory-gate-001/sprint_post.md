# PRE-COMMIT-INVENTORY-GATE-001 — Sprint Post

**Date:** 2026-08-13
**Branch:** `reh3376_dev01`
**Trigger:** Operator directive 2026-08-13, after PR 615 CI failure: "Jiminy should catch this before commit." Same class as PR 614 (route inventory miss) — both were preventable at commit time, not just at CI.

## Problem

Both recent CI failures had the same shape:
- **PR 614**: `HITL-AUTOGRADE-PREVIEW-001` shipped `POST /v1/review/autograde-preview` without paired `docs/api/route_consumer_inventory.json` adjudication → CI Lint FAIL (`Route consumer guard`)
- **PR 615**: `RERANK-LENGTH-STRICT-001` shipped `mdemg_retrieval_rerank_length_mismatch_total` counter without paired `docs/api/metrics_consumer_inventory.json` adjudication → CI Lint FAIL (`metrics: 145 declared, 144 inventoried; DRIFT`)

Both DORMANT-CENSUS-001 / DORMANT-CENSUS-003 forcing functions fired correctly at CI time — but only AFTER the operator had to approve a PR with a red-line, wait for CI to run, then push a fix-commit. Wasted round-trip. The verifiers are already local; the miss is catchable at `git commit` time.

## Shipped

**`.claude/hooks/pre-bash-check.py`** + **`internal/cli/hook_templates/pre-bash-check.py`** (HOOKSYNC-001 parity preserved):

- New `_INVENTORY_CHECKS` table mapping trigger-file regex → verifier script → inventory file → human-readable kind:
  - `internal/api/server.go` → `verify_route_consumers.py` → `route_consumer_inventory.json` (kind: "route")
  - `internal/metrics/collectors.go` → `verify_metrics_consumers.py` → `metrics_consumer_inventory.json` (kind: "metric")
  - `internal/tsdb/migrations/\d+_[\w-]+\.sql` → `verify_tsdb_consumers.py` → `tsdb_consumer_inventory.json` (kind: "tsdb table")
- New `check_inventory_adjudication(command)` function:
  - Returns `None` (allow) for non-`git commit` commands
  - Runs `git diff --cached --name-only`; on git-error → **fail-CLOSED** with clear message ("refusing to commit blind")
  - Filters staged files through each trigger regex
  - For each triggered check: runs the verifier script; on returncode != 0 → returns a full BLOCK reason with the verifier output + the exact 4-step fix (--generate → adjudicate → git add → commit again) + sprint reference
- Wired into `main()` between destructive-guard (#1) and jiminy-classify (#3) — new #2 slot
- Uses `_deny(...)` (existing hook shape) so the deny surfaces to the LLM with Claude Code's standard permission-denied UX

**Fail-CLOSED philosophy** (deliberate inversion of JIMINY-ENFORCE-002's fail-open):
- Fail-open (Jiminy-classify): unreachable server → allow, so tools don't wedge
- Fail-closed (inventory gate): unverifiable inventory → block, so silently-broken commits can't land. "Can't confirm inventory is honest" is the same signal as "inventory IS broken" for merge safety.

**`--no-verify` has no effect** — this hook runs at PreToolUse Claude-side, not git's own pre-commit surface. Deliberate: the operator's bypass path is `git commit --no-verify` for git-hooks; the Claude-side hook must remain the always-on invariant.

## Live Tier-3 (mdemg-dev, 2026-08-13)

Two synthetic scenarios:

**Happy path** (nothing suspicious staged):
- Hook invoked with `git commit -m "test"` input → exit 0, no stdout → command allowed silently ✓

**Fail path** (fake metric `pcig_test_synthetic_total` injected into `internal/metrics/collectors.go` + staged):
- Verifier directly: `FAIL: 1 metric(s) declared in Go but absent from the inventory`
- Hook gate: exit 0 with `decision: deny` and reason containing:
  - `COMMIT BLOCKED — metric inventory adjudication missing.`
  - Full verifier output
  - The 4-step fix path (--generate → adjudicate → git add → commit again)
  - Sprint reference `(PRE-COMMIT-INVENTORY-GATE-001 — enforces DORMANT-CENSUS-001/002/003 forcing functions BEFORE commit lands, not at CI.)`
- Cleanup: `collectors.go` restored + index reset + `git checkout --` → repo clean

## One arch rule pinned (CLAUDE.md)

**When adding a new DORMANT-CENSUS-\* forcing function (a new trigger-file → verifier → inventory triple), extend `_INVENTORY_CHECKS` in BOTH `.claude/hooks/pre-bash-check.py` AND `internal/cli/hook_templates/pre-bash-check.py`** (HOOKSYNC-001 parity contract) in the SAME PR. Without the hook extension, the CI check catches the miss only at merge time — the entire class of PR-614/PR-615 late-CI-failures returns. Follow the shipped pattern: trigger regex + verifier path + inventory path + human-readable kind. The gate is `fail-CLOSED` — verifier-missing / git-error / etc. → BLOCK with a clean "refusing to commit blind" message, mirror-inverting JIMINY-ENFORCE-002's fail-open. `--no-verify` has no effect on this Claude-side hook.

## Follow-ups disclosed

- **Constraint-node recording** — a durable rule via `POST /v1/conversation/observe` naming the pattern "adding X requires paired inventory adjudication" would give Jiminy's LLM classifier additional signal on similar future patterns. Deferred — the deterministic hook gate is the primary defense; the classifier is defense-in-depth.
- **Extend to other forcing functions as they land** — future DORMANT-CENSUS-* sprints (config_consumers, ULTS specs, etc.) SHOULD extend `_INVENTORY_CHECKS`. The rule-pin above enforces this.
- **CI parity check** — future CI addition: verify that live hook + template are byte-identical modulo the SPACE_ID placeholder (HOOKSYNC-001) after any hook edit. Currently manual per-sprint; a `verify_hook_parity.sh` step would automate.
- **Verifier-timeout hardening** — 30s subprocess timeout should be enough for all shipped verifiers; if a future verifier is slower, bump. If a verifier hangs indefinitely, the gate fails-closed (correct).

## Documents Accessed

- `CLAUDE.md` pins: DORMANT-CENSUS-001 (route inventory forcing function), DORMANT-CENSUS-002 (TSDB inventory), DORMANT-CENSUS-003 (metrics inventory), HOOKSYNC-001 (hook parity contract), JIMINY-ENFORCE-002 (fail-open shape this inverts)
- `.claude/hooks/pre-bash-check.py` — edit site
- `internal/cli/hook_templates/pre-bash-check.py` — template mirror (HOOKSYNC-001 parity)
- `scripts/verify_route_consumers.py` + `verify_metrics_consumers.py` + `verify_tsdb_consumers.py` — the 3 shipped verifiers this gate invokes
- Task #114 (JIMINY-RULES-UI-001) sprint plan — the ongoing sprint whose next epic (Epic 2 backend endpoints) will exercise this gate
- Live: PR 614 + PR 615 CI failure logs (the two class-instances that motivated this sprint); synthetic-inject test with real metric declaration; cleanup verification
