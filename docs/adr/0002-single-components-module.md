# ADR 0002 — One Go module for all three components

## Status
Accepted

## Context
The three custom components could live in three separate Go modules (as they do
in `opentelemetry-collector-contrib`) or in one module with three packages.

## Decision
Use a single module `github.com/jjsanda/otel-collector-ai-lab/components` with
`processor/`, `connector/`, and `receiver/` packages plus a shared
`internal/genai` package.

## Consequences
- **Pro:** one `go.mod`/`go.sum`, one `go test ./...`, and a single `replace`
  in the ocb manifest — the lowest build and maintenance surface, which matters
  on a resource-constrained dev machine.
- **Pro:** the shared `internal/genai` package (attribute keys, pricing, PII
  scrubber) is reused by both the processor and the connector without a
  published dependency.
- **Con:** the components version together rather than independently. Acceptable
  for a lab; a production project publishing components separately would split
  them. The directory layout still mirrors contrib so the split is trivial later.
