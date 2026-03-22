"""Tests for the tier model and predict-tier endpoint."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def client_with_tier():
    """Create a test client with tier model mocked."""
    mock_reranker = MagicMock()
    mock_reranker.is_loaded = True
    mock_reranker.model_name = "test-rerank-model"
    mock_reranker.last_inference_ms = 10.0
    mock_reranker.rerank.return_value = []

    mock_nli = MagicMock()
    mock_nli.is_loaded = True
    mock_nli.model_name = "test-nli-model"
    mock_nli.last_inference_ms = 5.0

    mock_tier = MagicMock()
    mock_tier.is_loaded = True
    mock_tier.model_name = "test-tier-model"
    mock_tier.last_inference_ms = 3.0
    mock_tier.predict.return_value = (2, 0.85)

    with patch("neural_sidecar.app._reranker", mock_reranker), \
         patch("neural_sidecar.app._nli", mock_nli), \
         patch("neural_sidecar.app._tier_model", mock_tier):
        from neural_sidecar.app import app
        yield TestClient(app)


@pytest.fixture
def client_no_tier():
    """Create a test client without tier model."""
    mock_reranker = MagicMock()
    mock_reranker.is_loaded = True
    mock_reranker.model_name = "test-rerank-model"
    mock_reranker.last_inference_ms = 10.0

    mock_nli = MagicMock()
    mock_nli.is_loaded = True
    mock_nli.model_name = "test-nli-model"
    mock_nli.last_inference_ms = 5.0

    with patch("neural_sidecar.app._reranker", mock_reranker), \
         patch("neural_sidecar.app._nli", mock_nli), \
         patch("neural_sidecar.app._tier_model", None):
        from neural_sidecar.app import app
        yield TestClient(app)


def test_predict_tier(client_with_tier):
    resp = client_with_tier.post("/protocol/predict-tier", json={
        "constraint_text": "never force push to main",
        "agent_context": "git operations",
        "trust_score": 0.8,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["predicted_tier"] == 2
    assert data["confidence"] == 0.85
    assert data["model"] == "test-tier-model"
    assert data["latency_ms"] >= 0


def test_predict_tier_no_model(client_no_tier):
    """When tier model is not loaded, returns fallback (0, 0.0)."""
    resp = client_no_tier.post("/protocol/predict-tier", json={
        "constraint_text": "test constraint",
        "agent_context": "test context",
        "trust_score": 0.5,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["predicted_tier"] == 0
    assert data["confidence"] == 0.0


def test_predict_tier_validation(client_with_tier):
    """trust_score must be in [0, 1]."""
    resp = client_with_tier.post("/protocol/predict-tier", json={
        "constraint_text": "test",
        "agent_context": "ctx",
        "trust_score": 1.5,
    })
    assert resp.status_code == 422


def test_health_with_tier(client_with_tier):
    resp = client_with_tier.get("/health")
    data = resp.json()
    assert "test-tier-model" in data["models_loaded"]


def test_health_without_tier(client_no_tier):
    resp = client_no_tier.get("/health")
    data = resp.json()
    assert "test-tier-model" not in data["models_loaded"]


def test_tier_model_predict_returns_valid_tier():
    """Unit test the TierModel.predict() method with mocked CrossEncoder."""
    from neural_sidecar.tier_model import TierModel

    model = TierModel("test-model")
    # Before loading: returns fallback
    assert model.predict("test", "ctx", 0.5) == (0, 0.0)
    assert not model.is_loaded

    # Mock the internal model
    mock_ce = MagicMock()
    mock_ce.predict.return_value = [0.8]  # high score → T1
    model._model = mock_ce

    tier, conf = model.predict("constraint text", "agent context", 0.7)
    assert tier == 1
    assert conf > 0.0
    assert model.is_loaded
    assert model.last_inference_ms >= 0


def test_tier_model_predict_mid_score():
    """Mid-range score should return T2."""
    from neural_sidecar.tier_model import TierModel

    model = TierModel("test-model")
    mock_ce = MagicMock()
    mock_ce.predict.return_value = [0.55]  # mid score → T2
    model._model = mock_ce

    tier, conf = model.predict("constraint", "ctx", 0.5)
    assert tier == 2


def test_tier_model_predict_low_score():
    """Low score should return T3."""
    from neural_sidecar.tier_model import TierModel

    model = TierModel("test-model")
    mock_ce = MagicMock()
    mock_ce.predict.return_value = [0.2]  # low score → T3
    model._model = mock_ce

    tier, conf = model.predict("constraint", "ctx", 0.5)
    assert tier == 3
