# MDEMG Usage Corpus (`mdemg_usage_v1`)

**Sprint**: MDEMG-USAGE-CORPUS-CURATE-001 (2026-08-24) · task #144
**Status**: shipped — 1,198 rows curated deterministically from `mdemg-dev` substrate; leak-audit CLEAN vs `valid_clean.jsonl`
**Family location**: `training_data/sft/mdemg_usage_v1/{train.jsonl,manifest.json,distribution_report.txt,leak_audit.json}`

## Why

`mdemg-llm-v1` (the shipped LoRA on Qwen3-14B-4bit) knows Claude Code deeply via the `claude_code_knowledge` family (2,706 rows), but knows **NOTHING** about MDEMG itself. Retrieval-augmented generation via the substrate (MDEMG-DOCS-INGEST-001 + RETRIEVAL-META-DOC-SUPPRESSION-001) closes the reactive gap — "given the query, retrieve then answer." A small usage-oriented SFT family closes the proactive gap — "produce MDEMG-shaped answers WITHOUT hitting retrieval every turn."

This is the corpus a future `MDEMG-USAGE-LORA-001` sprint would train on. **This sprint is corpus-only; no retrain.**

Operator directive 2026-08-24 (deep-dive workflow `wf_b389463a-61b` final rec Alt 1): "only fine-tune on how to use the mdemg framework."

## What ships

- `train.jsonl` — 1,198 Q&A rows in the shipped `{"messages":[{system,user,assistant}], "meta":{...}}` shape (byte-identical to `claude_code_knowledge_v3_stripped`)
- `manifest.json` — SHA + per-surface counts + per-path-prefix top-20 + word-count histogram + leak-audit summary + provenance (base model, source substrate, source ingest sprint)
- `distribution_report.txt` — human-readable curation summary
- `leak_audit.json` — every candidate row's overlap verdict vs every `valid_clean.jsonl` row (asymmetric 3-gram, threshold 0.30)

## Choices

### Deterministic-only curation (no LLM in the loop)

Mirrors `neural/training/curate_claude_docs.py` (the shipped `claude_code_knowledge` producer). One row per surviving H2/H3 section, question templated from the section header + a path-derived feature name. Zero LLM cost, zero LLM noise, fully auditable.

**Why not teacher-augmented**: the substrate is the source-of-truth for MDEMG's documented behavior. Adding an LLM to synthesize cross-section Q&A introduces the same drift class that `claude_code_knowledge_v3_stripped` was created to escape. Deterministic yield of 1198 is a substrate-limited ceiling; the plan's optional Epic 3 augment was intentionally skipped after measurement showed even min_words=10 yields only 1457 rows.

### Min-words = 30 threshold

Sweep at plan Epic 2 gate:

| min_words | rows | notes |
|---|---|---|
| 40 | 1103 | too many stubby sections dropped |
| **30** | **1198** | **shipped** — good yield, minimum quality |
| 25 | 1245 | marginal gain; opens door to low-signal 25-30-word snippets |
| 20 | 1327 | quality drops (single-sentence sections) |
| 10 | 1457 | still doesn't hit 1500; not worth the noise |

Ships at 30 as the pareto knee.

### `task_name = 'mdemg.usage'` (new task)

Distinct from `claude.code_knowledge` (the frozen Claude Code corpus). Retrain configs can weight or hold-out separately.

### Sampling group `T` (teacher = self / substrate)

The corpus is authored by MDEMG's own docs — no OpenAI teacher involvement. `sampling_group=T` matches the shipped taxonomy from `claude_code_knowledge*`.

### Stable row_id via source_node_id hash

`meta.row_id = "mdemg_usage__" + sha256(source_node_id)[:16]` — deterministic. If the substrate ingests a doc twice at different times, the two produce identical `row_id`s and downstream dedup catches it.

### Frozen families explicitly untouched

`claude_code_knowledge_v3_stripped` + `family_reasoning_think` + `family_classify_notink` + `family_structured_notink` + `tier1` are FROZEN per operator directive. Epic 6 SHA-precheck + SHA-postcheck asserts they're byte-identical to baseline. `mdemg_usage_v1` is a NEW family; nothing existing is modified.

## How it works

```
mdemg-dev substrate (Neo4j)
  ├── MemoryNode role_type=any, path CONTAINS one of:
  │     docs/features/… (997)  docs/user/… (~275)  docs/api/… (~300)
  │     cli-reference/… (37)   CLAUDE.md (5)
  │
  ▼ scripts/mdemg_usage_export_docs.py
raw/nodes.jsonl (1,583 rows)
  │  {node_id, path, surface, section_header, content, content_sha256}
  │
  ▼ scripts/mdemg_usage_curate.py --min-words 30
mdemg_usage_v1/train.jsonl (1,198 rows)
  │  {messages:[system,user,assistant], meta:{task_name:mdemg.usage, ...}}
  │  (skip: 362 min_words, 13 nav_header, 10 junk_path)
  │
  ▼ scripts/mdemg_usage_leak_audit.py --threshold 0.30
mdemg_usage_v1/leak_audit.json  (clean=true, 0 hits vs 290 valid_clean rows)
  │
  ▼ scripts/mdemg_usage_manifest.py
mdemg_usage_v1/manifest.json + distribution_report.txt
```

## How to use

### Reproduce end-to-end (against live mdemg-dev)

```bash
# Epic 1: export ingested docs from substrate
python3 scripts/mdemg_usage_export_docs.py

# Epic 2: curate Q&A rows
python3 scripts/mdemg_usage_curate.py --min-words 30

# Epic 4: leak audit (hard-gates Epic 5)
python3 scripts/mdemg_usage_leak_audit.py

# Epic 5: manifest + distribution report
python3 scripts/mdemg_usage_manifest.py
```

### Dry-run the yield curve

```bash
for mw in 40 30 20 15 10; do
  echo "=== min_words=$mw ==="
  python3 scripts/mdemg_usage_curate.py --dry-run --min-words $mw
done
```

### Verify frozen families untouched after any regeneration

```bash
sha256sum training_data/sft/claude_code_knowledge_v3_stripped/train.jsonl \
          training_data/sft/tier1/train.jsonl
# Compare against training_data/mdemg_usage/raw/frozen_families_baseline.sha256
```

### Consume in a future LoRA retrain

Point the trainer's manifest at `training_data/sft/mdemg_usage_v1/manifest.json` alongside existing families (`claude_code_knowledge_v3_stripped`, `family_*`, `tier1`). Do NOT concatenate — the trainer's own sampler weights by manifest per family. Reference: `neural/training/dataset_versioner.py` shape.

## How to extend

Adding a new surface to the substrate corpus:

1. Ingest the new source into `mdemg-dev` via MDEMG-DOCS-INGEST-001's pipeline (see `docs/features/mdemg-docs-ingest.md`).
2. Add a `SurfaceRule` entry to `scripts/mdemg_usage_export_docs.py::SURFACES`.
3. Update the WHERE clause in `build_query()` with the new `n.path CONTAINS` predicate.
4. Re-run the 4-step pipeline above.
5. Compare row-count delta + per-surface delta in the new `distribution_report.txt`.
6. Re-verify leak-audit clean.
7. Bump manifest's family_name if the shape change is significant (`mdemg_usage_v2` etc.).

Curator quality knobs (env-tunable via CLI flags per `never-hardcode-config`):

| Flag | Default | Purpose |
|---|---|---|
| `--min-words` | 40 (shipped: 30) | Drop stubby sections |
| `--in` | `training_data/mdemg_usage/raw/nodes.jsonl` | Epic 1 output |
| `--out` | `training_data/sft/mdemg_usage_v1/train.jsonl` | Row destination |
| `--dry-run` | off | Print counts + skip reasons only, no write |

## Verification results

See `docs/development/mdemg-usage-corpus-curate-001/sprint_post.md` for the shipping-day evidence:
- 39/39 unit tests pass (`scripts/test_mdemg_usage_curate.py`)
- Integration end-to-end pipeline green
- Live Tier 3: 1198 rows / 0 leaks / 5 surfaces covered / SHA-verified / frozen families untouched / 3 sample node_ids resolve back to substrate

## References

- MDEMG-DOCS-INGEST-001 (task #142) — supplied the substrate
- RETRIEVAL-META-DOC-SUPPRESSION-001 (task #143) — retrieval-quality cleanup (parallel, not blocking here)
- PHASE-E1-CORPUS-AUDIT-001 (task #132) — leak-audit shape source
- PHASE-E2-CORPUS-CURATION-001 (task #133) — manifest shape source
- `neural/training/curate_claude_docs.py` — canonical deterministic curator (reference implementation)
- Deep-dive workflow `wf_b389463a-61b` — 2026-08-24 operator ratification
