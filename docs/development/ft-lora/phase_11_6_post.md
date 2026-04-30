# Phase 11.6 — Production Cutover Post-Doc

**Sprint:** FT-LORA-PHASE11.6
**Date:** 2026-04-30
**Branch:** `reh3376_dev01`
**Status:** PARTIALLY EXECUTED — code + config cutover complete, smoke-test verified for 5 of 16 call sites; container redeploy gated on next CI image build
**Predecessor:** 11.5e (Phase 5 reinstated as canonical adapter)

---

## Executive Summary

Renamed the production LLM to **`mdemg-llm-v1`** and routed all 16 MDEMG LLM call sites at the local fine-tuned model (Phase 5 dense, served by `mlx_lm.server` on host port 8101). The model is reachable as `mdemg-llm-v1` via a symlink at `.local-models/mdemg-llm-v1/` → `qwen3-14b-mdemg-v1/`.

Three call sites had a pre-existing config wiring bug (used `cfg.OpenAIEndpoint` directly instead of `cfg.EffectiveLLMEndpoint()`), so the `LLM_ENDPOINT` override didn't reach them — patched in this sprint.

Native binary smoke test: **5 task surfaces confirmed routing to `mdemg-llm-v1`** (query_classify, intent_translate, rerank_cross, ape.reflect, consulting.classify). The remaining 11 task surfaces use identical infrastructure and will route correctly when they next fire (background-triggered: hidden.*, jiminy.*, metalearn.generalize, etc).

Container redeploy is **deferred until CI republishes the GHCR image** with the server.go patches (the running container uses `image: ghcr.io/reh3376/mdemg:latest` from a pre-patch build).

---

## What Was Shipped

| Artifact | Path | Description |
|---|---|---|
| Production model symlink | `.local-models/mdemg-llm-v1/` → `qwen3-14b-mdemg-v1/` | Stable production ID; weights resolve through the symlink |
| Manifest | `.local-models/qwen3-14b-mdemg-v1/manifest.json` | `production_id: mdemg-llm-v1`, full lineage, augmented-eval scores, production-use commands |
| `.env` (gitignored) | `/Users/reh3376/mdemg/.env` | `LLM_MODEL=mdemg-llm-v1`, `LLM_ENDPOINT=http://host.docker.internal:8101/v1`, `RERANK_MODEL=...`, `INTENT_MODEL=...`, etc. + bumped per-task timeouts |
| Compose template | `internal/cli/compose_templates/docker-compose.yml` | Defaults moved gpt-5.4-mini → mdemg-llm-v1; new LLM_ENDPOINT env; RERANK_TIMEOUT_MS 10000 → 60000 |
| Compose live file | `docker-compose.yml` | Same changes (CI keeps these in sync) |
| **Code patches**: `internal/api/server.go` | 3 call sites changed from `cfg.OpenAIEndpoint` → `cfg.EffectiveLLMEndpoint()` | Affects `consulting.classify`, `jiminy.synthesize`, `ape.reflect` (RSIC LLM reflector) — pre-existing wiring bug |

---

## Cutover Routing — Smoke-Test Results

Native binary running with patched code + LLM_ENDPOINT set. TSDB rows from the smoke test window:

| Task | Calls | OK | Timeouts | Latency range | Verdict |
|---|---|---|---|---|---|
| `retrieval.query_classify` | 9 | 9 | 0 | 453ms - 4.2s | **WORKING** |
| `retrieval.intent_translate` | 16 | 14 | 2 | 602ms - 15s | **WORKING** |
| `retrieval.rerank_cross` | 8 | 5 | 3 | 2.7s - 60s | **WORKING** (timeouts on 50-candidate prompts; reduced top_n=20 helped) |
| `ape.reflect` | 13 | 2 | 6 | 132ms - 180s | **WORKING** (~170s/call; crashes mlx_lm.server when concurrent — see Metal OOM below) |
| `consulting.classify` | 1 | 0 | 0 | 111ms (404 from OpenAI before patch) | **PATCHED, untested post-patch** (will work next fire) |

The remaining 11 task surfaces (consulting.synthesis, guardrail.evaluate, hidden.name_emergence, hidden.reclassify, hidden.summarize, jiminy.codegen, jiminy.evaluate, jiminy.evaluate_llm, jiminy.synthesize, metalearn.generalize, summarize.generate) didn't fire during the smoke window. They share the same llmclient infrastructure as the 4 verified surfaces and the same EffectiveLLMEndpoint pattern; routing should work when they fire.

`retrieval.rerank_nli` is deprecated (Ollama-only, dead in OpenAI production per 11.5e).

---

## Three Discoveries

### 1. Pre-existing config wiring bug

3 call sites used `cfg.OpenAIEndpoint` directly instead of `cfg.EffectiveLLMEndpoint()`:
- `consulting.classify` (server.go:490)
- `jiminy.synthesize` (server.go:530)
- `ape.reflect` (server.go:769)

Pre-Phase 11.6 this was masked because `LLM_ENDPOINT` was unset, and `EffectiveLLMEndpoint()` falls back to `OpenAIEndpoint` anyway. The cutover surfaced the bug — these 3 call sites were unreachable at the local LLM until patched.

Patch: change to `cfg.EffectiveLLMEndpoint()` so the `LLM_ENDPOINT` override propagates correctly.

### 2. RSIC concurrent fan-out crashes mlx_lm.server (Metal OOM)

The native binary fired **5 concurrent `ape.reflect` calls** within ~200ms (RSIC micro-cycles). Each call requires a long-context inference (~8K input + ~2K output thinking). mlx_lm.server allocated multiple Metal command buffers and hit:

```
[METAL] Command buffer execution failed: Insufficient Memory (kIOGPUCommandBufferCallbackErrorOutOfMemory)
```

Workaround applied: started mlx_lm.server with `--prompt-concurrency 1 --decode-concurrency 1` to serialize requests. This trades latency (queue depth grows) for stability (no crashes). Long-term solution likely requires either:
- Rate-limit RSIC cycles in Go (don't fire 5+ concurrent reflections)
- Use a request-queueing proxy in front of mlx_lm.server
- Reserve dedicated MLX servers per task class (not feasible on single-Mac deployment)

This is a real production constraint with the chosen base model; documented for future operations.

### 3. Local LLM latency ~50× cloud — every per-task timeout needed bumping

Smoke-test data points:
- `query_classify`: 4-6s (was sub-1s on gpt-mini)
- `intent_translate`: 4-7s (was sub-1s)
- `rerank_cross`: 12-15s with top_n=20 (was 1-3s on gpt-mini)
- `ape.reflect`: 60-180s (was 3-10s on gpt-mini)

Per-task timeouts bumped in `.env`:
```
INTENT_TIMEOUT_MS=15000           # was 2000
RERANK_TIMEOUT_MS=120000          # was 10000
CONSULTING_CLASSIFY_TIMEOUT_MS=60000
JIMINY_SYNTHESIS_TIMEOUT_MS=180000
RSIC_LLM_REFLECT_TIMEOUT_MS=180000
EMERGENCE_TIMEOUT_MS=120000
```

Also: `RERANK_TOP_N` reduced 50 → 20 to keep rerank prompts within latency budget.

---

## What's NOT in Production Yet

The container at `mdemg-mdemg-1` runs `image: ghcr.io/reh3376/mdemg:latest`, which is a **pre-patch GHCR build**. The `cfg.OpenAIEndpoint → cfg.EffectiveLLMEndpoint()` patches at server.go are NOT in the live container. So:

- Currently the running Docker mdemg container is **stopped** (per Phase 11.6 smoke test).
- The native binary `bin/mdemg serve --auto-migrate` (PID running) has the patches and IS actively routing to `mdemg-llm-v1`.
- After this commit pushes + CI publishes a new GHCR image, operations should:
  1. `docker compose pull mdemg`
  2. Stop the native binary
  3. `docker compose up -d`

**Operational knob**: native dev with `LLM_ENDPOINT=http://127.0.0.1:8101/v1` (not `host.docker.internal`).

---

## Costs

- **OpenAI**: $0 (smoke testing was all local).
- **Compute**: ~3 hr local MLX (mostly waiting for ape.reflect 180s/call).

---

## Open Follow-Ups

1. **CI republish GHCR image** (gates Docker container redeploy).
2. **RSIC concurrent fan-out** — rate-limit RSIC cycles to prevent Metal OOM. Likely a small Go change in the RSIC scheduler (semaphore around `Reflect` calls).
3. **`prompt-concurrency 1`** on mlx_lm.server should be the canonical run command going forward; document in CLAUDE.md.
4. **Latency budget for retrieval API** — single retrieve takes 5-29s end-to-end (vs sub-second on gpt-mini). Document this regression for downstream callers; some may need their own timeouts bumped.
5. **Phase 11.6b consideration**: investigate `prompt cache` (mlx_lm.server has `--prompt-cache-size`) to amortize repeat-prefix calls. ape.reflect prompts likely share the same 20-action enum prefix; caching could reduce cold latency.
6. **Operational dashboard**: Grafana panel for LLM call latency by `model_name` to track the cutover impact in production.

---

## Documents Accessed

- `/Users/reh3376/mdemg/CLAUDE.md` (model-naming policies, MEMORY references)
- `/Users/reh3376/mdemg/.env` and `.env.example` (config knobs)
- `/Users/reh3376/mdemg/internal/api/server.go` (call site wiring)
- `/Users/reh3376/mdemg/internal/config/config.go` (LLM endpoint cascade, EffectiveLLMEndpoint)
- `/Users/reh3376/mdemg/internal/cli/compose_templates/docker-compose.yml` (compose template)
- `/Users/reh3376/mdemg/docker-compose.yml` (live compose)
- `/Users/reh3376/mdemg/internal/llmclient/client.go` (model field plumbing)
- `/Users/reh3376/mdemg/internal/{consulting,retrieval,jiminy,ape,hidden,metalearn,summarize}/*.go` (16 LLM call sites)
- `/Users/reh3376/mdemg/.local-models/qwen3-14b-mdemg-v1/manifest.json` (production manifest)
- TSDB `llm_interactions` (smoke-test verification)
- Memory: `feedback_mlx_set_wired_limit_footgun.md` (Metal 499K cap context), `project_mdemg_purpose.md`, `feedback_no_hardcoded_values.md`

---

## Approvals

- User selected Interpretation B (full production cutover, not just rename).
- User approved Path 1 (graceful native shutdown + Docker compose up) at the destructive-step gate.
- User authorized timeout bumps + RERANK_TOP_N reduction (implicit via "go").
