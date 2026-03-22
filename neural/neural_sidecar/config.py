"""Configuration via pydantic-settings (env vars)."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Sidecar configuration loaded from environment variables."""

    host: str = "0.0.0.0"
    port: int = 8100
    rerank_model: str = "cross-encoder/ms-marco-MiniLM-L-6-v2"
    nli_model: str = "cross-encoder/nli-deberta-v3-xsmall"
    tier_model: str = ""  # Empty = disabled. Path or HuggingFace model name.
    device: str = "cpu"
    log_level: str = "info"

    model_config = {"env_prefix": "NEURAL_"}


settings = Settings()
