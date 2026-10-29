# The base must satisfy requires-python in pyproject.toml (>=3.14 since 2026-10-24);
# pip refuses the install otherwise and the Docker Publish workflow goes red.
FROM python:3.14-slim
WORKDIR /app
COPY pyproject.toml .
COPY neural_sidecar/ neural_sidecar/
RUN pip install --no-cache-dir .
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=60s \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1
CMD ["uvicorn", "neural_sidecar.app:app", "--host", "0.0.0.0", "--port", "8000"]
