# GitHub Support Report — Push-event workflow triggers silently dropped

**Repository:** `reh3376/mdemg` (public)
**Date of incident:** 2026-08-06
**Affected workflows:** ALL push-triggered workflows (`auto-pr.yml`, `ci.yml`, `branch-naming.yml`, `docker-publish.yml`, `uxts-canonical-specs.yml`)
**Affected branch:** `reh3376_dev01`
**Impact:** Blocks the standard dev-branch → auto-PR → CI → review flow. Maintainer workaround (manual `gh pr create`) has the side-effect that PR author = maintainer, so branch-protection self-approval rule blocks merge without admin bypass.

---

## Summary

Starting at ~18:30 UTC on 2026-08-06, GitHub Actions began silently dropping `push` events on this repository for the `reh3376_dev01` branch. Every push after `c28d119a` (the last successful auto-trigger at 18:50:32 UTC) has:
- Registered on the repository's public `/events` API as a `PushEvent` with `actor: reh3376`
- Created ZERO workflow runs (`/repos/reh3376/mdemg/actions/runs?head_sha=<sha>` returns empty for every affected SHA)

Meanwhile, `pull_request` triggered workflows on the same branch continue to fire normally, and `pull_request` merge-triggered workflows (`sync-dev-after-merge.yml`) also work fine. Only `push` event delivery is affected.

## Timeline of dropped push events

All pushes below returned `git push` success codes and appear in `GET /repos/reh3376/mdemg/events`. None triggered any workflow runs.

| Timestamp (UTC) | SHA | Actor | Auto-PR triggered? |
|---|---|---|---|
| 2026-08-06T18:20:33Z | `c28d119a` | reh3376 | ✅ **LAST successful trigger** (workflow started 30 min later at 18:50:32Z) |
| 2026-08-06T18:26:17Z | `65d36e13` | reh3376 | ❌ silent drop |
| 2026-08-06T18:31:20Z | `2ebe3a63` | reh3376 | ❌ silent drop |
| 2026-08-06T18:42:07Z | `9c489932` | reh3376 | ❌ silent drop |
| 2026-08-06T20:54:39Z | `2e00d6bb` | reh3376 | ❌ silent drop |
| 2026-08-06T21:04:18Z | `1521ce5b` | reh3376 | ❌ silent drop |
| 2026-08-06T21:09:02Z | `97332401` | reh3376 | ❌ silent drop |
| 2026-08-06T21:52:06Z | `b9c066e7` | reh3376 | ❌ silent drop |
| 2026-08-06T22:27:18Z | `5dfabb9c` | reh3376 | ❌ silent drop |
| 2026-08-06T~22:35:00Z | `20ab433b` | reh3376 | ❌ silent drop (test empty-commit push after diagnosis) |

## Pre-incident degradation signal (17:11-17:30 UTC)

For roughly 20 minutes leading up to the failure, 5 push-triggered workflow runs on the same branch queued but **never executed any steps**. They show runners assigned + 45-minute duration + `steps: []` + `conclusion: failure|cancelled`:

- Run [31121502674](https://github.com/reh3376/mdemg/actions/runs/31121502674) — sha `088a02fb`, started 17:11:23Z, completed 17:29:26Z with 0 steps executed
- Run [31122523117](https://github.com/reh3376/mdemg/actions/runs/31122523117) — sha `310f3dc7`, started 17:18:34Z, completed **18:03:34Z** (45 min) with 0 steps executed
- Run [31122535626](https://github.com/reh3376/mdemg/actions/runs/31122535626) — sha `d79ded6e`, started 17:14:16Z, completed 17:29:26Z with 0 steps
- Run [31123305821](https://github.com/reh3376/mdemg/actions/runs/31123305821) — sha `f048c341`, cancelled with 0 steps

Then ONE successful run at 18:50 UTC (`c28d119a`), then push-event delivery stopped entirely.

## Configuration ruled out

We verified none of the following are the cause:

| Check | Result |
|---|---|
| `.github/workflows/auto-pr.yml` last modified | 2026-02-03 (~6 months ago, byte-identical since prior successes) |
| Workflow state | `state: active`, `id: 230237010` |
| Repository `actions/permissions` | `enabled: true, allowed_actions: all` |
| Branch protection on `reh3376_dev01` | 404 not protected (only `main` is protected) |
| Repository rulesets | Empty `[]` |
| Custom repository webhooks | Empty `[]` |
| Actor identity across successful vs failed runs | Same PAT (`reh3376`) worked at 18:50, silently fails after |
| PR event workflows | Continue to fire normally on the same SHAs |

## Reproduction

Confirmed reproducible at 22:35 UTC with a bare empty-commit push:

```bash
git commit --allow-empty -m "test"
git push origin reh3376_dev01
# Push succeeds; sha = 20ab433b
gh api "repos/reh3376/mdemg/actions/runs?head_sha=20ab433b"
# → {"total_count": 0, "workflow_runs": []}
```

## Ask

Please investigate why push events on `reh3376_dev01` in `reh3376/mdemg` are being registered on the `/events` API but not creating any workflow runs. If this is a systemic issue affecting other repositories, please advise. If it's isolated to this repo, please advise on remediation.

## Local resilience change (does not depend on GitHub support resolution)

We're adding `workflow_dispatch:` as a fallback trigger to `.github/workflows/auto-pr.yml` so the maintainer can invoke the auto-PR job manually via the Actions UI or `gh workflow run` while push-event delivery is degraded. This preserves the automated normal case AND provides a working fallback.

## Contact

Roger Henley — rogerhenley345@gmail.com
