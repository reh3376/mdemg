# MDEMG-USAGE-LORA-001-GGUF — Sprint Plan (v1.0)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | MDEMG-USAGE-LORA-001-GGUF |
| Task | #150 |
| Filed | 2026-09-01 (from #145 revised verdict) |
| Author | Roger Henley + Claude (proactive mode) |
| Branch | `reh3376_dev01` |
| Predecessors | #145 MDEMG-USAGE-LORA-001 (revised verdict: parity within noise on mlx_lm.server); #139 ADAPTER-SWAP-STANDARDIZE-001 (bench-serve helper — but bench-serve targets mlx_lm.server, not llama-server, so not directly reused here); Phase 13.5 cutover (established the fuse → convert_hf_to_gguf → llama-quantize pipeline for shipped v1) |
| Format version | 12-section v1.0 |
| Est. wall-clock | ~2-3h |
| Est. spend | $0 (all local) |

## 2. Problem Statement

#145's revised verdict established my adapter is at same-runtime parity with v1 (aggregate 0.8388 vs 0.8449 on mlx_lm.server = −0.006, within noise). v1's shipped-runtime score (llama.cpp GGUF Q5_K_M) is 0.9188 — a +0.074 runtime bonus over its mlx_lm.server score. **If my adapter picks up the same runtime bonus when converted to fused GGUF and served via llama.cpp, its production score is estimated 0.9128 — competitive with v1's shipped baseline.**

This sprint measures that ESTIMATE with real data. Two possibilities:
1. My adapter picks up ~+0.074 runtime bonus → prod aggregate ≥ 0.90 → PROMOTABLE
2. My adapter picks up less bonus (or none) → prod aggregate < 0.90 → not promotable, stay on v1

The single-task regression (retrieval.query_classify −10% same-runtime) is the other axis: even if aggregate ≥ 0.90, a large one-task loss might block promotion depending on operator preference.

**Not solved by extrapolation** — the +0.074 runtime bonus is a v1 measurement; adapter weights differ from v1 weights, and mlx→GGUF conversion + Q5_K_M quantization can affect them non-uniformly. Only real measurement decides.

## 3. Scope & Constraints

### In scope

- Fuse `mdemg_usage_lora_001` iter-7200 adapter into base model via `mlx_lm.fuse --dequantize` → bf16 HF safetensors (~28 GB)
- Convert bf16 HF safetensors → f16 GGUF via `convert_hf_to_gguf.py --outtype f16` (~29.5 GB)
- Quantize f16 → Q5_K_M GGUF via `llama-quantize` (~11 GB)
- Bench-serve the Q5_K_M GGUF via `llama-server` on port **8103** (DO NOT touch production 8102)
- Run standard `neural.benchmarks.run_benchmark` against `valid_clean.jsonl` + `benchmark_phase10.yaml` config
- Compare aggregate to v1 shipped baseline 0.9188 AND to v1-same-runtime 0.8449
- Per-row inspection of `retrieval.query_classify` −10% regression (before final promote decision, per revised verdict Option 2)
- SHA-verify every conversion step (fusable-back-to-source guarantee)
- Sprint plan + verdict + post + PR comment

### Out of scope (explicit)

- **NO promotion to production** — that's `PHASE-E4-GATE-PROMOTE-001` (separate sprint, HITL-gated)
- **NO touch to production llama-server on 8102** — bench-serve uses 8103
- **NO change to shipped `mdemg-llm-v1-gguf/` files** — new GGUF lives in dedicated `mdemg_usage_lora_001-gguf/` dir
- **NO change to `.local-models/serving/current.gguf` symlink** — production points at v1
- **NO change to training** — the iter-7200 adapter is frozen
- **NO adapter-only GGUF (LoRA GGUF adapter)** — this sprint uses the FUSED path (adapter merged into base weights, single GGUF file for llama.cpp); adapter-only GGUF is a MODEL-DIST-002 concern
- **NO benchmark against models other than v1 shipped** — same 13-task valid_clean.jsonl

### Constraints (must obey)

- `plan-mode-before-change` — this plan BEFORE any conversion
- `sequential-epics` — Epic N complete before N+1
- `never-hardcode-config` — every knob configurable via env or CLI
- `must-use-cuid2` — any new IDs
- `mandatory-feature-docs` — extend existing `docs/features/local-model-distribution.md` if the sprint produces a reusable pattern; no new feature doc needed (single-shot conversion)
- `end-with-docs-accessed` — all docs
- `must-follow-12-section-format` — this file
- `must-comment-sprint-summary-on-pr` — PR summary comment
- `lint-before-commit` — nothing to lint (no Go code changes)
- `unit-integration-e2e-docs` — 3 testing tiers (light unit + integration + Tier-3 live)
- `live-testing-tier-required` — the entire sprint IS a live Tier-3 test
- `iterate-break-fix-verify` — verdict via real benchmark, not extrapolation
- **PHASE-E3 arch rule 5** (pinned in #145): benchmark comparisons MUST be same-runtime; this sprint IS the same-runtime measurement for the shipped 0.9188 baseline
- `no-stash-for-release` — n/a (no release step)
- `no-direct-main-commits` — commit on `reh3376_dev01` per convention
- **Protection**: any command that could touch `.local-models/serving/current.gguf` or `com.mdemg.llama-server` launchd is a hard STOP requiring explicit operator confirmation

## 4. Dependencies

| Dependency | Status | Notes |
|---|---|---|
| `adapters/mdemg_usage_lora_001/adapters.safetensors` (iter 7200 frozen) | ✅ shipped, SHA `de2675b58800…` | Source |
| `.local-models/qwen3-14b-4bit-base/` (raw base) | ✅ shipped, 7.8 GB | Fuse target base |
| `mlx_lm.fuse` (in `neural/.venv/bin/python`) | ✅ installed | Verified with --help; has --dequantize + --export-gguf |
| `/Users/reh3376/llama.cpp-src/convert_hf_to_gguf.py` | ✅ shipped | Phase 13.5 pipeline tool |
| `/opt/homebrew/bin/llama-quantize` | ✅ installed | Homebrew llama.cpp |
| `/opt/homebrew/bin/llama-server` | ✅ installed | Homebrew llama.cpp (same as prod 8102) |
| `neural/.venv/bin/python -m neural.benchmarks.run_benchmark` | ✅ shipped | Same runner as #145 |
| `configs/benchmark_phase10.yaml` | ✅ shipped | Same eval config |
| `training_data/eval/valid_clean.jsonl` | ✅ shipped | Same 13-task holdout |
| Free disk (need ~50 GB peak — bf16 + f16 + Q5_K_M + working intermediates) | ✅ 704 GiB free | |
| Production llama-server on 8102 healthy | ✅ verified pre-sprint | MUST stay healthy through sprint |
| Port 8103 free | ✅ | |

**No blocking dependencies.**

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Fuse adapter into base weights (~5-10 min)

```bash
neural/.venv/bin/python -m mlx_lm.fuse \
    --model .local-models/qwen3-14b-4bit-base \
    --adapter-path adapters/mdemg_usage_lora_001 \
    --save-path .local-models/qwen3-14b-mdemg-usage-fused \
    --dequantize
```

⚠️ `--dequantize` produces bf16 HF safetensors (per Phase 13.5 arch note: MLX 4bit-fused adapter can't `convert_hf_to_gguf` directly; must dequantize first). Output ~28 GB.

**Gate**: `save-path` exists, `model.safetensors.index.json` present, total size ~28 GB. `sha256sum` all safetensors files → record in `docs/development/mdemg-usage-lora-001-gguf/sha256.txt`.

### Epic 2 — Convert bf16 HF safetensors → f16 GGUF (~5 min)

```bash
python3 /Users/reh3376/llama.cpp-src/convert_hf_to_gguf.py \
    .local-models/qwen3-14b-mdemg-usage-fused \
    --outtype f16 \
    --outfile .local-models/qwen3-14b-mdemg-usage-fused.f16.gguf
```

**Gate**: output f16.gguf exists, ~29.5 GB. SHA record.

### Epic 3 — Quantize f16 → Q5_K_M GGUF (~2 min)

```bash
llama-quantize \
    .local-models/qwen3-14b-mdemg-usage-fused.f16.gguf \
    .local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf \
    Q5_K_M
```

**Gate**: Q5_K_M.gguf exists, ~11 GB. SHA record.

### Epic 4 — Sanity test served output (~2 min)

Per HOMEBREW-INSTALLER-QWEN-UPDATE-001 arch rule 2 ("sanity-test every published artifact locally BEFORE push, especially when pipeline had known-broken variants" — this sprint isn't publishing but the same principle applies to bench-serve):

```bash
llama-server \
    --model .local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf \
    --port 8103 --host 127.0.0.1 \
    --ctx-size 32768 --parallel 4 --cont-batching --metrics --jinja &
# Wait for readiness
until curl -s http://127.0.0.1:8103/v1/models | grep -q data; do sleep 3; done
# Sanity chat
curl -s http://127.0.0.1:8103/v1/chat/completions -H 'Content-Type: application/json' \
  -d '{"model":"a","messages":[{"role":"user","content":"reply ok"}],"max_tokens":10}'
```

**Gate**: server returns a coherent short response; NOT `tensor '<name>' not found` (HOMEBREW-INSTALLER-QWEN-UPDATE-001 catch) or gibberish.

### Epic 5 — Benchmark on llama.cpp (~1-1.5h)

```bash
neural/.venv/bin/python -m neural.benchmarks.run_benchmark \
    --config configs/benchmark_phase10.yaml \
    --out training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json \
    --mlx-base-url http://127.0.0.1:8103/v1 \
    --mlx-model-name .local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf \
    --mlx-timeout-s 180
```

Note: `--mlx-*` flags are the runner's OpenAI-compat endpoint override; llama-server IS OpenAI-compat, so the flag applies.

Expected wall-clock: v1 GGUF on llama-server should be ~1-1.5h at 10-15 req/min (llama.cpp is faster than mlx_lm.server). Uses `run_in_background: true` in Bash tool (10-min ceiling avoidance).

**Gate**: benchmark writes JSON with `aggregate_weighted_score` field. Zero fatal errors.

### Epic 6 — Verdict comparison + per-row query_classify investigation (~30 min)

**Aggregate comparison**:

| Setup | Aggregate | Runtime |
|---|---:|---|
| v1 fused GGUF (shipped baseline) | 0.9188 | llama.cpp |
| v1 fused (same runtime as #145) | 0.8449 | mlx_lm.server |
| mdemg_usage_lora_001 (same runtime as #145) | 0.8388 | mlx_lm.server |
| **mdemg_usage_lora_001 GGUF** | **TBD** | **llama.cpp** ← this sprint |

**Verdict rubric**:
- ✅ PROMOTE if aggregate ≥ 0.9088 (baseline 0.9188 − 0.01)
- ⚠️ MIXED if aggregate in [0.89, 0.9088) — operator decision
- ❌ NO-PROMOTE if aggregate < 0.89

**Per-row query_classify inspection** (bounded — only if aggregate ≥ 0.89):
- Load `training_data/eval/valid_clean.jsonl` retrieval.query_classify rows
- Load `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` per-row detail for that task
- Load same for `v1_fused_mlxserver_baseline.json`
- Find rows where mine < v1 by ≥ 0.3
- Classify: (a) legitimate capability loss (systematic pattern of mis-classification) or (b) borderline variance (individual off-by-one)
- Report as inputs to promote decision

### Epic 7 — Sprint post + verdict + PR + follow-ups (~30 min)

- `docs/development/mdemg-usage-lora-001-gguf/verdict.md`
- `docs/development/mdemg-usage-lora-001-gguf/sprint_post.md`
- CHANGELOG.md Unreleased entry
- PR summary comment
- Task #150 completed
- If ✅ PROMOTE verdict, activate PHASE-E4-GATE-PROMOTE-001 (separate sprint filing)

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests

N/A — pipeline is Python subprocess orchestration, no new Go/Python code beyond CLI invocations.

### Tier 2 — Integration test

Epic 4 IS the integration test: server serves the Q5_K_M and responds coherently to a chat prompt before benchmark starts.

### Tier 3 — Live e2e on real system

The entire sprint IS Tier 3. Full-scope benchmark against real valid_clean.jsonl on real llama-server on real Q5_K_M GGUF of the actual adapter.

## 7. Commit Strategy

Single terminal commit per epic-cluster:
1. `bench(training): MDEMG-USAGE-LORA-001-GGUF Epic 1-4 — fuse + convert + quantize + sanity ✓`
2. `bench(training): MDEMG-USAGE-LORA-001-GGUF Epic 5-6 — production-runtime benchmark verdict`
3. `docs(training): MDEMG-USAGE-LORA-001-GGUF Epic 7 — verdict + sprint post + PR summary`

Or fold into one commit if all wall-clock lands in-session. Auto-PR fires on push.

## 8. Verification Checklist

- [ ] Epic 1: fused HF safetensors exist at `.local-models/qwen3-14b-mdemg-usage-fused/`, ~28 GB, SHA recorded
- [ ] Epic 2: f16 GGUF exists at `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf`, ~29.5 GB
- [ ] Epic 3: Q5_K_M GGUF exists at `.local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf`, ~11 GB
- [ ] Epic 4: llama-server on 8103 responds to sanity chat with coherent reply
- [ ] Epic 4: production 8102 UNTOUCHED (`curl :8102/v1/models` unchanged pre + post)
- [ ] Epic 5: benchmark JSON written with `aggregate_weighted_score`
- [ ] Epic 5: production 8102 STILL untouched (verified again post-bench)
- [ ] Epic 6: verdict determined; per-row query_classify inspection done if applicable
- [ ] Epic 7: sprint post + verdict + CHANGELOG + PR comment shipped
- [ ] Task #150 status updated
- [ ] `~/Library/LaunchAgents/com.mdemg.llama-server.plist` byte-identical pre + post sprint
- [ ] `.local-models/serving/current.gguf` symlink byte-identical pre + post sprint
- [ ] `.local-models/mdemg-llm-v1-gguf/` byte-identical pre + post sprint

## 9. Documentation Update (final epic)

- `docs/development/mdemg-usage-lora-001-gguf/{sprint_plan,verdict,sprint_post}.md` (new)
- CHANGELOG.md Unreleased entry
- PR summary comment
- If ✅ PROMOTE: `docs/development/mdemg-usage-lora-001/verdict.md` updated with pointer to this sprint's outcome
- If GGUF produced is retained for future retrain-loop use, add a note to `docs/features/local-model-distribution.md`

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Q5_K_M conversion produces broken GGUF (HOMEBREW-INSTALLER-QWEN-UPDATE-001 saw this on Qwen3.5 arch) | Epic 4 sanity test catches this before Epic 5 runs; abort with fresh f16 → Q4 fallback if Q5_K_M is broken |
| llama-server on 8103 starves production 8102 for GPU RAM | Both share unified memory on M5 Max; benchmark run is bounded (~1.5h); operator can suspend if noticed |
| Wall-clock overrun beyond 3h | Epic 5 is the long pole; use `run_in_background: true` + Monitor arm, not foreground Bash |
| Bench-serve orphan process on wrapper kill | Standard `pkill -f "llama-server.*8103"` cleanup; production launchd `com.mdemg.llama-server` untouched |
| Aggregate on llama.cpp turns out < 0.85 (worse than same-runtime baseline) | Real capability regression; verdict NO-PROMOTE; document the surprise + investigate whether it's a GGUF conversion artifact vs true adapter deficit |
| Aggregate in [0.86, 0.89) — borderline | Operator decision; per-row inspection to understand which tasks moved |
| Fuse step consumes ~28 GB — accidentally overwrites something | Save-path is a new directory that must not exist; script fails safe if it does |
| Runtime bonus doesn't transfer proportionally | Actual verdict from Epic 5; no extrapolation shipped |
| PR promote assumed but never asked | Sprint plan §3 out-of-scope: no promotion in THIS sprint; separate sprint for E4-gate |
| Adapter weights + LoRA rank aren't properly captured in mlx_lm.fuse dequant step (silent quality loss) | Epic 4 sanity chat + Epic 5 benchmark both surface this; the benchmark aggregate would collapse if fuse broke the adapter |

## 11. Documents Accessed

- `docs/development/mdemg-usage-lora-001/verdict.md` (revised — source of the estimate this sprint measures)
- `docs/development/mdemg-usage-lora-001/sprint_post.md` (reversal banner)
- `docs/development/mdemg-usage-lora-001/sprint_plan.md` (predecessor 12-section plan)
- `docs/development/adapter-swap-standardize-001/sprint_post.md` (#139 bench-serve tool — for pattern; not directly used here since llama-server != mlx_lm.server)
- `docs/development/homebrew-installer-qwen-update-001/PUBLISH_GUIDE.md` (the fuse → convert → quantize pipeline reference)
- `configs/benchmark_phase10.yaml` (same eval config as #145)
- `configs/sft_mdemg_usage_lora_001.yaml` (training config — captures the adapter's provenance)
- `training_data/eval/valid_clean.jsonl` (same 13-task benchmark)
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (#145 my adapter on mlx_lm.server)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (#145 fair-comparison baseline on mlx_lm.server)
- `.local-models/mdemg-llm-v1-gguf/` (existing shipped Q5_K_M reference)
- `.local-models/qwen3-14b-4bit-base/` (raw base for fuse)
- `adapters/mdemg_usage_lora_001/` (adapter under test, iter 7200 frozen SHA de2675b58800…)
- CLAUDE.md pins:
  - Phase 13.5 cutover (the fuse → convert → quantize pipeline shipped v1's Q5_K_M via this exact path)
  - HOMEBREW-INSTALLER-QWEN-UPDATE-001 arch rules (sanity-test every quant before publish; here, before bench)
  - MODEL-DIST-001/002 (distribution semantics; adapter-only GGUF is a different concern)
  - #145 revised verdict + PHASE-E3 arch rule 5 (same-runtime comparison requirement)
  - `iterate-break-fix-verify`, `live-testing-tier-required`, `must-master-data-pipelines`
- Operator directive 2026-09-01 ("proceed with #150")

## 12. Rollback Procedures (destructive ops)

**Destructive surface**: this sprint writes ~40+ GB of new files under `.local-models/`. No deletions, no overwrites of shipped artifacts. Production 8102 llama-server + `.local-models/serving/current.gguf` symlink + `.local-models/mdemg-llm-v1-gguf/` all UNTOUCHED.

**Full rollback**:

```bash
# Remove generated artifacts (frees ~40 GB)
rm -rf .local-models/qwen3-14b-mdemg-usage-fused/           # bf16 HF (~28 GB)
rm -f  .local-models/qwen3-14b-mdemg-usage-fused.f16.gguf   # ~29.5 GB
rm -f  .local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf # ~11 GB

# Kill any orphan bench llama-server (if left running)
pkill -f "llama-server.*8103"

# Production llama-server on 8102 is UNTOUCHED — no restore needed
curl -s http://127.0.0.1:8102/v1/models | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d['data'][0]['id'])"
```

**Zero substrate mutation** (no Neo4j, no TSDB writes unless `--apply-tsdb` is passed to run_benchmark; not passed by default). Adapter safetensors preserved. All training artifacts preserved.
