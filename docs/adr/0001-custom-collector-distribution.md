# ADR 0001 — Build a custom Collector distribution with ocb

## Status
Accepted

## Context
The project needs a Collector that performs GenAI-specific redaction and
metric derivation. Options:

1. Use the stock `otelcol-contrib` distribution and configure existing
   components (e.g. the `redactionprocessor`, `transformprocessor`/OTTL,
   `spanmetricsconnector`).
2. Build a **custom distribution** with the OpenTelemetry Collector Builder
   (`ocb`) that bundles purpose-built components written in Go.

## Decision
Build a custom distribution (`otelcol-genai`) with `ocb` and implement three
components from scratch (`genairedaction` processor, `genaimetrics` connector,
`genaievents` receiver).

## Consequences
- **Pro:** exercises real Collector internals (factories, `pdata`, lifecycle)
  and lets the components encode GenAI-specific behavior (cost estimation,
  provider normalization, semconv metric names) that stock components don't
  provide out of the box.
- **Pro:** the build is reproducible and pinned; the manifest doubles as
  documentation of exactly what's in the binary.
- **Con:** more code to own and test than a pure-config approach. Mitigated by
  keeping components small and well-tested (~90% coverage).
- Stock components (`otlp`, `batch`, `memory_limiter`, `prometheus`, `otlphttp`,
  `debug`, `healthcheck`, `zpages`) are still used where they already do the job.
