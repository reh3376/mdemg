# Testing + Simulation

## 1) Unit tests

- schema validation (required props exist)
- constraints creation + uniqueness
- edge update math (Hebbian and decay)

## 2) Synthetic graph generation tests (recommended)

Generate:

- clusters (dense internal edges)
- bridges (few edges connecting clusters)
- hubs (high degree nodes)
- contradictory pairs

Assertions:

- query near a cluster should retrieve cluster members
- activation should traverse bridges only when seeded strongly
- hubs should not dominate when hub penalty enabled
- consolidation creates correct abstraction edges and does not merge unrelated clusters

## 3) Regression tests: “memory quality”

Maintain a frozen set:

- queries → expected top-K nodes (within tolerance)
- run after changing tuning knobs or learning rules
- track drift metrics: recall@K, novelty rate, redundancy

## 4) Load tests

- ingestion throughput: events/sec with embedding generation on/off
- retrieval latency at different graph sizes
- maintenance job runtime bounds

## 5) Explainability tests

For each retrieval, assert:

- returned explanation paths are valid edges in the graph
- evidence_count and timestamps exist
- no restricted nodes leak under policy
