"""NLI (Natural Language Inference) using cross-encoder model."""

from __future__ import annotations

import time
import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from sentence_transformers import CrossEncoder

logger = logging.getLogger(__name__)

LABELS = ["entailment", "neutral", "contradiction"]


class NLIClassifier:
    """Wraps a cross-encoder NLI model for entailment/contradiction/neutral classification."""

    def __init__(self, model_name: str, device: str = "cpu") -> None:
        self.model_name = model_name
        self.device = device
        self._model: CrossEncoder | None = None
        self._last_inference_ms: float = 0.0

    def load(self) -> None:
        """Lazy-load the NLI model."""
        if self._model is not None:
            return
        from sentence_transformers import CrossEncoder

        logger.info("Loading NLI model: %s (device=%s)", self.model_name, self.device)
        self._model = CrossEncoder(self.model_name, device=self.device)
        logger.info("NLI model loaded successfully")

    @property
    def is_loaded(self) -> bool:
        return self._model is not None

    @property
    def last_inference_ms(self) -> float:
        return self._last_inference_ms

    def classify(self, premise: str, hypothesis: str) -> dict[str, float]:
        """Classify the relationship between premise and hypothesis.

        Returns dict with keys: entailment, neutral, contradiction (values are probabilities).
        """
        self.load()
        assert self._model is not None

        start = time.perf_counter()
        scores = self._model.predict([(premise, hypothesis)], apply_softmax=True)
        elapsed_ms = (time.perf_counter() - start) * 1000
        self._last_inference_ms = elapsed_ms

        # scores shape: (1, 3) → [entailment, neutral, contradiction]
        probs = scores[0] if hasattr(scores, '__len__') and len(scores) > 0 else scores
        result = {}
        for i, label in enumerate(LABELS):
            result[label] = float(probs[i]) if i < len(probs) else 0.0

        return result
