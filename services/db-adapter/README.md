# db-adapter

A tiny, fully OpenTelemetry-instrumented database adapter for the
`otel-collector-ai-lab` demo. The Go **worker** service calls it over HTTP; it
propagates W3C trace context so it appears as a child span in the distributed
trace and exports **traces, metrics, and logs** to the collector over OTLP
`http/protobuf`.

## HTTP contract

| Method & path | Request | Response |
| --- | --- | --- |
| `POST /query` | `{"key": string, "query": string}` | `200 {"rows": int, "items": [string], "latency_ms": number}` — or `~3%` of the time `500 {"error": string}` |
| `GET /healthz` | – | `200 {"status": "ok"}` |

Listens on `PORT` (default `8082`), host `0.0.0.0`. Row counts are derived
deterministically from `key` (see `src/db.ts`); each request adds 5–40 ms of
simulated latency.

## OpenTelemetry

`src/instrumentation.ts` boots the OTel SDK and is loaded via `node --require`
**before** any app code, so HTTP/Express are auto-instrumented (incl. W3C
`tracecontext` propagation). On top of the auto-created request span the
`/query` handler adds:

- **Custom span attributes**: `db.system=sqlite-sim`, `db.operation`,
  `db.query.key`, `db.query.rows`.
- **Custom metrics**: `db.query.rows` (counter) and `db.query.latency` (ms
  histogram).
- **Structured log** per request, emitted through the OTel logs pipeline.

Configured purely via standard env vars (defaults in parentheses):
`OTEL_EXPORTER_OTLP_ENDPOINT` (`http://localhost:4318`),
`OTEL_EXPORTER_OTLP_PROTOCOL` (`http/protobuf`),
`OTEL_SERVICE_NAME` (`db-adapter`),
`OTEL_RESOURCE_ATTRIBUTES` (`deployment.environment=dev`).

Exporter/collector failures are logged but never thrown, so the service boots
and serves traffic with **no external dependencies** — only export requires a
reachable collector.

## Build / run choice: `tsc` (compiled)

Compiled with `tsc` to `dist/` (not `tsx`) because it yields the simplest,
leanest **production** image: the multi-stage Dockerfile builds with dev deps,
then ships only `dist/` + production deps as plain JS — no transpiler at runtime
(better for the RAM-constrained stack). CommonJS output is used so the OTel
auto-instrumentations' `require`-patching reliably wraps `http`/`express`
without ESM loader hooks. `tsx` is kept only for a no-build `npm run dev`.

```bash
npm install
npm run build       # tsc -> dist/
npm start           # node --require ./dist/instrumentation.js ./dist/server.js
npm run dev         # tsx, no build step
npm run lint        # eslint
npm test            # vitest (row-count logic)
```

### Docker

```bash
docker build -t db-adapter .
docker run --rm -p 8082:8082 \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318 \
  db-adapter
```

### Quick check

```bash
curl -s -XPOST localhost:8082/query \
  -H 'Content-Type: application/json' \
  -d '{"key":"abc","query":"select 1"}'
# {"rows":..,"items":[..],"latency_ms":..}
curl -s localhost:8082/healthz
# {"status":"ok"}
```
