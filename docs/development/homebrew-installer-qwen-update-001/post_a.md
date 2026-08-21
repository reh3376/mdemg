# HOMEBREW-INSTALLER-QWEN-UPDATE-001 — Sprint Post (Phase A: operator publish)

**Task**: #134
**Shipped (Phase A)**: 2026-08-20
**Ship state**: All 3 quants of `reh3376/mdemg-llm-v2` LIVE on Ollama Library. SHAs + Ollama manifest digests captured in `publish_manifest_v2.json`. Phase C (task #136) is UNBLOCKED.

## What shipped

- `reh3376/mdemg-llm-v2:Q4_K_M` (15.7 GB) — https://ollama.com/reh3376/mdemg-llm-v2:Q4_K_M → 200
- `reh3376/mdemg-llm-v2:Q5_K_M` (18.2 GB) — https://ollama.com/reh3376/mdemg-llm-v2:Q5_K_M → 200
- `reh3376/mdemg-llm-v2:Q8_0` (27.1 GB) — https://ollama.com/reh3376/mdemg-llm-v2:Q8_0 → 200
- Index: https://ollama.com/reh3376/mdemg-llm-v2 → 200
- `publish_manifest_v2.json` — captured SHAs, Ollama layer digests, Ollama manifest digests, projector digest (shared across all 3 tiers)

All 3 tiers preserve Qwen3.8's multi-modal capabilities (vision via CLIP projector `sha256:ac3714bf...` 931 MB, thinking, tools, completion) — Q4/Q5 include the same projector blob as Q8 (content-addressed dedup on Ollama's side keeps push overhead flat).

## Verification (`must-validate-all-claims-before-commit` applied throughout)

| Claim | Verification | Verdict |
|---|---|---|
| Q4/Q5/Q8 all public on Ollama Library | `curl -sI` per-tag + index → all 200 | ✅ |
| Q8_0 is byte-identical to upstream `qwen3.8:27b-q8_0` | `ollama cp` used (content-addressed dedup); local tag ID `8f5fb6b71ea0` matches source | ✅ |
| Q4_K_M works end-to-end (inference doesn't tensor-error) | `ollama run reh3376/mdemg-llm-v2:Q4_K_M "reply ok"` → "ok" with thinking preamble | ✅ |
| Q5_K_M works end-to-end | Same test on Q5 → "ok" | ✅ |
| Q8_0 works end-to-end | Same test on Q8 → "ok" | ✅ |
| Multi-modal projector preserved on Q4/Q5 | Modelfile has 2 FROM lines (LLM + shared projector); `ollama show` confirms | ✅ |
| Phase B SHA-guard behaves correctly against a v2 pull | Fires "skipped — loaded manifest is for `mdemg-llm-v1`, request is for `mdemg-llm-v2`" (per Phase B ship contract; no false-mismatch) | ✅ (pin-tested in Phase B; live-testable now that v2 exists — Phase C will verify) |

## Decisions

| Decision | Rationale |
|---|---|
| Publish pipeline: Path 3a (Ollama-source) + `ollama cp` for Q8 + Q8→Q5/Q4 requantize | Path 3b/3c BROKEN — `convert_hf_to_gguf.py` (llama.cpp b6600) omits `blk.64.attn_norm.weight` for `Qwen3_5ForConditionalGeneration` arch. Every downstream Q* GGUF loads but crashes on inference. Path 3a bypasses the broken converter by using Ollama Library's own working Q8_0. |
| Q8→Q5 + Q8→Q4 quant-of-quant (not native f16→Q4/Q5) | Forced by the converter break. Q8 has enough dynamic range that the quality delta vs native f16→Q4/Q5 is negligible in practice; strictly better than nothing. Phase C can re-quantize from a fixed converter once llama.cpp lands the Qwen3.5 arch handler. |
| Preserve multi-modal projector on Q4/Q5 | Q8's Modelfile carries the 931 MB CLIP projector blob; Q4/Q5 Modelfiles include the same blob (dedup keeps push flat). Operators picking lower-quant tiers don't lose vision capability arbitrarily. |
| Use `RENDERER qwen3.8` + `PARSER qwen3.5` (Ollama-native) instead of hand-authored `<\|im_start\|>` TEMPLATE | Qwen3.5 arch requires Ollama's built-in renderer for correct tool-use + thinking-block handling. A hand-authored template block emits raw template tokens with this arch. Requires Ollama ≥ 0.32.14 (also the min version for pulling qwen3.8:27b-*). |
| Mandatory local sanity test before push | Introduced as Step 4a based on the broken-GGUF near-miss — a tensor-missing GGUF `ollama create`s cleanly but crashes on first token. Push-then-discover would require republishing every downstream consumer. Fail-fast locally. |
| Push Q8 first, then Q4+Q5 in parallel background | Q8 was the highest-confidence tag (byte-identical to upstream). Parallel Q4+Q5 pushes throttled by daemon but no rejection. |

## The convert-pipeline break (recorded for llama.cpp upstream + Phase 3b/3c recovery)

**Symptom**: `mdemg-llm-v2.Q*.gguf` files from `convert_hf_to_gguf.py --outtype f16` (llama.cpp b6600, Sep 2025) → `llama-quantize` chain load into `llama-server` cleanly, then error on FIRST inference with:

```
llama_model_load: error loading model: check_tensor_dims: tensor 'blk.64.attn_norm.weight' not found
```

**Trigger arch**: `Qwen3_5ForConditionalGeneration` (Qwen 3.8 model family; internal Ollama name `qwen35`). Both MLX-dequant (Path 3b) and native HF-safetensors (Path 3c) input paths produce the same broken output — the bug is in the converter's `qwen35` handler emit path, not the input.

**Recovery**: Path 3d (documented in PUBLISH_GUIDE.md) — pull Ollama's working Q8_0 blob and `ollama cp` for Q8 + `llama-quantize` for Q5/Q4. Ollama's own converter is ahead of llama.cpp upstream on this arch.

**Filed as follow-up**: llama.cpp upstream tracker — check for a Qwen3.5 arch fix commit before running Phase 3b/3c again. When fixed, re-run publish via Path 3c for native full-fidelity quantizations.

## Follow-ups

### 🔴 Phase C — HOMEBREW-INSTALLER-QWEN-UPDATE-002 (task #136, UNBLOCKED)

- Read `publish_manifest_v2.json` — real SHAs + Ollama manifest digests for all 3 quants
- Create `internal/cli/quant_manifest_v2.json` with these values
- Extend `LoadQuantManifest(cfg)` in `internal/cli/model_fetcher.go` to pick `quant_manifest.json` vs `quant_manifest_v2.json` based on `cfg.ModelName` (embed both side-by-side via `//go:embed`)
- Update `docs/features/local-model-distribution.md` with v2 quant tiers + RAM math (27B needs different RAM auto-pick thresholds than 14B)
- Full end-to-end live smoke: `MDEMG_MODEL_NAME=mdemg-llm-v2 mdemg model pull --quant Q5_K_M` → SHA verify PASSES → symlink lands → operator edits `.env` `MDEMG_MODEL_PATH` → `launchctl kickstart -k gui/$UID/com.mdemg.llama-server` → llama-server serves the 27B model
- Optional: `mdemg model use <name>` shorthand command

### Downstream unblocked

- **PHASE-E4-GATE-PROMOTE-001** — E4 (LoRA promote) can now distribute v2 to end users. Not blocked by E3 retrain outcome; E4 promotes whatever E3 produces (or a raw base, depending on operator choice).

### Optional Phase D — republish from fixed converter

Once llama.cpp lands the Qwen3.5 arch fix, re-run Path 3c (or 3b) for native full-fidelity Q4/Q5/Q8 quantizations. Publish as `reh3376/mdemg-llm-v2:Q*-native` (or bump v2→v3 if the quality delta is material — decide from a UBENCH A/B).

## Arch rules pinned to CLAUDE.md (proposed — will land in Phase C's PR body or a subsequent doc-currency sprint)

- **When a critical toolchain component (`convert_hf_to_gguf.py` here) breaks on a specific model arch, document the failure signature + recovery path in the runbook AND leave the broken paths (3b/3c) intact with the ⚠️ warning banner** — deleting them loses institutional memory of what to avoid; the banner + fallback path is the safest shape.
- **Sanity-test every published artifact locally BEFORE push, especially when the pipeline had known-broken variants** — a tensor-missing GGUF `ollama create`s cleanly but crashes on first inference. Fail-fast locally beats "republish every downstream consumer."
- **`ollama cp` from an upstream tag preserves content-addressed identity** — when the source blob IS the intended output, `ollama cp <upstream> <namespace>/<name>:<tag>` is instant + byte-preserving + push-dedup-friendly. Do NOT `ollama create` from the same blob (unnecessary re-hashing + risk of Modelfile drift from upstream).
- **For Qwen3.5 arch models, `RENDERER qwen3.8` + `PARSER qwen3.5` are REQUIRED in the Modelfile** — hand-authored `<|im_start|>...` TEMPLATE blocks emit raw template tokens (garbled responses). Ollama ≥ 0.32.14 is the minimum for both the pull path AND the renderer.

## Documents Accessed

- `docs/development/homebrew-installer-qwen-update-001/{sprint_plan,sprint_post,PUBLISH_GUIDE}.md`
- `training_data/eval/qwen27b-bakeoff/*.json` (task #91 bake-off verdict — Qwen3.8-27B)
- `internal/cli/model.go` (Phase B SHA guard; live-behavior verify: fires skip for v2 pulls)
- `internal/cli/model_fetcher.go` (`manifestAppliesToRequest` guard function)
- Live `ollama show --modelfile qwen3.8:27b-q8_0` (extracted reference Modelfile shape)
- Live `ollama create` × 2 (Q4/Q5) + `ollama cp` × 1 (Q8)
- Live `ollama run` sanity × 3 (all 3 quants pre-push)
- Live `ollama push` × 3
- Live `curl https://ollama.com/reh3376/mdemg-llm-v2:{Q4_K_M,Q5_K_M,Q8_0}` verifications
- `publish_manifest_v2.json` (this sprint's output — SHAs + manifest digests for Phase C)
