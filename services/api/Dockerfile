# syntax=docker/dockerfile:1
# NOTE: build context must be the repository root (needs sibling services/shared).
#   docker build -f services/api/Dockerfile -t otel-genai/api .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY services/shared/ services/shared/
COPY services/api/ services/api/
WORKDIR /src/services/api
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath -ldflags="-s -w" -o /out/api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
