# Sprint Plan: OpenAI Fine-Tuning Data Generation (Test Run)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-OAI-001 |
| Title | OpenAI Fine-Tuning Data Generation (Test Run) |
| Date | 2026-04-20 |
| Owner | reh3376 |
| Branch (parent) | `reh3376_dev01` (off `main` post-RELEASE-v0.8.5 merge) |
| Repository | `/Users/reh3376/mdemg` |
| Output directory | `/Users/reh3376/mdemg/training_data/` (git-ignored) |
| Predecessors | RELEASE-v0.8.5 (merged 2026-04-20) |
| Type | Feature + pipeline validation (reversible pre-upload; OpenAI spend irreversible post-Epic 6) |
| Target completion | 1 day (export → generate → validate → upload) |

### Target Model (resolved)

- **Primary:** `gpt-4.1-mini-2025-04-14`
- **Tokenizer:** `o200k_base` (via `tiktoken.encoding_for_model("gpt-4.1-mini-2025-04-14")`)
- **Fine-tune context:** 131,072 tokens (confirm against OpenAI docs at run time)
- **Configurability requirement:** The `openai_ft_adapter.py` CLI MUST accept `--model <name>` with a module-level `_MODEL_PROFILES` dict that maps model name → `{tokenizer_encoding, context_limit, provider}`. Default: `gpt-4.1-mini-2025-04-14`. This lets future runs target other OpenAI models without code changes.
- **Non-OpenAI providers (Qwen 3.5, etc.):** **Out of scope for this sprint.** They require a separate provider adapter (different upload API, different tokenizer). Note in §10 Risks / follow-up. The `_MODEL_PROFILES` design leaves the door open: adding a `provider: "qwen"` entry plus a sibling `qwen_ft_adapter.py` is a future sprint.

### Epic 0 — Pre-Sprint Readiness Gate

Before any other epic begins:
- [ ] Confirm `reh3376_dev01` is fast-forwarded with `main` after RELEASE-v0.8.5 merge (`git log main..reh3376_dev01` empty; `git log reh3376_dev01..main` empty).
- [ ] Confirm v0.8.5 formula available via Homebrew tap (`brew info mdemg` shows 0.8.5).
- [ ] TSDB running and reachable (`mdemg tsdb status`).
- [ ] `OPENAI_API_KEY` exported in shell.

**Gate:** All 4 boxes checked. Block otherwise.

---

## 2. Problem Statement

MDEMG has been collecting LLM interaction data into TimescaleDB since v0.7.0 via the `llm_interactions` table, with privacy scrubbing, guidance_id correlation, RAFT context enrichment, and think-content extraction (PRs #217, #218, #219). The UAITS framework (10th UxTS framework) declares 4 training datasets for Qwen3.6-35B-A3B LoRA fine-tuning on Apple Silicon (`docs/tests/uaits/specs/mdemg.uaits.json`). The existing pipeline (`quality_filter.py` → `format_converter.py` → `dataset_versioner.py`) produces MLX chat format JSONL; `mdemg data curate --paradigm sft` orchestrates the full chain.

**What's missing:** A **test run that validates the pipeline end-to-end** against an actual third-party fine-tuning provider — specifically OpenAI. This will:

1. Prove the collected data is clean, formatted, and non-empty enough to fine-tune against.
2. Surface any format incompatibilities before committing to the full Qwen3 training cycle.
3. Give us an observable baseline for how a commodity model performs when fine-tuned on MDEMG's SFT data.
4. Exercise the governance path: export → filter → convert → validate → split → upload.

**Key compatibility observation:**
OpenAI's supervised fine-tuning format is `{"messages": [{"role": "system"|"user"|"assistant", "content": "..."}]}`. The existing `format_converter.py` already produces **exactly this shape** — the MLX chat format and OpenAI chat format are the same structure. The only adaptations required are:

- **Strip `<think>...</think>` blocks** from assistant content (OpenAI models don't emit Qwen3-style reasoning blocks).
- **Enforce OpenAI's token and count limits** (10 min examples; gpt-4o-mini fine-tune supports **65,536 tokens** of training context as of late-2024 OpenAI docs; gpt-4.1-mini supports more).
- **Produce a train/validation split** matching OpenAI's recommended layout.

---

## 3. Scope & Constraints

### 3a. Scope

### In Scope

- Export all `llm_interactions` from local TSDB via `mdemg data export`.
- Inspect raw data (per-task counts, model distribution, token distribution, error rate).
- Run existing `quality_filter.py` and `format_converter.py` on the export.
- Build **one new Python module**, `openai_ft_adapter.py`, that:
  - Strips think blocks.
  - Validates OpenAI token/count constraints.
  - Performs stratified temporal train/validation split.
  - Writes per-task and combined JSONL outputs to `/Users/reh3376/mdemg/training_data/openai_ft/<date>/`.
  - Produces a manifest with row counts, token estimates, cost projection, and validation status.
- Create `/Users/reh3376/mdemg/training_data/` directory with `.gitignore` entry (training data is never committed).
- Run OpenAI's local validation (`tiktoken` pass + structural check) before upload.
- Upload training + validation files to OpenAI via API.
- Launch one fine-tuning job on `gpt-4o-mini-2024-07-18` with auto hyperparameters.
- Document the resulting fine-tuned model ID and baseline metrics.

### Out of Scope

- DPO pair generation (requires `constraint_outcomes` data; separate sprint).
- RAFT dataset with resolved Neo4j node content (current pipeline uses ids_only mode; separate sprint).
- Any Qwen3.6-35B-A3B training (separate hardware track; requires M-series Mac 128GB).
- Automated evaluation of the fine-tuned model (separate sprint — use existing `evaluate_ft.py` infrastructure).
- Modifying the existing `quality_filter.py` or `format_converter.py` (they are correct as-is for MLX; any OpenAI-specific deviation goes in the new adapter).
- UAITS spec modification (OpenAI doesn't get a new paradigm entry; it reuses `sft_interactions` data with a post-processing step).

---

### 3b. Hard Constraints

- **Data never leaves `training_data/` until validated.** No direct upload from TSDB. Every record passes through the 8-gate quality filter first.
- **No modification to existing Python modules** in `neural/training/`. The adapter is additive.
- **Privacy gate is inviolable.** `quality_filter.py` must return zero privacy violations before proceeding. If violations appear, STOP and debug the source.
- **Temporal split, not random.** Validation set must come from a later time window than training set (matches existing `dataset_versioner.py` behavior).
- **Minimum viable dataset:** OpenAI requires ≥10 examples. **Sprint-level gate:** ≥10 train + ≥2 val **per retained task**, AND ≥50 train combined, AND ≥20 val combined. Tasks below the per-task floor are dropped from `by_task/` output but kept in combined files.
- **`training_data/` must be git-ignored.** Never commit raw exports, filtered data, or final JSONL.
- **Context limit for fine-tuning:** gpt-4o-mini supports **65,536 tokens** of training context (as of late-2024 OpenAI docs); gpt-4.1-mini supports more. MDEMG system prompts average 2–4K tokens — nowhere near the cap. Records exceeding the configured `OPENAI_CONTEXT_LIMIT` must be **rejected and logged**, not truncated.

### 3c. Soft Constraints

- **Prefer `gpt-4o-mini-2024-07-18`** as the base model for the first run (cheapest, fastest, covers the format validation goal). `gpt-4o-2024-08-06` only if the mini model shows clear format-capture failure.
- **Keep `<think>` blocks stripped for OpenAI.** Do not train OpenAI models to emit Qwen3-specific reasoning blocks.
- **Use combined dataset first.** Per-task specialist fine-tunes come later if the combined run shows task-level contamination.

---

## 4. Dependencies

### Upstream

- TSDB running on localhost with accumulated `llm_interactions` data.
- `mdemg` CLI built at `bin/mdemg` (current: v0.8.5).
- `neural/` Python environment with training deps installed (`jsonschema>=4.20.0`, `cuid2>=2.0.0`).
- `OPENAI_API_KEY` set in shell environment.
- `openai` Python SDK installed: `pip install 'openai>=1.50' tiktoken` (≥1.50 ensures DPO/preference-tuning wrapper is present for any follow-up sprint).
- `scripts/cleanup_training_data.sql` available for TSDB cleanup between re-runs (optional; use with `mdemg data clean` or psql).

### Downstream (Informs Later Work)

- The adapter output format informs Phase 4A (RAFT retrieval context capture) and Phase 4B (ULTS spec framework) design.
- Success/failure of this run determines whether we pursue OpenAI fine-tuning as a parallel track or commit fully to Qwen3.

### Validation Requirements

- Before OpenAI upload: every JSONL file passes OpenAI's documented format requirements.
- Before sprint close: fine-tuning job reaches `succeeded` status OR fails with an actionable error message.

---

## 5. Implementation Plan

Execute sequentially. Each epic has an explicit **Gate** that must pass before the next begins (per CLAUDE.md sprint-format rule "Do NOT parallelize epics").

| Epic | Covers (tasks below) | Summary |
|---|---|---|
| E1 | Task 1 | Directory setup + gitignore |
| E2 | Task 2, 3 | TSDB readiness + raw export |
| E3 | Task 4, 5 | Quality filter + format conversion |
| E4 | Task 6 | Build `openai_ft_adapter.py` + tests |
| E5 | Task 7, 8 | Run adapter + local OpenAI format check |
| E6 | Task 9, 10, 11 | Upload + launch fine-tune + smoke test |
| E7 | Task 12 | Documentation (MANDATORY — never cut) |

### Pipeline Topology (resolved)

**Primary path (E2+E3): `mdemg data curate --paradigm sft`**

This single command orchestrates export → filter → convert → dataset_versioner via `paradigm_router.py:119`. Use this for the live run.

**Debug fallback (if any E2/E3 step fails or produces unexpected manifests):** Re-run the equivalent 3-step manual chain (`mdemg data export` → `quality_filter.py` → `format_converter.py`) to isolate the failing stage. Both paths are documented in Tasks 2–5 below — Task 2/3 (readiness + export) and Tasks 4/5 (filter + convert) serve as the debug breakdown. This is the first live run, so debuggability is load-bearing.

**Rule:** Start with `mdemg data curate`. Only drop to the manual chain if the single-call version produces unexpected output, and document the reason in `run_notes.md`.

### Task 1: Create `training_data/` Directory with Gitignore

**Effort:** XS (5 min)

1. `mkdir -p /Users/reh3376/mdemg/training_data/openai_ft`
2. Append to `/Users/reh3376/mdemg/.gitignore`:
   ```
   # Training data exports — never commit
   training_data/
   !training_data/.gitkeep
   !training_data/README.md
   ```
3. Create `training_data/README.md` explaining the directory's purpose (pipeline outputs, never committed, cleared after each run).
4. Create empty `training_data/.gitkeep`.

**Exit criteria:** `git status` shows only the README and .gitkeep; `training_data/openai_ft/` is ignored.

---

### Task 2: Verify TSDB Has Usable Data

**Effort:** XS (5 min)

Before exporting, confirm the data exists.

```bash
psql "$TSDB_DSN" -c "
  SELECT task_name, COUNT(*) AS total,
         COUNT(*) FILTER (WHERE error IS NULL OR error = '') AS clean,
         MIN(time) AS earliest, MAX(time) AS latest
  FROM llm_interactions
  WHERE space_id = 'mdemg-dev'
  GROUP BY task_name
  ORDER BY total DESC;"
```

**Exit criteria:**
- At least one task has ≥50 `clean` rows.
- Date range spans ≥7 days (supports meaningful temporal split).
- If fewer than 100 total clean rows exist, abort sprint and return to data collection.

---

### Task 3: Export TSDB to Local Archive

**Effort:** XS (5 min)

```bash
cd /Users/reh3376/mdemg
./bin/mdemg data export \
  --space-id mdemg-dev \
  --output training_data/raw/export-$(date +%Y%m%d).tar.gz \
  --tables llm_interactions \
  --since $(date -u -v-90d +%Y-%m-%dT%H:%M:%SZ)
```

Extract:
```bash
mkdir -p training_data/raw/extracted
tar -xzf training_data/raw/export-$(date +%Y%m%d).tar.gz \
  -C training_data/raw/extracted --strip-components=1
ls training_data/raw/extracted/   # expect: manifest.json, llm_interactions.jsonl
```

**Exit criteria:**
- Export completes without privacy violations.
- `llm_interactions.jsonl` is non-empty.
- Export manifest is valid JSON.

---

### Task 4: Run Existing Quality Filter

**Effort:** XS (10 min)

```bash
cd /Users/reh3376/mdemg/neural
PYTHONPATH=. python3 -m training.quality_filter \
  --input ../training_data/raw/extracted/llm_interactions.jsonl \
  --output ../training_data/filtered/llm_interactions.jsonl \
  --ults-dir ../docs/tests/ults/specs/ \
  --report ../training_data/filtered/filter_report.json \
  --dedup-key prompt
```

Review `filter_report.json`. Check:
- `excluded.privacy_violation == 0` (hard requirement).
- `output_rows / input_rows > 0.5` (if below 50%, investigate why).
- `task_distribution` shows reasonable spread.

**Exit criteria:** Filtered JSONL exists, zero privacy violations, per-task distribution reviewed and sane.

---

### Task 5: Run Existing Format Converter

**Effort:** XS (5 min)

```bash
cd /Users/reh3376/mdemg/neural
# --raft-ratio 0.0 → SFT-only test run per §3a Out of Scope;
# a separate RAFT sprint will revisit with --raft-ratio 0.8.
PYTHONPATH=. python3 -m training.format_converter \
  --input ../training_data/filtered/llm_interactions.jsonl \
  --output ../training_data/converted/mlx_chat.jsonl \
  --raft-ratio 0.0
```

Sample-inspect output:
```bash
head -1 /Users/reh3376/mdemg/training_data/converted/mlx_chat.jsonl | python3 -m json.tool
```

**Exit criteria:** Output JSONL has valid `{"messages": [...]}` structure per line with system/user/assistant roles.

---

### Task 6: Build `openai_ft_adapter.py`

**Effort:** M (2-3 hours)

**Location:** `neural/training/openai_ft_adapter.py`

### Split Architecture (resolved — two-stage)

**Upstream stage — `dataset_versioner.py` (via `mdemg data curate`):**
- Owns temporal train/val/test split (`dataset_versioner.py:126–157`).
- Produces MLX chat JSONL (`train.jsonl`, `val.jsonl`, `test.jsonl`) + `manifest.json` with temporal ranges, task distribution, dedup counts, quality gates.

**Downstream stage — `openai_ft_adapter.py` (NEW):**
- Pure post-processor. Does **not** re-split.
- Consumes the MLX `train.jsonl` and `val.jsonl` produced upstream.
- Outputs OpenAI-compatible JSONL + an OpenAI-specific manifest.

**Responsibilities:**

1. **Input:** MLX chat JSONL files (`train.jsonl`, `val.jsonl` from the curated output directory). Does NOT re-read raw filtered data.
2. **Think block stripping:** Remove `<think>...</think>` blocks from assistant content. Preserve the post-think response exactly.
3. **Token counting:** Use `tiktoken.encoding_for_model(model_name)` — resolves to `o200k_base` for `gpt-4.1-mini-2025-04-14` and other 4o/4.1 family models. Compute per-record total tokens (system + user + assistant).
4. **Length gate:** Reject records exceeding the model's `context_limit` from `_MODEL_PROFILES`. Log rejections to `rejection_log.jsonl`.
5. **No split logic** — that's dataset_versioner's job. Adapter processes `train.jsonl` and `val.jsonl` independently and writes `combined_train.jsonl` + `combined_val.jsonl` + `by_task/*.jsonl`.
6. **Output layout:**
   ```
   training_data/openai_ft/<YYYYMMDD>/
   ├── combined_train.jsonl          # All tasks, training split
   ├── combined_val.jsonl            # All tasks, validation split
   ├── by_task/
   │   ├── ape.reflect.train.jsonl
   │   ├── ape.reflect.val.jsonl
   │   ├── consulting.classify.train.jsonl
   │   ├── consulting.classify.val.jsonl
   │   └── ... (one pair per task with ≥10 train + ≥2 val records)
   ├── manifest.json                 # See schema below
   └── rejection_log.jsonl           # Records that failed OpenAI gates
   ```
7. **Manifest schema:**
   ```json
   {
     "generated_at": "2026-04-20T19:30:00Z",
     "source_export": "export-20260420.tar.gz",
     "source_export_rows": 4523,
     "quality_filter_rows": 3891,
     "think_blocks_stripped": 1247,
     "oversized_rejected": 42,
     "files": {
       "combined_train.jsonl": { "rows": 3078, "tokens_estimated": 4521903, "tasks": {...} },
       "combined_val.jsonl":   { "rows": 771,  "tokens_estimated": 1130482, "tasks": {...} },
       "by_task/ape.reflect.train.jsonl": { "rows": 245, "tokens_estimated": 382104 },
       "...": "..."
     },
     "openai_cost_estimate": {
       "model": "gpt-4o-mini-2024-07-18",
       "training_tokens": 4521903,
       "price_per_1m_tokens_usd": 3.00,
       "epochs": 3,
       "estimated_cost_usd": 40.70
     },
     "validation_checks": {
       "all_records_under_context_limit": true,
       "all_records_have_assistant_message": true,
       "all_records_valid_json": true,
       "min_examples_met_per_file": true
     }
   }
   ```
8. **CLI signature:**
   ```bash
   python3 -m training.openai_ft_adapter \
     --chat-input training_data/converted/mlx_chat.jsonl \
     --metadata-input training_data/filtered/llm_interactions.jsonl \
     --output-dir training_data/openai_ft/$(date +%Y%m%d)/ \
     --model gpt-4o-mini-2024-07-18 \
     --train-ratio 0.8 \
     --min-per-task 10 \
     --strip-think
   ```

**Tests (`neural/training/tests/test_openai_ft_adapter.py`):**
- Think block stripping preserves non-think content exactly.
- Records over the resolved `OPENAI_CONTEXT_LIMIT` (e.g. 65536 for gpt-4o-mini) are rejected; manifest records the count.
- Temporal split uses time, not random shuffle.
- `<10` train or `<2` val records per task → task omitted from per-task output but kept in combined.
- Manifest validates against its own JSON shape.
- Empty input produces empty output + non-zero exit code.

**Exit criteria:** Module implemented, 6+ tests passing, runs end-to-end on the filtered data.

---

### Task 7: Run OpenAI Adapter Against Real Data

**Effort:** XS (5 min)

```bash
cd /Users/reh3376/mdemg/neural
PYTHONPATH=. python3 -m training.openai_ft_adapter \
  --chat-input ../training_data/converted/mlx_chat.jsonl \
  --metadata-input ../training_data/filtered/llm_interactions.jsonl \
  --output-dir ../training_data/openai_ft/$(date +%Y%m%d)/ \
  --model gpt-4o-mini-2024-07-18 \
  --train-ratio 0.8 \
  --min-per-task 10 \
  --strip-think
```

Review `manifest.json`:
- `combined_train.jsonl` rows ≥ 80 (OpenAI's practical minimum).
- `combined_val.jsonl` rows ≥ 20.
- Total estimated cost for 3 epochs is acceptable.
- All validation checks pass.

**Exit criteria:** Manifest shows all green, per-task files present for tasks meeting threshold, combined files ready to upload.

---

### Task 8: Run OpenAI's Local Format Check

**Effort:** XS (10 min)

OpenAI provides a cookbook-style validation script that catches structural issues before upload. Create `scripts/openai_ft_check.py`:

```python
#!/usr/bin/env python3
"""Pre-upload format check for OpenAI fine-tuning JSONL files.
Mirrors the OpenAI cookbook validation logic.
"""
import json, sys, tiktoken
from collections import defaultdict
from pathlib import Path

# Context limit: resolve from the model you're fine-tuning.
# gpt-4o-mini-2024-07-18 fine-tune ≈ 65,536 tok; gpt-4.1-mini larger.
_CONTEXT_LIMITS = {
    "gpt-4o-mini-2024-07-18": 65536,
    "gpt-4.1-mini":           131072,  # confirm against current OpenAI docs
}

def check_file(path: Path, model: str = "gpt-4o-mini-2024-07-18") -> dict:
    # encoding_for_model resolves to o200k_base for gpt-4o/4.1 families;
    # cl100k_base is wrong for both and would under-count tokens.
    enc = tiktoken.encoding_for_model(model)
    context_limit = _CONTEXT_LIMITS.get(model, 65536)
    errors = defaultdict(int)
    total_tokens = []
    assistant_tokens = []
    n = 0

    with path.open() as f:
        for i, line in enumerate(f, 1):
            try:
                rec = json.loads(line)
            except Exception:
                errors["invalid_json"] += 1
                continue
            if "messages" not in rec:
                errors["missing_messages"] += 1
                continue
            msgs = rec["messages"]
            if not msgs or msgs[-1]["role"] != "assistant":
                errors["no_assistant_last"] += 1
            roles = {m["role"] for m in msgs}
            if not roles.issubset({"system", "user", "assistant"}):
                errors["invalid_role"] += 1
            rec_tokens = sum(len(enc.encode(m["content"])) for m in msgs)
            total_tokens.append(rec_tokens)
            for m in msgs:
                if m["role"] == "assistant":
                    assistant_tokens.append(len(enc.encode(m["content"])))
            if rec_tokens > context_limit:
                errors["over_context_limit"] += 1
            n += 1

    return {
        "file": str(path),
        "rows": n,
        "errors": dict(errors),
        "total_tokens_sum": sum(total_tokens),
        "total_tokens_mean": sum(total_tokens) / max(len(total_tokens), 1),
        "total_tokens_max": max(total_tokens) if total_tokens else 0,
        "assistant_tokens_mean": sum(assistant_tokens) / max(len(assistant_tokens), 1),
    }

if __name__ == "__main__":
    for path in map(Path, sys.argv[1:]):
        result = check_file(path)
        print(json.dumps(result, indent=2))
        if result["errors"]:
            sys.exit(1)
```

Run:
```bash
python3 scripts/openai_ft_check.py \
  /Users/reh3376/mdemg/training_data/openai_ft/$(date +%Y%m%d)/combined_train.jsonl \
  /Users/reh3376/mdemg/training_data/openai_ft/$(date +%Y%m%d)/combined_val.jsonl
```

**Exit criteria:** Both files report zero errors.

---

### Task 8.5: Downsample for Initial Run (RESOLVED — scope addition)

**Effort:** XS (20 min, one code change + rerun adapter)

**Decision (locked 2026-04-20):** Initial fine-tune run uses a bounded subset, not the full 30,729-row curated set. The full set at 3 epochs costs ~$1,429 which vastly overshoots the `--max-cost-usd 50` cap and is not proportionate to a format-validation run.

**Targets (locked):**
- 2,000 train rows / 400 val rows (random-seeded proportional — at 6.5% sample, random converges to population task distribution)
- 3 epochs
- Expected cost: ~$93 (raised `--max-cost-usd` cap to $100)

**Implementation:** Add `--max-train-rows`, `--max-val-rows`, `--sample-seed` flags to `openai_ft_adapter.py`. Record the subsample seed + hash in `manifest.json` so a later re-run reproduces the exact subset.

**Exit criteria:** `manifest.json` `totals.cost_estimate_usd` < 100; row counts match targets; sampling is deterministic on repeat runs with same seed.

---

### Task 8.6: Baseline Evaluation (RESOLVED — scope addition)

**Effort:** M (new script + one live run against base model)

**Rationale:** We cannot claim the fine-tune *improves* the model without measuring where we started. Baseline evaluation is the comparator — same 300-row sample from `test.jsonl`, same metrics, same harness, run against base `gpt-4.1-mini-2025-04-14` BEFORE fine-tuning.

**Harness:** `scripts/openai_ft_baseline_eval.py`
- Loads 300 records from curated `test.jsonl` (proportional by task — task labels recovered by joining `test.jsonl` records to `filtered.jsonl` via `sha256(system_prompt + user_prompt)` lookup)
- For each record: strips the assistant message → sends system+user to `--model` → captures response
- Metrics:
  - **Cosine similarity** between response and ground-truth assistant, embedded via `text-embedding-3-small`
  - **JSON-parseability pass rate** (many mdemg tasks emit JSON-shaped output; parse-fail is a clear regression signal)
- `--max-cost-usd` cap (default $10 — each eval run is ~$1–2)
- Outputs: `eval/baseline/results.jsonl` + `eval/baseline/summary.json` with per-task mean similarity, parse rates, and the exact 300-record sample so re-runs are deterministic

**Exit criteria:** `eval/baseline/summary.json` exists with `run_id`, `model`, `mean_cosine`, `parse_pass_rate`, `per_task_breakdown`, and `sample_seed`.

---

### Task 9: Upload to OpenAI

**Effort:** XS (5 min, then wait for ingestion)

```python
from openai import OpenAI
from pathlib import Path

client = OpenAI()
base = Path(f"/Users/reh3376/mdemg/training_data/openai_ft/{date_str}/")

train_file = client.files.create(
    file=open(base / "combined_train.jsonl", "rb"),
    purpose="fine-tune",
)
val_file = client.files.create(
    file=open(base / "combined_val.jsonl", "rb"),
    purpose="fine-tune",
)
print(f"train: {train_file.id}")
print(f"val:   {val_file.id}")
```

Wait until both show `status: processed` (usually 1-5 minutes for files under 10MB).

**Exit criteria:** Both files ingested successfully.

---

### Task 10: Launch Fine-Tuning Job

**Effort:** XS (5 min setup, 20-90 min training)

### Cost Cap (resolved — $50 default, hard gate)

`scripts/openai_ft_upload_and_launch.py` MUST:
- Accept `--max-cost-usd <float>` flag, **default 50.00**.
- Before `client.fine_tuning.jobs.create()`, read `manifest.json` → `openai_cost_estimate.estimated_cost_usd`.
- If estimate > `--max-cost-usd`: exit with code 2 and message `ABORT: estimated $X.XX exceeds cap $50.00. Re-run with --confirm-cost to override.`
- `--confirm-cost` flag bypasses the cap but requires explicit user-supplied value.

This is a **hard gate** — no job is launched without the check passing. Documented in `run_notes.md` whether the cap was enforced or bypassed.

```python
job = client.fine_tuning.jobs.create(
    training_file=train_file.id,
    validation_file=val_file.id,
    model="gpt-4o-mini-2024-07-18",
    suffix="mdemg-sft-v1",
    # Hyperparameters intentionally omitted — see §7 for rationale.
)
print(f"job id: {job.id}")
```

Monitor via `client.fine_tuning.jobs.retrieve(job.id)` or the dashboard.

**`run_notes.md` required fields** (template — create regardless of success or failure):
```
- job_id:
- base_model:
- fine_tuned_model (populated on success):
- start_time (UTC):
- end_time (UTC):
- n_epochs_actual:
- final_train_loss:
- final_val_loss:
- total_cost_usd:
- error (populated on failure, else null):
- observations (free-form):
```

**Exit criteria:** Job reaches `status: succeeded` with a `fine_tuned_model` ID. If it fails, capture the error and populate all fields that apply.

---

### Task 11: Post-FT Re-Evaluation + Comparison Report (RESOLVED — supersedes qualitative smoke test)

**Effort:** S (15 min + API time)

Quantitative replacement for the old qualitative smoke test. Runs the **same 300-record sample** from Epic 5.5 through the fine-tuned model (deterministic via shared seed) and produces a formal comparison report.

**Scripts:**
- `scripts/openai_ft_baseline_eval.py --model ft:gpt-4.1-mini-2025-04-14:...:JOBID --output-dir .../eval/ft/`
- `scripts/openai_ft_compare.py --baseline .../eval/baseline/ --ft .../eval/ft/ --output .../eval_comparison.md`

**Comparison metrics** (`eval_comparison.md`):
- Mean cosine-similarity delta (FT − baseline), overall and per task
- JSON-parseability delta
- Win/loss/tie counts at per-record resolution (threshold: cosine delta > 0.05)
- Tasks that regressed vs tasks that improved (decision input for v2)
- Cost per percentage-point improvement (effectiveness dollars)
- Qualitative appendix: 5 worst-regressing records and 5 best-improving records

**Exit criteria:** `eval_comparison.md` exists and contains all four metric categories; summary line states whether FT improved mean cosine by ≥ +0.05 (subjective call for the user, not a hard gate — negative result is valid scientific signal).

---

### Task 12: Documentation Phase (MANDATORY — DO NOT SKIP)

**Effort:** S (30-45 min)

Update:

1. **`CHANGELOG.md`** — `[Unreleased]` section:
   - Add: `ft: openai fine-tuning pipeline validated end-to-end (sprint FT-OAI-001)`
   - Note the adapter module addition and test count.

2. **`AGENT_HANDOFF.md`** — "Open Work Items" section:
   - Mark "OpenAI format validation" item complete (if listed) or add closing note.
   - Add summary row to training data section: rows processed, job ID, fine-tuned model ID.

3. **`docs/features/fine-tuning-pipeline.md`** — Create if missing, update if exists:
   - Section: "OpenAI Fine-Tuning Path"
   - Document the `openai_ft_adapter` workflow with command-line examples.
   - Link to `neural/training/README.md`.

4. **`neural/training/README.md`** — Add section "OpenAI Fine-Tuning (Supervised)":
   - The 5-command pipeline (export → filter → convert → adapt → upload).
   - Link to `openai_ft_adapter.py` docstring.
   - Link to `scripts/openai_ft_check.py`.

5. **`training_data/README.md`** — Already created in Task 1. Add one section documenting the date-stamped output layout.

6. **Submodule pointer:** If any docs submodule changed, bump the pointer.

**Exit criteria:** All 5 files edited, `git status` reviewed, documentation changes included in the sprint PR.

---

## 6. Testing Plan

Three mandatory tiers per `skill:sprint-planning`.

### Tier 1 — Unit / Lint / Static

- **`neural/training/tests/test_openai_ft_adapter.py`** — ≥6 tests covering think stripping, oversized rejection (using `tiktoken.encoding_for_model`), temporal split, per-task threshold, manifest validity, empty input handling.
- **Lint:** `ruff check neural/training/openai_ft_adapter.py`, `mypy neural/training/openai_ft_adapter.py`.
- **Go sanity:** `go build ./...` (no Go changed, but catches accidental breakage).
- **Type check on check script:** `python3 -m py_compile scripts/openai_ft_check.py`.

### Tier 2 — Integration / Dry-Run

- **Small-subset E2E dry run:** manual pipeline run on 1–2 tasks × 20 records (not the full export). Confirms the full chain (export → filter → convert → adapt → local check) is internally consistent before spending on the full run.
- **`scripts/openai_ft_check.py` returns zero errors** on both combined_train.jsonl and combined_val.jsonl before `client.files.create()` is called. **Pre-upload gate.**
- **Manifest self-consistency:** `openai_ft_adapter` manifest row counts equal sum of per-task row counts; token estimates non-zero.

### Tier 3 — End-to-End / Live (OpenAI)

- **Upload success:** both `train_file.id` and `val_file.id` reach `status: processed`.
- **Job completion:** `client.fine_tuning.jobs.retrieve(job.id)` reaches a terminal state (`succeeded` or `failed`), with either a `fine_tuned_model` ID or an actionable error captured in `run_notes.md`.
- **Smoke test (Task 11):** ≥3 prompts through the fine-tuned model compared against base `gpt-4o-mini`; output format, tokens, and subjective quality recorded in `smoke_test.md`.
- **Cost reconciliation:** actual spend from OpenAI dashboard matches `manifest.json` `openai_cost_estimate` within ±20%. Divergence recorded in `run_notes.md`.

---

## 7. Hyperparameters: Decision and Rationale

> **Pricing note:** Any $-per-token figures below are illustrative based on late-2024 OpenAI documentation. **Confirm current fine-tune pricing against `https://openai.com/api/pricing` on the day of the run** — tiers and base prices have rotated before and the `manifest.json` cost estimate must reflect current numbers, not these examples.

### Recommendation: **Let OpenAI Auto-Select**

On the first run, leave `n_epochs`, `batch_size`, and `learning_rate_multiplier` unspecified. OpenAI's documented best practice is to allow the platform to default these based on dataset size and model, then tune from there if results disappoint.

**Direct quote from OpenAI's fine-tuning best practices:**
> "We recommend initially training without specifying any of these, allowing us to pick a default for you based on dataset size, then adjusting if you observe..."

### Reference Ranges (For Follow-Up Tuning)

Based on public OpenAI documentation and community benchmarks:

| Hyperparameter | Default Behavior | Safe Range | When to Adjust |
|----------------|-------------------|------------|----------------|
| **`n_epochs`** | Auto (typically 3-4 for datasets under 1K examples) | 2–10 | Increase by 1-2 if model doesn't follow training data enough. Decrease by 1-2 if model loses diversity. |
| **`batch_size`** | Auto: ~0.2% of examples, capped at 256 | 1–32 (mini UI cap) | Larger batches slow training but stabilize results. For datasets <500 examples, `auto` will pick small batch. |
| **`learning_rate_multiplier`** | Auto: 0.05 / 0.1 / 0.2 depending on batch size | 0.02–0.2 | Increase if model doesn't appear to converge. Larger LR pairs with larger batch. |

### MDEMG-Specific Defaults (If Autopicking Fails or for v2)

Given the dataset characteristics we expect (a few hundred to a few thousand examples, mixed tasks, long system prompts):

- **Start:** `n_epochs=3, batch_size=auto, learning_rate_multiplier=auto`
- **If underfit (model doesn't capture task format):** `n_epochs=5`
- **If overfit (validation loss diverges from training loss):** `n_epochs=2`
- **If convergence is slow:** `learning_rate_multiplier=0.15`
- **If output becomes too repetitive:** `n_epochs=2, learning_rate_multiplier=0.05`

### Why We Wait to Tune

1. The first run's goal is **format validation**, not optimal model quality. Any successful completion satisfies the sprint exit criteria.
2. Hyperparameter tuning without observing baseline metrics is speculation. OpenAI's auto-selection gives us the baseline.
3. The fine-tuning dashboard shows training and validation loss curves. Without those, any manual setting is blind.
4. Each tuning run costs money. Start with the default, look at loss curves, adjust one variable per subsequent run.

---

## 8. Validation Data Strategy

### Why Include a Validation File

OpenAI's fine-tuning job accepts a `validation_file` that is **not used for training**. Its purpose is observability:

- Produces a **validation loss curve** in the dashboard alongside the training loss curve.
- If training loss drops but validation loss plateaus or rises, the model is overfitting — indicator to reduce epochs.
- If both curves track closely and both drop, training is healthy.
- Without a validation file, you have **no observability signal** to decide whether to run more epochs, fewer, or change the data.

### Split Ratio: **80/20 Temporal**

**Ratio decision:**
- **80% training / 20% validation** is the right split for a dataset of a few hundred to a few thousand records. It gives enough training data to learn the tasks while reserving a statistically meaningful validation sample.
- For datasets of 10K+ records, a 90/10 split is acceptable.
- For datasets under 200 records, a 70/30 split preserves validation reliability at the cost of training signal.

**Split method: temporal, not random.**
- Training set = records from the earliest 80% of the time window.
- Validation set = records from the most recent 20%.
- This is the convention `dataset_versioner.py` already enforces and it's the correct choice for three reasons:
  1. **No temporal leakage.** Random splits let future records inform past ones, inflating validation scores.
  2. **Matches production.** At inference time, the model will see prompts the training data could not have contained.
  3. **Detects concept drift.** If validation loss is much worse than training loss, the task's distribution has changed — useful signal.

**Per-task stratification:**
The split happens per-task independently, then files are combined. This prevents the validation set from excluding an entire task just because that task's records happened to be early.

### Quality Requirements for the Validation File

- **Same quality gates as training.** No privacy violations, no errors, no think blocks, valid format.
- **Minimum 20 records.** OpenAI shows validation loss per step; fewer than 20 records produces noisy, unusable curves.
- **Minimum 2 records per task** that appears in training. If a task has only 5 total records, it goes in the combined file but is omitted from the per-task validation file (no meaningful per-task metric possible).

---

## 9. Commits

Commit plan (conventional commits, one logical change per commit):

1. `chore: add training_data/ directory and gitignore exclusion`
2. `feat(neural): add openai_ft_adapter for OpenAI fine-tuning JSONL output`
3. `test(neural): test_openai_ft_adapter covering think stripping, temporal split, rejection`
4. `feat(scripts): add openai_ft_check pre-upload format validator`
5. `docs: document openai fine-tuning workflow in feature docs + neural README`
6. `docs(changelog): record ft:openai sprint completion`

All commits on `reh3376_dev01`, pushed with auto-PR to main.

---

## 10. Verification

Sprint verification checklist (all must be green before PR merge):

- [ ] `go build ./...` passes (no Go code modified, but sanity check).
- [ ] `go test ./internal/...` passes.
- [ ] `python3 -m pytest neural/training/tests/ -v` passes, including new test file.
- [ ] `python3 -m pytest neural/training/tests/test_openai_ft_adapter.py -v` passes ≥6 tests.
- [ ] `scripts/openai_ft_check.py` returns zero errors on final JSONL output.
- [ ] `training_data/` is properly git-ignored (`git check-ignore training_data/openai_ft/`).
- [ ] No privacy violations reported by `quality_filter.py`.
- [ ] OpenAI fine-tuning job reaches `succeeded` status OR documented failure in run notes.
- [ ] Fine-tuned model ID captured in `training_data/openai_ft/<date>/run_notes.md`.
- [ ] Smoke test output documented in `training_data/openai_ft/<date>/smoke_test.md`.
- [ ] UATS specs and UPTS specs unchanged (no API contract impact).
- [ ] Documentation phase complete (all 5 targets updated).

---

## 11. Documentation Updates (Explicit — MUST NOT BE CUT)

Reiterating what's covered in Task 12 — this section exists because past sprints have treated docs as optional. They aren't.

| File | Required Update |
|------|-----------------|
| `CHANGELOG.md` | `[Unreleased]` section: new entry under `ft:` or `added:` |
| `AGENT_HANDOFF.md` | Open work items section: mark this sprint complete with job ID + model ID |
| `docs/features/fine-tuning-pipeline.md` | Create or update: OpenAI path section |
| `neural/training/README.md` | Append: OpenAI fine-tuning section |
| `training_data/README.md` | Append: date-stamped output layout documentation |

**Any submodule touched by doc updates gets its pointer bumped.**

---

## 12. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| TSDB has <100 clean records | Medium | Blocks sprint | Task 2 checks this before anything else. If fails, abort and return to data collection. |
| System prompts exceed 65K context | Low | Records logged + rejected | MDEMG system prompts average 2–4K tokens, far below the 65,536-tok gpt-4o-mini fine-tune cap. Adapter rejects + logs; manifest surfaces the count. Inverse risk: we under-use available context and could bundle more retrieval signal per record — revisit in RAFT sprint. |
| Privacy violations in exported data | Low | Hard stop | `quality_filter.py` blocks export. If triggered, investigate scrubber.go and add new pattern. |
| OpenAI rejects JSONL despite local check passing | Low | Wasted upload | `openai_ft_check.py` mirrors OpenAI cookbook; any divergence would be documented as a finding. |
| Fine-tuning job fails during training | Low | Sprint documented as negative result | Capture error, document in run_notes.md, still a valid sprint outcome (we learned the format fails). |
| Cost overrun on the first run | Low | Financial | Manifest produces cost estimate; review before Task 10. Auto hyperparameters cap at reasonable epoch counts. |
| Fine-tuned model output is worse than base | Medium | Expected on first run | Not a sprint failure. The goal is pipeline validation, not model quality. Captured in smoke test. |
| Adapter bugs cause bad data to reach OpenAI | Low | Money spent on garbage | 6+ unit tests + pre-upload format check + OpenAI's own validation = 3 gates. |

---

## 13. Documents Accessed

Sources consulted during plan drafting and review (per CLAUDE.md "ALWAYS include Documents Accessed list"):

- `docs/tests/uaits/specs/mdemg.uaits.json` — canonical 4-paradigm UAITS spec (SFT / DPO / RAFT / curriculum source-table bindings).
- `docs/tests/uaits/schema/uaits.schema.json` — JSON-schema validator for the spec.
- `neural/training/quality_filter.py:300–320` — CLI flags `--ults-dir`, `--dedup-key`, `--input`, `--output`, `--report`.
- `neural/training/format_converter.py:290–310` — CLI flag `--raft-ratio` (default 0.8); `--format chat|dpo`.
- `neural/training/paradigm_router.py:64–73, 119` — paradigm dispatch (`sft`, `dpo`, `raft`, `curriculum`); SFT path calls `quality_filter.run_filter` → `format_converter.run_converter` → `dataset_versioner.run_versioner`.
- `neural/training/dataset_versioner.py:126–157, 184–210, 285–318` — temporal split logic, MLX JSONL `write_split`, manifest generation.
- `internal/cli/data_curate.go:35` — `mdemg data curate` shells out to paradigm_router.
- `internal/tsdb/exporter.go` — UTDS archive export (`mdemg data export`).
- `scripts/cleanup_training_data.sql` — TSDB reset utility between runs.
- `/Users/reh3376/mdemg/CLAUDE.md` — project conventions, `skill:sprint-planning`, mandatory 3-tier testing rule, LLM retry defaults, DH-004/DH-005 context.
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` — `feedback_sprint_plan_format`, `feedback_mandatory_testing_tiers`, `feedback_sequential_epics`, `feedback_no_hardcoded_values`.
- OpenAI fine-tuning docs (late-2024 public pages): context window 65,536 for gpt-4o-mini fine-tune; `o200k_base` tokenizer for gpt-4o/4.1 families; auto-hyperparameters guidance; DPO beta support.

---

## 14. Success Criteria (Binary)

Sprint is complete when **all** of the following are true:

1. ✅ `training_data/openai_ft/<date>/combined_train.jsonl` exists with ≥80 records.
2. ✅ `training_data/openai_ft/<date>/combined_val.jsonl` exists with ≥20 records.
3. ✅ `manifest.json` shows zero privacy violations, zero format errors.
4. ✅ `scripts/openai_ft_check.py` returns zero errors on both files.
5. ✅ OpenAI fine-tuning job reaches terminal state (`succeeded` or `failed`), documented either way.
6. ✅ `neural/training/openai_ft_adapter.py` exists and has ≥6 passing tests.
7. ✅ All 5 documentation targets updated (CHANGELOG, AGENT_HANDOFF, feature doc, neural README, training_data README).
8. ✅ Sprint PR merged to main via auto-PR from `reh3376_dev01`.

---

## Appendix A: Quick Reference — Command Sequence

```bash
# From /Users/reh3376/mdemg/ — single-terminal run-through

# 1. Directory setup (once)
mkdir -p training_data/openai_ft
echo "training_data/" >> .gitignore

# 2. Verify data exists
psql "$TSDB_DSN" -c "SELECT task_name, COUNT(*) FROM llm_interactions WHERE space_id='mdemg-dev' GROUP BY task_name ORDER BY 2 DESC;"

# 3. Export
./bin/mdemg data export --space-id mdemg-dev \
  --output training_data/raw/export-$(date +%Y%m%d).tar.gz \
  --tables llm_interactions --since $(date -u -v-90d +%Y-%m-%dT%H:%M:%SZ)
mkdir -p training_data/raw/extracted
tar -xzf training_data/raw/export-*.tar.gz -C training_data/raw/extracted --strip-components=1

# 4. Filter
cd neural && PYTHONPATH=. python3 -m training.quality_filter \
  --input ../training_data/raw/extracted/llm_interactions.jsonl \
  --output ../training_data/filtered/llm_interactions.jsonl \
  --ults-dir ../docs/tests/ults/specs/ \
  --report ../training_data/filtered/filter_report.json \
  --dedup-key prompt

# 5. Convert to chat format (SFT-only test run → raft-ratio 0.0)
PYTHONPATH=. python3 -m training.format_converter \
  --input ../training_data/filtered/llm_interactions.jsonl \
  --output ../training_data/converted/mlx_chat.jsonl \
  --raft-ratio 0.0

# 6. OpenAI adapter (NEW)
PYTHONPATH=. python3 -m training.openai_ft_adapter \
  --chat-input ../training_data/converted/mlx_chat.jsonl \
  --metadata-input ../training_data/filtered/llm_interactions.jsonl \
  --output-dir ../training_data/openai_ft/$(date +%Y%m%d)/ \
  --model gpt-4o-mini-2024-07-18 \
  --train-ratio 0.8 --min-per-task 10 --strip-think

# 7. Pre-upload check
cd ..
python3 scripts/openai_ft_check.py \
  training_data/openai_ft/$(date +%Y%m%d)/combined_train.jsonl \
  training_data/openai_ft/$(date +%Y%m%d)/combined_val.jsonl

# 8-10. Upload and launch (Python snippet — see Tasks 9, 10)
python3 scripts/openai_ft_upload_and_launch.py \
  --train training_data/openai_ft/$(date +%Y%m%d)/combined_train.jsonl \
  --val training_data/openai_ft/$(date +%Y%m%d)/combined_val.jsonl \
  --model gpt-4o-mini-2024-07-18 \
  --suffix mdemg-sft-v1

# 11-12. Smoke test + docs — manual steps
```

---

## Appendix B: File Inventory (What This Sprint Produces)

### New Files (Repo)

| Path | Purpose |
|------|---------|
| `neural/training/openai_ft_adapter.py` | OpenAI-specific adapter (think stripping, temporal split, manifest). |
| `neural/training/tests/test_openai_ft_adapter.py` | Unit tests for the adapter. |
| `scripts/openai_ft_check.py` | Pre-upload format validator. |
| `scripts/openai_ft_upload_and_launch.py` | Upload + job launch helper. |
| `training_data/.gitkeep` | Keeps the directory in git even though contents are ignored. |
| `training_data/README.md` | Documents the directory's purpose. |
| `docs/features/fine-tuning-pipeline.md` | Feature documentation (new or updated). |

### New Files (Local, Git-Ignored)

| Path | Purpose |
|------|---------|
| `training_data/raw/export-<date>.tar.gz` | TSDB export archive. |
| `training_data/raw/extracted/llm_interactions.jsonl` | Extracted raw data. |
| `training_data/filtered/llm_interactions.jsonl` | After `quality_filter.py`. |
| `training_data/filtered/filter_report.json` | Filter report. |
| `training_data/converted/mlx_chat.jsonl` | After `format_converter.py`. |
| `training_data/openai_ft/<date>/combined_train.jsonl` | Upload to OpenAI as `training_file`. |
| `training_data/openai_ft/<date>/combined_val.jsonl` | Upload to OpenAI as `validation_file`. |
| `training_data/openai_ft/<date>/by_task/*.jsonl` | Per-task files for optional specialist runs. |
| `training_data/openai_ft/<date>/manifest.json` | Complete run metadata. |
| `training_data/openai_ft/<date>/rejection_log.jsonl` | Records that failed OpenAI gates. |
| `training_data/openai_ft/<date>/run_notes.md` | Job ID, model ID, observations. |
| `training_data/openai_ft/<date>/smoke_test.md` | Qualitative comparison. |

---

**End of sprint plan.**
