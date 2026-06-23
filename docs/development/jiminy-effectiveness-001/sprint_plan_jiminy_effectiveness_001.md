# Sprint Plan — JIMINY-EFFECTIVENESS-001: Trust as a Recoverable Windowed Signal (J17 T1 Unblocker)

## 1. Header & Metadata
2026-06-23 · branch `reh3376_dev01` · second of the two guidance sprints (after
JIMINY-SIGNAL-001 made the signals honest) · effort ~1d · risk medium (changes
the per-session trust update that gates J17 encoding tier; getting it wrong
mis-tiers guidance — but the change makes trust HONEST + RECOVERABLE, validated
live against the now-trustworthy gauge).

## 2. Problem Statement
J17 never promotes to T1 because the only live session's trust is pinned at the
**0.04 floor**. Re-investigation (this sprint's recon) corrected the original
hypothesis: the surfaced guidance is **not** `confidence:0` junk — it's filtered
above `JiminyMinConfidence=0.3` (confidence 0.7–0.9) and the **LLM** classifier
(not the heuristic) judges ~79% of it `Ignored`. So the agent genuinely isn't
following confident, on-topic guidance, and the trust mechanic
(`trust.go::RecordOutcome`) is a **monotonic ratchet** (Followed +0.05, Ignored
−0.02, …) — 1,343 mostly-ignored outcomes drove trust from the 0.65 start to the
0.04 floor, where it is **stuck forever**: it can never recover to the 0.75 T1
threshold even if recent guidance becomes effective, because the ratchet only
remembers the cumulative history, not the recent regime.

The fix is to make trust an **honest, recoverable signal** — an exponential
moving average (EMA) of recent per-outcome effectiveness — so trust *tracks
recent effectiveness* and can rise again when guidance starts being followed.
This is the structural T1 unblocker; it does not paper over the real ~21%
effectiveness (the gauge JIMINY-SIGNAL-001 made honest), it makes trust reflect
it *and* be able to climb.

## 3. Scope & Constraints
**In:** (1) Replace the monotonic ratchet in `TrustScorer.RecordOutcome`
(`internal/jiminy/trust.go`) with an **EMA**:
`trust ← trust + α·(target(outcome) − trust)`, where the per-outcome
effectiveness anchors are Followed=1.0, PartialCompliance=0.6, Ignored=0.2,
Contradicted=0.0 (the metric definition; a mostly-ignored session converges
toward ~0.2 < 0.75, a following session climbs toward 1.0 > 0.75 — recoverable).
(2) Config knobs (no-hardcoding): `JIMINY_TRUST_MODE` (`ema`|`ratchet`, default
`ema`) and `JIMINY_TRUST_EMA_ALPHA` (default 0.1, range (0,1]); `ratchet`
preserves the legacy behavior for rollback. (3) Keep `Initial` (0.65),
thresholds (0.75/0.35), TTL unchanged — only the *update rule* changes.
**Out:** the guidance-relevance / what-gets-surfaced problem (Option B — a
larger retrieval-quality effort, its own line); changing the LLM classifier;
the explicit T1 exploration probe (unneeded once trust can climb organically);
any change to the now-honest gauge.

**Constraints:** forward-only — existing Neo4j `J17TrustState` scores self-heal
toward their recent-outcome regime under the EMA (the 0.04 live session recovers
as outcomes accrue); no migration. Tier 3 live required. The EMA targets are the
metric definition (like reward-function anchors), α is the tunable knob.

## 4. Dependencies
`internal/jiminy/trust.go` (`RecordOutcome`, `TrustConfig`, `NewTrustScorer`);
`internal/jiminy/service.go` (the `aggregateOutcome` → `RecordOutcome` call,
~1779); `internal/config/config.go` (J17 trust config block + new knobs);
`internal/api/server.go` (`NewTrustScorer(TrustConfig{...})` wiring, ~184); the
honest follow-rate gauge (JIMINY-SIGNAL-001) for live validation; live Neo4j
`J17TrustState` + TSDB `constraint_outcomes`.

## 5. Implementation Plan
Epic 0 plan (this) · **Epic 1** EMA trust update — add `Mode`/`EMAAlpha` to
`TrustConfig`, EMA branch in `RecordOutcome` (target anchors as documented
consts), config knobs in config.go + wiring in server.go; unit tests (EMA
recovers off a low floor after a run of Followed; converges to ~target on a
steady regime; ratchet mode unchanged; bounds [0,1]) · **Epic 2 (live Tier 3)**
restart; observe over real RSIC/feedback cycles: the live 0.04 session's trust
**rises off the floor** as outcomes accrue (and would cross 0.75 under a
following regime), the J17 tier distribution becomes promotion-capable, and the
honest follow-rate gauge + trust track each other — measured on the real stack ·
**Epic 3** docs (feature-doc note on `jiminy-effectiveness-tracking.md` or a new
one, CHANGELOG, post), push.

## 6. Testing Plan (3 tiers)
T1: EMA unit — from 0.04, a run of Followed climbs past 0.75 within N outcomes
(recoverable); a steady all-Ignored regime converges to ~0.2 (honest-low, not 0);
mixed regime tracks the weighted recent rate; `ratchet` mode reproduces the old
monotonic behavior; clamp [0,1]; α range validation. T2: `go test
./internal/jiminy/... ./internal/config/...`; golangci-lint; config scanner sees
the new knobs. T3 (live): on the running stack — the live session's
`J17TrustState` score climbs off 0.04 across feedback cycles; trust no longer
monotonic-floors; spot-check the EMA math against the recent outcome stream.

## 7. Commit Strategy
Per-epic; gofmt/vet + lint each; push once; PR summary; CI watch. Live-surprise
fixes get their own commit.

## 8. Verification Checklist
- [ ] `RecordOutcome` EMA: trust recovers off a low floor after Followed; converges to target on a steady regime; bounds [0,1]
- [ ] `JIMINY_TRUST_MODE` (ema default) + `JIMINY_TRUST_EMA_ALPHA` config, range-validated, scanner-clean; `ratchet` preserves legacy
- [ ] Initial/thresholds/TTL unchanged; wiring in server.go passes the new config
- [ ] LIVE: the 0.04 session's trust climbs off the floor across cycles; trust tracks the honest gauge
- [ ] Unit tests + go build + lint green
- [ ] Feature-doc note + CHANGELOG + post

## 9. Documentation Update — Epic 3 (never cut).

## 10. Risks & Mitigations
EMA makes a low-effectiveness session *too* promotable → the steady-Ignored
regime converges to ~0.2 (well below the 0.75 T1 threshold), so only genuinely-
following sessions cross it; α (0.1) is slow enough that a single lucky Followed
can't promote. Wrong α → config-tunable; range-validated. Existing scores
mis-track at first → forward-only EMA self-heals over a few outcomes; documented.
Mis-tiering during transition → the change only affects the *update*; thresholds
+ encoder unchanged; `ratchet` mode is a one-flag rollback. Masking the real
effectiveness problem → it does NOT: the gauge stays honest (~0.24) and trust
converges to the honest recent rate; the relevance fix (Option B) remains the
explicit follow-up for *raising* effectiveness.

## 11. Documents Accessed
The two guidance investigations (J17 T1 + dashboard); `internal/jiminy/{trust,
service}.go`; `internal/config/config.go`; `internal/api/server.go`; live
`J17TrustState` + `constraint_outcomes` (confidence 0.7–0.9, LLM-classified,
~79% ignored — the recon that corrected the premise).

## 12. Rollback Procedures
`JIMINY_TRUST_MODE=ratchet` restores the legacy monotonic behavior without a code
change. The EMA is the new default; reverting the commit restores the ratchet as
default. No schema, no migration, no data change — existing scores re-track under
whichever mode is active.
