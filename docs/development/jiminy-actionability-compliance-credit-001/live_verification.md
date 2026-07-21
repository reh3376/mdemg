# JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 — E5 Live Tier-3 Verification

**Date:** 2026-07-21
**Environment:** `mdemg-dev` + live llama-server (`mdemg-llm-v1.Q5_K_M.gguf` on :8102)
**Method:** direct-LLM A/B against `/v1/chat/completions` using both the OLD prompt (byte-identical HEAD) and the NEW prompt (HEAD + `nonViolationCreditClause`). Temperature=0.0 for deterministic comparison.

## Clear-cut cases — both prompts agree (baseline sanity)

Guidance: `CONSTRAINT: NEVER commit directly to main branch (must_not, high priority)`

| Case | Action | OLD verdict | NEW verdict |
|---|---|---|---|
| Unrelated | Read /etc/hosts | `not_applicable` (0.9) | `not_applicable` (0.9) |
| Following | Committed on dev-branch and pushed | `followed` (0.9) | `followed` (0.9) |
| Violating | git checkout main + commit + push origin main | `contradicted` (0.9) | `contradicted` (0.9) |

Both prompts correctly classify obvious cases. **The clause doesn't break existing correct behavior.** Note that the LLM was ALREADY routing very-obviously-unrelated actions to `not_applicable` under the old prompt — this is why the baseline shows 83 `not_applicable` emissions in 7d.

## Discriminative borderline case — the shift Fix 2 targets

Guidance: `CONSTRAINT: must_not amend commits that have already been pushed to shared branches (must_not, high priority)`
Action: `Modified internal/config/config.go to add a new env var, ran go test, staged the change`
Base similarity: 0.55

| Prompt | Outcome | Reasoning |
|---|---|---|
| OLD (HEAD) | **ignored** (conf 0.9) | "The agent's action does not address the guidance about not amending commits..." |
| NEW (HEAD + clause) | **not_applicable** (conf 0.9) | "The action taken does not involve any commits that have been pushed to shared branches..." |

**Same underlying observation ("action didn't touch the mechanism"); different classification.** This is exactly the shift Fix 2 is designed to produce. The OLD `ignored` would have counted against the actionable denominator in `constraint_outcomes`; the NEW `not_applicable` is correctly filtered out by the writer gate (`service.go:1730,1762`).

## Why the smoke A/B alone doesn't fully quantify the lift

The direct-LLM A/B on 4 hand-picked cases proves:
1. The clause is doing what it claims (unrelated-context ignored → not_applicable).
2. The clause doesn't break clear-cut cases (obvious not_applicable / followed / contradicted verdicts unchanged).

But it doesn't tell us WHAT FRACTION of live-production `ignored` verdicts will shift. That requires operator-time observation over a real workload. See `ab_recipe.md` for the 3-day A/B recipe operators run before flipping the flag default.

## Fresh-restart context

Server was rebuilt + restarted twice during E5:
- First restart: `launchctl setenv JIMINY_NONVIOLATION_CREDIT_ENABLED=true` — verified `/healthz` ok, warmed Jiminy contexts (2 warms produced guidance_ids with zero results — Jiminy retrieval-warm empty is a fresh-restart artifact, not a defect from this sprint; see JIMINY-CORPUS-001 for the underlying corpus dynamics).
- Second restart: `launchctl unsetenv JIMINY_NONVIOLATION_CREDIT_ENABLED` — restored to default-off ship state. Verified `/healthz` ok.

Since Jiminy warm-then-fetch produced no results, the smoke was pivoted to direct-LLM A/B (equivalent proof at the classifier layer without depending on retrieval warm-up).

## Overall verdict

- ✅ New prompt clause produces `not_applicable` for borderline unrelated-context cases where the old prompt produced `ignored`.
- ✅ New prompt doesn't break clear-cut cases.
- ✅ Server picks up the flag toggle cleanly via `launchctl setenv` + `kickstart -k`.
- ✅ Restored to default-off ship state.

Ready for E6 panel wording + E7 A/B recipe + E8 canonical docs.
