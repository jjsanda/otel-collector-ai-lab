# Troubleshooting

## Build

**`go: downloading go1.XX ... toolchain not available`**
The Collector modules require Go 1.25. If your system Go is older, set
`GOTOOLCHAIN=auto` (the default) so the correct toolchain is fetched, or install
Go 1.25+. All `make` targets pin `GOTOOLCHAIN` and set `GOWORK=off`.

**`ocb` build fails resolving the local components module**
The components are wired via the manifest's per-entry `path: ./components`, which
emits a `replace` into the generated `collector/_build/go.mod`. Run the build
from the repository root with `GOWORK=off` so the workspace doesn't fight the
generated module. `make collector` does this for you.

## Runtime

**No traces in Jaeger**
- Confirm services point at the collector: `OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318`.
- Confirm Jaeger has OTLP enabled (`COLLECTOR_OTLP_ENABLED=true`, set in Compose).
- Give it a few seconds — the collector `batch` processor and the SDK batchers
  buffer briefly. Generate load: `make loadtest`.
- Check the collector is healthy: `curl localhost:13133`.

**A trace is split (services don't share one trace)**
Almost always a non-instrumented outbound HTTP client somewhere. Every service
must use its instrumented transport (Go `otelhttp.NewTransport`, Node
auto-instrumentations, Python `opentelemetry-instrument`). Verify by finding a
trace under service `api` that also contains `worker`, `db-adapter`, and
`llm-simulator` spans.

**No `gen_ai_*` metrics in Prometheus**
- Prometheus scrapes the collector every 10s — wait one interval after load.
- Metric names carry standard Prometheus suffixes: `gen_ai_client_operation_count_total`,
  `gen_ai_client_cost_usd_total`, `gen_ai_client_operation_duration_seconds_bucket`,
  `gen_ai_client_token_usage_sum`.
- Inspect the raw exposition: `curl localhost:8889/metrics | grep gen_ai_`.

**Prompts appear un-redacted**
The redaction only applies where `genairedaction` is in the pipeline. Confirm it
is present in the `traces` (and `logs`) pipeline and that the sensitive keys
match `gen_ai.input.messages` / `gen_ai.output.messages` /
`gen_ai.system_instructions`.

## Resources

**Containers get OOM-killed / the host is slow**
The stack is tuned for a laptop. Stop the load generator, and avoid running a
kind cluster at the same time as Compose. Every service has a `mem_limit` you
can lower further in `deploy/compose/docker-compose.yaml`.

## Kubernetes

**`kubeconform` reports unknown CRD schemas**
Use `-ignore-missing-schemas` (already set in `make k8s-validate`); the built-in
resources still validate strictly.

**kind smoke test**
`make kind-up` builds nothing by itself — it expects the local images
(`otel-genai/*:latest`) to exist and loads them into the cluster. Build them
first with `make up` (or `docker compose build`), then run the smoke test with
Compose stopped.
