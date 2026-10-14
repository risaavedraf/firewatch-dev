import { isRecord } from './guards';

// Rows for the SELECTED zone, as the API returned them. These functions are the
// whole of the search and status filter: they take rows already in memory and
// return a narrowed view, so a filter can never reach another zone or issue a
// read. The badge keeps reporting what the API returned while the visible count
// reports only what survived the filters.

export interface HistoryFilters {
  query: string;
  status: string;
}

export function defaultHistoryFilters(): HistoryFilters {
  return { query: '', status: '' };
}

export function historyStatusOptions(rows: readonly unknown[]): string[] {
  const statuses = new Set<string>();
  for (const row of rows) {
    if (!isRecord(row)) continue;
    const value = row.data_status === null || row.data_status === undefined ? '' : String(row.data_status);
    if (value !== '') statuses.add(value);
  }
  return [...statuses].sort();
}

// Keeps the previous selection only while it still exists in the fresh read, so
// a stale option cannot silently narrow rows that no longer carry it.
export function selectHistoryStatus(previous: string, options: readonly string[]): string {
  return options.includes(previous) ? previous : '';
}

export function filterHistoryRows(rows: readonly unknown[], filters: HistoryFilters): unknown[] {
  const query = filters.query.trim().toLowerCase();
  return rows.filter((row) => {
    if (!isRecord(row)) return false;
    if (filters.status !== '' && String(row.data_status ?? '') !== filters.status) return false;
    if (query === '') return true;
    return [row.requested_at, row.data_status, row.id].some((value) =>
      String(value ?? '')
        .toLowerCase()
        .includes(query),
    );
  });
}

export function historyVisibleCount(visible: number, total: number): string {
  return `${visible} de ${total}`;
}
