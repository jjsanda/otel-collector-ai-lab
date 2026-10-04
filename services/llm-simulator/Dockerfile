# syntax=docker/dockerfile:1
FROM python:3.12-slim

# --- runtime / OpenTelemetry defaults (override in Compose/K8s as needed) ---
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PORT=8083 \
    OTEL_SERVICE_NAME=llm-simulator \
    OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
    OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
    OTEL_TRACES_EXPORTER=otlp \
    OTEL_METRICS_EXPORTER=otlp \
    OTEL_LOGS_EXPORTER=otlp \
    OTEL_PYTHON_LOGGING_AUTO_INSTRUMENTATION_ENABLED=true

WORKDIR /app

# Install runtime deps first for better layer caching.
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Application code.
COPY app.py ./

# Run as an unprivileged user.
RUN useradd --create-home --uid 10001 app \
    && chown -R app:app /app
USER app

EXPOSE 8083

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD python -c "import os,urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:'+os.environ.get('PORT','8083')+'/healthz', timeout=2).status==200 else 1)"

# Zero-code auto-instrumentation (inbound server + outbound requests) wraps
# uvicorn; the app creates the manual gen_ai CLIENT span. Shell form so ${PORT}
# expands at runtime.
CMD opentelemetry-instrument uvicorn app:app --host 0.0.0.0 --port ${PORT}
