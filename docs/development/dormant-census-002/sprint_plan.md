# DORMANT-CENSUS-002 — Sprint Plan

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #8.
**Parent sprint:** DORMANT-CENSUS-001 (shipped via
`verify_route_consumers.py` + `route_consumer_inventory.json`).

## 1. Header & Metadata

**Enabling / forcing-function sprint** — mix of investigation + code.
Effort ~1-2 days. Risk low: forcing functions are additive (a new CI
check + a new inventory file); doesn't touch runtime behavior.

## 2. Problem Statement

DORMANT-CENSUS-001 shipped `verify_route_consumers.py` — every API
route now has an adjudicated consumer inventory + CI check. The Q4
deep-dive named this as a repeatable pattern that should extend to
other surface classes:

> This deep-dive named `mdemg_ft_production_drift` gauge as
> unemitted / `suggested_guidance` table as nonexistent /
> heuristic-classifier-share as unwatched — the class recurs.

Three specific defects to verify + two new signals surfaced in this
session's investigation sprints:

**Named in Q4 deep-dive:**
1. `mdemg_ft_production_drift` gauge as unemitted
2. `suggested_guidance` table as nonexistent
3. heuristic-classifier-share as unwatched

**Surfaced this session:**
4. Empty-`constraint_code` guidance `rlgol248e1ftcdknf8t8zjpp` — 50+
   outcomes over 7d with no code (found in
   JIMINY-CEILING-INVESTIGATION-001)
5. Retrieval reverse-lookup gap — no forcing function catches when a
   reader (dashboard, alert rule, RSIC consumer) reads a
   never-emitted signal (found in RETRIEVAL-QUALITY-AUDIT-001 by
   different symptom but same class)

Coverage gap analysis:

| Surface class | Verifier shipped |
|---|---|
| API routes | ✓ DORMANT-CENSUS-001 (`verify_route_consumers.py`) |
| Env vars in docs | ✓ DOC-CURRENCY-002 (`verify_doc_env_vars.py`) |
| Config fields | ✓ CONFIG-DEADFLAG-001 (`verify_config_consumers.py`) |
| **TSDB tables** | ✗ no verifier — writer/reader gaps invisible |
| **Metrics registry gauges** | ✗ no verifier — emitter/consumer gaps invisible |
| Alert rules | ✗ partial — sweep tests pin structural properties but not consumer readership |
| Grafana dashboard panels | ✗ partial — GRAFANA-PANEL-FILTER-001 pin_test walks JSON but doesn't verify data-source presence |

The two highest-value gaps to close: **TSDB tables** and **metrics
registry gauges**. Ship one forcing function that catches both.

## 3. Scope & Constraints

**In scope (4 sequential steps):**

- **S1 — Verify 3 named deep-dive defects** (~30 min)
- **S2 — Investigate 2 signals from this session** (~1 hour)
- **S3 — Broader writer↔reader inventory pass** (~3-4 hours) —
  TSDB tables (which writers write, which consumers read) + metrics
  gauges (which code emits, which dashboards/alert-rules consume)
- **S4 — Ship the highest-value forcing function** (~2-3 hours) — a
  drift-check script + adjudicated inventory JSON, wired into CI
  alongside the existing DOC-CURRENCY-002 verifiers

**Out of scope:**

- Ripping out dormant code found during inventory — that's per-defect
  cleanup work, not this sprint's job. Findings go into the follow-up
  disclosure list.
- Extending the verifier to Grafana panel data-sources —
  GRAFANA-PANEL-FILTER-001 already has a pin test that handles the
  most-load-bearing case; broader coverage is a separate sprint.
- Cross-space inventory (guidance corpus / benchmark spaces) — this
  sprint targets the `mdemg-dev` production substrate.

## 4. Method — 4 sequential steps

**S1 — Verify 3 named deep-dive defects**
- `mdemg_ft_production_drift`: grep the code for the gauge emitter;
  query `metric_samples` for the metric_name; verdict = still-true or
  now-false
- `suggested_guidance` table: check for existence via
  `\d suggested_guidance` in psql; grep the code for writes/reads
- heuristic-classifier-share: verify CLASSIFIER-CONSISTENCY-001's
  `heuristic_share_high` rule is live (alert evaluator, config knob,
  no shipped defect)

**S2 — Investigate 2 signals from this session**
- Empty-constraint_code `rlgol248e1ftcdknf8t8zjpp`: pull the actual
  guidance content + trace back to what emitted it; classify as data
  hole vs missing-classifier-path defect
- Reverse-lookup: confirm the retrieval gap doesn't have any existing
  reader/writer signal (dashboards + alert rules); if it does, it's
  measurable and a follow-up sprint can target it

**S3 — Inventory pass**
- **TSDB tables**: grep `internal/tsdb/*writer*.go` for every table
  written; grep every `internal/`, `docs/`, dashboards for readers of
  each table. Enumerate mismatches.
- **Metrics gauges**: grep `internal/metrics/registry.go` for every
  gauge definition; grep dashboards + alert rules for each gauge name.
  Enumerate mismatches.
- Focus on the mismatches that map to real dormancy (a writer with no
  reader = dormant surface; a reader with no writer = broken/false
  signal).

**S4 — Ship forcing function**
- Design: mirror `verify_route_consumers.py`'s shape — a Python script
  that greps the code for writers/readers and diffs against an
  adjudicated inventory JSON. New CI check `verify_metric_consumers.py`
  (or `verify_tsdb_consumers.py` depending on where the highest-value
  gap is).
- Adjudicate initial inventory: every current mismatch gets a status
  (`IN_USE` / `DORMANT_INTENTIONAL` / `DORMANT_TO_REMOVE`).
- Wire into `.github/workflows/ci.yml` alongside the shipped
  verifiers. Merge-blocking for new drift; existing dormancy is
  documented + intentional (matches DORMANT-CENSUS-001's pattern).
- Docs: `post.md` with findings + CLAUDE.md pin

## 5. Testing / Verification

- **Tier 1**: verifier script runs locally + errors on synthetic
  drift (adding a new gauge without adjudicating it)
- **Tier 2**: `make` target integration; `verify_*` scripts remain
  the source of truth for their surface classes
- **Tier 3**: live run in CI on the shipping commit — should pass
  because we ship the adjudicated inventory in the same commit

## 6. Commit Strategy

Single commit (investigation report + inventory + verifier + wire +
docs). Small forcing-function sprints don't need per-epic commits.

## 7. Risks

- **False-positive rate**: the verifier may flag legitimate patterns
  that don't look like consumers (e.g. a gauge read via a
  computed-name grep). Mitigation: allowlist file + comment (matches
  DOC-CURRENCY-002's allowlist pattern).
- **Sprint scope creep**: an inventory pass might surface dozens of
  dormancy items. Discipline: enumerate them in the post + defer
  cleanup to follow-up sprints, don't rip anything out this pass.
- **Timing with the pending PR 553**: the two prior investigation
  sprints are still in PR 553 (unmerged); this sprint's forcing-
  function findings depend on what's shipped. If a finding surfaces
  that's already covered by an unmerged sprint, note that in the post
  rather than rediscovering later.

## 8. Documents Accessed

Filled in `post.md`.
