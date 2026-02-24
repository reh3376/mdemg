# Leakage Test Spec (Branch Isolation)

## Purpose

Validate strict space isolation when the same repository is ingested at two different branches.
This test proves that retrieval evidence never leaks across spaces or branches.

## Scope

- Ingest the same repo at two branches into two spaces.
- Run a leakage suite against both spaces.
- Audit evidence to confirm it resolves to the correct space and branch.

## Definitions

- Space A: `space_id=A` (Repo R @ Branch A)
- Space B: `space_id=B` (Repo R @ Branch B)
- Evidence: file:line references, node IDs, or symbol evidence attached to results
- Evidence ref (precise): file:line OR node_id OR symbol_id that is resolvable to (space_id, path, line_range).
- Branch-differentiating question: same path, different truth between branches

## Prerequisites

- Ingest pipeline supports `space_id`.
- Evidence resolver can map evidence to:
  - space_id
  - repo_commit or branch metadata (if stored)
- Retrieval returns evidence refs (file:line, node IDs, symbol evidence).

## Setup

1. Ingest Repo R @ Branch A → `space_id=A`
2. Ingest Repo R @ Branch B → `space_id=B`
3. Consolidate both spaces.
4. Capture repo metadata:
   - `repo_commit_A`, `repo_commit_B` (or branch names).

## Query Protocol

Run the same leakage suite against both spaces:

For each question `q`:

1. Run retrieve/consult against `space_id=A` → top-k results.
2. Parse evidence refs (file:line, node IDs, symbol evidence).
3. Verify each evidence ref resolves to:
   - the requested space_id AND
   - the expected repo_commit/branch metadata (if stored).
4. Repeat for `space_id=B`.

Definition of k:

k refers to the top-k retrieved items returned by retrieve/consult per question.

## Evidence Resolution Rules

- If evidence ref resolves to a different space_id → leakage.
- If evidence ref resolves to correct space_id but different repo_commit/branch → branch confusion.
- If evidence is missing or cannot be resolved → unverifiable evidence.

Track unverifiable evidence separately to avoid hiding leakage.

All leakage metrics are computed from resolver outputs, not from the model’s claimed metadata.

## Metrics

1) Leakage Rate @k (LR@k) [citation-level]

LR@k = (cross-space evidence refs) / (total evidence refs across top-k)

Target: 0.000

1) Cross-space Citation Rate (CSR)

CSR = (# questions with any cross-space evidence) / (# questions)

Target: 0.000

1) Branch Confusion Rate (BCR) (hard mode)

For branch-differentiating questions:

BCR = (# answers that cite correct space but give other branch’s value)
      / (# branch-differentiating questions)

Target: 0.000

Optional supporting metrics:

- Unresolvable Evidence Rate (URER) =
  (# questions with any unresolvable evidence) / (# questions)

- Leakage Severity @k =
  (# cross-space evidence hits) / k

## Question Design (Forcing Leakage if it Exists)

Use “same path, different truth” prompts:

- "What is the default value of X in path/to/file?"
- "Which enum variants exist for Y?"
- "What is the value of constant Z and where defined?"
- "Which feature flag gates behavior Q and what’s its default?"

Require answers to include evidence refs.

## Negative Control (Sensitivity Check)

Run a control where space scoping is intentionally disabled.
Expected: LR@k and CSR rise above 0.000.
This proves the test detects leakage when it exists.

## Reporting (Skeptic-Proof Line)

Space Isolation: Ingested identical repo across two branches into separate space_ids.
Leakage Rate@k = 0.000, CSR = 0.000, BCR = 0.000 on branch-differentiating queries
(evidence audited by resolver).

## Required Output Table

Per run, log:

- space_id_in_request (verbatim)
- space_id_requested
- space_id_transformed (true|false)
- space_id_transform_reason (alias|default|trim|invalid)
- top_k
- cross_space_hits
- LR@k
- CSR
- BCR
- URER (optional)
- space_id_resolved_distribution (counts by space across evidence)
- repo_commit_A
- repo_commit_B

Standardized Report Table (example header):

| run_id | space_id_in_request | space_id_requested | space_id_transformed | space_id_transform_reason | top_k | cross_space_hits | LR@k | CSR | BCR | URER | space_id_resolved_distribution | repo_commit_A | repo_commit_B | notes |
|:------|:--------------------|:------------------|:---------------------|:--------------------------|:-----|:-----------------|:----|:----|:----|:-----|:------------------------------|:-------------|:-------------|:------|

Standardized Report Table (example row):

| 2026-01-22T183012Z | A | A | false | - | 10 | 0 | 0.000 | 0.000 | 0.000 | 0.000 | A:120,B:0,unknown:0 | 3f9c2a1 | 9b4d11e | baseline run |

## Acceptance Criteria

- LR@k = 0.000
- CSR = 0.000
- BCR = 0.000 (for branch-differentiating questions)
- URER <= 0.01 (or explicitly reported as non-zero)
- Negative control shows LR@k > 0.000

Hard fail rule:

If URER exceeds the threshold, the run is invalid for leakage claims and must
be reported as an evidence failure.

## Notes

- If repo_commit metadata is not stored, BCR must be computed
  using branch-differentiating ground truth only.
- If evidence refs are missing, the test must record URER to avoid false passes.

## Branch Truth and Preflight

BCR requires an answer key with branch-specific ground truth
(value + file:line) for each branch-differentiating question.

Preflight:

Confirm each branch-differentiating question has non-identical ground truth
across A and B before inclusion in the BCR set.
