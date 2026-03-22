"""Pydantic request/response models for the sidecar API."""

from pydantic import BaseModel, Field


# --- Re-rank ---

class RerankRequest(BaseModel):
    query: str
    documents: list[str]
    top_n: int = Field(default=10, ge=1, le=100)


class RerankResult(BaseModel):
    index: int
    relevance_score: float


class RerankResponse(BaseModel):
    results: list[RerankResult]
    model: str
    latency_ms: float


# --- NLI ---

class NLIRequest(BaseModel):
    premise: str
    hypothesis: str


class NLIScores(BaseModel):
    entailment: float
    contradiction: float
    neutral: float


class NLIResponse(BaseModel):
    label: str  # "entailment", "contradiction", or "neutral"
    scores: NLIScores
    model: str
    latency_ms: float


# --- Health ---

class HealthResponse(BaseModel):
    status: str
    models_loaded: list[str]
    last_inference_ms: float | None = None


# --- Tier Prediction (J17) ---

class TierPredictRequest(BaseModel):
    constraint_text: str
    agent_context: str
    trust_score: float = Field(ge=0.0, le=1.0)


class TierPredictResponse(BaseModel):
    predicted_tier: int   # 1, 2, or 3 (0 = fallback)
    confidence: float     # 0.0-1.0
    model: str
    latency_ms: float
