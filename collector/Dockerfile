# syntax=docker/dockerfile:1
# Builds the custom otelcol-genai distribution from source with ocb.
# Build context = repository root:
#   docker build -f collector/Dockerfile -t otel-genai/collector .
FROM golang:1.25 AS build
ENV GOWORK=off CGO_ENABLED=0
WORKDIR /src
COPY components/ components/
COPY collector/builder-config.yaml collector/builder-config.yaml
RUN go install go.opentelemetry.io/collector/cmd/builder@v0.155.0 \
 && builder --config collector/builder-config.yaml

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /src/collector/_build/otelcol-genai /otelcol-genai
COPY collector/config/ /etc/otelcol/
# 4317/4318 OTLP, 8889 Prometheus, 8099 GenAI events, 13133 health, 55679 zpages
EXPOSE 4317 4318 8889 8099 13133 55679
USER nonroot:nonroot
ENTRYPOINT ["/otelcol-genai"]
CMD ["--config", "/etc/otelcol/collector.yaml"]
