# Ollama Library Publishing — `mdemg-llm-v1`

This directory holds the Modelfiles used to publish `mdemg-llm-v1` to Ollama Library.
See Sprint MODEL-DIST-001 (`docs/development/model-dist-001/`) for the full pipeline.

## Files

- `Modelfile.Q4_K_M` — 4-bit K-quant medium variant (~9.0 GB, 12 GB RAM min, 16 GB recommended)
- `Modelfile.Q5_K_M` — 5-bit K-quant medium variant (~11 GB, 14 GB RAM min, 24 GB recommended) — **production canonical**
- `Modelfile.Q8_0` — 8-bit variant (~16 GB, 20 GB RAM min, 32 GB recommended)

## Publishing (operator workflow)

Prerequisites:
- `ollama` CLI installed (`brew install ollama`)
- ollama.com account with namespace claimed (defaults to `reh3376`; override via the `ollama create` argument)
- GGUF files built locally per Sprint MODEL-DIST-001 Epic 1; paths in the `FROM` directive are relative to this directory (`../../.local-models/mdemg-llm-v1-gguf/...`)

Create each model locally:

```
cd packaging/ollama
ollama create reh3376/mdemg-llm-v1:Q4_K_M -f Modelfile.Q4_K_M
ollama create reh3376/mdemg-llm-v1:Q5_K_M -f Modelfile.Q5_K_M
ollama create reh3376/mdemg-llm-v1:Q8_0   -f Modelfile.Q8_0
ollama list   # verify all three appear
```

Push to Ollama Library (**one-way action**; verify locally first):

```
ollama push reh3376/mdemg-llm-v1:Q4_K_M
ollama push reh3376/mdemg-llm-v1:Q5_K_M
ollama push reh3376/mdemg-llm-v1:Q8_0
```

After publishing, capture the resulting manifest digests into `docs/development/model-dist-001/quant_manifest.json` (per-quant `ollama_manifest_digest` field) so `mdemg model verify` can SHA-check against the canonical record.

## Customizing for a fork

Forks publishing their own `mdemg-llm-v1` derivative under a different namespace:

1. Build your own GGUFs (your fine-tune output → quantize pipeline; see Epic 1 of the sprint plan).
2. Edit the `FROM ./<path>` line in each Modelfile to point at your local GGUF.
3. Edit the `SYSTEM` block if your variant has a different positioning.
4. Run `ollama create <your-namespace>/<your-name>:<quant> -f Modelfile.<quant>` for each.
5. Operators using `mdemg model pull` against your fork set `MDEMG_MODEL_NAMESPACE=<your-namespace>` (and optionally `MDEMG_MODEL_NAME=<your-name>`); see the Configurability Contract in `docs/development/model-dist-001/sprint_plan_model_dist_001.md` §3.

## Adapter (LoRA-only) Modelfile

Deferred to Sprint MODEL-DIST-002 — see `docs/development/model-dist-001/epic_2_forensic.md` for the deferral rationale (MLX → PEFT → GGUF LoRA conversion has tooling gaps that exceed this sprint's Epic 2 budget).
