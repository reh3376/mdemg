"""Configuration via pydantic-settings (env vars)."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Sidecar configuration loaded from environment variables."""

    # HOOKSYNC-001: loopback by default — the sidecar serves an internal
    # scoring API; only the local mdemg server calls it. Override via the
    # NEURAL_HOST env var (env_prefix below) for multi-host setups.
    host: str = "127.0.0.1"
    port: int = 8100
    rerank_model: str = "cross-encoder/ms-marco-MiniLM-L-6-v2"
    nli_model: str = "cross-encoder/nli-deberta-v3-xsmall"
    tier_model: str = ""  # Empty = disabled. Path or HuggingFace model name.
    device: str = "cpu"
    log_level: str = "info"

    model_config = {"env_prefix": "NEURAL_"}


settings = Settings()
