# Neural Sidecar — Operator Guide

> First operator doc for the sidecar (DOC-CURRENCY-002 disclosed gap; written
> 2026-07-22, every value verified against code + the live stack).

The neural sidecar is a small FastAPI service (`neural/neural_sidecar/`)
hosting three CPU cross-encoder models the Go server calls over localhost
HTTP. It is **not** the LLM (that's llama-server on :8102) — it serves
sub-second scoring primitives:

| Endpoint | Model (default) | Consumer |
|---|---|---|
| `POST /rerank` | `cross-encoder/ms-marco-MiniLM-L-6-v2` | Retrieval rerank when `RERANK_PROVIDER=neural` (~20× faster than the LLM path; quality parity per NEURAL-RERANK-QUALITY-AB-001 — default stays `openai`) |
| `POST /nli` | `cross-encoder/nli-deberta-v3-xsmall` | J17 comprehension scoring (`J17_NLI_*` family) |
| `POST /protocol/predict-tier` | disabled unless `NEURAL_TIER_MODEL` set | J17 tier-prediction shadow client |
| `GET /health` | — | probes; reports `models_loaded` + `last_inference_ms` |

## Two run modes (know which one you're on)

1. **Docker (primary)** — compose service `neural-sidecar`, container port
   8000, host-mapped to **8100**. In-compose the server reaches it at
   `http://neural-sidecar:8000`; a native server binary uses
   `http://localhost:8100`. This is what the dev box's `.env`
   (`J17_SIDECAR_URL=http://localhost:8100`) and the `NEURAL_RERANK_URL`
   default point at.
2. **Native launchd** — `com.mdemg.neural-sidecar` runs
   `uvicorn neural_sidecar.app:app` on **8101** for dockerless setups
   (plist template in `packaging/launchd/`).

⚠️ On a hybrid box running BOTH (this dev machine), all traffic goes to the
Docker instance on :8100 — verify with `last_inference_ms` in each
`/health`: the serving instance shows a number, the idle one `null`. The
idle native instance still holds both models in RAM; `launchctl bootout
gui/$UID/com.mdemg.neural-sidecar` reclaims it (re-`bootstrap` to undo).
Note the two modes may also load DIFFERENT NLI models (the Docker image
pins its own; native uses the `config.py` default) — a reason to keep
comparisons single-instance.

## Configuration (pydantic-settings, `env_prefix="NEURAL_"`)

The sidecar's own env vars are the `Settings` fields in
`neural/neural_sidecar/config.py` with the `NEURAL_` prefix — they never
appear literally in code, so grep for the field names:

| Env var | Default | Field |
|---|---|---|
| `NEURAL_HOST` | `127.0.0.1` | bind host (loopback by design — HOOKSYNC-001) |
| `NEURAL_PORT` | `8100` | bind port (the launchd plist overrides via `--port`) |
| `NEURAL_RERANK_MODEL` | `cross-encoder/ms-marco-MiniLM-L-6-v2` | rerank cross-encoder |
| `NEURAL_NLI_MODEL` | `cross-encoder/nli-deberta-v3-xsmall` | NLI cross-encoder |
| `NEURAL_TIER_MODEL` | `""` (disabled) | tier-prediction model path/HF name |
| `NEURAL_DEVICE` | `cpu` | torch device |
| `NEURAL_LOG_LEVEL` | `info` | uvicorn log level |

Server-side knobs that decide whether/when the sidecar is called:
`RERANK_PROVIDER` + `NEURAL_RERANK_ENABLED` + `NEURAL_RERANK_URL` +
`NEURAL_RERANK_MIN_BUDGET_MS` (pre-check, 1500); `J17_SIDECAR_URL` /
`J17_SIDECAR_TIMEOUT_MS` (1000, floor 100 — DH-004) / `J17_SIDECAR_MODE` /
`J17_SIDECAR_CONFIDENCE_FLOOR` / circuit-breaker pair
`J17_SIDECAR_CB_FAILURE_THRESHOLD`+`J17_SIDECAR_CB_TIMEOUT_SEC`;
`J17_NLI_COMPREHENSION_ENABLED` / `J17_NLI_OBSERVATIONAL_ENABLED`.

## Operating it

```bash
# Health (serving instance shows last_inference_ms as a number)
curl -s http://127.0.0.1:8100/health | jq

# Docker mode lifecycle
docker compose up -d neural-sidecar
docker compose logs -f neural-sidecar

# CLI helpers (agent/sidecar management)
mdemg sidecar status        # --format json for scripts
mdemg sidecar up|down|restart

# Real NLI-call observability (DASHBOARD-TRUTH-001):
# mdemg_j17_nli_requests_total / _latency_ms — the older
# mdemg_j17_sidecar_requests counts ONLY the dormant tier-prediction
# shadow client, not NLI calls.
curl -s http://localhost:9999/v1/metrics/snapshot | \
  jq '.data | with_entries(select(.key | test("j17_nli")))'
```

Behavior when the sidecar is down: all consumers fail open — rerank falls
back to the configured provider chain, NLI comprehension falls back to the
heuristic stream (`GetNLICalibrationReport()` returns nil when
non-operational, so no phantom bias metrics — ALERT-TRUTH-001), and the
J17 circuit breaker stops hammering after the threshold.

## Troubleshooting

- **Timeouts inflating NLI bias**: `J17_SIDECAR_TIMEOUT_MS` below ~1000 on
  cold models reproduces the DH-004 56%-timeout pathology; keep ≥1000.
- **"0 sidecar requests" red herring**: read `mdemg_j17_nli_requests_total`,
  not `mdemg_j17_sidecar_requests` (tier-shadow only).
- **Port confusion on hybrid boxes**: `lsof -nP -iTCP:8100 -sTCP:LISTEN`
  (expect Docker) vs `:8101` (native launchd). `.env` decides which one the
  server actually uses.
- **Model downloads**: first boot pulls from Hugging Face into the
  container/venv cache; air-gapped installs must pre-seed the cache or set
  `NEURAL_*_MODEL` to local paths.
