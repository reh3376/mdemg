"""Cross-encoder re-ranking using sentence-transformers."""

from __future__ import annotations

import time
import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from sentence_transformers import CrossEncoder

logger = logging.getLogger(__name__)


class Reranker:
    """Wraps a cross-encoder model for document re-ranking."""

    def __init__(self, model_name: str, device: str = "cpu") -> None:
        self.model_name = model_name
        self.device = device
        self._model: CrossEncoder | None = None
        self._last_inference_ms: float = 0.0

    def load(self) -> None:
        """Lazy-load the cross-encoder model."""
        if self._model is not None:
            return
        from sentence_transformers import CrossEncoder

        logger.info("Loading rerank model: %s (device=%s)", self.model_name, self.device)
        self._model = CrossEncoder(self.model_name, device=self.device)
        logger.info("Rerank model loaded successfully")

    @property
    def is_loaded(self) -> bool:
        return self._model is not None

    @property
    def last_inference_ms(self) -> float:
        return self._last_inference_ms

    def rerank(self, query: str, documents: list[str], top_n: int = 10) -> list[tuple[int, float]]:
        """Re-rank documents by relevance to query.

        Returns list of (original_index, relevance_score) sorted by score descending.
        """
        self.load()
        assert self._model is not None

        pairs = [(query, doc) for doc in documents]
        start = time.perf_counter()
        scores = self._model.predict(pairs)
        elapsed_ms = (time.perf_counter() - start) * 1000
        self._last_inference_ms = elapsed_ms

        # Pair each score with its original index
        indexed_scores = list(enumerate(float(s) for s in scores))
        indexed_scores.sort(key=lambda x: x[1], reverse=True)

        return indexed_scores[:top_n]
