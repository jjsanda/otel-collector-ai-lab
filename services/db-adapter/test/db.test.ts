import { describe, expect, it } from 'vitest';
import { buildItems, MAX_ROWS, PAGE_SIZE, rowCountForKey } from '../src/db';

describe('rowCountForKey', () => {
  it('is deterministic for a given key', () => {
    expect(rowCountForKey('abc')).toBe(rowCountForKey('abc'));
  });

  it('always returns an integer within [1, MAX_ROWS]', () => {
    const keys = ['', 'a', 'abc', 'select 1', 'a-very-long-key-1234567890', '🔑'];
    for (const key of keys) {
      const rows = rowCountForKey(key);
      expect(Number.isInteger(rows)).toBe(true);
      expect(rows).toBeGreaterThanOrEqual(1);
      expect(rows).toBeLessThanOrEqual(MAX_ROWS);
    }
  });

  it('varies across different keys', () => {
    const counts = new Set(['alpha', 'bravo', 'charlie', 'delta'].map(rowCountForKey));
    expect(counts.size).toBeGreaterThan(1);
  });
});

describe('buildItems', () => {
  it('caps the returned page at PAGE_SIZE', () => {
    expect(buildItems('k', MAX_ROWS)).toHaveLength(PAGE_SIZE);
  });

  it('returns exactly `rows` items when below the page size', () => {
    expect(buildItems('k', 3)).toEqual(['k:row-0', 'k:row-1', 'k:row-2']);
  });

  it('returns an empty page for zero rows', () => {
    expect(buildItems('k', 0)).toEqual([]);
  });
});
