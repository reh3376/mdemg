FROM python:3.12-slim
WORKDIR /app
COPY pyproject.toml .
COPY neural_sidecar/ neural_sidecar/
RUN pip install --no-cache-dir .
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=60s \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1
CMD ["uvicorn", "neural_sidecar.app:app", "--host", "0.0.0.0", "--port", "8000"]
