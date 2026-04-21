---
created: 2026-04-21
updated: 2026-04-21
version: v0.8.6
author: reh3376
status: active
phase: FT-OAI-001
---

# OpenAI Fine-Tuning Pipeline

## Summary

**Feature**: OpenAI Fine-Tuning Post-Processor + Evaluation Harness
**Summary**: Converts MDEMG's curated MLX chat JSONL into OpenAI-shaped train/val files, launches and monitors an OpenAI fine-tuning job, then runs a seeded cosine-similarity evaluation of the fine-tuned model against its base. Produced the first in-house fine-tune: `ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq`.

## Vision & Goals

MDEMG generates high volumes of structured LLM interactions across ~10 task types (`ape.reflect`, `consulting.classify`, `retrieval.query_classify`, `jiminy.*`, `hidden.*`, …). Each task has a fixed JSON output schema defined in its ULTS spec. Off-the-shelf base models (e.g. `gpt-4.1-mini-2025-04-14`) follow these schemas well enough for bootstrap but drift on rarer task variants and occasionally hallucinate fields.

The fine-tuning pipeline closes that loop: feed the captured `llm_interactions` (quality-filtered + sanitized + temporally split) back into OpenAI's SFT API, then gate promotion of the fine-tuned model behind a measurable lift on a held-out test slice. This forms the **outer loop** of MDEMG's self-improvement — complementing RSIC's online micro-corrections with an offline batched retraining pass.

Goals:
- Close the write→train→eval→promote loop for hosted models (OpenAI first; extensible to other providers).
- Preserve task-specific JSON structure (parse-pass rate MUST NOT regress).
- Surface per-task wins/losses so the next batch can target known weak tasks.
- Keep the adapter a pure post-processor on the existing curate pipeline — no duplication of temporal-split or quality-gate logic.

## Current State

**FT-OAI-001 status**: ✅ COMPLETE (2026-04-21)

First production fine-tune delivered. Held-out eval (300 records, seed=42, 10 tasks) shows:

| Metric | Baseline | Fine-Tuned | Δ |
|---|---|---|---|
| Mean cosine similarity | 0.8322 | 0.8641 | **+0.032** |
| JSON parse-pass rate | 0.973 | 0.973 | **±0.000** |
| Win / Loss / Tie (±0.05) | — | 133 / 17 / 150 | **7.8:1 ratio** |

Verdict: **MARGINAL** (mean Δ below the +0.05 bar, but W/L ratio strong and no format regression). Run logged at `training_data/openai_ft/20260420/run_notes.md`.

### Architecture

```
TSDB (llm_interactions)
     │
     ▼
mdemg data curate --paradigm sft              # existing pipeline
     │      (quality_filter → format_converter → dataset_versioner)
     ▼
training_data/curated/sft_interactions/versioned/
     ├── train.jsonl    (MLX chat format, temporally split)
     ├── val.jsonl
     └── test.jsonl     (held out from both training and adapter input)
     │
     ▼
neural/training/openai_ft_adapter.py          # POST-PROCESSOR (new)
     │      think-block strip • schema check • tiktoken count
     │      • per-record context-limit gate • cost estimate
     │      • optional by_task/ specialist split • manifest.json
     ▼
training_data/openai_ft/<YYYYMMDD>/
     ├── combined_train.jsonl
     ├── combined_val.jsonl
     ├── manifest.json
     ├── rejection_log.jsonl
     └── by_task/ …                           (optional)
     │
     ▼
scripts/openai_ft_upload_and_launch.py        # network step — gated by --max-cost-usd
     │      openai.files.create() ×2 → fine_tuning.jobs.create()
     ▼
OpenAI fine-tuning job (ftjob-…)
     │      monitored via scripts/openai_ft_check.py (polls /fine_tuning/jobs/<id>)
     ▼
Fine-tuned model: ft:<base>:<org>:<suffix>:<hash>
     │
     ├─► scripts/openai_ft_baseline_eval.py   # eval harness
     │      seeded sample (seed=42, n=300) from test.jsonl
     │      calls either base or FT model, scores against ground truth
     │      via text-embedding-3-small cosine, parse_pass, finish_reason
     │
     └─► scripts/openai_ft_compare.py         # side-by-side comparator
            eval_comparison.md + per-task W/L/T + 5 worst regressions + 5 best gains
```

**Key design choice**: the adapter is a **post-processor**, not a replacement for `dataset_versioner.py`. Temporal split lives in one place. Adding a new provider (Anthropic, etc.) means a sibling adapter with the same contract, not forking the curate pipeline.

### Workflow

End-to-end execution (FT-OAI-001 reference run):

```bash
# 1. Curate — uses existing pipeline (quality gates, temporal split, MLX manifest)
mdemg data curate --paradigm sft \
  --space-id mdemg-dev \
  --out training_data/curated/sft_interactions/versioned \
  --version v1

# 2. Adapter — strip <think>, tokenize, validate, build manifest
python -m training.openai_ft_adapter \
  --input-dir training_data/curated/sft_interactions/versioned \
  --output-dir training_data/openai_ft/20260420 \
  --model gpt-4.1-mini-2025-04-14 \
  --by-task

# 3. Review manifest, decide whether to launch (cost + row count)
cat training_data/openai_ft/20260420/manifest.json | jq .cost_estimate_usd

# 4. Upload + launch (hard-gated by --max-cost-usd)
python scripts/openai_ft_upload_and_launch.py \
  --input-dir training_data/openai_ft/20260420 \
  --model gpt-4.1-mini-2025-04-14 \
  --suffix mdemg-ftoai001 \
  --max-cost-usd 175.00

# 5. Monitor
python scripts/openai_ft_check.py --job-id ftjob-oJoclV0D0uakQBZ93NzlEFPV --watch

# 6. Baseline eval (runs base model on seeded sample)
python scripts/openai_ft_baseline_eval.py \
  --model gpt-4.1-mini-2025-04-14 \
  --test-file training_data/curated/sft_interactions/versioned/test.jsonl \
  --output-dir training_data/openai_ft/20260420/eval/baseline \
  --sample-size 300 --seed 42 --max-output-tokens 4096 --max-cost-usd 10.00

# 7. FT eval (same seed → same records → apples-to-apples)
python scripts/openai_ft_baseline_eval.py \
  --model ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq \
  --test-file training_data/curated/sft_interactions/versioned/test.jsonl \
  --output-dir training_data/openai_ft/20260420/eval/ft \
  --sample-size 300 --seed 42 --max-output-tokens 4096 --max-cost-usd 10.00

# 8. Compare
python scripts/openai_ft_compare.py \
  --baseline-dir training_data/openai_ft/20260420/eval/baseline \
  --ft-dir training_data/openai_ft/20260420/eval/ft \
  --output training_data/openai_ft/20260420/eval_comparison.md
```

### Configuration

| Component | Flag / env var | Default | Purpose |
|---|---|---|---|
| Adapter | `--model` | _required_ | Resolves tokenizer (via `tiktoken.encoding_for_model`) + context limit + price. |
| Adapter | `--by-task` | off | Also emit per-task specialist `{train,val}.jsonl` under `by_task/`. |
| Adapter | `--context-limit-override` | profile default | Per-record token cap (65,536 for gpt-4.1-mini / gpt-4o-mini FT). |
| Upload | `--max-cost-usd` | 50.00 | Hard-gate: aborts before `files.create()` if manifest estimate exceeds cap. |
| Upload | `--suffix` | _required_ | Becomes the middle segment of the FT model ID. |
| Eval | `--sample-size` / `--seed` | 300 / 42 | Seeded deterministic subsample — identical records across base + FT runs. |
| Eval | `--max-output-tokens` | 1024 | Per-call completion cap. **FT run used 4096** to avoid truncation (see Known Limitations). |
| Eval | `--max-cost-usd` | 10.00 | Hard-gate before issuing any requests; refuses to run if projected > cap. |
| Eval | `--embed-model` | `text-embedding-3-small` | Used for cosine scoring only — not for FT input. |

## Notes

### Known Limitations

- **Baseline/FT eval asymmetry (FT-OAI-001 run)**: baseline was first eval'd with `--max-output-tokens 1024`; when ~60% of FT responses exceeded that, the FT re-eval was run at 4096. The FT win of +0.032 was measured with this asymmetric cap. Strict apples-to-apples requires re-running the baseline at 4096 (captured as an item for FT-OAI-002).
- **Per-record `parse_ok` bug**: the aggregate `parse_pass_rate` in `summary.json` is computed correctly (97.3%), but the per-record `parse_ok` column in `results.jsonl` is always `False`. Cosmetic — does not affect aggregate or the comparator — but blocks per-record forensics. FT-OAI-002 G1.
- **`finish_reason` not populated**: all records carry `finish_reason: null`. Eval script reads the wrong attribute off the OpenAI response object. FT-OAI-002 G1.
- **Auto-hyperparameters**: OpenAI picked `n_epochs=3`, `batch_size=4`, `lr_multiplier=2` for the FT-OAI-001 run. Actual billed cost (~$155) ran ~1.66× the single-epoch estimate ($93.13). The adapter's cost estimator does not model auto-epoch multipliers; FT-OAI-002 will add an `--epochs auto` cost envelope.
- **Mild overfitting after step 1200**: `best_val_loss=0.68360` at step 1200 vs `final_val_loss=0.81301` at step 1500. Future runs should consider `n_epochs=2` when the loss curve confirms this pattern.

### Risks & Gaps

- **Training signal noise on `__unattributed__` task**: the 5 worst regressions are all `{"type":"none","summary":""}` ground-truth records where the FT model hallucinates a recommendation. The 5 best gains are the inverse — baseline hallucinated, FT correctly returned `none`. Indicates the task-type attribution is genuinely ambiguous in training data, not that the model is getting worse.
- **`retrieval.intent_translate` regression (Δ=−0.079, n=4, 0W/3L)**: flagged for attention in FT-OAI-002. Small n but the direction is clear.
- **No cost cap on OpenAI side**: once `fine_tuning.jobs.create()` succeeds, the only ceiling is the project hard quota. Set the project hard limit to something tolerable before launching (FT-OAI-001 used $172).
- **Provider lock-in**: current adapter is OpenAI-only. Same architecture would work for Anthropic FT API / Fireworks / etc. — sibling adapter pattern is preferred over parameterising the existing one.

### Future Improvements

Tracked as Sprint **FT-OAI-002** (see `scripts/tsdb_data_review_2026-04-01.json` and follow-up task):
- **G1** — fix `parse_ok` / `finish_reason` / token-count recording in eval harness
- **A1–A7** — add per-record fields: input_tokens, output_tokens, latency_ms, retry_count, truncation_flag, embedding_model_version, hallucination_indicator
- **T1–T5** — training signal capture: per-epoch val loss curves persisted locally (not just the one OpenAI CSV), n_epochs override, `best_val_loss` step tracked, per-task sample weights, RAFT ratio >0 experiment
- **O1–O4** — operational: queue-wait telemetry, automatic re-submit on queue-stuck, project-quota pre-check, cost envelope for auto hyperparameters

## API Endpoints

None. This pipeline is entirely offline/batch — it does not expose runtime endpoints. The produced fine-tuned model is called via the standard OpenAI chat completions API with the `ft:…` model ID.

## CLI Commands

| Command | Description |
|---------|-------------|
| `python -m training.openai_ft_adapter …` | Convert MLX chat JSONL → OpenAI FT-shaped train/val files + manifest. |
| `python scripts/openai_ft_upload_and_launch.py …` | Upload files to OpenAI + launch FT job (cost-capped). |
| `python scripts/openai_ft_check.py --job-id <id>` | Poll job state; `--watch` streams transitions. |
| `python scripts/openai_ft_baseline_eval.py …` | Seeded cosine-similarity eval of base or FT model. |
| `python scripts/openai_ft_compare.py …` | Side-by-side comparator → `eval_comparison.md`. |

## Configuration Reference

No server-side env vars. Script-level flags documented under **Configuration** above.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| `mdemg data curate --paradigm sft` | **requires** — produces the MLX-chat input. |
| `dataset_versioner.py` | **requires** — owns the temporal split; adapter is a pure post-processor. |
| `quality_filter.py` | **feeds-into** — upstream quality gates applied before adapter sees data. |
| OpenAI SDK (`openai>=1.50`) | **requires** — files API + fine_tuning.jobs API. |
| `tiktoken>=0.7` | **requires** — via `encoding_for_model()` to match `o200k_base` for gpt-4o/4.1 families. |
| RSIC outer loop | **feeds-into** — FT results inform which tasks RSIC should prioritise for online correction. |

## Related Files

- `neural/training/openai_ft_adapter.py` — adapter implementation
- `neural/training/tests/test_openai_ft_adapter.py` — unit tests
- `scripts/openai_ft_check.py` — job monitor
- `scripts/openai_ft_upload_and_launch.py` — uploader + launcher (cost-gated)
- `scripts/openai_ft_baseline_eval.py` — eval harness
- `scripts/openai_ft_compare.py` — base vs FT comparator
- `training_data/openai_ft/20260420/run_notes.md` — FT-OAI-001 run log (attempts, metrics, verdict)
- `training_data/openai_ft/20260420/eval_comparison.md` — FT-OAI-001 eval report
- `training_data/openai_ft/20260420/ft_training_metrics.csv` — OpenAI-provided training loss trajectory
