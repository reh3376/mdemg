---
created: 2026-05-04
updated: 2026-05-04
version: v0.6.0
author: reh3376
status: active
phase: phase 13.5 + phase 13.5-telemetry
---

# Local LLM Runtime (llama.cpp llama-server + GGUF)

## Summary

**Feature**: `local-llm-runtime`
**Summary**: Production local-LLM runtime is `llama.cpp llama-server` (b9000+, OpenAI-compatible) at `127.0.0.1:8102`, serving the production fine-tune as `mdemg-llm-v1.Q5_K_M.gguf` (10 GB). Replaced `mlx_lm.server` on 2026-05-03 per the data-decided bake-off. Architecturally-bounded KV cache eliminates the Metal-OOM crash cycle that plagued the prior runtime; latency p50 dropped 17 s → 3.0 s; UVTS quality at perfect parity. launchd-supervised; rollback path documented; endpoint health events persist to TSDB V0018 (Phase 13.5 telemetry follow-up).

## Vision & Goals

The MDEMG vision treats the local LLM as the cognitive substrate's vocal cords — every cognition cycle (RSIC reflection, Jiminy guidance, consulting classification, retrieval rerank) emits one or more LLM calls, and a flaky vocal cord degrades cognition itself. The pre-13.5 runtime (`mlx_lm.server`) had two unfixable structural issues:

1. **Officially "not recommended for production"** by its own maintainers (mlx-lm README disclaimer)
2. **Unbounded KV cache** → Metal OOM → SIGABRT every ~14 minutes under sustained load (`ape.reflect` ~5800-token prompts triggered it reliably)

Phase 13.5 was triggered by the cumulative cost: hundreds of crashes per day, retry storms masking which calls actually failed, and the Phase 11.6.3 watchdog firing constantly. The cognitive-substrate framing demanded a runtime that's structurally suited to always-on production, not a research notebook server.

Phase 13.5's bake-off compared four candidates (llama.cpp F1, MLC-LLM F2, Ollama, LM Studio) on the same fine-tuned weights converted to each runtime's preferred format. The decision rule was data-driven, framed against the vision, not against acute crash counts (per `feedback_no_short_term_mlx_patches.md`).

**Result**: llama.cpp won by a clear margin. **0 crashes / 160 min / 301 calls** in stress test. p50 latency 17 s → 3.0 s (5.6× faster). UVTS quality at perfect parity (0.396 = 0.396). HTTP-alive on OOM (graceful HTTP 500 instead of SIGABRT) — meaning the watchdog and retry layer can actually do their job.

## Current State

### Architecture

| Component | Path / Identity | Role |
|---|---|---|
| LLM server | `llama-server` (Homebrew `llama.cpp`, b9000+) at `127.0.0.1:8102` | OpenAI-compat HTTP endpoint that mdemg's 16 LLM call sites use |
| Production model | `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (10.5 GB) | Phase 5 dense base + Phase 11.5e production fine-tune; Q5_K_M quant from MLX-baked safetensors |
| launchd plist | `~/Library/LaunchAgents/com.mdemg.llama-server.plist` | KeepAlive on crash, ThrottleInterval=30 s |
| Health writer | `internal/tsdb/llm_endpoint_health_writer.go` | V0018 hypertable; one row per state transition + per fast-fail burst |
| Watchdog | `internal/mlxprobe/` | See `mlx-watchdog.md`. Probes the llama-server endpoint at the same interval the prior runtime used |
| Conversion pipeline | (one-shot, archived) | MLX safetensors → bf16 HF → f16 GGUF → Q5_K_M |

The legacy `~/Library/LaunchAgents/com.mdemg.mlx-server.plist` was renamed to `.disabled-phase13_5` (kept for emergency rollback, not active).

### Production config

```bash
llama-server \
  --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf \
  --port 8102 \
  --host 127.0.0.1 \
  --ctx-size 32768 \
  --parallel 4 \
  --cont-batching \
  --metrics \
  --jinja
```

KV cache bound: 32768 / 4 = 8 K per slot. Production `ape.reflect` prompts are ~5800 tokens so this fits comfortably. The `--parallel 4` slot count means 4 concurrent calls share the model without copy overhead.

### Workflow

1. `mdemg start` runs `internal/cli/preflight_mlx.go` which probes `LLM_ENDPOINT` synchronously
2. If probe fails AND `MDEMG_ALLOW_NO_MLX != "1"`, mdemg refuses to start (always-on policy)
3. After successful preflight, the watchdog goroutine starts probing `/v1/models` every 5 s (see `mlx-watchdog.md`)
4. LLM calls flow through `internal/llmclient` → llama-server's `/v1/chat/completions`
5. State transitions + fast-fail bursts persist to V0018 via the writer
6. Grafana "LLM Endpoint Health" panel reads V0018 for historical crash-rate + recovery-time charts

### Configuration

| Env Var | Default | Description |
|---|---|---|
| `LLM_ENDPOINT` | `http://127.0.0.1:8102/v1` | The OpenAI-compat endpoint. **Phase 13.5 default** (was `8101` for mlx-server) |
| `LLM_MODEL` | `mdemg-llm-v1` | The model name advertised to callers (matches `--model` filename's logical name) |
| `LLM_MAX_TOKENS` | `4096` | Ceiling on completion length per call; per-task overrides exist |
| `MLX_MERGED_MODEL_PATH` | (path) | MLX form of the model — research/training only; production callers use the GGUF via the endpoint |
| `MDEMG_ALLOW_NO_MLX` | unset | When `1`, mdemg startup proceeds even if endpoint unreachable. Naming is historical; honored across runtimes |

### V0018 telemetry (Phase 13.5 follow-up)

`llm_endpoint_health_events` hypertable rows:

| Column | Notes |
|---|---|
| `event_id` | CUIDv2 |
| `recorded_at` | TS |
| `endpoint_url` | `http://127.0.0.1:8102/v1` |
| `event_kind` | `state_transition` / `fast_fail_burst` / `probe_recovery` |
| `from_state` / `to_state` | `up` / `degraded` / `down` |
| `error_message` | Last probe error (truncated 500 chars) |
| `consecutive_failures` | Probe failure counter at event time |
| `burst_short_circuits` | For fast-fail bursts: how many calls were short-circuited in this window |
| `state_numeric` | 0=up, 1=degraded, 2=down (fast Grafana queries) |

Drives the Fine-Tuning Pipeline dashboard's "LLM Endpoint Health" row.

## Choices that were made

### Why llama.cpp over the alternatives

| Candidate | Decision | Why |
|---|---|---|
| **llama.cpp / llama-server** ✅ | **Production winner** | Architecturally-bounded KV cache (`--ctx-size × --parallel`); HTTP-alive on OOM; mature open-source ecosystem; OpenAI-compat surface; `--jinja` chat-template support |
| MLC-LLM | Disqualified | F2 was 1.6× slower than F1 on every percentile in bake-off. Slight UVTS regression (-0.006). TVM `.dylib` format is hardware-target-locked (rebuild per CPU/GPU change). Smaller community |
| Ollama | Disqualified | Definitively broken on M5 + macOS 26.3.x across 0.20.5–0.22.1 (8+ open issues, matmul2d static_assert). The runtime that should have been the easy answer wasn't |
| LM Studio | Disqualified | Closed-source operability risk for a cognitive-substrate framework |
| `mlx_lm.server` (status quo) | Disqualified | Maintainer disclaimer is structural; `--max-kv-size` for the server is an open mlx-lm issue (Nov 2025, #615); not in 0.31.3. Patching it would not address the structural issue |

### Why GGUF Q5_K_M (not Q4_K_M or Q6_K)

Q5_K_M is the sweet spot on the size-vs-quality curve for this model. The conversion pipeline (MLX safetensors → bf16 HF → f16 GGUF → Q5_K_M via `llama-quantize`) takes ~5 min on M5 Max. UVTS regression at Q5_K_M was zero (parity with the MLX-served form). Q4_K_M would save 2 GB but the bake-off didn't justify the additional quality risk. Q6_K would save zero meaningful operational cost vs Q5_K_M.

### Why `--parallel 4` (not 1 or 8)

Production sees 1–3 concurrent LLM calls during a typical RSIC + Jiminy + retrieval rerank flow. `--parallel 4` covers the burst without copy overhead. `--parallel 1` would queue concurrent calls (unacceptable for interactive flow). `--parallel 8` would waste KV cache memory on slots that rarely fire.

### Why launchd (not systemd / docker-compose)

Same reason as the watchdog — macOS-native operators are MDEMG's primary target (Apple Silicon for the local LLM). KeepAlive on crash + ThrottleInterval=30 s gives recovery semantics without a Docker dependency. The Linux/Windows launchers are scoped for future sprints.

### Why the watchdog name was kept ("MLX")

See `mlx-watchdog.md` notes — Phase 13.6 batches the rename of `MLX_*` env vars + package `mlxprobe` along with other backend-agnostic cleanups. The runtime cutover (Phase 13.5) and the naming sweep were intentionally separated to keep the cutover diff small.

### Why V0018 telemetry as a follow-up (not in 13.5 itself)

Phase 13.5 was the cutover; V0018 was the observability layer that benefits from the cutover but isn't required for it. Splitting them kept the cutover commit reviewable. The follow-up sprint (Phase 13.5-telemetry) was small (~80 LOC of writer + adapter + 60 LOC migration).

## Notes

### Known limitations

- **Operator-installed**: llama.cpp must be installed via Homebrew (`brew install llama.cpp`). The plist references the binary path; if Homebrew rotates the path, the plist needs an update. mdemg doesn't bundle llama-server.
- **Single-architecture coverage**: macOS / Apple Silicon only. Linux operators currently have to install llama.cpp + adjust the plist equivalent themselves. Native multi-arch support is queued.
- **`--jinja` for chat templates**: Without it, the OpenAI-compat layer falls back to a generic template that doesn't match the fine-tune's training distribution. Always set `--jinja` for this model.

### Risks & gaps

- **Q5_K_M sensitivity to llama.cpp updates**: future llama.cpp versions could change quantization handling subtly. Pin `b9000` or higher and re-validate UVTS quality on llama.cpp upgrades.
- **GGUF model format isn't training-format**: the GGUF cannot be fine-tuned in place — fine-tuning happens on the MLX safetensors form, then re-converts. Operators expecting "just modify weights" need to understand the conversion pipeline.

### Future improvements

- Backend-agnostic naming (Phase 13.6, queued)
- Linux/Windows native plist equivalents
- Predictive Apple-Silicon-GPU-memory metrics

## API Endpoints

The runtime itself exposes the OpenAI-compat surface (`/v1/chat/completions`, `/v1/models`, `/metrics`). MDEMG doesn't expose new endpoints; it consumes them.

| Method | Endpoint | Description |
|---|---|---|
| GET | `/healthz` | mdemg's own health check; `checks.llm_endpoint` reflects watchdog state |

## CLI Commands

| Command | Description |
|---|---|
| `mdemg watchdog status [--json]` | See `mlx-watchdog.md`. Reports the state of this runtime via the watchdog |
| `mdemg service install --with-mlx` | Installs the launchd plist for llama-server alongside mdemg's own plist (the `--with-mlx` name is historical; backend-agnostic since 13.5) |

## Configuration Reference

See "Configuration" table above. The llama-server flags above are not env-driven — they're set in the launchd plist at install time. To change them, edit `~/Library/LaunchAgents/com.mdemg.llama-server.plist` and `launchctl bootstrap` it.

## Dependencies

| Feature | Relationship |
|---|---|
| `mlx-watchdog` | Probes this runtime; fast-fails when it's `Down` |
| TSDB V0018 (`llm_endpoint_health_events`) | Persistence for state transitions + fast-fail bursts |
| All 16 LLM call sites (RSIC, Jiminy, consulting, retrieval rerank, ape.reflect, embeddings, …) | Consume this endpoint |
| Grafana "LLM Endpoint Health" panel | Reads V0018 |

## Related Files

- `~/Library/LaunchAgents/com.mdemg.llama-server.plist` — supervisor
- `~/Library/LaunchAgents/com.mdemg.mlx-server.plist.disabled-phase13_5` — legacy (kept for rollback)
- `.local-models/mdemg-llm-v1-gguf/` — GGUF directory (production)
- `.local-models/mdemg-llm-v1/` — MLX safetensors form (research/training)
- `internal/llmclient/client.go` — call layer
- `internal/cli/preflight_mlx.go` — startup probe
- `internal/tsdb/llm_endpoint_health_writer.go` — V0018 writer
- `internal/tsdb/migrations/018_llm_endpoint_health.sql` — V0018 schema
- `docs/development/post-ft-lora/phase_13_5_bakeoff_results.md` — bake-off data
- `docs/development/post-ft-lora/phase_13_5_post.md` — cutover sprint
- `docs/development/post-ft-lora/phase_13_5_telemetry_post.md` — V0018 follow-up
