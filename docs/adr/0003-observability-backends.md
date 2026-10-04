# ADR 0003 — Backend stack: Jaeger + Prometheus + Loki + Grafana

## Status
Accepted

## Context
The collector needs somewhere to send the three signals. Candidates: the
Jaeger/Prometheus/Loki/Grafana stack, a single ClickHouse store, or the
all-in-one `grafana/otel-lgtm` image.

## Decision
Export traces to **Jaeger**, metrics to **Prometheus**, and logs to **Loki**,
all visualized in **Grafana**.

## Consequences
- **Pro:** each backend is the recognizable, de-facto standard for its signal.
- **Pro:** relatively light, so the whole stack fits in ~2 GiB with per-service
  memory limits.
- **Exporter details (verified against v0.155):**
  - Jaeger all-in-one ingests OTLP natively (`COLLECTOR_OTLP_ENABLED=true`);
    traces go via `otlphttp` to `jaeger:4318`.
  - The contrib `lokiexporter` was **removed**; logs go via `otlphttp` to Loki's
    native OTLP endpoint `http://loki:3100/otlp`.
  - Metrics use the `prometheus` exporter (scrape) rather than remote-write.
- **Con:** three backends instead of one, but each stays lightweight in practice
  (the whole Compose stack idles well under 1 GiB). The Kubernetes manifests are
  self-contained and don't require any backend to run.
