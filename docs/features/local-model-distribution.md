# Local Model Distribution

**Sprint**: MODEL-DIST-001 (2026-05-11), MODEL-DIST-002 adapter path (2026-05-25)
**Status**: Default-on for Apple Silicon. Fused-GGUF + adapter-only paths both shipped.
**Feature surface**: `mdemg model pull [--adapter] | list | verify | remove | where`

## Why

The Phase 13.5 cutover (2026-05-03) made `mdemg-llm-v1.Q5_K_M.gguf` (~10 GB, Phase 5 dense Qwen3-14B fine-tune) the production LLM, served via `llama.cpp llama-server` on port 8102. Before this sprint, the GGUF existed only on developer machines via a manual MLX → bf16 → GGUF pipeline. Operators running `brew install mdemg` got a working binary but no way to obtain the model. `mdemg`'s startup preflight (`LLM_WATCHDOG_ENABLED=true` default since Hotfix 11.6.3.1) refuses to start when the LLM endpoint is unreachable — so the local-LLM use case was effectively dev-only.

This sprint ships a one-command path: `mdemg model pull` fetches the model from the configured distribution backend and writes a symlink under `<MDEMG_MODEL_DIR>` that `llama-server` can serve. Three quants serve three RAM tiers.

## Choices

### Distribution backend: Ollama Library

Decision matrix (against the alternatives surfaced during planning):

| Backend | Verdict | Notes |
|---|---|---|
| **Ollama Library** | ✅ chosen for v1 | Free CDN, resumable, operators may already have `ollama` for embeddings, well-documented Modelfile format. |
| Hugging Face Hub | reserved as `HFFetcher` plug-in slot | Canonical for PEFT, but operator preference is Ollama. |
| GitHub Release assets | reserved as `GitHubReleaseFetcher` slot | 2 GB per-file cap would need chunked-archive reassembly. |
| Self-hosted CDN / S3 | reserved as `S3Fetcher` slot | Enterprise / air-gapped use case, deferred. |
| Local file URI | reserved as `FileFetcher` slot | Offline import, deferred. |

The framework uses a **pluggable `Fetcher` interface** — adding a backend is one new file + one branch in `NewFetcher`; the CLI surface stays unchanged.

### Artifact form: fused GGUF (default)

The Phase 13.5 production model is a **fused** form (LoRA adapter merged into base weights, then quantized to GGUF). The fused form is what `llama-server` consumes; it's the simplest distribution form.

An **adapter-only path** (LoRA weights for advanced users who want to layer mdemg's Phase 5 SFT over their own Qwen3-14B base, or who want a ~257 MB download instead of the ~9 GB fused GGUF) shipped in MODEL-DIST-002 as `reh3376/mdemg-llm-v1-adapter:latest`. Operators pull it via `mdemg model pull --adapter`, which symlinks to `<MDEMG_MODEL_DIR>/mdemg-llm-v1-adapter.gguf` and load via `llama-server --model <qwen3-14b-base.gguf> --lora <adapter.gguf>`. Pipeline: MLX safetensors → `scripts/mlx_adapter_to_peft.py` → PEFT directory → `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` → GGUF LoRA (257 MB f16, 560 tensors over 40 layers × 7 target modules × 2 (a + b)).

### Apple Silicon only for v1

Linux/CUDA inference paths and vLLM multi-LoRA serving are deferred. The Phase 13.5 production stack is Apple Silicon + `llama.cpp llama-server`; cross-platform parity is a follow-up sprint.

### Why not Ollama runtime?

Ollama provides both a registry (ollama.com) and a runtime (`ollama serve` on port 11434). This sprint uses **Ollama as distribution only** — the production runtime remains `llama.cpp llama-server` on port 8102. CLAUDE.md documents Ollama runtime as broken on M5 + macOS 26.3.x across versions 0.20.5–0.22.1 (matmul2d static_assert). The `mdemg model pull` flow leverages Ollama for the download + manifest management, then points llama-server at the resulting blob.

## How it works

```
        ┌──────────────────────────┐
        │   mdemg model pull       │
        │  (internal/cli/model.go) │
        └────────────┬─────────────┘
                     │  builds FetchRequest from
                     │  cfg.Model* (env / flags / defaults)
                     ▼
        ┌─────────────────────────────┐
        │   NewFetcher(cfg) dispatch  │
        │  on cfg.ModelBackend        │
        └────────────┬────────────────┘
                     │ "ollama" → OllamaFetcher
                     ▼
        ┌────────────────────────────────────────┐
        │   OllamaFetcher.Fetch(ctx, req)         │
        │ 1. preflight: which ollama?            │
        │ 2. exec `ollama pull <ns>/<name>:<q>`  │
        │ 3. read manifest at                    │
        │    <OLLAMA_MODELS>/manifests/<host>/   │
        │    <ns>/<name>/<quant>                 │
        │ 4. select layer with mediaType         │
        │    application/vnd.ollama.image.model  │
        │ 5. resolve digest → blob path          │
        │    <OLLAMA_MODELS>/blobs/sha256-<d>    │
        │ 6. symlink                             │
        │    <MDEMG_MODEL_DIR>/<name>.<q>.gguf   │
        │    → <blob path>                       │
        │ 7. SHA256 the resolved file            │
        └────────────┬───────────────────────────┘
                     │
                     ▼
        ┌──────────────────────────────────────┐
        │ Verify result.SHA256 against         │
        │ embedded quant_manifest.json         │
        │ (or MDEMG_MODEL_MANIFEST_PATH        │
        │  override for air-gapped setups)     │
        └────────────┬─────────────────────────┘
                     │
                     ▼
        ┌──────────────────────────────────────┐
        │ Write V0021 model_install_events row │
        │ (backend, ns, name, quant, success,  │
        │  latency, sha, size). Non-fatal if   │
        │  TSDB unreachable.                   │
        └──────────────────────────────────────┘
```

The pluggable interface and the dynamic config contract are the load-bearing design decisions. Implementing `HFFetcher` (or any other backend) is one new file at `internal/cli/model_fetcher_hf.go` that satisfies the `Fetcher` interface; the CLI subcommands don't need to change.

## How to use

### Quick start

```bash
# Prerequisites: ollama installed
brew install ollama        # macOS
# (Linux: curl -fsSL https://ollama.com/install.sh | sh)

# Pull — RAM-auto picks the right quant for your hardware
mdemg model pull
# → Resolved: backend=ollama namespace=reh3376 name=mdemg-llm-v1 quant=Q5_K_M ...

# Set the path in your .env (printed by `mdemg model pull`):
echo "MDEMG_MODEL_PATH=$HOME/.mdemg/models/mdemg-llm-v1.Q5_K_M.gguf" >> .env

# Restart llama-server with the new model path:
launchctl kickstart -k gui/$UID/com.mdemg.llama-server

# Verify:
curl -s http://127.0.0.1:8102/v1/models | jq '.data[0].id'
```

### Explicit quant selection

```bash
mdemg model pull --quant Q4_K_M   # 9 GB, 12 GB RAM min, 16 GB recommended
mdemg model pull --quant Q5_K_M   # 11 GB, 14 GB RAM min, 24 GB recommended (production canonical)
mdemg model pull --quant Q8_0     # 16 GB, 20 GB RAM min, 32 GB recommended
```

### Managing pulled models

```bash
mdemg model list                  # tabular: name, symlink target, size, sha256-prefix
mdemg model verify                # re-check SHAs against the embedded quant manifest
mdemg model where                 # print resolved path (for shell scripting)
mdemg model remove --yes          # unlinks symlink + invokes `ollama rm <tag>`
```

### Forks and enterprise use

Publishing under your own namespace:

```bash
# Override via env (sticky for shell session)
export MDEMG_MODEL_NAMESPACE=acme
export MDEMG_MODEL_NAME=custom-llm
mdemg model pull --quant Q5_K_M
# → pulls acme/custom-llm:Q5_K_M

# Or override per-invocation via flags
mdemg model pull --namespace acme --name custom-llm --quant Q5_K_M
```

Air-gapped deployments with a custom quant manifest:

```bash
mdemg model pull --manifest /etc/mdemg/internal-quant-manifest.json
```

### Resource matrix

| Quant | GGUF size | Min RAM | Recommended RAM | BPW | Notes |
|---|---|---|---|---|---|
| Q4_K_M | ~9.0 GB | 12 GB | 16 GB | 4.87 | Smallest fidelity tier. UVTS regression vs Q5_K_M empirically TBD. |
| Q5_K_M | ~11 GB | 14 GB | 24 GB | 5.69 | **Production canonical** (Phase 13.5). |
| Q8_0 | ~16 GB | 20 GB | 32 GB | 8.50 | Highest fidelity. ~50% bigger than Q5_K_M for marginal quality gain on a 14B fine-tune. |

The "Min RAM" column covers GGUF weights + ~3 GB working set (KV cache headroom for `--ctx-size 32768 --parallel 4`). Recommended RAM gives the OS + other apps comfortable headroom.

**Apple Silicon only for v1.** Linux/CUDA support deferred.

### Configurability Contract — env vars and flags

Every operator-visible value is dynamic. Defaults match the v1 production reality so `mdemg model pull` with no flags Just Works for the common case.

| Concern | Env Var | CLI Flag | Default |
|---|---|---|---|
| Distribution backend | `MDEMG_MODEL_BACKEND` | `--backend` | `ollama` |
| Registry namespace | `MDEMG_MODEL_NAMESPACE` | `--namespace` | `reh3376` |
| Model name | `MDEMG_MODEL_NAME` | `--name` | `mdemg-llm-v1` |
| Available quants (allowlist) | `MDEMG_MODEL_QUANTS` | `--quants` | `Q4_K_M,Q5_K_M,Q8_0` |
| RAM-tier auto-pick (JSON) | `MDEMG_MODEL_RAM_TIERS` | `--ram-tiers` | `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}` |
| Selected quant override | `MDEMG_MODEL_QUANT` | `--quant` | `auto` (triggers RAM dispatch) |
| Adapter pull form | n/a | `--adapter` | `false` (fused quant). Set `true` to pull `<ns>/<name>-adapter:latest` instead. |
| Adapter base model | `MDEMG_ADAPTER_BASE` | `--adapter-base` | `qwen3:14b` (referenced by `Modelfile.adapter` via `FROM`) |
| Local symlink dir | `MDEMG_MODEL_DIR` | `--model-dir` | `$HOME/.mdemg/models` |
| Ollama blob root | `OLLAMA_MODELS` | (ollama-standard) | `$HOME/.ollama/models` |
| Ollama registry host | `OLLAMA_HOST` | (ollama-standard) | `registry.ollama.ai` |
| Quant manifest source | `MDEMG_MODEL_MANIFEST_PATH` | `--manifest` | embed.FS `quant_manifest.json` |

Flag overrides take precedence over env, env over default.

### Observability (V0021 model_install_events)

Each `mdemg model pull|verify|remove` writes one row to the TSDB `model_install_events` hypertable (7-day chunks). Columns: `event_id`, `recorded_at`, `event_type`, `backend_name`, `namespace`, `model_name`, `quant`, `adapter`, `success`, `latency_ms`, `sha256`, `size_bytes`, `err_message`. Writes are best-effort (non-blocking; 2s connect timeout). Grafana panels deferred to Sprint B.

## Troubleshooting

### `ollama: command not found`

```bash
brew install ollama   # macOS
curl -fsSL https://ollama.com/install.sh | sh   # Linux
```

If you don't want Ollama as a dependency: manually obtain a GGUF, set `MDEMG_MODEL_PATH` in `.env`, and skip `mdemg model pull`. The framework's runtime is `llama-server`, not Ollama — Ollama is only a distribution channel.

### `SHA mismatch for QX_K_M`

The pulled blob doesn't match the SHA in the embedded quant manifest. Two paths to resolution:
1. **The manifest is stale relative to the published model.** Update the binary: `brew upgrade mdemg` (or `mdemg upgrade`).
2. **The blob was corrupted in transit or you're pulling from a fork with different SHAs.** For forks: override the manifest with `MDEMG_MODEL_MANIFEST_PATH=/path/to/your-manifest.json`.

### `quant "QX" not in MDEMG_MODEL_QUANTS allowlist`

Either you typed a quant name not in the published set, or your env has overridden the allowlist. `mdemg model pull --quants Q4_K_M,Q5_K_M,Q8_0` resets the allowlist for one invocation.

### `RAM auto-detection failed`

Pass `--quant <Q4_K_M|Q5_K_M|Q8_0>` explicitly. The `auto` mode uses `sysctl hw.memsize` on darwin and `/proc/meminfo` on linux; other OS detect as 0 GB and require explicit selection.

### `out of disk` during pull

Total disk footprint for all 3 quants is ~36 GB. The default pull picks ONE quant via RAM auto-detection — operators must explicitly opt-in to multi-quant install. Use `mdemg model remove --yes` to free space.

### Symlink already exists

`mdemg model pull` is idempotent — it removes the existing symlink before creating the new one. If `os.Remove` fails with a permission error, check that `<MDEMG_MODEL_DIR>` is writable by the invoking user.

## Forward-looking

- **MODEL-DIST-002**: ✅ Shipped 2026-05-25. Adapter-only path live at `reh3376/mdemg-llm-v1-adapter:latest`; `mdemg model pull --adapter` symlinks the GGUF LoRA at `<MDEMG_MODEL_DIR>/mdemg-llm-v1-adapter.gguf`.
- **Sprint B**: ✅ Shipped (GRAFANA-AUDIT-001). Grafana panels for `model_install_events` (per-quant pull rate, failure rate, latency distribution).
- **Future backends**: `HFFetcher`, `S3Fetcher`, `GitHubReleaseFetcher`, `FileFetcher` — each one new file + factory branch.
- **Cross-platform**: Linux/CUDA inference path + vLLM multi-LoRA serving.

## References

- Sprint plan: [`docs/development/model-dist-001/sprint_plan_model_dist_001.md`](../development/model-dist-001/sprint_plan_model_dist_001.md)
- Epic 2 deferral: [`docs/development/model-dist-001/epic_2_forensic.md`](../development/model-dist-001/epic_2_forensic.md)
- Quant manifest (canonical): [`docs/development/model-dist-001/quant_manifest.json`](../development/model-dist-001/quant_manifest.json)
- Quant manifest (runtime embed): [`internal/cli/quant_manifest.json`](../../internal/cli/quant_manifest.json)
- Modelfiles: [`packaging/ollama/`](../../packaging/ollama/)
- Source of truth code:
  - `internal/cli/model.go` — Cobra subcommand registration
  - `internal/cli/model_fetcher.go` — `Fetcher` interface + factory + quant resolution
  - `internal/cli/model_fetcher_ollama.go` — `OllamaFetcher` implementation
  - `internal/tsdb/model_install_writer.go` — V0021 single-row INSERT writer
  - `internal/tsdb/migrations/021_model_install_events.sql`
