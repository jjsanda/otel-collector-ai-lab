# syntax=docker/dockerfile:1
# NOTE: build context must be the repository root (needs sibling services/shared).
#   docker build -f services/worker/Dockerfile -t otel-genai/worker .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY services/shared/ services/shared/
COPY services/worker/ services/worker/
WORKDIR /src/services/worker
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath -ldflags="-s -w" -o /out/worker .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/worker /worker
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/worker"]
