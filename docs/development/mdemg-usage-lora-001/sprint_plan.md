# MDEMG-USAGE-LORA-001 — Sprint Plan (v1.0)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | MDEMG-USAGE-LORA-001 |
| Task | #145 |
| Filed | 2026-08-24 |
| Author | Roger Henley + Claude (proactive mode) |
| Branch | `reh3376_dev01` |
| Predecessors | MDEMG-USAGE-CORPUS-CURATE-001 (task #144) — supplied `mdemg_usage_v1` (1,198 rows, 0 leaks); PHASE-E3-RETRAIN-BENCHMARK-001 (task #138) — supplied training config shape, wall-clock arithmetic rules, and the "trained-in-isolation regressed" lesson |
| Format version | 12-section v1.0 |
| Est. wall-clock | ~30h training + ~30 min benchmark + ~2h ceremony = ~32h |
| Est. spend | $0 (local compute; no OpenAI API calls in this sprint) |

## 2. Problem Statement

`mdemg-llm-v1` (production LoRA on Qwen3-14B-4bit) knows Claude Code deeply but knows **NOTHING** about MDEMG itself — the exact gap MDEMG-USAGE-CORPUS-CURATE-001 was scoped to close. That sprint shipped the corpus (`training_data/sft/mdemg_usage_v1/`, 1,198 rows curated deterministically from `mdemg-dev` substrate, 0 leaks vs `valid_clean.jsonl`). This sprint trains a new LoRA that teaches the model MDEMG-usage patterns WITHOUT catastrophically forgetting the 5 existing families the shipped v1 adapter was trained on.

**Direct application of PHASE-E3 lesson**: E3 trained on a STRIPPED corpus (removed 2,203 rows on the "substrate serves those facts" premise), then benchmarked the LoRA in ISOLATION → aggregate 0.7658 vs 0.9188 baseline = FAIL. Correct fix: retrain on the FULL shipped corpus PLUS the new family (additive, not subtractive). This sprint does that.

**Precise capability delta targeted**: model volunteers MDEMG-specific facts (feature names, config keys, endpoints, CLI subcommands, architectural decisions) unprompted, without requiring RAG round-trip on every classifier call.

## 3. Scope & Constraints

### In scope

- Split `mdemg_usage_v1` 80/10/10 → `{train (~958), valid (~120), benchmark-holdout (~120)}`
- Extend PHASE-E3 assembler to add `mdemg_usage_v1` as the 6th family
- Write training config `configs/sft_mdemg_usage_lora_001.yaml` — mirrors E3's reduced-scope config (batch=2, max_seq=4096) but scaled to 6-family × 2 epochs
- Train LoRA — wall-clock ~30h; llama-server auto-quiesces (MDEMG mostly non-functional during training window; expected)
- Benchmark against `valid_clean.jsonl` (13 tasks) + new `mdemg_usage_v1_holdout.jsonl` (`mdemg.usage` task); aggregate + per-task deltas vs 0.9188 v1 baseline
- Manifest with adapter SHA + training corpus SHA + benchmark row + timestamps
- NO PROMOTION in this sprint (E4 gate is a separate sprint)

### Out of scope (explicit)

- **NO promotion / no llama-server swap-in** — that's `PHASE-E4-GATE-PROMOTE-001` (unblocked but downstream)
- **NO v2 base (Qwen3.8-27B)** — v1 base is proven pipeline; v2 arm is a follow-up (`MDEMG-USAGE-LORA-002`) if v1 arm succeeds. Rationale: sequential + reversible; don't take on two novel variables at once
- **NO adapter-swap standardization** — that's task #139 `ADAPTER-SWAP-STANDARDIZE-001`; would eliminate manual dance but a separate sprint
- **NO retrain-from-scratch base training** — this is LoRA-only on the shipped Qwen3-14B-4bit
- **NO extension of `valid_clean.jsonl`** — the new mdemg.usage benchmark holdout is a NEW file consumed alongside valid_clean, not merged into it (leaves the anchor intact)

### Constraints (must obey)

- `plan-mode-before-change` — this plan file BEFORE any config or training code
- `sequential-epics` — Epic N completes before N+1
- `never-hardcode-config` — every knob CLI / env / YAML override
- `must-use-cuid2` — adapter run_id via CUIDv2
- `mandatory-feature-docs` — no new feature doc needed (extends existing `docs/features/local-model-distribution.md` + `docs/features/mdemg-usage-corpus.md`)
- `end-with-docs-accessed` — all docs
- `must-follow-12-section-format` — this file
- `must-comment-sprint-summary-on-pr` — PR comment
- `lint-before-commit` — python lint via ruff before commit
- `unit-integration-e2e-docs` — 3 testing tiers
- `live-testing-tier-required` — Tier 3 with real system (real LoRA training on real base + real benchmark)
- PHASE-E3 arch rules (all 4): (1) wall-clock from Tokens/sec × tokens/iter, not dry-run It/sec; (2) N/A here (no strip); (3) 2-consecutive early stop; (4) peak GPU memory documented

## 4. Dependencies

| Dependency | Status | Notes |
|---|---|---|
| `training_data/sft/mdemg_usage_v1/train.jsonl` (task #144) | ✅ shipped, SHA `d271a825d3…c89e2ff26` | Source of new family |
| PHASE-E3 assembler pattern (`scripts/phase_e3_assemble_corpus.py`) | ✅ shipped, extendable | Byte-verbatim + SHA-verify-pre+post + deterministic-split; extend for 6th family |
| Shipped 5 families (`tier1`, `family_reasoning_think`, `family_classify_notink`, `family_structured_notink`, `claude_code_knowledge_v3_stripped`) | ✅ shipped, SHAs pinned per E3 | Frozen source; SHA pre+post check |
| Qwen3-14B-4bit local base (`.local-models/qwen3-14b-4bit-base`) | ✅ shipped | E3 verified same digest as `mlx-community/Qwen3-14B-4bit` |
| `mlx_lm.lora` (mlx-community/mlx-lm) | ✅ installed | Used by E3 |
| `training_data/eval/valid_clean.jsonl` | ✅ shipped | Baseline anchor (0.9188 aggregate) |
| `neural/benchmarks/run_benchmark.py` | ✅ shipped | E3 used it; extend to load multi-file eval set if not native |
| Free GPU RAM (~30 GB for batch=2 × max_seq=4096) | ✅ available | E3 arch rule 4 confirmed on M5 Max |
| `mlx_lm.server` / `llama-server` quiesced during training | ⚠️ operator-visible | Training loop auto-quiesces MDEMG's `com.mdemg.llama-server` (E3 pattern); MDEMG partially non-functional during training window |

**No blocking dependencies.**

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Split mdemg_usage_v1 (80/10/10) + extend assembler (~1h)

**Goal**: create `mdemg_usage_v1/{train,valid,benchmark_holdout}.jsonl` splits + append the family to the E3 assembler.

Deliverables:
- `scripts/split_mdemg_usage_v1.py` — deterministic split (SHA-256(row_id) mod 10 buckets: 0-7 train, 8 valid, 9 benchmark_holdout — reproducible + stable across re-runs)
- Updates `training_data/sft/mdemg_usage_v1/manifest.json` to reflect new split counts
- Emits `training_data/sft/mdemg_usage_v1/{train_split.jsonl, valid_split.jsonl, benchmark_holdout.jsonl}` alongside the original `train.jsonl` (preserved)
- Extends `scripts/phase_e3_assemble_corpus.py` OR writes sibling `scripts/mdemg_usage_lora_assemble.py` that adds `mdemg_usage_v1` as a family with valid.jsonl
- Emits `training_data/sft/mdemg_usage_lora_001/{train.jsonl, valid.jsonl, manifest.json}` — combined 6-family corpus

Gate: assembler exits 0; manifest lists 6 families; train.jsonl row count ≈ 6753 + 958 = 7711; valid.jsonl row count ≈ 750 + 120 = 870; SHAs stable across re-runs.

### Epic 2 — Training config (~30 min)

**Goal**: `configs/sft_mdemg_usage_lora_001.yaml` byte-adjacent to E3's config, with per-file docstring explaining diffs.

Diffs from `configs/sft_phase_e3_v1_base_v3.yaml`:
- `data:` → `training_data/sft/mdemg_usage_lora_001`
- `iters:` → `7711 / batch=2 × 2 epochs = 7711` (2 epochs on combined; E3 did 1 epoch = 3376)
- `adapter_path:` → `adapters/mdemg_usage_lora_001`
- Everything else IDENTICAL to E3 (batch=2, max_seq=4096, LR 1e-5, 7 modules × rank 32, seed=0, grad_checkpoint, val_batches=20, steps_per_eval=200, save_every=400)

Wall-clock estimate: E3 was ~90 s/iter × 3376 iters ≈ 84h (but E3 report says ~13h; discrepancy is because E3 early-stopped at iter 1750). Conservative estimate for THIS sprint: 90 s/iter × 7711 iters ≈ 193h (8 days). If FT-OAI-001 early-stop fires around iter 3500-5000 (mid-run), actual wall-clock 4-5 days.

⚠️ **Wall-clock is a hard operator-visibility issue** — MDEMG is degraded for 4-8 days. Recommend reducing epochs to 1 (iters=3855) to halve the window at the cost of shallower coverage. **Config default: 2 epochs; operator can override via `--iters 3855` CLI arg for 1-epoch mode**. Data-decided at Epic 2 gate after checking early-stop behavior.

Gate: config exists; iters/adapter_path values consistent; passes YAML lint.

### Epic 3 — LoRA training (~30-96h wall-clock, monitored)

**Goal**: produce `adapters/mdemg_usage_lora_001/000XXXX_adapters.safetensors` at the best-val-loss checkpoint.

Flow:
1. Confirm llama-server is up + `curl :9999/healthz` OK BEFORE training start
2. Snapshot baseline: `sha256sum training_data/sft/{tier1,family_*,claude_code_knowledge_v3_stripped,mdemg_usage_v1}/train.jsonl` → archive to sprint dir
3. `launchctl bootout gui/$UID/com.mdemg.llama-server` (quiesce; frees ~10 GB GPU + KV cache)
4. `nohup python -m mlx_lm.lora --config configs/sft_mdemg_usage_lora_001.yaml > logs/training.log 2>&1 &`
5. Monitor tokens/sec + val_loss every ~30 min
6. Watchdog: kill if `val_loss > best × 1.05` for 2 consecutive evals (FT-OAI-001 rule 3) OR if wall-clock > 120h
7. On completion: restart llama-server on shipped v1 adapter (`launchctl bootstrap gui/$UID/… + launchctl kickstart`)
8. Capture: best-adapter path + train.log + adapter_config.json + SHA

Gate: adapter safetensors produced; best-val-loss recorded; llama-server back up serving v1.

### Epic 4 — Benchmark vs baseline (~30 min)

**Goal**: apples-to-apples aggregate on `valid_clean.jsonl` (13 tasks) + new `mdemg.usage` task via `benchmark_holdout.jsonl`.

Deliverables:
- `neural/benchmarks/run_benchmark.py` invocation with `--adapters adapters/mdemg_usage_lora_001` + `--eval-files valid_clean.jsonl,mdemg_usage_v1_holdout.jsonl` (extend runner if it doesn't already support multi-file)
- `training_data/eval/mdemg_usage_lora_001_bench.json` — per-task pass rates + aggregate
- SQL sidecar for TSDB persistence (E3 pattern)

Verdict rubric (from task #145):
| Outcome | Aggregate delta vs 0.9188 | Next step |
|---|---|---|
| ✅ | ≥ -0.01 | `PHASE-E4-GATE-PROMOTE-001` next; operator ratifies |
| ⚠️ | -0.01 to -0.02, with `mdemg.usage ≥ 0.5` | Operator decision |
| ❌ | < -0.02 | Keep v1; iterate on scope/corpus |

### Epic 5 — Verification + hygiene (~1h)

- Frozen families untouched (SHA post-check vs pre-check snapshot)
- llama-server serving v1 healthy post-training (`curl :9999/healthz`)
- Adapter SHA recorded
- Corpus SHAs preserved
- MDEMG functional post-training (spot-check `/v1/memory/retrieve`)
- llm_interactions error backfill (E3 arch pattern — quiesce-window errors → error='' so RSIC alert_llm_health doesn't re-fire)

### Epic 6 — Sprint post + PR comment + follow-ups (~30 min)

Deliverables:
- `docs/development/mdemg-usage-lora-001/sprint_post.md` (this file's twin)
- CHANGELOG.md Unreleased entry
- PR summary comment
- File `MDEMG-USAGE-LORA-002` (v2 base variant) if v1 arm succeeds
- File `PHASE-E4-GATE-PROMOTE-001` follow-up sprint IF verdict ✅

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests (python)

- `test_split_mdemg_usage_v1.py`: given a 100-row fixture, split → 80/10/10 partition; SHA-of-row_id-mod-10 assignment; determinism (re-run → identical partitions)
- Extension of assembler unit tests (or new tests) to cover the 6th-family injection

### Tier 2 — Integration tests

- End-to-end assembler on 6-family fixture: SHAs verified; row counts match manifest; byte-verbatim preservation confirmed via first/last-row sampling

### Tier 3 — Live e2e (`live-testing-tier-required`)

- Live LoRA training on real Qwen3-14B-4bit base against real 7,711-row corpus (~30-96h wall-clock)
- Live benchmark on `valid_clean.jsonl` + `mdemg_usage_v1_holdout.jsonl`
- Aggregate + per-task deltas recorded in `training_data/eval/mdemg_usage_lora_001_bench.json`
- llama-server health pre + post + serving v1 both times

## 7. Commit Strategy

Sequential commits:

1. `feat(training): MDEMG-USAGE-LORA-001 Epic 1 — split mdemg_usage_v1 + assemble 6-family corpus`
2. `feat(training): MDEMG-USAGE-LORA-001 Epic 2 — training config`
3. `train(mdemg-usage): MDEMG-USAGE-LORA-001 Epic 3-4 — adapter shipped + benchmark result` (single commit for the training + benchmark artifacts)
4. `docs(training): MDEMG-USAGE-LORA-001 Epic 5-6 — sprint post + verification + follow-ups`

Each commit lint-clean per `lint-before-commit`. All commits on `reh3376_dev01`, auto-PR to main.

## 8. Verification Checklist

- [ ] Epic 1: `train_split.jsonl` + `valid_split.jsonl` + `benchmark_holdout.jsonl` exist; row counts ≈ 958/120/120; splits deterministic (re-run → identical SHAs)
- [ ] Epic 1: `training_data/sft/mdemg_usage_lora_001/{train.jsonl, valid.jsonl, manifest.json}` exist; row counts ≈ 7711/870
- [ ] Epic 2: `configs/sft_mdemg_usage_lora_001.yaml` exists; iters/adapter_path/data consistent
- [ ] Epic 3: adapter safetensors produced at best-val-loss checkpoint
- [ ] Epic 3: llama-server serving v1 restored to healthy post-training
- [ ] Epic 4: `training_data/eval/mdemg_usage_lora_001_bench.json` exists with per-task + aggregate
- [ ] Epic 4: verdict determined per rubric; `mdemg.usage` per-task pass rate recorded
- [ ] Epic 5: frozen family SHAs post-check match pre-check
- [ ] Epic 5: MDEMG healthy end-to-end
- [ ] Epic 6: sprint post + CHANGELOG + PR comment
- [ ] Task #145 status updated (in_progress → completed)
- [ ] Follow-up tasks filed as appropriate

## 9. Documentation Update

- `docs/development/mdemg-usage-lora-001/sprint_post.md` (new)
- CHANGELOG.md Unreleased entry
- PR summary comment
- Extend `docs/features/mdemg-usage-corpus.md` with a "Trained-adapter reference" pointer once E4 verdict is in
- (NO new feature doc — this sprint is training + benchmarking of an already-documented feature family)

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Wall-clock 4-8 days blocks MDEMG for that window | Default to 2 epochs = ~90h; operator can override to 1 epoch = ~45h. Data-decidable: FT-OAI-001 early stop may fire mid-run and cut it shorter. |
| llama-server not restored after training crash | Epic 3 step 7 wraps in try/finally; also documented in sprint post; recovery command is idempotent |
| Corpus assembly picks up wrong SHAs mid-run | E3's SHA pre+post check pattern reused |
| Aggregate regresses on 13-task valid_clean (not just gains on mdemg.usage) | Verdict rubric explicit; ⚠️ MIXED branch allows operator decision; ❌ branch defers PHASE-E4 |
| max_seq=4096 truncates rerank_cross prompts (E3 arch note) | Documented; if rerank_cross regresses > 0.05, iterate on max_seq=8192 config (would need batch=1 → ~180 s/iter → 15 days wall-clock; DO NOT do this in the same sprint) |
| Val_batches=40 rows → variance in early-stop signal | Per E3 arch rule 3, keep 2-consecutive requirement; consider val_batches=40 → 80 if signal too noisy (Epic 3 monitoring decision) |
| Peak GPU memory OOM (E3 arch rule 4: ~30 GB at batch=2 × max_seq=4096) | Config already reduced-scope; won't approach the 100 GB limit at batch=4 × max_seq=8192 |
| Frozen family accidentally modified | SHA pre-check + SHA post-check at Epic 5 |
| mdemg_usage_v1 benchmark holdout too small (~120 rows) to give a stable per-task pass rate | Accept variance; document in verdict; ~120 rows is a lower bound for meaningful per-task; if aggregate signal weak, iterate on holdout size in follow-up sprint (would need corpus restructuring) |
| Substrate ingest state change during training window | Not this sprint's concern; substrate is read-only in this sprint |
| Operator wants to cancel mid-training | Trivially safe: SIGTERM the training process; llama-server restore via documented command; no state corruption |

## 11. Documents Accessed

- `docs/development/mdemg-usage-corpus-curate-001/{sprint_plan,sprint_post}.md` (this sprint's direct predecessor)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (arch rules + wall-clock lessons)
- `configs/sft_phase_e3_v1_base_v3.yaml` (canonical training config shape)
- `scripts/phase_e3_assemble_corpus.py` (canonical assembler shape to extend)
- `training_data/sft/mdemg_usage_v1/{manifest.json,train.jsonl}` (source of new family)
- `training_data/sft/{tier1,family_*,claude_code_knowledge_v3_stripped}/{train,valid}.jsonl` (frozen families)
- `training_data/eval/valid_clean.jsonl` (benchmark anchor)
- `neural/benchmarks/run_benchmark.py` (benchmark runner)
- `.local-models/qwen3-14b-4bit-base/` (base model)
- CLAUDE.md pins:
  - `must-follow-12-section-format`, `mandatory-feature-docs`, `end-with-docs-accessed`
  - `sequential-epics`, `never-hardcode-config`, `unit-integration-e2e-docs`
  - `live-testing-tier-required`, `lint-before-commit`, `must-comment-sprint-summary-on-pr`
  - `must-use-cuid2`
  - PHASE-E3 arch rules 1-4
  - Operator directive 2026-08-24 (proceed with MDEMG-USAGE-LORA-001)

## 12. Rollback Procedures (destructive ops)

**Genuinely destructive steps**: Epic 3 quiesces `com.mdemg.llama-server` (MDEMG partially non-functional during window). Reversal is deterministic:

```bash
# Restore v1 llama-server (idempotent)
launchctl bootstrap gui/$UID ~/Library/LaunchAgents/com.mdemg.llama-server.plist
launchctl kickstart -k gui/$UID/com.mdemg.llama-server
# Verify
curl -s http://127.0.0.1:8102/v1/models | jq '.data[0].id'
```

**Adapter output rollback**: `rm -rf adapters/mdemg_usage_lora_001/` — reversible; no substrate side effects.

**Corpus + config rollback**:

```bash
rm -rf training_data/sft/mdemg_usage_lora_001/
rm -rf training_data/sft/mdemg_usage_v1/{train_split,valid_split,benchmark_holdout}.jsonl
rm configs/sft_mdemg_usage_lora_001.yaml
# Frozen families untouched by design; no restore needed
# Original mdemg_usage_v1/train.jsonl preserved (not modified) — split files are additive
```

**Training window recovery**: if training hangs, SIGTERM the mlx_lm.lora python process; adapter checkpoints at last save_every=400 remain usable (or discardable via `rm -rf adapters/mdemg_usage_lora_001/`).

**Zero substrate mutation**. No Neo4j writes. No TSDB writes except the optional benchmark_runs row at Epic 4 (single INSERT; reversible via TSDB DELETE if needed).
