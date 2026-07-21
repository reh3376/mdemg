# J17-TIER-GATE-001 — Sprint Post

**Shipped:** 2026-07-21 | **Result: first live T1 messaging in the protocol's history**

## Epics
- E0 plan; E1+E2+E3 config + encoder + truth-table tests (one commit); gofmt follow-up; E4 live Tier-3 (`live_verification.md` — 6 T1 pipe-coded lines live after real feedback lifted in-process comprehension to 1.0); E5 docs (this).

## Key decisions
- Promotion axis = measured comprehension; compliance-trust retired from promotion (kept for everything else). Demotion gate untouched as the safety net.
- Default `trust` byte-identical (pin-tested) — `.env` opt-in per the flag discipline; operator authorized `comprehension` live.
- Floor 0.6 anchored to the existing ineffective-threshold; documented flap-tuning path.

## Discovered during smoke
- In-process comprehension is composition-sensitive (0.000 across 15 tier1-band-ignored samples); post-restart T1 warm-up ~1h of traffic. Documented in feature doc + CLAUDE.md.
- The COMPLIANCE-CREDIT clause visibly routed `not_applicable` on unrelated items in the same feedback batches (6 NA / 7 followed / 2 ignored) — the two sprints compound: fewer 0-comprehension ignored events ALSO feed cleaner comprehension averages.

## Rollback
`.env` mode=trust + restart; per-commit revert.

## Next
Compression ratio expected to climb toward 3-5× over hours/days — observable on the J17 dashboard. Sprint B (APE-REFLECT-EVAL-REFRESH-001) follows immediately.
