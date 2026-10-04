/**
 * Pure datastore-simulation helpers for the db-adapter service.
 *
 * The row count is derived deterministically from the request key so that the
 * resulting traces/metrics are reproducible and this logic is trivially
 * unit-testable: there is no I/O, randomness, or OpenTelemetry state in here.
 */

/** Maximum number of rows the simulated datastore will report for any key. */
export const MAX_ROWS = 100;

/** Maximum number of item strings returned in a single response page. */
export const PAGE_SIZE = 8;

/**
 * Deterministically derive a row count in the inclusive range [1, MAX_ROWS]
 * from a query key using a small 32-bit FNV-1a rolling hash.
 */
export function rowCountForKey(key: string): number {
  let hash = 2166136261; // FNV-1a 32-bit offset basis.
  for (let i = 0; i < key.length; i++) {
    hash ^= key.charCodeAt(i);
    hash = Math.imul(hash, 16777619); // FNV-1a 32-bit prime.
  }
  // `>>> 0` coerces to an unsigned 32-bit int before the modulo.
  return ((hash >>> 0) % MAX_ROWS) + 1;
}

/**
 * Build the (capped) page of item strings returned alongside the row count.
 * Real datastores page large result sets, so we mirror that by returning at
 * most PAGE_SIZE sample rows while still reporting the full `rows` total.
 */
export function buildItems(key: string, rows: number): string[] {
  const count = Math.max(0, Math.min(rows, PAGE_SIZE));
  const items: string[] = [];
  for (let i = 0; i < count; i++) {
    items.push(`${key}:row-${i}`);
  }
  return items;
}
