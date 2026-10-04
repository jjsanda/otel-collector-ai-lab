# syntax=docker/dockerfile:1

# ---- build stage: compile TypeScript -> dist/ ----
FROM node:22-alpine AS build
WORKDIR /app
# Install all deps (incl. dev) against the lockfile for a reproducible build.
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json ./
COPY src ./src
RUN npm run build

# ---- runtime stage: production deps + compiled JS only ----
FROM node:22-alpine AS runtime
ENV NODE_ENV=production \
    PORT=8082 \
    OTEL_SERVICE_NAME=db-adapter \
    OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
    OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
    OTEL_RESOURCE_ATTRIBUTES=deployment.environment=dev
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev && npm cache clean --force
COPY --from=build /app/dist ./dist

# Run as the unprivileged user shipped in the base image.
USER node
EXPOSE 8082

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8082/healthz || exit 1

# instrumentation.js is loaded via --require so the OTel SDK is active before
# any application/server code runs.
CMD ["node", "--require", "./dist/instrumentation.js", "./dist/server.js"]
