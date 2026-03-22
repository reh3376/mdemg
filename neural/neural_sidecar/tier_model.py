"""Cross-encoder tier prediction for J17 protocol encoding."""

from __future__ import annotations

import logging
import time
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from sentence_transformers import CrossEncoder

logger = logging.getLogger(__name__)


class TierModel:
    """Predicts optimal encoding tier (1-3) for constraints via cross-encoder."""

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

        logger.info("Loading tier model: %s (device=%s)", self.model_name, self.device)
        self._model = CrossEncoder(self.model_name, device=self.device, num_labels=1)
        logger.info("Tier model loaded successfully")

    @property
    def is_loaded(self) -> bool:
        return self._model is not None

    @property
    def last_inference_ms(self) -> float:
        return self._last_inference_ms

    def predict(self, constraint_text: str, agent_context: str, trust_score: float) -> tuple[int, float]:
        """Predict optimal encoding tier for a constraint.

        Args:
            constraint_text: The constraint description.
            agent_context: Current agent context.
            trust_score: Session trust score (0.0-1.0).

        Returns:
            (predicted_tier 1-3, confidence 0.0-1.0).
            Returns (0, 0.0) if model unavailable — caller uses rule-based fallback.
        """
        if self._model is None:
            return 0, 0.0

        # Construct input pair: constraint text vs context with trust info
        text_a = constraint_text
        text_b = f"trust={trust_score:.2f} {agent_context}"

        start = time.perf_counter()
        scores = self._model.predict([(text_a, text_b)])
        elapsed_ms = (time.perf_counter() - start) * 1000
        self._last_inference_ms = elapsed_ms

        # Map regression score to tier via thresholds
        score = float(scores[0])
        # Higher score → more compressed encoding needed (T1)
        # Lower score → full NL needed (T3)
        if score >= 0.7:
            tier = 1  # T1: coded (high trust / familiar constraint)
        elif score >= 0.4:
            tier = 2  # T2: telegraphic
        else:
            tier = 3  # T3: full natural language

        # Confidence is the distance from the nearest threshold boundary
        boundaries = [0.4, 0.7]
        min_dist = min(abs(score - b) for b in boundaries)
        confidence = min(1.0, min_dist / 0.3)  # normalize to 0-1

        return tier, confidence
