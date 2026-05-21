# Sprint MODEL-DIST-002 — Adapter-Only Distribution Path

## 1. Header & Metadata

- **Sprint ID**: MODEL-DIST-002
- **Sprint line**: `docs/development/model-dist-002/`
- **Date opened**: 2026-05-21
- **Target version**: v0.10.1 (patch — additive feature, no breaking changes)
- **Estimated effort**: 1–2 dev-days
- **OpenAI spend**: $0
- **Risk level**: Medium (MLX → PEFT tensor transposition must be exactly right; Ollama push is one-way)

## 2. Problem Statement

Operators who want to apply mdemg's Phase 5 LoRA fine-tune over their own Qwen3-14B base — or who want a ~200-400 MB download instead of the ~9 GB fused GGUF — currently have no path. `mdemg model pull --adapter` errors with `ErrAdapterDeferred`. This sprint ships the path.

## 3. Scope & Constraints

**In scope:**
- Build `scripts/mlx_adapter_to_peft.py` — MLX → PEFT directory.
- Vendor `convert_lora_to_gguf.py` from llama.cpp source into `scripts/vendor/llama_cpp/`.
- Pipeline: MLX adapter → PEFT directory → GGUF LoRA.
- Live verify via `llama-server --lora`.
- Author `packaging/ollama/Modelfile.adapter`; `ollama create` + operator-gated push.
- Wire `mdemg model pull --adapter` end-to-end (remove `ErrAdapterDeferred` guard).
- Update `quant_manifest.json` in both locations with adapter SHA + Ollama digest.
- Tier 3 e2e: real `mdemg model pull --adapter`; load via llama-server; sanity inference.
- Documentation: `docs/features/local-model-distribution.md` adapter section flips to "shipped."

**Out of scope:**
- Runtime adapter swap on the running llama-server.
- Adapter A/B benchmarking.
- New adapter quants beyond the one Phase 5 SFT adapter.
- Re-training or modifying adapter weights.
- New backends beyond Ollama.

**Constraints:**
- Sequential epics; Tier 3 live testing required.
- No-hardcoding: existing `MDEMG_ADAPTER_BASE` env var handles base override.
- Ollama push is one-way; Epic 4 gated on operator confirmation.

## 4. Dependencies

- MODEL-DIST-001 deliverables on `main`: `mdemg model pull` CLI, `OllamaFetcher`, embedded `quant_manifest.json`, `Fetcher` interface with adapter tag pattern.
- External: `convert_lora_to_gguf.py` from llama.cpp source (vendored into `scripts/vendor/llama_cpp/`).
- Python deps: `peft` (Epic 0 install into `neural/.venv`); existing `torch + transformers + gguf` from MODEL-DIST-001 Epic 1.
- Operator action (Epic 4 push gate): `reh3376` namespace already claimed; same SSH key auth.
- Existing artifacts: `adapters/tier1/adapters.safetensors` (514 MB MLX), `mdemg-llm-v1.f16.gguf` (30 GB, preserved from MODEL-DIST-001 Epic 1).

## 5. Implementation Plan

**Epic 0 — Sprint plan + workspace prep**
Commit this plan. `pip install peft` into `neural/.venv`. Vendor `convert_lora_to_gguf.py` + `LICENSE.llama_cpp` + `README.md` into `scripts/vendor/llama_cpp/`.

**Epic 1 — MLX → PEFT converter + Tier 1 tests**
Build `scripts/mlx_adapter_to_peft.py`:
- Input: MLX `adapter_config.json` + `adapters.safetensors`.
- Key rename: `model.layers.<N>.<module>.lora_a` → `base_model.model.model.layers.<N>.<module>.lora_A.default.weight`; same for `lora_b` → `lora_B.default.weight`.
- Tensor transpose: MLX `(in, rank)` → PEFT `(rank, in)` for `lora_A`; MLX `(rank, out)` → PEFT `(out, rank)` for `lora_B`.
- Write PEFT-schema `adapter_config.json` (rank, alpha, target_modules, base_model_name_or_path, peft_type=LORA, task_type=CAUSAL_LM, fan_in_fan_out=false, init_lora_weights=true).
- Write `adapter_model.safetensors`.
- 10+ Tier 1 tests: key mapping, shape inversion, schema correctness.

**Epic 2 — PEFT → GGUF LoRA**
Run `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` against PEFT dir. Expected output ~200-400 MB with 560 tensors. SHA capture.

**Epic 3 — Live verification**
`llama-server --model <f16-base> --lora <adapter.gguf> --port 18103`. Sanity inference vs fused-merged baseline at port 8102. Document in `verification.md`.

**Epic 4 — Modelfile + Ollama publish (operator-gated)**
Author `packaging/ollama/Modelfile.adapter`. `ollama create reh3376/mdemg-llm-v1-adapter:latest -f Modelfile.adapter`. Verify locally. **Operator-gated push.**

**Epic 5 — CLI: enable `--adapter`**
Remove `ErrAdapterDeferred` guard in `internal/cli/model_fetcher_ollama.go::Fetch` (line 116-118). Replace `TestOllamaFetcher_AdapterDeferred` with happy-path tests. Update `quant_manifest.json` adapter block.

**Epic 6 — Tier 3 live e2e**
`mdemg model pull --adapter` → verify symlink + SHA + TSDB V0021 row with `adapter=true` + llama-server load + sanity inference.

**Epic 7 — Documentation Update**
Feature doc adapter section flips to "shipped." CHANGELOG, CLAUDE.md, post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1**: `scripts/mlx_adapter_to_peft_test.py` (10+ tests). `internal/cli/model_fetcher_ollama_test.go` happy-path tests for adapter.
- **Tier 2**: Full chain validation: MLX → PEFT loads via `peft.PeftConfig.from_pretrained`; GGUF parses via `gguf-py`.
- **Tier 3**: Epic 3 + Epic 6 live e2e against real llama-server + Ollama.

## 7. Commit Strategy

Sequential per epic on `reh3376_dev01`. Epic 4 operator-gated. Final commit promotes CHANGELOG.

## 8. Verification Checklist

- [ ] MLX → PEFT converter passes 10+ Tier 1 tests
- [ ] PEFT output loads via `peft.PeftConfig.from_pretrained`
- [ ] GGUF LoRA has 560 tensors (40 layers × 7 target_modules × 2)
- [ ] llama-server loads adapter against f16 base; sanity inference coherent
- [ ] (Base + adapter) outputs semantically agree with (fused merged) outputs
- [ ] Ollama push complete; `reh3376/mdemg-llm-v1-adapter:latest` live
- [ ] `mdemg model pull --adapter` end-to-end works
- [ ] V0021 TSDB row written with `adapter=true`
- [ ] `quant_manifest.json` updated with adapter SHA + Ollama digest
- [ ] No `ErrAdapterDeferred` references in committed code
- [ ] `golangci-lint` clean
- [ ] Feature doc adapter section ships as "available"
- [ ] CHANGELOG, CLAUDE.md, post.md updated

## 9. Documentation Update — Epic 7 above

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| MLX → PEFT key/transpose mismatch → garbage outputs | Medium | High | Tier 1 tests on shape inversion; Epic 3 inference comparison catches silent divergence |
| `convert_lora_to_gguf.py` API drift from upstream | Medium | Medium | Pinned snapshot in `scripts/vendor/`; re-fetch documented in README |
| Qwen3:14b on Ollama uses different dims vs our adapter | Low | High | Same Qwen3-14B base (per MODEL-DIST-001 Epic 0 forensic); Epic 3 load test catches |
| Ollama push one-way | Medium | Low | Operator gate; `ollama push --delete` exists for rollback |
| `peft` install conflict in `neural/.venv` | Low | Medium | Test Epic 0; fallback to separate venv |

## 11. Documents Accessed

- `docs/development/model-dist-001/epic_2_forensic.md` (original deferral rationale)
- `docs/development/model-dist-001/sprint_plan_model_dist_001.md`
- `docs/development/model-dist-001/quant_manifest.json`
- `internal/cli/model_fetcher_ollama.go` (existing adapter tag path)
- `internal/cli/model_fetcher.go` (`ErrAdapterDeferred` definition)
- `adapters/tier1/adapters.safetensors` (input)
- `adapters/tier1/adapter_config.json` (MLX adapter metadata)
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.f16.gguf` (Epic 3 verification base)
- `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` (PEFT → GGUF tool)

## 12. Rollback Procedures

- Ollama publish: `ollama push --delete reh3376/mdemg-llm-v1-adapter:latest`. Operators with local pulls retain them.
- CLI changes: revert the OllamaFetcher edit; restore `ErrAdapterDeferred` guard.
- Vendored script: delete `scripts/vendor/llama_cpp/`.
- `peft` install: keep (useful for future sprints).
