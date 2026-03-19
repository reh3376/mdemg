"""Tests for the sidecar API endpoints."""

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def client():
    """Create a test client. Models may not be available in CI, so we mock them."""
    from unittest.mock import MagicMock, patch

    # Mock the models so tests work without downloading them
    mock_reranker = MagicMock()
    mock_reranker.is_loaded = True
    mock_reranker.model_name = "test-rerank-model"
    mock_reranker.last_inference_ms = 10.0
    mock_reranker.rerank.return_value = [(0, 0.95), (2, 0.80), (1, 0.60)]

    mock_nli = MagicMock()
    mock_nli.is_loaded = True
    mock_nli.model_name = "test-nli-model"
    mock_nli.last_inference_ms = 5.0
    mock_nli.classify.return_value = {"entailment": 0.1, "neutral": 0.2, "contradiction": 0.7}

    with patch("neural_sidecar.app._reranker", mock_reranker), \
         patch("neural_sidecar.app._nli", mock_nli):
        from neural_sidecar.app import app
        yield TestClient(app)


def test_health(client):
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert len(data["models_loaded"]) == 2


def test_rerank(client):
    resp = client.post("/rerank", json={
        "query": "how to handle errors",
        "documents": ["error handling guide", "logging setup", "retry patterns"],
        "top_n": 3,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert "results" in data
    assert len(data["results"]) == 3
    assert data["results"][0]["relevance_score"] >= data["results"][1]["relevance_score"]


def test_rerank_empty_documents(client):
    resp = client.post("/rerank", json={
        "query": "test",
        "documents": [],
        "top_n": 5,
    })
    assert resp.status_code == 200


def test_nli(client):
    resp = client.post("/nli", json={
        "premise": "You must always use error handling",
        "hypothesis": "Error handling should be skipped for performance",
    })
    assert resp.status_code == 200
    data = resp.json()
    assert data["label"] in ("entailment", "neutral", "contradiction")
    assert "scores" in data
    assert data["scores"]["entailment"] >= 0
    assert data["scores"]["contradiction"] >= 0
    assert data["scores"]["neutral"] >= 0


def test_nli_entailment(client):
    """Verify the mock returns contradiction for our test case."""
    resp = client.post("/nli", json={
        "premise": "The sky is blue",
        "hypothesis": "The sky is not blue",
    })
    data = resp.json()
    assert data["label"] == "contradiction"


def test_rerank_validation(client):
    """top_n must be >= 1."""
    resp = client.post("/rerank", json={
        "query": "test",
        "documents": ["doc1"],
        "top_n": 0,
    })
    assert resp.status_code == 422  # Validation error


def test_health_shows_models(client):
    resp = client.get("/health")
    data = resp.json()
    assert "test-rerank-model" in data["models_loaded"]
    assert "test-nli-model" in data["models_loaded"]


def test_nli_missing_field(client):
    """Missing hypothesis should return 422."""
    resp = client.post("/nli", json={"premise": "test"})
    assert resp.status_code == 422


def test_rerank_missing_query(client):
    """Missing query should return 422."""
    resp = client.post("/rerank", json={"documents": ["doc1"]})
    assert resp.status_code == 422
