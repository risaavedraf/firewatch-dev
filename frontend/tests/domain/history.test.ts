import { describe, expect, test } from 'vitest';

import {
  defaultHistoryFilters,
  filterHistoryRows,
  historyStatusOptions,
  historyVisibleCount,
  selectHistoryStatus,
} from '../../src/domain/history';

const rows = [
  { id: 'lote-uno', requested_at: '2026-01-01T00:00:00Z', data_status: 'sufficient' },
  { id: 'lote-dos', requested_at: '2026-02-02T00:00:00Z', data_status: 'insufficient' },
  { id: 'lote-tres', requested_at: '2026-03-03T00:00:00Z', data_status: 'sufficient' },
];

function ids(values: readonly unknown[]): unknown[] {
  return values.map((value) => (value as { id?: unknown }).id);
}

describe('history search and status filter', () => {
  test('search narrows the rows already returned without mutating them', () => {
    const visible = filterHistoryRows(rows, { query: 'lote-uno', status: '' });
    expect(visible).toEqual([rows[0]]);
    expect(rows).toHaveLength(3);
  });

  test('the status filter narrows the visible rows and keeps the returned length', () => {
    const visible = filterHistoryRows(rows, { query: '', status: 'sufficient' });
    expect(visible).toHaveLength(2);
    expect(historyVisibleCount(visible.length, rows.length)).toBe('2 de 3');
  });

  test('search and status compose over the same returned rows', () => {
    const visible = filterHistoryRows(rows, { query: 'lote', status: 'insufficient' });
    expect(ids(visible)).toEqual(['lote-dos']);
  });

  test('search is a case-insensitive substring over id, status or request time', () => {
    expect(ids(filterHistoryRows(rows, { query: 'LOTE-DOS', status: '' }))).toEqual(['lote-dos']);
    expect(ids(filterHistoryRows(rows, { query: '2026-03', status: '' }))).toEqual(['lote-tres']);
    // Substring, not equality: "sufficient" also occurs inside "insufficient".
    expect(ids(filterHistoryRows(rows, { query: 'sufficient', status: '' }))).toEqual([
      'lote-uno',
      'lote-dos',
      'lote-tres',
    ]);
  });

  test('derives the status options from the rows actually returned', () => {
    expect(historyStatusOptions(rows)).toEqual(['insufficient', 'sufficient']);
    expect(historyStatusOptions([])).toEqual([]);
    expect(historyStatusOptions([{ data_status: '' }, { data_status: null }, {}])).toEqual([]);
  });

  test('a status option missing from a fresh read is dropped, not kept', () => {
    expect(selectHistoryStatus('sufficient', ['insufficient'])).toBe('');
    expect(selectHistoryStatus('sufficient', ['sufficient'])).toBe('sufficient');
  });

  test('reset filters clears the query and the status', () => {
    expect(defaultHistoryFilters()).toEqual({ query: '', status: '' });
  });
});
