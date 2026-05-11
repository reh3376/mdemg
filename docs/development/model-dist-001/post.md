# Sprint MODEL-DIST-001 — Sprint Close Post

**Sprint ID**: MODEL-DIST-001
**Opened**: 2026-05-07
**Closed**: 2026-05-11
**Duration**: ~5 dev-days (estimated 5–7, in-budget)
**OpenAI spend**: $0 (estimated $0)
**Risk realized**: Epic 2 contingency triggered as planned (adapter deferred to MODEL-DIST-002)

## Outcome

`mdemg model pull` is the canonical one-command path from `brew install mdemg` to a working local LLM. All 3 fused quants published to Ollama Library and live for operators to pull:

- https://ollama.com/reh3376/mdemg-llm-v1:Q4_K_M (8.4 GB, 12 GB RAM min)
- https://ollama.com/reh3376/mdemg-llm-v1:Q5_K_M (9.8 GB, 14 GB RAM min — production canonical, Phase 13.5)
- https://ollama.com/reh3376/mdemg-llm-v1:Q8_0 (14.6 GB, 20 GB RAM min)

## Process

The sprint plan (12-section v1.0 format) at `sprint_plan_model_dist_001.md` was the operating contract. 9 epics, sequential per memory rule. Operator decisions captured via `AskUserQuestion` at planning time (distribution channel, artifact form, quant matrix); rest of planning data-decided per memory.

Mid-sprint correction: operator surfaced the "no-hardcoding" rule as load-bearing for the framework's design. The plan was revised in-place to add a **Configurability Contract** (11 env vars + flag overrides) and a **pluggable `Fetcher` interface** before code was written. The revision is captured in CHANGELOG's Unreleased entry + the feature doc.

## Findings

### Smooth parts

1. **Pluggable `Fetcher` interface stayed clean.** Zero Ollama-specific knowledge leaked into `internal/cli/model.go`; all of it lives in `model_fetcher_ollama.go`. Adding `HFFetcher` next would be ~150 LOC + one factory branch.
2. **Quant build pipeline was fast** — `llama-quantize` ran each conversion in 11–40 seconds on M5 Max (estimated ~5 min/quant per CLAUDE.md was conservative; SSD + Metal kernels are much faster than the conservative estimate).
3. **CLI tests pinned the Configurability Contract cleanly** — 22 unit tests cover every dynamic config knob's resolution path. Grep audit confirmed no behavior-controlling string literals in the new code.
4. **Ollama push was uneventful** — claim namespace + add SSH public key, then 3 sequential pushes succeeded first try. Layer deduplication across the 3 quants meant most config blobs uploaded once.
5. **End-to-end SHA integrity verified** — remote Ollama manifest digests match local Epic 1 SHAs exactly; the conversion + push + pull + verify chain is byte-identical at each stage.

### Friction / surprises

1. **`convert_hf_to_gguf.py` python deps gap.** The script shipped by `brew install llama.cpp` at `/opt/homebrew/bin/convert_hf_to_gguf.py` requires `torch + transformers + gguf + sentencepiece + protobuf`. The brew install includes none of them; the script silently fails with `ModuleNotFoundError`. Resolved by installing into `neural/.venv`. Worth a documentation note in MODEL-DIST-002 or a follow-up packaging cleanup.
2. **`mlx_lm.fuse` requires an `--adapter-path` even for dequantize-only flow.** First attempt with `mlx_lm fuse --model ... --dequantize` (no adapter) failed because the script defaults `--adapter-path adapters/` and errors on missing `adapter_config.json`. Resolved by passing the actual Phase 5 adapter (which is the correct call per the documented pipeline — the dequantize step re-fuses base + adapter and saves bf16).
3. **`convert_lora_to_gguf.py` is not in `brew install llama.cpp`.** Only available via llama.cpp source clone. This was the proximate trigger for Epic 2 deferral; combined with the MLX → PEFT tensor-transposition work, the total exceeded Epic 2's 1.5-day estimate. Contingency clause in the plan handled this cleanly — adapter scope moved to MODEL-DIST-002, sprint shipped fused-only.
4. **`mdemg tsdb migrate` requires `TSDB_PORT=5433` in env even though `.env` has it set.** `godotenv.Load()` runs only when CWD has `.env`; the running shell can lose its CWD across Bash invocations. Worth wrapping `mdemg tsdb migrate` (and other CLI commands) with a CWD-aware `.env` resolver in a follow-up sprint.
5. **Quant sizes from Epic 1 were approximate.** The Epic 1 numbers in quant_manifest.json (9.0 / 11 / 16 GB) were off vs the registry-reported exact bytes (8.4 / 9.8 / 14.6 GB) — corrected at Epic 3 closeout. No functional impact; the verify path uses SHA256, not size.

### Epic 2 deferral — exit decision audit

Per `epic_2_forensic.md`, MLX → PEFT → GGUF LoRA conversion estimated at 80–95 min vs ~30 min Epic 2 remaining budget. Hitting the contingency criterion documented in Sprint Plan §10 Risk #1 was the right call:

- **What we shipped**: fused-only path. Operators have a complete one-command pull, the primary user value.
- **What we deferred**: adapter-only "advanced users" path. The `--adapter` flag returns `ErrAdapterDeferred` with a clear MODEL-DIST-002 forward reference; machinery preserved.
- **Net**: primary user value unblocked; advanced-user surface lands in a focused follow-up sprint with proper time budget.

## Current state

| Layer | State |
|---|---|
| Ollama Library | 3 quants live, manifest digests captured in `quant_manifest.json` |
| Local quant store | All 3 GGUFs on operator's machine + 30 GB f16 intermediate retained for MODEL-DIST-002 adapter work |
| `mdemg model` CLI | 5 subcommands shipped, 22 unit tests, lint clean, integrated with V0021 TSDB |
| TSDB V0021 | Migration applied to dev TSDB; 2 rows landed during Tier 3 e2e (1 pull + 1 verify) |
| Docs | Feature doc + sprint plan + sprint close (this) + CHANGELOG + CLAUDE.md + main README + .goreleaser caveats — all updated |
| Adapter | Deferred to MODEL-DIST-002; forensic + contingency documented |

## Testing & benchmarking

- **Tier 1 (unit)**: 22 tests in `internal/cli/model_test.go`, all green. Backend factory + quant resolution + RAM-tier parsing + manifest load + Ollama tag/blob/manifest composition + adapter deferral all covered.
- **Tier 2 (integration)**: 3 fixture-filesystem tests cover Ollama manifest JSON parsing (mediaType filtering, malformed, no-model-layer).
- **Tier 3 (live e2e)**:
  - `mdemg model pull --quant Q5_K_M`: ollama pull → manifest discovery → blob symlink → SHA verify against embedded manifest. PASS, 4719 ms wall.
  - `mdemg model pull --quant Q4_K_M`: PASS, 3943 ms wall, V0021 row landed.
  - `mdemg model list`: tabular output OK.
  - `mdemg model verify`: SHA matches embedded manifest; V0021 verify-row landed.
  - V0021 TSDB rows confirmed via `docker exec mdemg-timescaledb-1 psql ...`. Both event_type values (`pull`, `verify`) recorded correctly with full metadata.

Live smoke documented in PR #385 comment per memory rule.

## Risks & opportunities (forward)

| Risk | Disposition |
|---|---|
| Operator without ollama installed | Documented in feature-doc troubleshooting; manual `MDEMG_MODEL_PATH` is the escape hatch |
| Ollama manifest layout changes between versions | Defensive parsing with mediaType filtering; documented in feature doc + tested in `TestOllamaFetcher_ReadModelBlobDigest_FiltersOnMediaType` |
| Disk overrun on multi-quant install | RAM-auto picks ONE by default; multi-quant requires explicit opt-in |
| V0021 schema bump requires migration on running TSDB | `auto-migrate=true` in Docker compose picks it up on container restart; manual fallback is `mdemg tsdb migrate` |

Opportunities:
- **MODEL-DIST-002**: ship adapter-only Modelfile + `convert_lora_to_gguf.py` integration. Operator can layer mdemg LoRA over their own Qwen3-14B base.
- **Sprint B (queued)**: Grafana panels for V0021 model_install_events (pull rate by quant, failure rate, latency p50/p95).
- **Cross-platform**: Linux/CUDA inference path. Operator interest TBD.
- **`HFFetcher`**: HuggingFace Hub mirror — useful for users who can't or won't install Ollama.
- **`mdemg tsdb migrate` CWD-aware .env loader**: small QoL fix; surfaced during Tier 3 e2e.

## Documents Accessed (post-sprint)

In addition to the sprint plan's Documents Accessed list:

- `epic_2_forensic.md` (this sprint's deferral artifact)
- Live Ollama Library manifest endpoints at `registry.ollama.ai/v2/reh3376/mdemg-llm-v1/manifests/<quant>` (verified end-to-end during Epic 3 closeout)
- TSDB V0021 row state via `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics`

## Sprint commits (10 total on `reh3376_dev01`, all merged into `main` via PR #385 squash + Epic 3 closeout follow-up)

| Commit | Epic |
|---|---|
| `8620529` | 0 — Sprint plan + forensic |
| `bfbaa67` | 1 — Built Q4_K_M + Q8_0 fused GGUFs |
| `764f3b6` | 2 — Adapter deferred (contingency) |
| `4f0f180` | 3 — Modelfiles + local ollama create |
| `a216b1f` | 4 — ModelFetcher interface + CLI |
| `66fc67f` | 5 — TSDB V0021 hypertable + writer |
| `1f5935b` | 7 — Feature doc |
| `d91a66e` | 8 — Documentation Update (main repo) |
| `87293f8` | 3 closeout — Ollama push complete |
| (next) | sprint close (this doc) |
