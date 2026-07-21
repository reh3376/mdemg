# JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 — Operator 3-day A/B Recipe

**When to run this**: after the sprint's PR lands (which ships the code default-off), before flipping `JIMINY_NONVIOLATION_CREDIT_ENABLED=true` in `.env`.

**Why**: the sprint's E5 direct-LLM smoke proved the prompt does the right thing on a discriminative borderline case (see `live_verification.md`), but the LIVE-WORKLOAD lift needs measurement against real production classifications over ≥3 days. This recipe gives you the SQL + procedure.

## Step 1 — Record the PRE-flip baseline (T−7d to T)

Capture the current 7-day window state RIGHT BEFORE you flip the flag. Run against `mdemg-timescaledb-1` (or the operator's TSDB pod):

```sql
-- Baseline snapshot (label as t=0 in your notes)
SELECT 'baseline_pre_flip' AS phase,
       COUNT(*) AS actionable_total,
       SUM(CASE WHEN outcome_type='followed' THEN 1 ELSE 0 END) AS followed,
       ROUND(100.0*SUM(CASE WHEN outcome_type='followed' THEN 1 ELSE 0 END)/NULLIF(COUNT(*),0)::numeric, 2) AS follow_pct
FROM constraint_outcomes
WHERE space_id='mdemg-dev' AND time > NOW() - INTERVAL '7 days'
  AND guidance_type IN ('constraint','correction');

-- LLM classifier not_applicable emission rate (pre-flip)
SELECT
  COUNT(*) FILTER (WHERE response ~ '"outcome":\s*"not_applicable"') AS na,
  COUNT(*) FILTER (WHERE response ~ '"outcome":\s*"ignored"') AS ignored,
  COUNT(*) FILTER (WHERE response ~ '"outcome":\s*"followed"') AS followed,
  COUNT(*) AS total
FROM llm_interactions
WHERE task_name='jiminy.evaluate_llm' AND time > NOW() - INTERVAL '7 days';
```

Save the numbers.

## Step 2 — Flip the flag

Choose ONE of:

**Option A (persistent — recommended)**: add to `.env`:
```
JIMINY_NONVIOLATION_CREDIT_ENABLED=true
```
Then restart the mdemg server: `launchctl kickstart -k gui/$(id -u)/com.mdemg.server`.

**Option B (session — for the 3-day window only)**:
```
launchctl setenv JIMINY_NONVIOLATION_CREDIT_ENABLED true
launchctl kickstart -k gui/$(id -u)/com.mdemg.server
```
When you `launchctl unsetenv JIMINY_NONVIOLATION_CREDIT_ENABLED` + kickstart, the flag reverts to default-off.

**Verify** the flag is active in server logs:
```bash
grep JIMINY_NONVIOLATION_CREDIT_ENABLED ~/.mdemg/logs/server.log | tail -3
```

## Step 3 — Live workload for ≥3 days

Use MDEMG normally. Every `/v1/jiminy/feedback` call now runs the classifier through the expanded prompt. Give it at least 3 full days to build up a meaningful post-flip sample (target: ≥200 tier-2 LLM classifications).

## Step 4 — Compare (T+3d)

```sql
-- Post-flip snapshot (same queries as Step 1, run 3 days after the flip)
SELECT 'post_flip_3d' AS phase,
       COUNT(*) AS actionable_total,
       SUM(CASE WHEN outcome_type='followed' THEN 1 ELSE 0 END) AS followed,
       ROUND(100.0*SUM(CASE WHEN outcome_type='followed' THEN 1 ELSE 0 END)/NULLIF(COUNT(*),0)::numeric, 2) AS follow_pct
FROM constraint_outcomes
WHERE space_id='mdemg-dev' AND time > NOW() - INTERVAL '3 days'
  AND guidance_type IN ('constraint','correction');

SELECT
  COUNT(*) FILTER (WHERE response ~ '"outcome":\s*"not_applicable"') AS na,
  COUNT(*) FILTER (WHERE response ~ '"outcome":\s*"ignored"') AS ignored,
  COUNT(*) FILTER (WHERE response ~ '"outcome":\s*"followed"') AS followed,
  COUNT(*) AS total
FROM llm_interactions
WHERE task_name='jiminy.evaluate_llm' AND time > NOW() - INTERVAL '3 days';
```

## Step 5 — Interpret

Compare pre vs post. **Success criteria** (from `baseline.md`):

| Signal | Predicted post-flip | Interpretation |
|---|---|---|
| Actionable follow rate | 18-25% (from baseline 10%) | ✅ predicted lift materialized — the fix is doing its job |
| LLM `not_applicable` emissions | +~50% | ✅ prompt IS routing more borderline cases to not_applicable |
| Total actionable-outcome volume in `constraint_outcomes` | drops ~40% | ✅ correctly-filtered rows aren't reaching the denominator |

**Tripwires** (revert flag if any fire):

| Tripwire | Meaning | Action |
|---|---|---|
| Actionable follow rate >50% | LLM over-crediting non-application; too many real "should have followed" verdicts routed to not_applicable | Revert flag; investigate whether the clause is too permissive |
| `contradicted` verdicts drop >30% | The clause is being over-applied to genuine violations | Revert flag |
| Total tier-2 LLM latency rises >20% | Longer prompt = slower classification | Not a hard revert; note for the CompressPrompts variant |

## Step 6 — Decide

- **Lift within 18-25% band + no tripwires**: keep the flag ON in `.env`; the fix is doing what it was designed to do.
- **Lift <5pp**: the clause isn't producing the expected shift on your specific workload. Two possibilities: (a) your constraint corpus doesn't have many "borderline unrelated" cases; (b) the LLM isn't applying the clause. Sample 10-20 recent LLM verdicts on constraint outcomes and read their reasoning; check whether the classifier acknowledged the new rule.
- **Tripwire fired**: revert flag, document what you observed, and open a follow-up (either tune the prompt clause or defer to the next-stage fix per the fix_spec.md).

## Rollback (safe at any time)

```
# Option A rollback
sed -i '' '/JIMINY_NONVIOLATION_CREDIT_ENABLED/d' .env
launchctl kickstart -k gui/$(id -u)/com.mdemg.server

# Option B rollback
launchctl unsetenv JIMINY_NONVIOLATION_CREDIT_ENABLED
launchctl kickstart -k gui/$(id -u)/com.mdemg.server
```

No data changes. Rows written under flag-on stay in `constraint_outcomes` as they were classified — flipping off doesn't retroactively re-classify.

## Cache caveat

The classifier caches verdicts by `(item.Content, actionSummary)` (LRU, capacity `JIMINY_OUTCOME_CACHE_SIZE`, default 256). Same-action rows will replay the cached verdict from before the flip until eviction. In practice the cache turns over quickly (256 entries at 700 classifications/week = ~2-day turnover). But if you want a clean cutover: restart the server WHEN you flip (already required for the flag itself to take effect).
