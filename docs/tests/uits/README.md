# UITS: Universal Iterative-Improvement Test Specification

**Framework #12** in the UxTS ecosystem.

Formalizes iterative self-improvement testing for any T1-encoded content that requires comprehension validation. LLM-dependent, non-deterministic, soft-fail CI gate.

## Quick Start

```bash
# Validate a single spec (run trials, report scores)
python docs/tests/uits/runners/uits_runner.py validate \
  --spec docs/tests/uits/specs/flow_retrieval.uits.json

# Validate all specs
python docs/tests/uits/runners/uits_runner.py validate-all \
  --spec-dir docs/tests/uits/specs/

# Optimize (iterative loop until convergence)
python docs/tests/uits/runners/uits_runner.py optimize \
  --spec docs/tests/uits/specs/flow_retrieval.uits.json

# Generate report
python docs/tests/uits/runners/uits_runner.py validate-all \
  --spec-dir docs/tests/uits/specs/ --report report.json
```

## Specs

| Spec | Content Type | Questions | Encoding |
|------|-------------|-----------|----------|
| flow_retrieval | architecture_map | 5 | t1_coded |
| flow_observe_learn_consolidate | architecture_map | 5 | t1_coded |
| flow_jiminy_guide | architecture_map | 5 | t1_coded |
| flow_rsic_cycle | architecture_map | 5 | t1_coded |
| schema_neo4j | architecture_map | 5 | t1_coded |
| dict_pkg_codes | architecture_map | 6 | t1_glossary |
| dep_pkg_graph | architecture_map | 5 | t1_coded |
| svc_external | architecture_map | 5 | t1_coded |
| dist_channels | architecture_map | 5 | t1_coded |
| uxts_frameworks | architecture_map | 6 | t1_coded |
| j17_constraints | constraint_code | 3 | t1_glossary |

## Scoring Profiles

Each spec defines scoring profiles with versioned weights. The default `arch_map_default` profile:

| Dimension | Weight | Description |
|-----------|--------|-------------|
| comprehension | 0.40 | Mean LLM judge score (0-10 scale) |
| compaction | 0.25 | Byte reduction vs human-readable baseline |
| token_efficiency | 0.20 | Token count vs budget |
| fidelity | 0.15 | Round-trip field recovery (placeholder) |

Thresholds: composite ≥ 0.80, comprehension ≥ 9.0, floor ≥ 7.0.

## Requirements

- Python 3.10+
- `requests` library
- `OPENAI_API_KEY` environment variable (or in `.env`)

## Relationship to Existing Tools

- **`scripts/map_optimization_test.py`**: Predecessor harness. UITS formalizes it as declarative specs + reusable runner.
- **`cmd/j17-comprehension-test/main.go`**: Reference implementation. J17 constraint testing is now a UITS consumer via `j17_constraints.uits.json`.
- **UETS**: Closest sibling framework (LLM-dependent, soft-fail CI). UITS follows the same runner patterns.
