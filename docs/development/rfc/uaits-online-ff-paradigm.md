---
type: rfc
status: draft
created: 2026-05-06
author: reh3376 (drafted by Claude as Workstream C Action 2)
target: UAITS framework v0.7.0 → v0.8.0
hard_prereq_for: Note 07 (Forward-Forward Shallow Heads)
---

# RFC — Add `online_ff` Paradigm to UAITS

## Summary

Note 07 (Forward-Forward Shallow Heads) trains three small classifiers — promotion / trust-bin / context-appropriateness — using Hinton's Forward-Forward (FF) algorithm over frozen LLM embeddings. FF is fundamentally different from the four paradigms UAITS currently governs (SFT, DPO, RAFT, curriculum):

- **No labels needed in the classical SFT/DPO sense** — FF distinguishes "positive" vs "negative" examples by per-layer goodness, not output text.
- **Online / streaming friendly** — examples can be processed one at a time without epoch boundaries.
- **No backprop** — the data contract carries different fields than chat/dpo/raft.
- **Shallow heads, not the LLM** — UAITS today implicitly assumes the trained artifact is a fine-tuned LLM; FF heads are 2-3-layer MLPs over frozen embeddings.

Adding a 5th paradigm to a governed framework requires an RFC + spec extension. This document proposes `online_ff`, lists the schema deltas, defines the quality gates, and identifies the open governance questions before merge.

**Cost to land if accepted: <1 day** of schema + runner + feature-doc work. **Hard prereq for Note 07** — that sprint cannot ship a UAITS-compliant dataset spec until this RFC is resolved.

---

## 1. Background — current 4-paradigm UAITS

| Paradigm | Source table | Output format | Records what |
|---|---|---|---|
| `sft` | `llm_interactions` | chat | input/output pairs for supervised fine-tuning |
| `dpo` | `llm_interactions` ⨝ `constraint_outcomes` | dpo (chosen/rejected) | preference pairs from Jiminy follow-vs-ignore signals |
| `raft` | `llm_interactions` (with `retrieval_node_ids`) | raft (with optional retrieved context) | retrieval-augmented fine-tuning triples |
| `curriculum` | `metric_samples` | metadata | quality-weighted scheduling signal, not direct training data |

All four target the **monolithic LLM**: the trained artifact is `mdemg-llm-v1` or a successor. Quality gates are tuned for chat/text content (privacy scan, response-length, latency, model-name, error-non-empty, dedup).

`internal/training/pipeline.go` routes each record by `paradigm` to a paradigm-specific processor; `docs/tests/uaits/schema/uaits.schema.json` enforces the contract; `docs/features/uaits-framework.md` is the operator surface.

## 2. Note 07's requirements (the empirical case for `online_ff`)

Note 07 ships three FF-trained heads:

| Head | Input | Label source | Cardinality |
|---|---|---|---|
| Promotion classifier | frozen LLM embedding + co-activation features | RSIC-labeled "this promotion was a mistake" + historical promotions | binary |
| Trust-bin classifier | observation source metadata | user overrides (rule baseline) | 3-way (trusted/normal/untrusted) |
| Context-appropriateness | query context fingerprint + candidate observation embedding | RSIC click-through / effectiveness | binary |

The training data is **not text**. It's:
- `head_id` (one of the 3)
- `embedding` ([]float32, 3072-dim from `text-embedding-3-large`)
- `features` (auxiliary scalars: co-activation, source metadata, fingerprint)
- `polarity` ("positive" or "negative" for FF goodness pairing)
- `weight` (confidence/freshness weight for streaming retraining)

Quality gates differ in two key ways:
1. **No PII regex scan needed** — embeddings are already opaque; the source-record privacy scan happens upstream when the embedding is generated, not at FF-dataset-build time.
2. **No latency / model_name / response gates** — these are LLM-output gates that don't apply to embedding-derived training records.

Different gates needed:
- Embedding dimensionality match (must match the LLM's embedding model native dims, e.g. 3072 for text-embedding-3-large)
- Polarity balance (FF needs both positive and negative examples; reject datasets with <5% of either class)
- Online-staleness bound (max age of training records in days — stale records hurt online-FF more than batch SFT)
- Per-head minimum sample count

## 3. Proposal

### 3.1 Schema delta — `uaits.schema.json` v0.7.0 → v0.8.0

```diff
   "paradigm": {
     "type": "string",
     "description": "Training paradigm determining the pipeline path",
-    "enum": ["sft", "dpo", "raft", "curriculum"]
+    "enum": ["sft", "dpo", "raft", "curriculum", "online_ff"]
   },
```

```diff
   "format": {
     "output_type": {
       "type": "string",
       "description": "Output format matching the paradigm",
-      "enum": ["chat", "dpo", "raft", "metadata"]
+      "enum": ["chat", "dpo", "raft", "metadata", "ff_pairs"]
     },
+    "ff_head_id": {
+      "type": "string",
+      "description": "FF head identifier (online_ff only)",
+      "enum": ["promotion", "trust_bin", "context_appropriateness"]
+    },
+    "ff_embedding_dims": {
+      "type": "integer",
+      "description": "Required embedding dimensionality (online_ff only); must match producer's emit dims",
+      "minimum": 1,
+      "maximum": 16384
+    },
+    "ff_polarity_min_balance": {
+      "type": "number",
+      "description": "Minimum fraction of minority class (positive or negative) (online_ff only)",
+      "minimum": 0.0,
+      "maximum": 0.5,
+      "default": 0.05
+    },
+    "ff_max_record_age_days": {
+      "type": "integer",
+      "description": "Maximum age (days) of records included in this dataset (online_ff only)",
+      "minimum": 1,
+      "default": 90
+    }
   }
```

**Quality gates** — additive only, no existing gate semantics change. Add to `quality_gates`:

```diff
+    "require_embedding_dims": {
+      "type": "integer",
+      "description": "Required embedding dimensionality on each record (online_ff hard gate)",
+      "minimum": 1
+    },
+    "min_polarity_balance": {
+      "type": "number",
+      "description": "Minimum minority-polarity fraction (online_ff hard gate)",
+      "minimum": 0.0,
+      "maximum": 0.5
+    },
+    "max_record_age_days": {
+      "type": "integer",
+      "description": "Reject records older than N days (online_ff soft gate; affects retraining freshness)",
+      "minimum": 1
+    }
```

These default to "not enforced" when the dataset's paradigm ≠ `online_ff`, so existing 4-paradigm specs stay backward-compatible.

### 3.2 Source-table contract

`online_ff` datasets read from a **new** TSDB table proposed by Note 07 sprint plan: `ff_training_examples`. Schema:

```sql
CREATE TABLE ff_training_examples (
  example_id TEXT PRIMARY KEY,                  -- CUIDv2
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  head_id TEXT NOT NULL,                         -- 'promotion' | 'trust_bin' | 'context_appropriateness'
  embedding REAL[] NOT NULL,                    -- length matches ff_embedding_dims
  feature_json JSONB NOT NULL,                   -- auxiliary scalars
  polarity TEXT NOT NULL,                        -- 'positive' | 'negative'
  weight REAL NOT NULL DEFAULT 1.0,
  source_record_id TEXT,                         -- foreign-ish to the upstream observation
  source_record_type TEXT                        -- 'memory_node' | 'rsic_outcome' | 'user_override'
);
SELECT create_hypertable('ff_training_examples', 'recorded_at', chunk_time_interval => INTERVAL '7 days');
```

This is owned by the Note 07 sprint, not this RFC. UAITS's job is to declare the contract by which a `paradigm: online_ff` dataset reads from this table.

### 3.3 Pipeline routing (no governance impact, listed for completeness)

`internal/training/pipeline.go` currently switches on `paradigm`:

```go
switch dataset.Paradigm {
case "sft":   sftRouter(...)
case "dpo":   dpoRouter(...)
case "raft":  raftRouter(...)
case "curriculum": curriculumRouter(...)
default: return fmt.Errorf("unknown paradigm: %s", dataset.Paradigm)
}
```

Note 07 adds `case "online_ff": ffRouter(...)`. Quality-gate evaluation extends to honor the new `*_age_days`, `min_polarity_balance`, `require_embedding_dims` gates only when `paradigm == "online_ff"`.

### 3.4 Documentation impact

- `docs/features/uaits-framework.md` — bump version to v0.8.0; add §3.5 "Paradigm: online_ff (Forward-Forward shallow heads)" with the Note 07 link
- `docs/tests/uaits/specs/mdemg.uaits.json` — add 3 datasets (one per FF head): `mdemg_ff_promotion`, `mdemg_ff_trust_bin`, `mdemg_ff_context_appropriateness`
- `docs/tests/uaits/runners/uaits_runner.py` — accept the new `format.output_type=ff_pairs` shape; add `validate_online_ff` per-record check (embedding-dim match, polarity in {pos, neg}, weight > 0)

## 4. Migration path / backward compatibility

Schema change is **additive**: new enum values, new optional fields. Existing `mdemg.uaits.json` validates unchanged. Pipelines without an `online_ff` dataset declared see no behavioral change.

**Operator deployment order** (when Note 07 ships):

1. Bump `uaits.schema.json` enum (this RFC)
2. Land Note 07's TSDB migration creating `ff_training_examples`
3. Add the 3 `online_ff` datasets to `mdemg.uaits.json`
4. Note 07 sprint runs `mdemg data curate` against the spec to produce the FF training data
5. Note 07's `ff_trainer.py` consumes the curated data and trains the heads

No coordination needed across applications/forks — each application's UAITS spec is independent.

## 5. Governance gate (must pass before this RFC merges)

| Gate | Owner | How to evaluate |
|---|---|---|
| **G1**: Note 07 sprint plan exists in finalized form | Author | Verify `docs/research/mdemg_sprint_ideas/07-ff-shallow-heads.md` has a "Sprint Plan" section, sprint ID assigned, predecessor declared |
| **G2**: TSDB schema for `ff_training_examples` reviewed | DB owner | Schema lints clean, hypertable interval matches the >7-day-window pattern from V0017/V0019/V0020 |
| **G3**: Quality gate semantics agreed | Training-data owner | `min_polarity_balance` default 0.05 is the right floor; `max_record_age_days` default 90 matches the FF online-retraining cadence Note 07 assumes |
| **G4**: ff_pairs format JSON shape locked | Training-data owner | One record per row, fields {example_id, head_id, embedding, feature_json, polarity, weight, recorded_at} |
| **G5**: No conflict with the 4 existing paradigms | Reviewers | Review each existing dataset in `mdemg.uaits.json` for unintended side effects under the new schema (none expected — additive change) |

## 6. Open questions

1. **Should curriculum scheduling apply to online_ff?** UAITS's curriculum paradigm produces `metadata` for quality-weighted scheduling. FF heads' streaming nature might not benefit from upstream weighting. Default: no curriculum integration in Note 07; can be added later if needed.

2. **Is `ff_pairs` the right output_type name?** Alternative: `embeddings_with_polarity`, `ff_examples`. `ff_pairs` is shorter and matches the literature.

3. **Should embedding dim be a hard gate or soft gate?** Hard. Mismatched dims cause runtime crashes in the FF trainer; rejecting at curate-time is cheaper than at train-time.

4. **Should min_polarity_balance default be 0.05 (5%) or higher?** 5% is the conservative floor — an FF head trained on 95/5 imbalance still works (slowly). Higher (e.g. 20%) prevents pathological imbalance but rejects datasets that would be salvageable with more data. Recommend starting at 0.05 and revisiting after Note 07 ships data.

5. **Per-head minimum sample count?** Currently absent from the proposed gates. Should we require e.g. 1000 examples per head before the dataset is "ready"? Recommend yes — add `min_record_count` (already in spec? check) to the dataset block.

## 7. Recommendation

**Approve as drafted.** Schema change is additive; Note 07's sprint plan is the natural deliverable to ship the runner extension and the 3 datasets. Land this RFC ahead of Note 07 sprint kickoff so spec governance doesn't block the implementation work.

## 8. Out of scope

- The Note 07 sprint plan itself (separate document)
- Implementation of the FF trainer
- The `ff_training_examples` table migration (lives in Note 07)
- Cross-application UAITS portability changes (none needed)

## References

- `docs/features/uaits-framework.md` — current 4-paradigm framework (v0.7.0)
- `docs/tests/uaits/schema/uaits.schema.json` — current JSON schema
- `docs/research/mdemg_sprint_ideas/07-ff-shallow-heads.md` — Note 07 plan
- `docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` §Note 07 — roadmap entry citing this RFC as hard prereq
- Hinton (2022) — "The Forward-Forward Algorithm: Some Preliminary Investigations" (Paper 3 in white-paper review)
- Ororbia & Mali — "FF underperforms at scale; use where scale doesn't matter" (Paper 4)
