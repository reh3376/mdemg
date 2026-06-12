# Neural Sidecar API Contract v1

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Status**: Active
**Service**: `neural-sidecar` (FastAPI, Python 3.10+)
**Default port**: 8100
**Config prefix**: `NEURAL_` (env vars)

---

## API Version

All responses include the header `X-Sidecar-API-Version: v1`.

Clients should send this header on requests. The sidecar accepts requests without the header but logs a warning.

---

## Endpoints

### POST /protocol/predict-tier

Predict the optimal J17 encoding tier for a constraint given agent context and trust.

**Request**

```json
{
  "constraint_text": "string (required)",
  "agent_context": "string (required)",
  "trust_score": 0.5
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `constraint_text` | string | Non-empty. The constraint description to encode. |
| `agent_context` | string | Non-empty. Current agent context (task summary, recent actions). |
| `trust_score` | float | Range [0.0, 1.0]. Per-session trust score from `TrustScorer`. |

**Response (200)**

```json
{
  "predicted_tier": 1,
  "confidence": 0.82,
  "model": "protocol-tier/v3",
  "latency_ms": 14.2
}
```

| Field | Type | Description |
|-------|------|-------------|
| `predicted_tier` | int | 1 (coded), 2 (telegraphic), or 3 (full NL). 0 = model unavailable, use rule-based fallback. |
| `confidence` | float | [0.0, 1.0]. Distance from nearest tier boundary, normalized. |
| `model` | string | Model identifier (path or HuggingFace name). `"none"` when no model loaded. |
| `latency_ms` | float | Inference wall-clock time in milliseconds. |

**Error responses**: 422 if required fields missing or `trust_score` out of range.

---

### POST /nli

Classify the Natural Language Inference relationship between a premise and hypothesis.

**Request**

```json
{
  "premise": "string (required)",
  "hypothesis": "string (required)"
}
```

| Field | Type | Constraints |
|-------|------|-------------|
| `premise` | string | Non-empty. The ground-truth statement (e.g., constraint text). |
| `hypothesis` | string | Non-empty. The statement to classify against the premise (e.g., agent action summary). |

**Response (200)**

```json
{
  "label": "entailment",
  "scores": {
    "entailment": 0.92,
    "contradiction": 0.05,
    "neutral": 0.03
  },
  "model": "cross-encoder/nli-deberta-v3-xsmall",
  "latency_ms": 8.7
}
```

| Field | Type | Description |
|-------|------|-------------|
| `label` | string | `"entailment"`, `"contradiction"`, or `"neutral"`. Highest-scoring class. |
| `scores.entailment` | float | [0.0, 1.0]. Softmax probability for entailment. |
| `scores.contradiction` | float | [0.0, 1.0]. Softmax probability for contradiction. |
| `scores.neutral` | float | [0.0, 1.0]. Softmax probability for neutral. |
| `model` | string | NLI model identifier. |
| `latency_ms` | float | Inference wall-clock time in milliseconds. |

**Error responses**: 422 if `premise` or `hypothesis` missing.

---

### GET /health

Health check. Reports loaded models and recency of inference.

**Response (200)**

```json
{
  "status": "ok",
  "models_loaded": [
    "cross-encoder/ms-marco-MiniLM-L-6-v2",
    "cross-encoder/nli-deberta-v3-xsmall",
    "protocol-tier/v3"
  ],
  "last_inference_ms": 14.2
}
```

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `"ok"` if at least one model loaded, `"loading"` otherwise. |
| `models_loaded` | string[] | Names of all successfully loaded models. |
| `last_inference_ms` | float or null | Wall-clock time of most recent inference across all models. `null` if no inference yet. |

---

### GET /version

Version metadata for the sidecar process.

**Response (200)**

```json
{
  "version": "0.1.0",
  "api_version": "v1",
  "python_version": "3.10.12",
  "models": {
    "rerank": "cross-encoder/ms-marco-MiniLM-L-6-v2",
    "nli": "cross-encoder/nli-deberta-v3-xsmall",
    "tier": "protocol-tier/v3"
  }
}
```

---

## Error Codes

| Status | Condition | Body |
|--------|-----------|------|
| 400 | Malformed JSON, unparseable request body | `{"detail": "..."}` |
| 422 | Validation failure (missing required field, value out of range) | `{"detail": [{"loc": [...], "msg": "...", "type": "..."}]}` |
| 500 | Model inference error (OOM, tensor shape mismatch, etc.) | `{"detail": "Internal model error"}` |
| 503 | Models still loading (startup not complete) | `{"detail": "Models loading"}` |

All error responses use standard FastAPI/Pydantic error format.

---

## Retry Policy

**No client-side retries.** The Go caller (`NLIComprehensionScorer`, shadow tier predictor) wraps sidecar calls with a circuit breaker at the MDEMG server level. Sidecar errors fall back to heuristic scoring (NLI) or rule-based tier selection (predict-tier). Adding client retries would double latency on transient failures without improving outcomes, since the fallback path is always available.

Circuit breaker parameters are controlled by `J17_SIDECAR_TIMEOUT_MS` (default: 200ms). A timeout or 5xx response trips the breaker; subsequent calls skip the sidecar entirely until the next health check succeeds.

---

## Latency SLO

| Endpoint | p50 Target | p99 Target |
|----------|-----------|-----------|
| `/protocol/predict-tier` | < 50ms | < 200ms |
| `/nli` | < 50ms | < 200ms |
| `/health` | < 5ms | < 10ms |
| `/version` | < 1ms | < 5ms |

Measured at the sidecar process boundary (excludes network round-trip from Go caller). The 200ms p99 target aligns with `J17_SIDECAR_TIMEOUT_MS` default -- requests exceeding this are killed by the Go client and treated as failures.

Latency is reported in every inference response via `latency_ms`. The Go caller logs shadow predictions with prefix `j17-shadow:` including observed latency for offline analysis.

---

## Backward Compatibility Rules

1. **Additive changes only.** New fields may be added to response bodies. New optional fields may be added to request bodies. Clients must tolerate unknown response fields.

2. **Never remove fields.** Once a field appears in a versioned response schema, it is permanent for that API version.

3. **Never change field types.** A field that returns `float` will always return `float`.

4. **Never change field semantics.** `predicted_tier: 1` always means T1 coded. `label: "entailment"` always means entailment.

5. **Breaking changes require a new API version.** If a field must be removed or its type/semantics changed, introduce `/v2/` endpoints and deprecate `/v1/` with a minimum 90-day overlap.

6. **The `predicted_tier: 0` sentinel is permanent.** It means "no model available, use fallback." Callers must handle this case for the lifetime of v1.

---

## Configuration Reference

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `NEURAL_HOST` | `0.0.0.0` | Bind address |
| `NEURAL_PORT` | `8100` | Listen port |
| `NEURAL_RERANK_MODEL` | `cross-encoder/ms-marco-MiniLM-L-6-v2` | Cross-encoder for `/rerank` |
| `NEURAL_NLI_MODEL` | `cross-encoder/nli-deberta-v3-xsmall` | NLI model for `/nli` |
| `NEURAL_TIER_MODEL` | `""` (disabled) | Tier prediction model path for `/protocol/predict-tier`. Empty = endpoint returns fallback. |
| `NEURAL_DEVICE` | `cpu` | PyTorch device (`cpu`, `cuda`, `mps`) |
| `NEURAL_LOG_LEVEL` | `info` | Logging level |

Go caller config (MDEMG server side):

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `J17_SIDECAR_URL` | `""` | Sidecar base URL (e.g., `http://localhost:8100`). Empty = sidecar disabled. |
| `J17_SIDECAR_TIMEOUT_MS` | `200` | HTTP client timeout for sidecar calls. |
