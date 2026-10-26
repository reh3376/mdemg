# E5 — Operator Live Review Notes

**Sprint**: ONBOARDING-TUTORIAL-ENDUSER-001 Epic 5
**Status**: AWAITING OPERATOR
**Review target**: `docs/onboarding/mdemg-onboarding-deck.md` served at `http://localhost:9999/ui/tutorial/`
**Current deck state**: shipped at commit `ae71c134` (E4 stitch complete)

> This is the HARD GATE between E4 stitch and E6 closeout. Operator reviews every slide U1-U17 against a running MDEMG instance, captures per-slide verdicts, and signs off (or loops back to E3/E4 for fixes).

---

## How To Run The Review

1. Confirm the stack is up:
   ```bash
   docker compose ps              # 5 × Up (healthy)
   mdemg healthz                  # status: ok
   ```
2. Open the tutorial:
   ```bash
   mdemg tutorial                 # opens http://localhost:9999/ui/tutorial/
   ```
3. Navigate the LANDING FORK → click **End User** path → confirm landing on `#end-user-path` (U1.1).
4. Walk through U1.1 → U17.4 in order using `← →` keyboard nav; `ESC` for overview grid.
5. For each slide: capture a verdict below (`pass` / `fix` / `comment`).
6. For CLI examples: copy/paste each verbatim into a terminal; confirm expected output.
7. For UI tab references: open `/ui/<tab>` alongside and confirm the slide's description matches reality.
8. For Grafana panel references: open `http://localhost:3030` + the named dashboard + the named panel.

**Natural stopping points** (per `global_roadmap.md` §4): after U3, U5, U8, U11, U14, U17.

---

## Verdict Template

```
### U<N>.<M> — <Title>
- Verdict: pass | fix | comment
- Notes: ...
```

---

## Review Verdicts

<!-- Operator fills in below as review progresses. Use the template above for each slide. -->

<!-- Example:
### U1.1 — Operator Onboarding — Start Here
- Verdict: pass
- Notes: Mini-TOC renders; fork link resolves; keyboard nav works.

### U1.2 — What Is MDEMG?
- Verdict: fix
- Notes: The analogy sentence should say "artificial neural network" lowercase; also want to add a line about the dev-path cross-link.
-->

---

## Spot-Check: Factual Provenance (per `presentation_spec.md` §5.1)

Operator picks **10 claims at random** across U1-U17 and traces each to:
- CLAUDE.md pin + date, OR
- feature doc path, OR
- sprint record path, OR
- shipped code file+line.

Record each spot-check below:

```
Slide: U<N>.<M>
Claim: "..."
Trace: <source + line/anchor>
Verdict: pass | revise
```

<!-- Operator fills in during review. 10 required for sign-off. -->

---

## Spot-Check: Trajectory Language (per `presentation_spec.md` §5.2)

Operator confirms NO sub-target metric is framed as "by design":
- Follow rate ~13% → named as JIMINY-CEILING-BREAK-2 target ≥80%? [ ]
- Cache hit ratio ~75% → named as DH-004 baseline, not "by design"? [ ]
- FT benchmark staleness → named as current trajectory to re-run? [ ]
- Any OTHER sub-target metric framed as "by design"? [ ]

---

## Spot-Check: CLI-Example Reproducibility

Operator paste-tests at least ONE example from each U-section against a live terminal:
- U1.4: 4-command happy path works
- U2.3: `docker compose ps` output shape matches
- U2.5: `mdemg healthz` output shape matches
- U3.5: `mdemg data observe` works
- U3.6: `mdemg data recall` works
- U3.7: `mdemg data correct` works
- U4.7: `mdemg jiminy mode` shows + toggles
- U4.8: `mdemg jiminy override apply` syntax valid
- U7: `mdemg review autograde --help` resolves
- U8.6: `python -m neural.benchmarks.run_benchmark` syntax valid
- U9.4: `mdemg backup create` syntax valid
- U14.*: at least one command per group validated via `--help`
- U15.5: `curl -s http://localhost:9999/healthz | jq` works

---

## Fork Link Smoke

Operator confirms:
- Landing fork `#landing-fork` → click End User → lands on `#end-user-path` (U1.1) [ ]
- U17.4 Thank You "Return to Landing Fork" link resolves to `#landing-fork` [ ]
- U17.4 "Return to TOC" link resolves to `#table-of-contents` [ ]
- Operator can advance from U1.1 to U17.4 using `→` keyboard nav alone (no jumps required) [ ]

---

## Operator Sign-Off (REQUIRED FOR E6)

```
all slides reviewed, sign-off: <YYYY-MM-DD>
all CLI examples paste-tested: <YYYY-MM-DD>
factual-provenance 10-claim spot-check passed: <YYYY-MM-DD>
trajectory-language audit passed: <YYYY-MM-DD>
fork-link smoke passed: <YYYY-MM-DD>
```

Until all 5 sign-off lines are filled in with dates, **E6 is BLOCKED**.

---

## Loop-Back Protocol

If operator finds any `fix` verdicts:
1. Operator describes the fix inline in the Verdict Notes
2. Agent edits `draft_slides.md` (NOT the stitched deck directly — audit trail)
3. Agent runs E4 stitch again: `python3 scripts/<re-splice-helper>` or Edit against the deck
4. Agent runs `make sync-tutorial-embed` + `make verify-tutorial-embed`
5. Agent commits with `fix(enduser): E5 revision — <slide>` conventional commit
6. Operator re-reviews ONLY the fixed slide, updates verdict here
7. Loop until all slides are `pass`

No E6 closeout until operator has signed off on all 5 lines above.
