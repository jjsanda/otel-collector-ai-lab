# llm-simulator (Python)

Simulates LLM completions and emits **GenAI (`gen_ai.*`) spans** following the
OpenTelemetry GenAI semantic conventions. The `worker` service calls it; its
spans are the raw material the custom collector redacts and turns into metrics.

## Endpoints
- `POST /generate` — body `{"prompt": str, "model"?: str, "conversation_id"?: str}` →
  `{"text", "model", "input_tokens", "output_tokens"}`. Creates a CLIENT span
  `"{operation} {model}"` with `gen_ai.*` attributes, including the (deliberately
  sensitive) prompt/response content so redaction is demonstrable downstream.
- `GET /healthz` → `{"status":"ok"}`

## Configuration (env)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`), `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`
- `PORT` (default `8083`)
- Optional real provider: set `LLM_REAL=1` plus `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` (defaults to simulation — no keys needed)

## Run
```bash
pip install -r requirements-dev.txt
ruff check . && pytest
opentelemetry-instrument uvicorn app:app --host 0.0.0.0 --port 8083
```

Instrumentation is zero-code via `opentelemetry-instrument` (inbound HTTP +
`requests`); the `gen_ai` span is created manually in `app.py`.
