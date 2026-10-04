/**
 * HTTP surface for the db-adapter service.
 *
 *   POST /query   -> simulate a datastore lookup; adds custom span attributes,
 *                    records custom metrics, and emits a structured OTel log.
 *   GET  /healthz -> liveness probe.
 *
 * OpenTelemetry is bootstrapped separately in `instrumentation.ts`, loaded via
 * `node --require` before this module. By the time these routes run the HTTP
 * server and Express are already auto-instrumented (incl. W3C tracecontext
 * propagation), so this file only adds service-specific telemetry on top of
 * the auto-created request span.
 */
import { performance } from 'node:perf_hooks';
import { metrics, SpanStatusCode, trace } from '@opentelemetry/api';
import { logs, SeverityNumber } from '@opentelemetry/api-logs';
import express, { type Request, type Response } from 'express';
import { buildItems, rowCountForKey } from './db';

const PORT = Number(process.env.PORT ?? 8082);
const HOST = '0.0.0.0';
/** Fraction of /query requests that fail, to produce error spans. */
const ERROR_RATE = 0.03;
const DB_SYSTEM = 'sqlite-sim';

const meter = metrics.getMeter('db-adapter');
const logger = logs.getLogger('db-adapter');

const rowsCounter = meter.createCounter('db.query.rows', {
  description: 'Total number of rows returned by simulated datastore queries.',
  unit: '{row}',
});
const latencyHistogram = meter.createHistogram('db.query.latency', {
  description: 'Latency of simulated datastore queries.',
  unit: 'ms',
});

const sleep = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));

const app = express();
// The contract is JSON-only, so parse every request body as JSON regardless of
// the incoming Content-Type (callers using `curl -d` send urlencoded by default).
app.use(express.json({ type: () => true }));

app.get('/healthz', (_req: Request, res: Response) => {
  res.status(200).json({ status: 'ok' });
});

app.post('/query', async (req: Request, res: Response) => {
  const start = performance.now();
  const body = (req.body ?? {}) as { key?: unknown; query?: unknown };
  const key = typeof body.key === 'string' ? body.key : '';
  const query = typeof body.query === 'string' ? body.query : '';

  // Decorate the active (auto-instrumented) request span with db semantics.
  const span = trace.getActiveSpan();
  span?.setAttributes({
    'db.system': DB_SYSTEM,
    'db.operation': 'select',
    'db.query.key': key,
  });

  // Simulate datastore latency (5-40ms).
  await sleep(5 + Math.random() * 35);
  const latencyMs = Math.round((performance.now() - start) * 100) / 100;

  // ~3% simulated failures -> error span + HTTP 500.
  if (Math.random() < ERROR_RATE) {
    const message = `simulated datastore failure for key "${key}"`;
    span?.setStatus({ code: SpanStatusCode.ERROR, message });
    latencyHistogram.record(latencyMs, { 'db.system': DB_SYSTEM, outcome: 'error' });
    logger.emit({
      severityNumber: SeverityNumber.ERROR,
      severityText: 'ERROR',
      body: message,
      attributes: {
        'db.query.key': key,
        'http.route': '/query',
        'http.response.status_code': 500,
      },
    });
    res.status(500).json({ error: message });
    return;
  }

  const rows = rowCountForKey(key);
  const items = buildItems(key, rows);

  span?.setAttribute('db.query.rows', rows);
  rowsCounter.add(rows, { 'db.system': DB_SYSTEM });
  latencyHistogram.record(latencyMs, { 'db.system': DB_SYSTEM, outcome: 'ok' });

  logger.emit({
    severityNumber: SeverityNumber.INFO,
    severityText: 'INFO',
    body: 'db query served',
    attributes: {
      'db.system': DB_SYSTEM,
      'db.query.key': key,
      'db.query.text': query,
      'db.query.rows': rows,
      'db.latency_ms': latencyMs,
      'http.route': '/query',
      'http.response.status_code': 200,
    },
  });

  res.status(200).json({ rows, items, latency_ms: latencyMs });
});

const server = app.listen(PORT, HOST, () => {
  console.log(`db-adapter listening on http://${HOST}:${PORT}`);
});

// Stop accepting new connections on shutdown; the OTel SDK (instrumentation.ts)
// flushes telemetry and exits the process on the same signals.
const closeServer = (): void => {
  server.close();
};
process.once('SIGTERM', closeServer);
process.once('SIGINT', closeServer);
