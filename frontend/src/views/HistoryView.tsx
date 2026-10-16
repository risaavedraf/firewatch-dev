import { useEffect, useMemo, useState } from 'react';

import { idleCopy, rejectedSessionCopy } from '../app/sessionCopy';
import {
  getHistoryFilters,
  setHistoryQuery,
  setHistoryStatus,
  useHistoryFilters,
  useSelectedZone,
  useSessionState,
} from '../app/store';
import { Icon, UnimplementedNote } from '../components/ui';
import { history, type Failure } from '../core/api';
import {
  filterHistoryRows,
  historyStatusOptions,
  historyVisibleCount,
  selectHistoryStatus,
} from '../domain';
import type { RiskBatch } from '../types/api';

// N8b — the history view. The panel shows the rows `/v1/risk/history` returned
// for the SELECTED zone (the route requires `zone_id`), and search plus the
// data-status filter narrow ONLY those rows in memory: they issue no read and
// cannot reach another zone. The badge keeps reporting what the API returned
// while the result count reports what survived the filters, and the per-zone
// limit stays stated inside the view.

type HistoryStatus =
  | { kind: 'signed-out' }
  | { kind: 'no-zone' }
  | { kind: 'loading' }
  | { kind: 'empty' }
  | { kind: 'ready'; allDemo: boolean }
  | { kind: 'failure'; failure: Failure };

interface LoadedHistory {
  rows: RiskBatch[];
  status: Extract<HistoryStatus, { kind: 'empty' | 'ready' | 'failure' }>;
}

const EMPTY_ROWS: RiskBatch[] = [];

function failureMessage(failure: Failure): string {
  if (failure.reason === 'zone_mismatch') {
    return 'La API devolvió un historial de otra zona; se descartó.';
  }
  switch (failure.kind) {
    case 'unauthorized':
      return rejectedSessionCopy();
    case 'forbidden':
      return 'La API denegó el permiso para consultar el historial (403). Solicita acceso a un administrador.';
    case 'malformed':
      return 'La API devolvió un historial ilegible; no se muestran filas.';
    case 'missing':
      return 'No existe historial para la zona consultada (404).';
    case 'unavailable':
      return failure.status !== undefined
        ? `No se pudo consultar el historial (HTTP ${failure.status}).`
        : 'No se pudo consultar el historial (sin conexión).';
  }
}

function stateMessage(status: HistoryStatus, signedOutMessage: string): string {
  switch (status.kind) {
    case 'signed-out':
      return signedOutMessage;
    case 'no-zone':
      return 'Selecciona una zona para consultar el historial.';
    case 'loading':
      return 'Consultando historial…';
    case 'empty':
      return 'La API devolvió un historial vacío.';
    case 'ready':
      return status.allDemo ? 'Registros sintéticos devueltos por la API.' : 'Registros devueltos por la API.';
    case 'failure':
      return failureMessage(status.failure);
  }
}

export function HistoryView() {
  const session = useSessionState();
  const selectedZone = useSelectedZone();
  const signedIn = session.status === 'signed-in';

  // The loaded rows travel with the zone they were read for, so a sign-out or a
  // zone change cannot keep showing another zone's rows: the derived status and
  // the rendered rows both require the stored key to match the current selection.
  const [stored, setStored] = useState<{ key: string; loaded: LoadedHistory } | null>(null);
  // The filters are shared state, not local: the footer's reset control owns them
  // too, and a search term must not survive a sign-out.
  const { query, status: statusFilter } = useHistoryFilters();

  useEffect(() => {
    if (!signedIn || selectedZone === '') return;
    const controller = new AbortController();
    let active = true;
    void history(selectedZone, controller.signal).then((outcome) => {
      if (!active) return;
      if (!outcome.ok) {
        setStored({
          key: selectedZone,
          loaded: { rows: [], status: { kind: 'failure', failure: outcome.failure } },
        });
        return;
      }
      const data = outcome.data;
      // A status option that no longer exists in the fresh read is dropped, so a
      // stale selection cannot silently narrow rows that no longer carry it.
      setHistoryStatus(selectHistoryStatus(getHistoryFilters().status, historyStatusOptions(data)));
      setStored({
        key: selectedZone,
        loaded: {
          rows: data,
          status:
            data.length === 0
              ? { kind: 'empty' }
              : { kind: 'ready', allDemo: data.every((batch) => batch.demo === true) },
        },
      });
    });
    return () => {
      active = false;
      controller.abort();
    };
  }, [signedIn, selectedZone]);

  const current =
    signedIn && selectedZone !== '' && stored !== null && stored.key === selectedZone ? stored.loaded : null;
  const status: HistoryStatus = !signedIn
    ? { kind: 'signed-out' }
    : selectedZone === ''
      ? { kind: 'no-zone' }
      : current !== null
        ? current.status
        : { kind: 'loading' };
  const rows = current !== null ? current.rows : EMPTY_ROWS;

  const options = useMemo(() => historyStatusOptions(rows), [rows]);
  const visible = useMemo(
    () => filterHistoryRows(rows, { query, status: statusFilter }) as RiskBatch[],
    [rows, query, statusFilter],
  );

  const loaded = status.kind === 'ready' || status.kind === 'empty';
  const countLabel = loaded ? String(rows.length) : '—';
  const visibleLabel = loaded ? historyVisibleCount(visible.length, rows.length) : '—';

  return (
    <section className="view" id="history" data-view="history" aria-labelledby="history-title">
      <div className="intro-row">
        <div>
          <p className="eyebrow">TRAZABILIDAD</p>
          <h2 id="history-title">Historial de lotes</h2>
        </div>
        <span className="count-badge" id="history-count">
          {countLabel}
        </span>
      </div>
      <p className="panel-note">
        Registros devueltos por <code>/v1/risk/history</code>; sin agregar métricas calculadas.
      </p>
      <div className="filter-bar">
        <div className="search-field">
          <Icon name="search" />
          <input
            id="history-query"
            type="search"
            aria-label="Buscar en el historial de la zona"
            placeholder="Buscar por fecha, estado o lote"
            value={query}
            onChange={(event) => setHistoryQuery(event.target.value)}
          />
        </div>
        <label className="filter-label">
          Estado del dato
          <select
            id="history-status-filter"
            aria-label="Filtrar el historial por estado del dato"
            value={statusFilter}
            onChange={(event) => setHistoryStatus(event.target.value)}
          >
            <option value="">Todos los estados</option>
            {options.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>
        <span className="result-count" id="history-visible-count">
          {visibleLabel}
        </span>
      </div>
      <div id="history-state" className={status.kind === 'failure' ? 'empty-state risk-error' : 'empty-state'}>
        {stateMessage(status, idleCopy(session, 'consultar el historial'))}
      </div>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Solicitado</th>
              <th>Estado</th>
              <th>Lote</th>
            </tr>
          </thead>
          <tbody id="history-rows">
            {visible.map((batch) => (
              <tr key={batch.id}>
                <td>{batch.requested_at ?? 'No informado'}</td>
                <td>{batch.data_status ?? 'No informado'}</td>
                <td>{batch.id}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <UnimplementedNote>
        Historial de todas las zonas: <code>/v1/risk/history</code> exige <code>zone_id</code> y no existe endpoint
        agregado, así que esta tabla muestra siempre la zona consultada.
      </UnimplementedNote>
    </section>
  );
}
