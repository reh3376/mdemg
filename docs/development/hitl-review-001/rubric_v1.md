# HITL Review Rubric — version `gr-v1`

The reproducible grading standard for the Human-in-the-Loop review platform
(HITL-REVIEW-001). Each **Rated** dimension is scored **0–4** with the written
anchor below; the normalized `gold_score` = mean(dimension scores) / 4 ∈ [0,1].
**Ranked** datasets (DPO) instead pick chosen vs rejected with an optional 0–4
confidence.

`rubric_version` is pinned on every grade. A grade made against a stale version
is recorded but is **not** "certified-current" (`REVIEW_RUBRIC_VERSION` names the
current one). Changing an anchor = a new version.

---

## Dataset: `guidance` (Rated)

The guidance corpus (JIMINY-RELEVANCE-001 `guidance_training_rows`). The
`outcome_label_correctness` dimension is what the guidance reinforcement sink
reads to derive the corrected outcome it applies to the live substrate.

### `relevance` — is the guidance about the agent's actual task?
| 0 | 1 | 2 | 3 | 4 |
|---|---|---|---|---|
| off-topic — unrelated to the agent's task | tangential — same area, not this task | related — touches the task obliquely | on-topic — clearly about this task | precise — directly addresses this exact task |

### `actionability` — can the agent act on it?
| 0 | 1 | 2 | 3 | 4 |
|---|---|---|---|---|
| advisory prose — no action implied | vague principle — no concrete step | general direction — a step could be inferred | specific guidance — a clear step is implied | executable directive — names a specific, executable action |

### `outcome_label_correctness` — was the auto verdict right?
| 0 | 1 | 2 | 3 | 4 |
|---|---|---|---|---|
| exactly wrong — auto verdict is the opposite of the truth | mostly wrong | unclear — auto verdict is defensible either way | mostly right | exactly right — auto verdict matches what the agent did |

**Sink mapping** (Epic 5): the corrected outcome the guidance sink applies is
derived from `outcome_label_correctness` against the item's `auto_label` — a high
score affirms the auto verdict; a low score inverts it (e.g. auto `ignored` +
`outcome_label_correctness=0` → corrected `followed`).

---

## Dataset: `stub` (Rated) — self-test only

Single dimension `quality` (0 unusable … 4 excellent). Gold-only (NoopSink).

---

## Future datasets (lightly scoped — `hitl-review-002`)

- **`sft`** (Rated): `correctness`, `helpfulness`, `style`. A gold ≥ threshold
  marks the row accepted for curation.
- **`dpo`** (Ranked): chosen/rejected over the pair's alternatives + confidence.
