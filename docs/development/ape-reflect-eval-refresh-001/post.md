# APE-REFLECT-EVAL-REFRESH-001 — Sprint Post

**Shipped:** 2026-07-21 | **Result: ape.reflect 0.623 → ~0.94; aggregate 0.8544 → 0.9188**

## Epics
E0 plan · E1 recon (5,487 clean post-fix candidates; old rows ~4,000 est. tokens) · E2 reproducible refresh script + splice (backup retained) · E3 leak-audit CLEAN 0/240 · E4 [AMD-2] re-pin with amendment entry · E5 benchmark re-run + sidecar apply · E6 verification (live_verification.md) · E7 docs (this).

## The learning worth keeping
An eval frozen from production captures the production PATHOLOGIES of its era. valid_clean's ape.reflect rows froze pre-APE-PROMPT-BUDGET-001 unbounded prompts; when serving moved to bounded KV slots, those rows measured the serving constraint, not the model. **When production behavior is intentionally changed (prompt budgets, serving limits), audit the pinned evals for rows that embody the OLD behavior.**

## Rollback
Restore `.pre_ape_refresh_bak` + revert the manifest amendment commits.

## Note
The sidecar-apply automation gap (watcher cwd quirk) reconfirms the FT-BENCH-REFRESH-001 follow-up: the runner should gain a direct-INSERT mode or an auto-apply step (FT-RECURSIVE-002 scope).
