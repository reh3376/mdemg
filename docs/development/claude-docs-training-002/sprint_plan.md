# CLAUDE-DOCS-TRAINING-002 — Sprint Plan

## 1. Header & Metadata

Sprint: `CLAUDE-DOCS-TRAINING-002` · opened 2026-08-15 · branch `reh3376_dev01`
Effort: ~2h prep + ~9-10h training compute + ~1h post-train A/B smoke
Target span: 24-36h (single training run is the wall-time bottleneck)
Risk: low (chunking is deterministic + reversible; adapter is default-not-promoted)

**Follows**: `CLAUDE-DOCS-TRAINING-001` (shipped 2026-08-14/15 as proof-of-concept; adapter learned shape but hallucinated specific enum values on 3/3 smoke fixtures due to truncation on long reference pages)

## 2. Problem Statement

CLAUDE-DOCS-TRAINING-001 Epic 5 shipped `adapters/claude_docs_001/` with val loss 2.735 → 1.251 (54% reduction) but Epic 6 smoke inference on 3 golden rows found the adapter **hallucinated specific enum values / type definitions** while learning general Claude Code SDK shape correctly.

Root-cause analysis identified **truncation loss** as the primary factor: 112 rows (5.1%) in `qa.jsonl` exceeded mlx_lm.lora's 2048-token `--max-seq-length`, including:
- `env-vars` "Variables" section: **129,797 tokens** truncated to 2,048 (98.4% content lost)
- `settings` "Available settings": 62,688 tokens
- `commands` "All commands": 49,954 tokens
- `cli-reference` "CLI flags": 25,764 tokens
- `agent-sdk--typescript` `Options`: 21,054 tokens

These are exactly the highest-signal reference pages (specific enum values, type signatures, CLI flags, environment variables) that the adapter hallucinated on. The rest of the corpus fit comfortably; only the reference-heavy tail suffered.

## 3. Scope & Constraints

**In scope:**

- (E1) `neural/training/chunk_claude_docs.py` — recursive markdown chunker that splits over-sized sections on H4/H5/H6 boundaries → paragraph boundaries → line boundaries. Produces `qa_v2.jsonl`.
- (E2) `neural/training/split_claude_docs_v2.py` — leak-safe split that **preserves the V1 golden verbatim** so adapter_001 vs adapter_002 A/B comparison is direct (same 50-row fixture, same SHA).
- (E3) SFT dataset prep at `training_data/sft/claude_code_knowledge_v2/` (train + valid + manifest).
- (E4) 3-epoch LoRA training (`--n-epochs 3`) — cap per shipped LORA_N_EPOCHS_CAP. Retrain against Phase-5 base at same rank=32 α=64 config as v1.
- (E5) Live Tier-3 smoke: identical 3 golden fixtures used in v1 smoke, run through adapter_002; compare hallucination behavior directly.
- (E6) Sprint post + arch-rules update + PR.

**Out of scope:**

- Full-sweep A/B benchmark on all 50 golden rows (would add 30-60min compute; sprint plan's judgment: 3-row smoke is sufficient for the shape-vs-facts question this sprint answers).
- HITL grading of adapter output (separate follow-up sprint).
- Retrain with `--max-seq-length 4096` (chunking eliminates the need; larger seq-length would double training time without additional benefit).
- Corpus expansion beyond the current 130-URL scrape.
- Promotion decision — operator-only via `mdemg ft-loop promote`.

**Constraints:**

- V1 golden holdout MUST be preserved byte-identical (enables direct A/B).
- Training uses `neural/.venv/bin/python -m neural.training.train_ft` (arch rule from CLAUDE-DOCS-TRAINING-001).
- All UBENCH/ULTS/UxTS drift checks must stay green (v1 already passed on PR #622; v2 shouldn't touch spec counts).

## 4. Dependencies

- **CLAUDE-DOCS-TRAINING-001** (shipped) — provides qa.jsonl input, V1 golden holdout, ULTS+UBENCH task registration, adapter_001 for A/B baseline.
- **Phase-5 base**: `mlx-community/Qwen3-14B-4bit` SHA-pinned `a54ec18f`.
- **`neural/.venv`** with `mlx_lm` installed + `tiktoken` for accurate token counting during chunking.

## 5. Implementation Plan (sequential)

**Epic 1 — Chunker** (`neural/training/chunk_claude_docs.py`):
- Reads `qa.jsonl` (2191 rows).
- For each row with completion > `--max-tokens` (default 1500), recursively split via header → paragraph → line boundaries.
- Emit `qa_v2.jsonl` with chunked rows carrying `chunk_index`, `chunk_total`, `original_row_id` for auditability.
- Templated prompts get "(part N of M)" suffix on chunk rows.
- Emit `chunk_manifest.json` with per-oversized-row provenance.
- Uses `tiktoken cl100k_base` for token counting (falls back to char/3 approximation if tiktoken missing).

**Epic 2 — v2 leak-safe split** (`neural/training/split_claude_docs_v2.py`):
- Reads V1 golden's 50 rows → extract `(source_url, section_index)` exclusion tuples.
- Filters `qa_v2.jsonl` to exclude any chunk of a V1 golden section.
- Two-layer leak audit: 0 tuple overlap AND 0 prompt-string overlap.
- Emits `train_v2.jsonl` + `split_v2_manifest.json`.

**Epic 3 — SFT dataset prep** — inline Python (not a new script): reshape `train_v2.jsonl` into `{messages, meta}` format, 95/5 SHA-order split → `training_data/sft/claude_code_knowledge_v2/{train,valid,manifest}.jsonl`. Manifest carries FT-LORA-DATA schema.

**Epic 4 — Training**:
```
neural/.venv/bin/python -m neural.training.train_ft \
    --tier 1 --mode sft \
    --base-model .local-models/qwen3-14b-4bit-base \
    --expected-sha256 a54ec18f... \
    --dataset training_data/sft/claude_code_knowledge_v2 \
    --adapter-path adapters/claude_docs_002/ \
    --n-epochs 3 --batch-size 4
```
2031 iters (3 epochs × 2706 rows / batch 4). ETA ~9-10h M5 Max wall.

**Epic 5 — Live Tier-3 smoke** (post-train):
- Same 3 golden fixtures as v1 smoke:
  - Golden #1: `query() vs ClaudeSDKClient`
  - Golden #2: `EffortLevel` (v1 hallucinated `low/medium/high/ultra/auto`; reference is `Literal["low","medium","high","xhigh","max"]`)
  - Golden #3: `McpServerStatusConfig` (v1 hallucinated TypedDict; reference is Union type)
- Compare adapter_002 output vs (a) reference completion, (b) adapter_001 output.
- Success criteria: at least 1/3 factuality improvement (e.g., correct EffortLevel enum values).

**Epic 6 — Sprint post + PR**:
- Feature doc update (docs/features/... if factuality genuinely improves).
- CLAUDE.md pin: chunking pattern for docs training corpora.
- Comparison table in sprint post: adapter_001 vs adapter_002 per fixture.
- Promotion recommendation to operator.

## 6. Testing Plan (3 tiers)

**Tier 1 (unit):** dry-run modes on chunker + split_v2. Zero-length body, single-section body, over-max-single-paragraph body edge cases.

**Tier 2 (integration):** chunk_manifest verify — sum of chunk_counts across oversized rows == chunks_produced_from_over_rows. Split_v2 audit — 0 tuple + 0 prompt overlap.

**Tier 3 (live, required):** 3-row smoke inference against adapter_002; report verdict + explicit comparison to adapter_001.

## 7. Commit Strategy

- Commit 1: chunker + split_v2 + SFT prep (docs-only + Python scripts; adapters gitignored per shipped `adapters/*/*.safetensors` rule) — enables PR to open while training runs.
- Commit 2 (post-training): adapter_002 metadata (train_report, adapter_config, train_config) + sprint post + smoke findings.

## 8. Verification Checklist

- [x] Chunker dry-run on qa.jsonl: 2191 → 2910 rows, 24 residuals bounded to 1500-1543 tokens
- [x] Split_v2 leak audit: 0/0 PASS
- [x] SFT dataset prepped: 2706 train + 142 valid
- [x] Training kicked off with `--n-epochs 3 --batch-size 4`, 2031 iters
- [ ] Training completes with monotonic val_loss descent
- [ ] Smoke inference on 3 golden rows
- [ ] Comparison table adapter_001 vs adapter_002
- [ ] Sprint post + PR
- [ ] Operator promotion decision — WILL NOT ISSUE unless factuality materially improves

## 9. Documentation Update (never cut)

- `docs/development/claude-docs-training-002/sprint_post.md` — post-training results + A/B verdict
- `docs/features/claude-code-knowledge-adapter.md` — extend with chunking pattern if promotion happens
- CLAUDE.md — arch rule: "when training on doc corpora with sections >2K tokens, ALWAYS chunk before training. Never rely on `--max-seq-length` truncation to handle the overflow."

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| Chunking breaks code-block or table content mid-structure | Low | Recursive split prefers header/paragraph boundaries; line-split is last resort. 24 residuals are all 1500-1543 tokens (very close to gate, minimal line-split damage). |
| 3 epochs still insufficient for fact-memorization | Med | If v2 smoke still hallucinates, next sprint tries n_epochs=5 (would need epoch_cap raise) OR corpus expansion. |
| Base-model saturation — LoRA rank=32 caps how much new knowledge fits | Low | Phase-5 baseline showed adapter absorbing 41.94M trainable params without saturating. Bumping rank to 64 is a knob if needed. |
| A/B on 3 fixtures is small-sample | Known | Explicit sprint scope: 3-fixture smoke is enough to answer "did shape learning improve to facts". Full 50-row A/B is separate follow-up. |
| Wall time 9-10h delays session | Accepted | Training runs in background; sprint post shipped after completion notification. |

## 11. Rollback Procedures

- **Chunking**: `qa_v2.jsonl` is additive; V1 `qa.jsonl` untouched. Delete v2 outputs to revert.
- **Adapter_002**: gitignored; delete `adapters/claude_docs_002/*.safetensors` to reclaim disk. adapter_001 unchanged and still available.
- **No promotion**: `mdemg ft-loop promote` will NOT be issued unless smoke shows material improvement. Production `mdemg-llm-v1` (no docs adapter) stays live regardless.

## 12. Documents Accessed

- `docs/development/claude-docs-training-001/{sprint_post,epic_1_2_report}.md` — root-cause + smoke findings.
- `neural/training/{curate,split,prep}_claude_docs*.py` — v1 shipped scripts.
- `training_data/claude-docs/curated/qa.jsonl` (2191 rows) + `scrape_manifest.json`.
- `training_data/eval/claude_code_knowledge_golden.jsonl` (V1 golden, preserved).
- `training_data/sft/tier1/manifest.json` — FT-LORA-DATA schema reference.
- mlx_lm.lora truncation warnings from v1 training log (`adapters/claude_docs_001/training_log.jsonl`).
- Live tiktoken cl100k_base for token counting during chunking.
