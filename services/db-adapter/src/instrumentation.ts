/**
 * OpenTelemetry bootstrap for the db-adapter service.
 *
 * This module MUST be evaluated before any application code so the Node
 * auto-instrumentations can patch `http`/`express` before they are required.
 * It is wired up via `node --require ./dist/instrumentation.js` (see the
 * `start` script and the Dockerfile) and is deliberately never imported from
 * `server.ts`.
 *
 * Traces, metrics, and logs are all exported over OTLP http/protobuf to the
 * collector. Exporter/connection failures are logged by the SDK (via the diag
 * logger below) but never thrown into the request path, so the service keeps
 * serving traffic when the collector is unreachable.
 */
import { diag, DiagConsoleLogger, DiagLogLevel } from '@opentelemetry/api';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';
import { OTLPLogExporter } from '@opentelemetry/exporter-logs-otlp-proto';
import { OTLPMetricExporter } from '@opentelemetry/exporter-metrics-otlp-proto';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-proto';
import { BatchLogRecordProcessor } from '@opentelemetry/sdk-logs';
import { PeriodicExportingMetricReader } from '@opentelemetry/sdk-metrics';
import { NodeSDK } from '@opentelemetry/sdk-node';

// Provide sane defaults while still honouring anything set in the environment.
// The OTLP exporters and resource detectors read these standard variables, so
// setting them here is enough to fully configure export + resource attributes.
process.env.OTEL_SERVICE_NAME ??= 'db-adapter';
process.env.OTEL_EXPORTER_OTLP_ENDPOINT ??= 'http://localhost:4318';
process.env.OTEL_EXPORTER_OTLP_PROTOCOL ??= 'http/protobuf';
process.env.OTEL_RESOURCE_ATTRIBUTES ??= 'deployment.environment=dev';

// Surface SDK/exporter diagnostics (e.g. "collector unreachable") on stderr
// without ever crashing the service.
diag.setLogger(new DiagConsoleLogger(), DiagLogLevel.ERROR);

const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter(),
  metricReaders: [
    new PeriodicExportingMetricReader({
      exporter: new OTLPMetricExporter(),
      exportIntervalMillis: 15_000,
    }),
  ],
  logRecordProcessors: [new BatchLogRecordProcessor(new OTLPLogExporter())],
  instrumentations: [
    getNodeAutoInstrumentations({
      // `fs` instrumentation is noisy and irrelevant here; disable to keep the
      // footprint small in the RAM-constrained Compose stack.
      '@opentelemetry/instrumentation-fs': { enabled: false },
    }),
  ],
});

sdk.start();

// Flush telemetry on shutdown so the final spans/metrics/logs are not lost.
function shutdown(): void {
  // Hard cap on shutdown: when the collector is unreachable the export flush
  // retries for several seconds, which would otherwise stall container stop.
  // The timer is unref'd so it never keeps the process alive on its own.
  const guard = setTimeout(() => process.exit(0), 2_000);
  guard.unref();
  sdk.shutdown().then(
    () => process.exit(0),
    (err: unknown) => {
      diag.error('OTel SDK shutdown failed', err);
      process.exit(0);
    },
  );
}

process.once('SIGTERM', shutdown);
process.once('SIGINT', shutdown);
