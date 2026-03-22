"""FastAPI application — POST /rerank, POST /nli, POST /protocol/predict-tier, GET /health."""

from __future__ import annotations

import logging
import time
from contextlib import asynccontextmanager
from typing import AsyncIterator

from fastapi import FastAPI

from .config import settings
from .nli import NLIClassifier
from .reranker import Reranker
from .tier_model import TierModel
from .schemas import (
    HealthResponse,
    NLIRequest,
    NLIResponse,
    NLIScores,
    RerankRequest,
    RerankResponse,
    RerankResult,
    TierPredictRequest,
    TierPredictResponse,
)

logger = logging.getLogger(__name__)

# Global model instances (initialized on startup)
_reranker: Reranker | None = None
_nli: NLIClassifier | None = None
_tier_model: TierModel | None = None


@asynccontextmanager
async def lifespan(_app: FastAPI) -> AsyncIterator[None]:
    """Load models on startup."""
    global _reranker, _nli, _tier_model  # noqa: PLW0603
    logging.basicConfig(level=getattr(logging, settings.log_level.upper(), logging.INFO))
    logger.info("Starting neural sidecar (host=%s, port=%d)", settings.host, settings.port)

    _reranker = Reranker(settings.rerank_model, settings.device)
    _nli = NLIClassifier(settings.nli_model, settings.device)

    # Eagerly load models so first request is fast
    _reranker.load()
    _nli.load()

    # Tier model is optional — only load if configured
    if settings.tier_model:
        _tier_model = TierModel(settings.tier_model, settings.device)
        _tier_model.load()
        logger.info("Tier model loaded: %s", settings.tier_model)
    else:
        logger.info("Tier model not configured — skipping")

    logger.info("All models loaded")

    yield

    logger.info("Shutting down neural sidecar")


app = FastAPI(
    title="MDEMG Neural Sidecar",
    version="0.1.0",
    lifespan=lifespan,
)


@app.post("/rerank", response_model=RerankResponse)
async def rerank(req: RerankRequest) -> RerankResponse:
    """Re-rank documents by relevance to query using cross-encoder."""
    assert _reranker is not None, "Reranker not initialized"

    start = time.perf_counter()
    results = _reranker.rerank(req.query, req.documents, req.top_n)
    latency_ms = (time.perf_counter() - start) * 1000

    return RerankResponse(
        results=[RerankResult(index=idx, relevance_score=score) for idx, score in results],
        model=_reranker.model_name,
        latency_ms=latency_ms,
    )


@app.post("/nli", response_model=NLIResponse)
async def nli(req: NLIRequest) -> NLIResponse:
    """Classify NLI relationship (entailment/contradiction/neutral)."""
    assert _nli is not None, "NLI classifier not initialized"

    start = time.perf_counter()
    scores = _nli.classify(req.premise, req.hypothesis)
    latency_ms = (time.perf_counter() - start) * 1000

    # Find the label with highest score
    label = max(scores, key=scores.get)  # type: ignore[arg-type]

    return NLIResponse(
        label=label,
        scores=NLIScores(**scores),
        model=_nli.model_name,
        latency_ms=latency_ms,
    )


@app.post("/protocol/predict-tier", response_model=TierPredictResponse)
async def predict_tier(req: TierPredictRequest) -> TierPredictResponse:
    """Predict optimal encoding tier (1-3) for a constraint."""
    if _tier_model is None:
        # No tier model loaded — return fallback signal
        return TierPredictResponse(
            predicted_tier=0,
            confidence=0.0,
            model="none",
            latency_ms=0.0,
        )

    start = time.perf_counter()
    tier, confidence = _tier_model.predict(req.constraint_text, req.agent_context, req.trust_score)
    latency_ms = (time.perf_counter() - start) * 1000

    return TierPredictResponse(
        predicted_tier=tier,
        confidence=confidence,
        model=_tier_model.model_name,
        latency_ms=latency_ms,
    )


@app.get("/health", response_model=HealthResponse)
async def health() -> HealthResponse:
    """Health check — reports loaded models and last inference time."""
    models_loaded: list[str] = []
    last_ms: float | None = None

    if _reranker and _reranker.is_loaded:
        models_loaded.append(_reranker.model_name)
        if _reranker.last_inference_ms > 0:
            last_ms = _reranker.last_inference_ms
    if _nli and _nli.is_loaded:
        models_loaded.append(_nli.model_name)
        if _nli.last_inference_ms > 0:
            last_ms = _nli.last_inference_ms
    if _tier_model and _tier_model.is_loaded:
        models_loaded.append(_tier_model.model_name)
        if _tier_model.last_inference_ms > 0:
            last_ms = _tier_model.last_inference_ms

    return HealthResponse(
        status="ok" if models_loaded else "loading",
        models_loaded=models_loaded,
        last_inference_ms=last_ms,
    )
