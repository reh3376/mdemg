# JIMINY-ACTIONABILITY-001 — Baseline Composition (Epic 1)

The before-state, from the live `constraint_outcomes` sink (read-only). The A/B
(Epic 4) compares against this. **Levers off** (defaults) when captured.

## Outcome-side composition + ignore/follow by `guidance_type` (live `mdemg-dev`, 30-day window)

| guidance_type | n | % of total | % ignored | % followed | class |
|---|---|---|---|---|---|
| pattern | 1113 | 40.5% | 64% | 14% | abstraction |
| learning | 1049 | 38.1% | 54% | 9% | abstraction |
| concept | 324 | 11.8% | 66% | 19% | abstraction |
| **constraint** | 149 | 5.4% | **35%** | 17% | actionable |
| **correction** | 115 | 4.2% | **27%** | 2% | actionable |

- **Abstraction class** (pattern+learning+concept) = **90.4%** of outcomes, ignored **54–66%**.
- **Actionable class** (constraint+correction) = **9.6%**, ignored **27–35%** (~½ the abstraction ignore rate).

This reproduces the Step-1 diagnostic's Findings 2/3/5 on current data: the surfaced set is ~90% non-actionable abstractions, and those are the ones ignored. The lever this sprint pulls — bias the **surfaced** set toward the actionable class — is measured by the new gauge `mdemg_jiminy_surfaced_actionable_fraction` (the surfaced side; this table is the outcome side, the downstream signal the win must move).

## SQL (reproducible)
```sql
WITH c AS (
  SELECT guidance_type, count(*) n,
         count(*) FILTER (WHERE outcome_type='followed') f,
         count(*) FILTER (WHERE outcome_type='ignored')  ig
  FROM constraint_outcomes WHERE time > now() - interval '30 days'
  GROUP BY 1)
SELECT guidance_type, n,
       round(100.0*n/sum(n) OVER(),1)  AS pct_of_total,
       round(100.0*ig/n,0)             AS pct_ignored,
       round(100.0*f/n,0)              AS pct_followed,
       CASE WHEN guidance_type IN ('constraint','correction') THEN 'actionable' ELSE 'abstraction' END AS class
FROM c ORDER BY n DESC;
```

## A/B verdict rule (Epic 4)
Levers-on **Arm B** must show:
1. **surfaced abstraction fraction** (gauge) drops from ≈90% toward a target (first milestone ≤ 60%), AND
2. **follow-rate rises / ignore-rate falls** in `constraint_outcomes` over the window, AND
3. **no actionable-set follow-rate regression** (constraint/correction's own follow-rate doesn't drop).

Composition shift alone is **not** a win — both (1) and (2) are required.
