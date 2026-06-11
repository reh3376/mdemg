# MAINT-LIVE-001 — Verification (Tiers 1–3)

**Date:** 2026-06-11 · **Stack:** native `mdemg serve` + Docker Neo4j/TSDB.
Space `mdemg-dev`.

## Tier 1

`MaintenanceLivenessRules` tests (defaults, custom lookback, SQL content);
plist copies lint + identical (CI parity step); cli suite green; lint 0.

## Tier 2 — rule SQL on the real table, both branches

Pre-sprint state made the rule **born-firing (a true positive)**: 1
maintenance row existed (2026-06-08 — itself a FAILURE, `success=f`) and
zero live runs → CASE = 1. Post-live-run: CASE = 0.

## Tier 3 — the first live maintenance run in MDEMG's history

**Preview (dry-run):** 235,027 edges scanned / 0 prunable (correct — decay
had never run, nothing eroded below the 0.01 floor yet); 4,999 tombstone
candidates / ALL protected (90d recency shield + the operator's
`constraint,conversation_observation` exclusions); 20,236 orphan
SymbolNodes would be deleted (re-derivable code artifacts — the bloat
bulk). Preview's job event landed with `metadata.dry_run=true`.

**Live attempt 1 — REAL BUG CAUGHT (fix `d9d8c0e`):** Neo4j
`TransactionStartFailed` — both orphan sweeps ran their batched
CALL-IN-TRANSACTIONS deletes inside `ExecuteWrite` (explicit tx), which
Neo4j forbids. **The dry-run path never executes the deleting statement,
so no preview or unit test could ever surface this — only live
execution.** The failure simultaneously proved the NOSILENT chain
end-to-end: the dispatcher fired `Scheduled job failed: maintenance`
(with the real error text) AND the evaluator's `scheduled_job_recent_
failure` rule fired — both delivered to the operator through the
prompt-context hook (HOOKSYNC alert lifecycle).

**Live attempt 2 — success (120s):**
- 20,236/20,236 orphan SymbolNodes deleted (space symbol count
  35,911 → 15,675)
- 0 edges pruned, 0 nodes tombstoned / 5,010 protected (as previewed)
- Job event `success=t, dry_run=false`

**Post-run verification:**
| Check | Result |
|---|---|
| `maintenance_no_live_run` SQL | 1 → **0** (born firing, silenced by the real run) |
| Job-event metadata plumbing | 3 rows: preview/true ✓, failure/false ✓ (alerted), success/false ✓ |
| Retrieval control probe | 0.431 (healthy RRF band) |
| Null-weight gauge | 0 |
| Installed plist | `--dry-run=false` + exclusions, agent bootstrapped |

## Operator decisions recorded

Orphan disposition is context-dependent (operator, 2026-06-11): schedule
excludes `constraint` (governance rules — never auto-tombstone) and
`conversation_observation` (history vs test junk differ by SESSION, which
a role-level knob cannot express — conservative exclusion until
session-aware policy exists). Aged hierarchy debris stays eligible.
Census that drove it: 5,388 conv-obs candidates (9 eligible same-day under
the 90d shield), 11 constraints, 238 hierarchy nodes.
