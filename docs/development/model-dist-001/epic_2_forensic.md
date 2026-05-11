# Epic 2 Forensic — Exit Decision: Defer Adapter to MODEL-DIST-002

**Date**: 2026-05-11
**Sprint**: MODEL-DIST-001
**Outcome**: Adapter path **deferred** per Sprint Plan §5 Epic 2 contingency clause.

## What was investigated

Per the sprint plan, Epic 2's goal was to convert `adapters/tier1/adapters.safetensors` (MLX format, 514 MB, Phase 5 SFT Iter 2400) to GGUF LoRA format so operators with their own Qwen3-14B base can layer the LoRA at runtime via `llama-server --lora` or Ollama's Modelfile `ADAPTER` directive.

### Findings (from forensic inspection)

1. **MLX adapter structure**:
   - 560 tensors = 40 layers × 7 target_modules × 2 (`lora_a` + `lora_b`)
   - target_modules: `self_attn.{q,k,v,o}_proj`, `mlp.{gate,up,down}_proj`
   - rank 32, alpha 64, scale 20.0 (MLX-specific scaling)
   - Tensor shapes: `lora_a` = `(input_features, rank)`, `lora_b` = `(rank, output_features)`

2. **PEFT format requirements**:
   - Key naming: `base_model.model.model.layers.<N>.<module>.lora_A.default.weight` (and `lora_B`)
   - Tensor shapes: `lora_A` = `(rank, input_features)`, `lora_B` = `(output_features, rank)` — **opposite of MLX**
   - `adapter_config.json` schema differs from MLX's `adapter_config.json`

3. **`convert_lora_to_gguf.py` availability**:
   - **NOT installed** by `brew install llama.cpp` on this M5 (only `convert_hf_to_gguf.py` is in `/opt/homebrew/bin/`).
   - Available in llama.cpp source repo (`gguf-py/scripts/`) but requires manual fetch + dependencies (`gguf` Python package, which we did install for Epic 1).

### Estimated work to complete Epic 2

| Step | Estimated time |
|---|---|
| Fetch `convert_lora_to_gguf.py` from llama.cpp source | 5 min |
| Write custom MLX → PEFT converter (key rename + tensor transpose + adapter_config.json schema translation) | 30 min |
| Live verify PEFT output loads via `peft.PeftConfig` | 10 min |
| Run PEFT → GGUF LoRA conversion | 5 min |
| Live verify `llama-server --lora <adapter.gguf>` against `qwen3:14b` GGUF base | 15 min |
| Debugging / format quirks (PEFT metadata, GGUF LoRA quirks) | 15-30 min |
| **Total** | **80-95 min** |

This exceeds the ~30 min budget I had time-boxed and lands well outside the 1.5-day Epic 2 estimate when factoring in the tooling-gap surprise.

## Decision

**Defer adapter publication to follow-up sprint MODEL-DIST-002.**

Rationale per `Sprint Plan §10 Risks & Mitigations` row 1:

> "MLX → PEFT → GGUF LoRA conversion is blocked by tooling gaps — Medium likelihood, High impact. Mitigation: Epic 2 forensic surfaces the path. Contingency: ship fused-only this sprint; adapter becomes MODEL-DIST-002."

The fused GGUF path (Epics 1, 3, 4, 5) delivers the **primary operator value** unblocked: a one-command `mdemg model pull` that fetches and configures the production canonical model. The adapter path is explicitly framed in the plan as the "advanced users" surface; it's not on the critical path for new operators onboarding.

## In-scope changes from this decision

1. **Sprint plan**: this exit decision is referenced; Epic 2's deliverables move to MODEL-DIST-002.
2. **Epic 3**: drop the `Modelfile.adapter` Modelfile authoring + adapter publish. Publish only the 3 fused-quant Modelfiles to Ollama Library.
3. **Epic 4 CLI**: the `--adapter` flag stays defined in the API but errors with "adapter distribution lands in MODEL-DIST-002; until then, use `--quant <q>` for the fused model." The flag's machinery (alternate tag name resolution) is kept for forward-compatibility; no functional regression vs the no-adapter design.
4. **Epic 6 e2e**: drop the adapter-pull step. Verify only fused-quant pulls.
5. **Epic 7 feature doc**: adapter section noted as "coming in MODEL-DIST-002" with brief rationale.
6. **quant_manifest.json**: adapter block flagged `status: "deferred to MODEL-DIST-002"` with the forensic findings linked.
7. **MODEL-DIST-002 scope** (new follow-up sprint, to be planned separately): write the MLX → PEFT converter, fetch `convert_lora_to_gguf.py`, end-to-end verify, publish `<NAMESPACE>/<NAME>-adapter:latest` to Ollama Library, ship the `--adapter` flag's full implementation.

## Artifacts preserved for MODEL-DIST-002

- `adapters/tier1/adapters.safetensors` (MLX, 514 MB) — input.
- `adapters/tier1/adapter_config.json` — MLX-format adapter config; contains all metadata needed to write PEFT's `adapter_config.json`.
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.f16.gguf` (30 GB) — intermediate from Epic 1; retained so MODEL-DIST-002 can use it as the base for `llama-server --lora` verification without re-running Epic 1's pipeline.
- This forensic doc + the quant_manifest.json adapter block.

## Time spent on Epic 2

~30 minutes (forensic inspection only). No build artifacts produced.

## Sign-off

Decision made by Claude (Opus 4.7) per Sprint Plan contingency clause. Operator-visible impact: zero (adapter was always "advanced users"; fused path is the primary surface). Documented in CHANGELOG at Epic 8.
