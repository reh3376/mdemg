# HITL Auto-Curation

**Sprint:** HITL-CURATION-002 (2026-07-27) — auto-curation layer over
HITL-REVIEW-001.

## Why

HITL-REVIEW-001 shipped a complete human-in-the-loop review platform (Review
tab, `/v1/review/*` endpoints, `contradicted_drafts` + `guidance` + 16
`llm:*` datasets, 6 sink types). Post-ship, **7-day HITL activity was
2 grades total.**

The friction wasn't the platform — it was operator attention. Drafts
accumulated in the queue; 5 contradicted-drafts sat unreviewed for 6 days;
59+ guardrail rows sat unlabeled. The corpus quality lever that JIMINY-
RELEVANCE-001 built the training corpus around wasn't being pulled.

## How it works

Auto-curation removes friction around the operator without removing the
operator. Three moving parts:

### 1. Autograder (`internal/review/autograder.go`)

An LLM-graded confidence-marker for any dataset with a rated rubric. Reads
the item's Content + Context, prompts the local `mdemg-llm-v1` with the
dataset's rubric anchors, returns a `GradeResult` carrying the model's
score-per-dimension + a confidence float + a rationale.

**The load-bearing invariant:** every autograde-authored row's `grader_id`
starts with `auto:` (grep-testable, dashboards + the stall-alert read the
prefix), and the CLI ALWAYS POSTs with `reinforce:false`. Auto-grade
NEVER triggers the substrate reinforcement side-effect. Only
operator-confirmed grades reinforce.

Datasets can optionally implement `AutogradePromptHinter` to supply
per-dataset anti-pattern language spliced into the system prompt — the
`contradicted_drafts` and `llm:guardrail.evaluate` datasets do this; the
15 other `llm:*` datasets fall through to a generic hint.

### 2. `mdemg review autograde` CLI

```bash
mdemg review autograde --dataset contradicted_drafts --space-id mdemg-dev
mdemg review autograde --dataset llm:guardrail.evaluate --space-id mdemg-dev --dry-run
mdemg review autograde --dataset X --min-confidence 0.90 --limit 20
```

Fetches pending items via `GET /v1/review/candidates` (new bulk-fetch
endpoint that bypasses the human-facing sampler bias), grades each,
POSTs high-confidence verdicts to `/v1/review/grade` with `reinforce:false`.
Items below the confidence threshold stay pending for the operator.

Defaults: `--min-confidence 0.80`, `--limit 50`, `--dry-run false`.

### 3. `mdemg review cadence` CLI

```bash
mdemg review cadence                        # human-readable summary
mdemg review cadence --out-format json      # machine-readable
```

Renders a compact "what's waiting for you" digest across all registered
review datasets. Designed to run periodically (via cron/launchd/manual);
the JSON output is machine-readable for a scheduler hook or alert body.

### 4. `hitl_curation_stalled` alert rule (evaluator)

Fires when BOTH:
- Pending contradicted-drafts ≥ `HITL_CURATION_STALL_MIN_PENDING` (default 5)
- Zero **operator** review-grades in the last `HITL_CURATION_STALL_LOOKBACK_HOURS`
  (default 168 = 7 days). The `auto:*` grader_id prefix is EXCLUDED from
  the operator count — else the sprint's own auto-clear would suppress
  the alert.

Distinct Service `hitl-curation` (NOSILENT-001 cooldown-key), Severity
MEDIUM, ForDuration 24h (won't flap on a slow week). Idle-safe COALESCE
aggregation (TSDB-CONSUME-001 contract).

## How to use

### Operator flow — run per week / on demand

```bash
# 1. See what's waiting
mdemg review cadence

# 2. Clear the boring 80% (default-off; opt in per invocation)
mdemg review autograde --dataset contradicted_drafts --space-id mdemg-dev --dry-run
# Inspect the verdicts. If they look right:
mdemg review autograde --dataset contradicted_drafts --space-id mdemg-dev

# 3. Review the low-confidence remainder at http://localhost:9999/ui/#review
```

### Scheduled cadence (optional)

Wire a supervised scheduler (cron/launchd) to run the cadence on the
operator's rhythm. The `hitl_curation_stalled` alert will surface via
the standard hook channel if the operator misses a cycle.

## Config knobs

| Env var | Default | Purpose |
|---|---|---|
| `HITL_CURATION_STALL_MIN_PENDING` | 5 | Fire threshold on pending draft count |
| `HITL_CURATION_STALL_LOOKBACK_HOURS` | 168 (7d) | Operator-grade lookback window |

The Autograder itself has no server-side config — invocation flags
(`--min-confidence`, `--limit`, `--dry-run`) control per-run behavior.

## Known limitations

**LLM-output correctness is not reliably autogradable.** For `llm:*`
datasets the `correctness` axis needs ground truth the model can't
independently derive. Guardrail null-verdicts (`{"violations":[],
"warnings":[]}`) default to `correctness=4` because the model reads the
verdict as "guardrail said no problem, therefore correct". The invariant
(NoopSink on all `llm:*` datasets, plus `reinforce:false`) means this
has ZERO substrate risk — auto-grade produces training-signal-quality
metadata, not live reinforcement. A stricter `--dimensions=format_validity`
variant is a candidate follow-up.

**Auto-grade never removes items from the pending queue** on
`contradicted_drafts`. Removal requires operator-confirmed approval
(the sink path mints an L0 correction obs); auto-grade only marks
`review_grades` metadata. Rerunning `mdemg review autograde` is
idempotent — the endpoint's existing `LatestGradeForItem` gate returns
409 on already-graded items, which the CLI treats as success.

## Sprint reference

Plan: `docs/development/hitl-curation-002/sprint_plan.md`
Post: `docs/development/hitl-curation-002/post.md`
