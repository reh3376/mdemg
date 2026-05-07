# Sprint MODEL-DIST-001 — Local LoRA Distribution via Ollama Library

## 1. Header & Metadata

- **Sprint ID**: MODEL-DIST-001
- **Sprint line**: `docs/development/model-dist-001/`
- **Date opened**: 2026-05-07
- **Target version**: v0.10.0 (minor — net-new operator-facing feature + new soft dependency)
- **Estimated effort**: 5–7 dev-days
- **OpenAI spend**: $0
- **Risk level**: Medium (Ollama publish is one-way; MLX → GGUF LoRA conversion is the riskiest engineering item)
- **Supersedes (partial)**: `docs/research/mdemg_sprint_ideas/MDEMG_FT_LORA_PACKAGING_SPEC.md` — see §3 below.

## 2. Problem Statement

Phase 13.5 cutover (2026-05-03) made `mdemg-llm-v1.Q5_K_M.gguf` (9.8 GB, fused Phase 5 dense Qwen3-14B fine-tune) the production LLM, served via `llama.cpp llama-server` on port 8102. The model exists only on developer machines today — there is no canonical way for a new operator running `brew install mdemg` to obtain it. mdemg's startup preflight (`LLM_WATCHDOG_ENABLED=true` default since Hotfix 11.6.3.1) refuses to start without a reachable LLM endpoint, so the framework is effectively dev-only for new users. This sprint ships the missing distribution path.

## 3. Scope & Constraints

### In scope

- Build 3 fused GGUF quants from the existing Phase 13.5 production tree (which today only ships Q5_K_M):
  - **Q4_K_M** (~6.5 GB, 16 GB Macs)
  - **Q5_K_M** (9.8 GB, current production canonical)
  - **Q8_0** (~14.6 GB, 32+ GB Macs)
- Convert the standalone LoRA adapter (`adapters/tier1/adapters.safetensors`, 514 MB MLX format) to GGUF LoRA format for `--lora`-style runtime application.
- Publish to Ollama Library:
  - `<NAMESPACE>/<NAME>:Q4_K_M`, `:Q5_K_M`, `:Q8_0` (default `<NAMESPACE>=reh3376`, `<NAME>=mdemg-llm-v1`)
  - `<NAMESPACE>/<NAME>-adapter:latest` (Modelfile uses `FROM qwen3:14b@sha256:…` + `ADAPTER ./<adapter>.gguf`)
- New CLI: `mdemg model pull|list|verify|remove|where` with RAM-tier auto-detection on `pull`.
- Operator-visible docs: feature doc, README quick-start, formula caveats, resource-tier matrix.
- TSDB V0021 `model_install_events` hypertable (Grafana panels deferred to Sprint B).

### Configurability contract — every value is dynamic

Per the framework's no-hardcoding rule (memory: `feedback_no_hardcoded_values.md`, reinforced by the operator on 2026-05-07): **no string literal in the new code carries an operator-visible value.** Every concern surfaces as an env var with a CLI flag override and a sensible default tuned for the v1 production reality, so `mdemg model pull` with no flags Just Works.

| Concern | Env Var | CLI Flag | Default |
|---|---|---|---|
| Distribution backend | `MDEMG_MODEL_BACKEND` | `--backend` | `ollama` |
| Registry namespace | `MDEMG_MODEL_NAMESPACE` | `--namespace` | `reh3376` |
| Model name | `MDEMG_MODEL_NAME` | `--name` | `mdemg-llm-v1` |
| Available quants (allowlist) | `MDEMG_MODEL_QUANTS` | `--quants` | `Q4_K_M,Q5_K_M,Q8_0` |
| RAM-tier auto-pick map (JSON) | `MDEMG_MODEL_RAM_TIERS` | `--ram-tiers` | `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}` |
| Selected quant override | `MDEMG_MODEL_QUANT` | `--quant` | `auto` (RAM-tier dispatch) |
| Adapter base model | `MDEMG_ADAPTER_BASE` | `--adapter-base` | `qwen3:14b` |
| Local model dir | `MDEMG_MODEL_DIR` | `--model-dir` | `~/.mdemg/models` |
| Ollama models root | `OLLAMA_MODELS` | (ollama-standard) | `~/.ollama/models` |
| Ollama registry host | `OLLAMA_HOST` | (ollama-standard) | `registry.ollama.ai` |
| Quant manifest source | `MDEMG_MODEL_MANIFEST_PATH` | `--manifest` | embed.FS `quant_manifest.json` |
| TSDB schema version | (existing migration system) | — | bumped 20→21 |

The implementation surfaces a `ModelFetcher` interface; v1 ships only `OllamaFetcher`. Future backends (`HFFetcher`, `S3Fetcher`, `GitHubReleaseFetcher`, `FileFetcher`) plug in via factory dispatch on `MDEMG_MODEL_BACKEND` without touching the CLI surface. The CLI has zero knowledge of Ollama-specific concepts — no `~/.ollama/` paths, no `ollama pull` invocation in the command file. Adding the next backend is one new file + one factory branch.

### Out of scope

- Linux/CUDA inference path (per spec §13). Apple Silicon only for v1.
- vLLM multi-LoRA (per spec §13).
- HF Hub publication (per operator direction; `HFFetcher` slot reserved).
- GitHub Releases as fallback (deferred until Ollama proves insufficient; `GitHubReleaseFetcher` slot reserved).
- Sigstore/cosign signing (spec §13).
- Gated/enterprise model access (spec §13).
- `mdemg model fuse` (current production already ships fused).
- `mdemg model export|import` for offline workflows (deferred to follow-up sprint per spec §13).
- Grafana panels (Sprint B).
- Training pipeline changes (spec §13).

### Constraints

- Sequential epics (memory: `feedback_sequential_epics.md`).
- Docs scaffold before implementation; final docs at end (Epic 8) — never cut.
- Tier-3 live testing required: real binary, real Ollama Library, real llama-server (memory: `feedback_live_testing_required.md`).
- Ollama publish is one-way. Epic 3 is gated on operator confirmation.

### Supersedes / aligns with prior art

The speculative spec at `docs/research/mdemg_sprint_ideas/MDEMG_FT_LORA_PACKAGING_SPEC.md` (v1.0, 2026-04-24) is prior art. This sprint **supersedes** the spec on three load-bearing decisions:

1. Spec proposed HF Hub as primary; this sprint uses **Ollama Library** per operator direction (HF reserved as a future plug-in slot).
2. Spec proposed adapter-only as canonical artifact; this sprint ships **both fused GGUF (default) and adapter-only (advanced)**, because Phase 13.5 made the fused GGUF the production runtime form, but adapter-only retains the spec's reproducibility advantages for power users.
3. Spec proposed cross-platform parity (MLX + llama.cpp + vLLM); this sprint scopes to **Apple Silicon + llama.cpp only** for v1.

The spec's CLI surface (`install|list|verify|remove|fuse|export|import`) is adopted with `pull` substituted for `install` to match Ollama vocabulary, and `fuse|export|import` deferred to follow-up sprints.

## 4. Dependencies

- **Operator action**: claim `reh3376` namespace on ollama.com, generate API token, add to local environment for `ollama push`.
- **External tool**: `convert_lora_to_gguf.py` from llama.cpp tooling.
- **External tool**: `convert_hf_to_gguf.py` from llama.cpp tooling (Epic 1: regenerate f16 GGUF intermediate, since only Q5_K_M survives on disk per Epic 0 forensic).
- **MLX → PEFT layout converter**: investigate during Epic 2; fallback is a small Python script using `mlx_lm` to load the MLX adapter and re-emit `adapter_config.json` + `adapter_model.safetensors` in PEFT layout.
- **Existing repo state** (Epic 0 forensic verified):
  - `adapters/tier1/adapters.safetensors` — 514 MB, present.
  - `.local-models/qwen3-14b-mdemg-v1/` — full MLX merged model with `chat_template.jinja`, `config.json`, `tokenizer.json`, `manifest.json`, `model-{1,2}-of-2.safetensors` (7.8 GB). Source for f16 GGUF regeneration.
  - `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` — 9.8 GB, SHA `144ad723101d688f0875a3129336b0cd0fa356ff5266034a3d35db5e6d5f5d54`.
- **Existing CLI infrastructure**: Cobra command tree in `internal/cli/root.go`; `loadConfig()` pattern in `internal/cli/config_loader.go`; TSDB migration pattern (V0001..V0020 examples); writer pattern at `internal/tsdb/sparse_gate_writer.go`.
- **Phase 13.5 plist** (PR #384, merged): `com.mdemg.llama-server.plist` references `MDEMG_MODEL_PATH` resolved by `resolveMDEMGModelPath()` in `internal/cli/service_darwin.go`. The default already points at the GGUF filepath the new CLI will write to.

## 5. Implementation Plan

### Epic 0 — Sprint plan + forensic (0.5 day) — IN PROGRESS

Commit this plan to `docs/development/model-dist-001/sprint_plan_model_dist_001.md`. Verify on-disk state. Live-fetch Ollama qwen3:14b manifest digest for adapter Modelfile pinning.

### Epic 1 — Build per-quant fused GGUFs (1 day)

1. Regenerate f16 GGUF intermediate from MLX merged model (`.local-models/qwen3-14b-mdemg-v1/` → bf16 HF safetensors via `mlx_lm.fuse --dequantize` → f16 GGUF via `convert_hf_to_gguf.py --outtype f16`). ~5 min.
2. **Q4_K_M**: `llama-quantize <f16.gguf> mdemg-llm-v1.Q4_K_M.gguf Q4_K_M` (~5 min, target ~6.5 GB).
3. **Q5_K_M**: copy from existing `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (no work).
4. **Q8_0**: `llama-quantize <f16.gguf> mdemg-llm-v1.Q8_0.gguf Q8_0` (~5 min, target ~14.6 GB).
5. SHA256 + size each; persist to `docs/development/model-dist-001/quant_manifest.json`.
6. Live smoke per quant: `llama-server --model <gguf> --port 18102 --ctx-size 32768 --jinja`; hit `/v1/models`; run a sanity inference; record latency p50.

### Epic 2 — MLX → PEFT → GGUF LoRA adapter conversion (1.5 day, riskiest)

1. Convert `adapters/tier1/adapters.safetensors` (MLX format, 514 MB) → PEFT layout (`adapter_config.json` + `adapter_model.safetensors`). Investigate `mlx_lm.fuse --export-peft` first; fallback is a small Python script using `mlx_lm.tuner.utils.load_adapters` + `peft.PeftConfig` writer. Output to `.local-models/mdemg-llm-v1-adapter-peft/`.
2. PEFT → GGUF LoRA: `convert_lora_to_gguf.py <peft-dir> --outfile mdemg-llm-v1-adapter.gguf`.
3. Live verify: pull `qwen3:14b` from Ollama (or use a local f16 base), run `llama-server --model qwen3-14b-base.gguf --lora mdemg-llm-v1-adapter.gguf`, sanity inference. Compare outputs to merged-GGUF outputs (small differences from quant noise expected; document tolerance in post.md).
4. **Contingency**: if MLX → PEFT conversion blocks on tooling gaps, ship fused-only this sprint and split adapter publication into MODEL-DIST-002. Document the exit decision in `docs/development/model-dist-001/epic_2_forensic.md`.

### Epic 3 — Modelfiles + Ollama Library publication (1 day)

1. Author 4 Modelfiles in `packaging/ollama/`:
   - `Modelfile.Q4_K_M`, `Modelfile.Q5_K_M`, `Modelfile.Q8_0`: each `FROM ./mdemg-llm-v1.<quant>.gguf` + `PARAMETER num_ctx 32768` + `PARAMETER num_predict 4096` + stop tokens + `SYSTEM` block. Rely on the GGUF's embedded chat_template (no Modelfile `TEMPLATE` block).
   - `Modelfile.adapter`: `FROM qwen3:14b@sha256:<manifest-digest-from-epic-0>` + `ADAPTER ./mdemg-llm-v1-adapter.gguf` + matching params.
2. `ollama create <NAMESPACE>/<NAME>:<quant> -f Modelfile.<quant>` × 3 + adapter (local-only side-effect).
3. **OPERATOR-GATED**: `ollama push <NAMESPACE>/<NAME>:<quant>` × 4 only after explicit operator confirmation (one-way action).
4. Capture published manifest digests into `docs/development/model-dist-001/quant_manifest.json` for `mdemg model verify`.

### Epic 4 — `ModelFetcher` interface + `mdemg model pull` CLI (1.5 day)

1. New `internal/cli/model.go` — registers `model` subcommand group. Subcommands: `pull`, `list`, `verify`, `remove`, `where`. **Zero Ollama-specific knowledge.**
2. New `internal/cli/model_fetcher.go` — defines:
   ```go
   type FetchRequest struct { Namespace, Name, Quant, DestDir string; Adapter, DryRun bool }
   type FetchResult struct { LocalPath, BlobPath, SHA256, BackendName string; SizeBytes, LatencyMS int64 }
   type Fetcher interface {
       Name() string
       Fetch(ctx context.Context, req FetchRequest) (*FetchResult, error)
       Verify(ctx context.Context, req FetchRequest) error
       Remove(ctx context.Context, req FetchRequest) error
   }
   func NewFetcher(cfg config.Config) (Fetcher, error)  // dispatches on MDEMG_MODEL_BACKEND
   ```
3. New `internal/cli/model_fetcher_ollama.go` — `OllamaFetcher` implementation. Encapsulates ALL Ollama-specific knowledge: `exec.LookPath("ollama")`, `ollama pull` invocation, manifest JSON parsing under `OLLAMA_MODELS/manifests/<OLLAMA_HOST>/<namespace>/<name>/<tag>`, blob digest extraction with `mediaType: application/vnd.ollama.image.model` filter, blob path resolution under `OLLAMA_MODELS/blobs/sha256-<digest>`.
4. `mdemg model pull` body: build `FetchRequest` from resolved config (flag → env → default precedence); call `NewFetcher(cfg).Fetch(ctx, req)`; verify SHA against `quant_manifest.json` (resolved via `MDEMG_MODEL_MANIFEST_PATH`, falls back to embedded); symlink `<MDEMG_MODEL_DIR>/<name>.<quant>.gguf → result.BlobPath`; print operator instructions; write TSDB row.
5. RAM-tier auto-pick: parse `MDEMG_MODEL_RAM_TIERS` JSON, detect host RAM (darwin: `sysctl -n hw.memsize`; linux: `/proc/meminfo MemTotal`), match thresholds → quants. Validate resolved quant is in `MDEMG_MODEL_QUANTS`.
6. Tier 1 + Tier 2 unit/integration tests (see §6).
7. Grep audit: `grep -rE 'reh3376|mdemg-llm-v1|Q[0-9]_K_M|Q[0-9]_[0-9]|qwen3:|\.ollama|\.mdemg/models' internal/cli/model*.go` must return zero matches in non-test code (test fixtures may use these strings).

### Epic 5 — TSDB observability (0.5 day)

1. New `internal/tsdb/migrations/021_model_install_events.sql`: hypertable on `recorded_at`, 7-day chunks. Columns: `event_id CUIDv2 PK`, `recorded_at TIMESTAMPTZ`, `event_type TEXT` (pull/verify/remove), `backend_name TEXT`, `namespace TEXT`, `model_name TEXT`, `quant TEXT`, `adapter BOOL`, `success BOOL`, `latency_ms INT`, `sha256 TEXT`, `size_bytes BIGINT`, `err_message TEXT`.
2. Bump `TSDB_REQUIRED_SCHEMA_VERSION` 20→21 in `internal/config/config.go`.
3. New `internal/tsdb/model_install_writer.go` mirroring `internal/tsdb/sparse_gate_writer.go` (buffered + 30s flush via CopyFrom).
4. Wire `modelInstallRecorderAdapter` in `internal/api/server.go::SetTSDBClient` (mirror sparse-gate pattern).
5. `mdemg model pull|verify|remove` calls `writer.RecordEvent(...)` async after each operation.
6. Grafana panels: deferred to Sprint B.

### Epic 6 — Tier 3 live e2e (0.5 day)

After Epic 3 publishes to ollama.com:

1. `mdemg model pull --quant Q5_K_M` on the operator's machine. Verify: symlink at `<MDEMG_MODEL_DIR>/mdemg-llm-v1.Q5_K_M.gguf` resolves to a real Ollama blob; SHA matches manifest; TSDB row in `model_install_events`.
2. Run `llama-server --model <symlink-target> --port 18102 --ctx-size 32768 --jinja` (port 18102 to avoid disrupting operator's currently-serving llama-server on 8102); curl `http://127.0.0.1:18102/v1/models`; run a sanity inference.
3. Adapter path: `mdemg model pull --adapter`; pull fetches the `<NAMESPACE>/<NAME>-adapter:latest` tag; verify ollama bootstraps qwen3:14b base + adapter via Modelfile `ADAPTER` directive; sanity inference outputs.
4. Configurability proof: `MDEMG_MODEL_NAMESPACE=acme MDEMG_MODEL_QUANT=Q4_K_M mdemg model pull --dry-run` resolves to `acme/mdemg-llm-v1:Q4_K_M`, exits without side effects.
5. Document smoke in PR comment.

### Epic 7 — Resource-requirements feature doc (0.5 day)

`docs/features/local-model-distribution.md`:

- **Why**: link to Phase 13.5 cutover; the gap between `brew install mdemg` and a working LLM endpoint.
- **Choices**: Ollama Library (vs HF, vs S3, vs GitHub Releases); fused vs adapter; per-quant matrix.
- **How it works**: ModelFetcher interface; OllamaFetcher blob-discovery walkthrough; symlink semantics.
- **How to use**: `mdemg model pull` with no flags; quant selection; Configurability Contract enumerated in full (one operator-readable table).
- **Resource matrix**:

| Quant | Disk | Min RAM | Recommended RAM | Notes |
|---|---|---|---|---|
| Q4_K_M | ~6.5 GB | 8 GB | 16 GB | Lowest fidelity; UVTS regression vs Q5_K_M empirically TBD |
| Q5_K_M | 9.8 GB | 12 GB | 24 GB | Production canonical (Phase 13.5) |
| Q8_0 | ~14.6 GB | 16 GB | 32 GB | Highest fidelity; ~50% bigger for marginal quality gain |

- Apple Silicon scope; troubleshooting (ollama not installed, pull fails, SHA mismatch, out-of-disk).

### Epic 8 — Documentation Update (0.5 day, never cut)

- `CHANGELOG.md` — promote Unreleased → v0.10.0 entry.
- `CLAUDE.md` — new "Model Distribution" section under Architecture Notes.
- `packaging/homebrew-mdemg/README.md` — Quick Start update + What's New v0.10.0.
- `packaging/homebrew-mdemg/CHANGELOG.md` — v0.10.0 entry.
- `packaging/homebrew-mdemg/mdemg.rb` — caveats mention `mdemg model pull` post-install.
- `README.md` (main repo) — Quick Start update.
- `docs/development/model-dist-001/post.md` — sprint close (sections: process, findings, current state, testing & benchmarking, risks & opportunities).

## 6. Testing Plan (3 tiers — required by memory rule)

### Tier 1 — Unit tests

`internal/cli/model_test.go`:
- Quant resolution: validates against `MDEMG_MODEL_QUANTS`; rejects unknown quants; accepts default + custom env-set quant lists.
- RAM-tier auto-detection: parses `MDEMG_MODEL_RAM_TIERS` JSON, mocks `sysctl`/`/proc/meminfo`, asserts correct quant pick across thresholds (default + operator-overridden tier maps).
- Backend factory dispatch: `MDEMG_MODEL_BACKEND=ollama` → `OllamaFetcher`; unknown backend → error.
- Namespace/name composition: builds correct registry tag from `<MDEMG_MODEL_NAMESPACE>/<MDEMG_MODEL_NAME>:<quant>`.
- Manifest-path override: `MDEMG_MODEL_MANIFEST_PATH` external file vs embed.FS fallback.

`internal/cli/model_fetcher_ollama_test.go`:
- Ollama manifest JSON parser (digest extraction, mediaType filtering, malformed-JSON handling).
- Blob path composition under `OLLAMA_MODELS` env override.
- Symlink creation idempotency under `MDEMG_MODEL_DIR` override.

### Tier 2 — Integration tests

`internal/cli/model_integration_test.go` (`-tags=integration`):
- Mock `~/.ollama/models/` filesystem with synthetic manifest + blob.
- Run `mdemg model pull --dry-run` against the mock; assert symlink target.
- TSDB writer integration test against testcontainer Postgres.

### Tier 3 — Live e2e

Per Epic 6 above: real `mdemg model pull` against the actually-published Ollama tags, real `llama-server`, real TSDB row.

## 7. Commit Strategy

Sequential commits per epic on `reh3376_dev01`. Each epic = one commit with conventional-commit message. Epic 3 (Ollama publish) produces in-repo diff only via `quant_manifest.json` updates; the publish itself is a side-effect noted in the commit body. Sprint completion = final commit promoting CHANGELOG Unreleased → v0.10.0.

## 8. Verification Checklist

- [ ] `mdemg model pull` with no flags succeeds via defaults (Ollama backend, `reh3376` namespace, RAM-auto quant).
- [ ] `mdemg model pull --backend ollama --namespace acme --name custom-model --quant Q5_K_M` succeeds (proves nothing is hardcoded — every value flows through config).
- [ ] `MDEMG_MODEL_NAMESPACE=acme MDEMG_MODEL_QUANT=Q4_K_M mdemg model pull` produces equivalent result to the flag-set version.
- [ ] `MDEMG_MODEL_RAM_TIERS='{"default":"Q8_0"}' mdemg model pull` ignores host RAM and pulls Q8_0.
- [ ] `mdemg model pull --adapter` succeeds; adapter loads against ollama qwen3:14b base (or graceful Epic 2 deferral with documented exit decision).
- [ ] Grep audit on `internal/cli/model*.go`: zero string literals matching `reh3376`, `mdemg-llm-v1`, `Q[0-9]_K_M`, `Q[0-9]_[0-9]`, `qwen3:`, `~/.ollama`, `~/.mdemg/models`. All such values come from config.
- [ ] Tier 3 live smoke: llama-server serves the pulled model, /v1/models OK, inference returns expected.
- [ ] TSDB row written per pull; `mdemg model verify` returns 0.
- [ ] All 3 tiers of tests pass on local macOS + Linux CI.
- [ ] `golangci-lint run ./internal/cli/... ./internal/tsdb/...` clean.
- [ ] CI plist sync-check passes (regression-free).
- [ ] CHANGELOG, CLAUDE.md, README, feature doc all updated. Feature doc enumerates every env var + flag in the Configurability Contract.
- [ ] Live smoke documented in PR comment per memory rule.

## 9. Documentation Update — Epic 8

Covered in §5 Epic 8 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| MLX → PEFT → GGUF LoRA conversion blocked by tooling gaps | Medium | High | Epic 2 forensic surfaces the path. Contingency: ship fused-only this sprint; adapter becomes MODEL-DIST-002. |
| Ollama Library has stated/unstated rate limits or quota at 30+ GB across 4 tags | Low | Medium | Stage push during Epic 3 (one tag at a time; verify quota); GitHub Release fallback documented (chunked) but not built. |
| Operator's hardware where Ollama runtime is broken (M5 + macOS 26.3.x) but Ollama is needed for distribution | Already true | Medium | Doc + caveats explicitly call out "Ollama is distribution-only; runtime is llama.cpp llama-server." Ollama is the registry client, never invoked for inference. |
| Adapter Modelfile pins `qwen3:14b` and Ollama drops/renames the upstream tag | Low | High | Pin exact manifest digest from Epic 0 forensic (`FROM qwen3:14b@sha256:<digest>`); refresh on each release. |
| Pulling 30+ GB across 3 quants overwhelms operators' disk | Medium | Low | Default `mdemg model pull` picks ONE quant via RAM auto-detection; operators must opt-in to multi-quant install. |
| TSDB schema bump 20→21 collides with another in-flight sprint | Low | Medium | Verify on `main` HEAD before allocating V0021 number; bump to next free. |
| `~/.ollama/models/manifests/...` JSON layout changes between Ollama versions | Low | High | Pin tested Ollama version range in feature doc; defensive parsing with explicit mediaType filtering; fall back to globbing blob digests by file size if manifest layout drifts. |
| Operator publishes wrong tag on first push (no draft/private mode in Ollama Library) | Low | Medium | Epic 3 dry-run via `ollama create` (creates locally only); verify with `ollama list`; only `ollama push` after explicit confirmation. |
| Configurability surface (12+ env vars) overwhelms operators | Medium | Low | Defaults Just Work for common case. Env vars documented in one feature-doc table. CLI `--help` lists flags with env-var equivalents. |

## 11. Documents Accessed

- `docs/research/mdemg_sprint_ideas/MDEMG_FT_LORA_PACKAGING_SPEC.md` (full read; this sprint supersedes §2, §6.1, §7 inference matrix scope)
- `CLAUDE.md` (Phase 13.5 + Phase 11.5e sections)
- `internal/config/config.go` (TSDB_REQUIRED_SCHEMA_VERSION pattern)
- `internal/cli/root.go` (Cobra command registration)
- `internal/cli/service_darwin.go` (`resolveMDEMGModelPath` resolved in Phase 13.5 sprint)
- `internal/cli/config_loader.go` (existing config loader pattern)
- `internal/tsdb/migrations/` directory layout (V0001..V0020)
- `internal/tsdb/sparse_gate_writer.go` (writer pattern reused in Epic 5)
- `adapters/tier1/adapters.safetensors` (Phase 5 SFT Iter 2400 best, 514 MB MLX)
- `.local-models/qwen3-14b-mdemg-v1/` (full MLX merged model with chat_template, config, tokenizer; source for f16 GGUF regeneration)
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (production canonical, 9.8 GB, SHA `144ad723101d688f0875a3129336b0cd0fa356ff5266034a3d35db5e6d5f5d54`)
- `https://registry.ollama.ai/v2/library/qwen3/manifests/14b` (qwen3:14b model layer digest `sha256:a8cc1361f3145dc01f6d77c6c82c9116b9ffe3c97b34716fe20418455876c40e`; full manifest digest computed at Epic 3)

## 12. Rollback Procedures

- **Ollama Library**: rollback Epic 3 via ollama.com web UI tag deletion (operator action; no CLI primitive). Operators who pulled retain working local copies until they `ollama rm`.
- **TSDB V0021**: `DROP TABLE model_install_events`. No data loss for any other table.
- **`mdemg model` CLI**: revert `internal/cli/model.go` registration. Users who symlinked into `<MDEMG_MODEL_DIR>` retain working installs.
- **Brew formula caveats**: revert via formula re-publish.
