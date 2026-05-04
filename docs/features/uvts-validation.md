---
created: 2026-05-04
updated: 2026-05-04
version: v0.6.0
author: reh3376
status: active
phase: phase 12
---

# UVTS — Universal Validation Test Specification

## Summary

**Feature**: `uvts-validation`
**Summary**: Spec-driven semantic-quality test framework. Operators define a JSON spec listing questions, expected evidence files, and grading thresholds; the runner hits the live `/v1/memory/retrieve` API for each question, an LLM grader scores responses on three axes (evidence + semantic + concept), the aggregator persists per-question + per-run rows to TSDB V0016, and the A/B harness compares two run grade-files with a strict merge gate (B mean ≥ A AND no per-question regression > threshold). It is the canonical Tier 3 live-testing harness for retrieval-affecting sprints.

## Vision & Goals

The MDEMG vision treats retrieval as the **connection layer** that "helps developers create software that connects in a manner that increases the likelihood of making better decisions" (memory: `project_mdemg_purpose.md`). Connection-layer changes (retrieval scorer tweaks, ranker rewrites, new columns, gates, fingerprints) must be evaluated against actual decision quality on real codebases, not synthetic benchmarks.

Phase 12 activated UVTS to:

1. **Make Tier 3 testing standard** — every connection-layer sprint produces a UVTS A/B verdict before merge. CLAUDE.md formalized this rule (commit `d10c1a5`): "live smoke: run X against the real system, observe Y in TSDB/Grafana/logs, confirm Z."
2. **Capture deltas as TSDB time-series** so fitness-over-time is a Grafana panel, not a one-off comparison
3. **Encode the merge gate as code** rather than tribal knowledge — the comparator emits a verdict (`pass` / `fail` / `drift`) the operator (or future CI) can act on
4. **Surface conflicts as ConflictTracker rows** when the gate fails, integrating UVTS verdicts into the J17 protocol-stability work

UVTS replaces the ad-hoc "run a few queries and eyeball" pattern that preceded it and unblocks all subsequent retrieval research extensions (Notes 04 column-voting, 05 fingerprints, 06 percentile gate).

## Current State

### Architecture

Five components:

| Component | Path | Responsibility |
|---|---|---|
| Spec | `docs/tests/uvts/specs/*.uvts.json` | JSON: validation metadata + per-question expected files + per-profile question subsets + `ab_mode` thresholds |
| Runner | `docs/tests/uvts/runners/uvts_runner.py` | Loads spec, hits `/v1/memory/retrieve` per question, calls LLM grader (gpt-5.4-mini), aggregates per-axis scores, optionally persists to TSDB V0016 |
| Grader | embedded in runner | LLM evaluates response against expected evidence; deterministic temperature, fixed prompt, three axes (evidence + semantic + concept) |
| A/B comparator | `docs/tests/uvts/runners/uvts_ab_compare.py` | Joins two `grades.json` files on shared question_ids; computes mean delta + per-question regression list; verdict (`pass` / `fail` / `drift`) by spec criterion |
| TSDB schema | V0016 (`uvts_runs` + `uvts_results` hypertables) | Persistence; drives Grafana fitness-over-time panels and ConflictTracker integration |

Final score per question follows a fixed formula encoded in the runner:

```
final_score = 0.70 × evidence_score + 0.15 × semantic_score + 0.15 × concept_score + citation_bonus
```

Where:
- **evidence_score** = file-location accuracy (does the response cite a file from the expected list with line tolerance?). 0.50 minimum if at-least-one expected file mentioned, scales to 1.0 with full match.
- **semantic_score** = LLM grader's qualitative answer quality, 0–1.
- **concept_score** = LLM grader's coverage of expected concepts, 0–1.
- **citation_bonus** = +0.10 if a specific file:line citation was provided.

The 0.70 evidence weight reflects the framework's emphasis on _grounded_ retrieval — answers without correct file citations are penalized heavily.

### Workflow

1. Operator authors a spec (or extends an existing one) with new questions + expected files
2. Lint pass: `make test-uvts-lint` validates schema for all `*.uvts.json`
3. Quick smoke: `make test-uvts-quick BASE_URL=http://localhost:9999` runs the spec's `quick` profile (typically 16 questions, ~10 min, ~$0.60)
4. For research / A/B: run with `--persist-tsdb --branch-label <name> --codebase-sha <sha>` to land rows in V0016
5. A/B compare: `python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline <run-A>/grades.json --candidate <run-B>/grades.json --spec <spec>.uvts.json --out verdict.json`
6. Verdict: exit 0 = pass, exit 1 = fail (mean regression OR per-question regression > threshold), exit 2 = drift (spec mismatch)
7. ConflictTracker (Phase 12 Epic 6) auto-records guidance conflicts when the gate fails — feeds into the J17 protocol-stability work

### Configuration

UVTS doesn't add env vars to mdemg directly; the runner takes CLI flags:

| Flag | Default | Description |
|---|---|---|
| `--spec` | required | Path to `*.uvts.json` |
| `--base-url` | `http://localhost:9999` | mdemg HTTP API |
| `--profile` | `quick` | `quick` / `standard` / `full` (spec defines question subsets per profile) |
| `--space-id` | (from spec) | Override the spec's `validation.space_id` (e.g. for cross-space A/B) |
| `--persist-tsdb` | off | Write to V0016 |
| `--branch-label` | (none) | Free-text label for grouping in `uvts_runs` |
| `--codebase-sha` | (none) | git SHA for run reproducibility |
| `--retrieve-timeout-s` | `30` | Per-question retrieve timeout (bump for slow dev pipelines) |

The grader uses OpenAI `gpt-5.4-mini` with deterministic temperature; cost scales linearly with question count.

## Choices that were made

### Why JSON specs (not YAML or Python)

JSON is operable from any language without a parser dependency, lints cleanly with standard tooling, and round-trips losslessly through Git diffs. YAML's whitespace sensitivity creates merge-conflict pain on larger specs. Python specs would conflate test data with executable code.

### Why three-axis grading (not single-score)

A single quality score conflates evidence (objective: did you cite the right file?) with semantic quality (subjective: was the answer good?). Decoupling lets failures attribute correctly: a regression that's evidence-only signals a retrieval problem; a regression that's semantic-only signals an LLM-generation problem.

### Why 0.70 evidence weight

Set empirically during Phase 12 Epic 1: baseline runs showed evidence scores were the most reliable signal across run-to-run variance. Semantic + concept fluctuated by ±5% on identical inputs (LLM grader noise) while evidence was deterministic given the same retrieval output. Weighting evidence heavily makes the framework less noise-prone.

### Why a strict per-question regression threshold (default 0.10)

A pure mean-comparison would let a candidate win on 18/20 questions while badly losing 2 — an average might still be positive but the user-experience regression is real. The strict per-question threshold catches that case. 0.10 was set during Phase 12 Epic 3 spec authoring; tightening to 0.05 was tried and produced too many flakes from grader noise.

### Why TSDB persistence (V0016 hypertable)

Grafana fitness-over-time panels need durable rows. A flat-file grades.json works for one A/B but not for trend analysis. V0016's two hypertables (`uvts_runs` for the metadata, `uvts_results` for per-question rows) make "show me the mean delta on `business_logic_constraints` over the past 30 days" a single SQL query.

### Why ConflictTracker integration (Phase 12 Epic 6)

A failed UVTS A/B is a conflict between expected behavior (the baseline) and observed behavior (the candidate). Surfacing it through ConflictTracker means the J17 protocol-stability dashboards capture retrieval-quality conflicts the same way they capture other guidance conflicts — one operator surface, not two.

## Notes

### Known limitations

- **Question count is operator-managed**: the framework doesn't generate questions; operators write them. The `lnl_demo_validation.uvts.json` spec has 120 questions; growing it requires manual authoring.
- **Grader cost**: at 16 questions/quick × 4 calls each (q + grader axes), quick is ~$0.60. 120q full is ~$10. A/B sweeps multiply this. The Phase 14 sprint ran 5 quick sweeps + 1 full at ~$13 total.
- **Single-grader bias**: the grader is `gpt-5.4-mini` with a fixed prompt. A bias in the grader (e.g. preference for certain phrasings) cannot be detected by UVTS alone. Cross-grader validation is queued as a future improvement.

### Risks & gaps

- **Floating-point boundary regressions**: when baseline = 0.45 and candidate = 0.35, the delta is `-0.1` exactly in display but `-0.10000000001` in float, which trips the `delta < -threshold` check. This produced 7 false-positive regressions in Phase 14 Epic 2 120q full. Phase 14.1 should add an `eps` tolerance to the comparator (1e-6 fixes it).
- **Mean-gate-but-per-question-fail asymmetry**: a verdict with 7 regressions and 7 offsetting improvements rejects the candidate even though aggregate quality is preserved. By design (the strict criterion catches user-visible regressions), but operators should know the asymmetry exists.

### Future improvements

- Cross-grader validation (run with two graders, require agreement)
- Automatic question generation from production `llm_interactions` patterns
- `eps` tolerance for floating-point boundary cases
- Per-category verdict (Phase 13.2 work — surfaces which categories regressed without binary fail)

## API Endpoints

UVTS does not expose HTTP endpoints. Persistence flows: runner → TSDB pool → V0016.

## CLI Commands

| Command | Description |
|---|---|
| `make test-uvts-lint` | Schema-validate all `*.uvts.json` (CI-safe, no live deps) |
| `make test-uvts-quick BASE_URL=http://localhost:9999` | 16-question quick profile, ~10 min |
| `make test-uvts-full BASE_URL=http://localhost:9999` | 120-question full corpus, ~55 min |
| `python3 docs/tests/uvts/runners/uvts_runner.py [...]` | Direct invocation with full flag set |
| `python3 docs/tests/uvts/runners/uvts_ab_compare.py [...]` | A/B verdict between two grade files |

## Configuration Reference

UVTS does not add env vars. CLI flags are the configuration surface. See "Configuration" table above for the runner.

The spec file's `ab_mode` block configures the merge gate:
```json
"ab_mode": {
  "regression_threshold_per_question": 0.10
}
```

## Dependencies

| Feature | Relationship |
|---|---|
| `column-voting-retrieval` | Phase 13/13.1 used UVTS as the merge gate |
| `local-llm-runtime` | The /v1/memory/retrieve calls go through llama-server |
| `sparse-retrieval` (Phase 14) | Phase 14 Epic 2 used UVTS for the gate verdict |
| TSDB V0016 (`uvts_runs`, `uvts_results`) | Persistence layer |
| ConflictTracker (Phase 12 Epic 6) | Auto-records conflicts on gate fail |

## Related Files

- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — the canonical spec
- `docs/tests/uvts/runners/uvts_runner.py` — main runner
- `docs/tests/uvts/runners/uvts_ab_compare.py` — A/B comparator
- `internal/tsdb/migrations/016_uvts_results.sql` — V0016 schema
- `Makefile` — `test-uvts-*` targets
- `docs/development/ft-lora/phase_12_post.md` — origin sprint
