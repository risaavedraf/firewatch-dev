import { useSyncExternalStore } from 'react';

import { getSessionSnapshot, subscribeSession, type SessionState } from '../core/session';
import type { RiskLevel } from '../types/api';

// Application state that more than one view needs. It lives outside React for one
// reason: the selected zone and the remembered colours are the same facts for the
// overview, the history and the job request, so a view has to be able to read them
// without owning them — otherwise moving between views would silently change
// which zone the page is talking about.
//
// The remembered colours are what keep a comuna's colour when the selection moves
// away: the page colours only what it has actually read this session, remembers
// each level it read, and never issues an extra request to do it.
//
// Everything here is cleared when the session ends, so a logout cannot leave the
// previous session's selection or colours on screen.

let selectedZone = '';
let zoneLevels: ReadonlyMap<string, RiskLevel> = new Map();
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of [...listeners]) listener();
}

export function getSelectedZone(): string {
  return selectedZone;
}

export function getZoneLevels(): ReadonlyMap<string, RiskLevel> {
  return zoneLevels;
}

export function subscribeZoneState(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function selectZone(code: string): void {
  if (code === selectedZone) return;
  selectedZone = code;
  emit();
}

// `null` clears a remembered colour on purpose: a zone whose latest read found no
// published batch must not keep a colour from an earlier read.
export function rememberZoneLevel(code: string, level: RiskLevel | null): void {
  const next = new Map(zoneLevels);
  if (level === null) next.delete(code);
  else next.set(code, level);
  zoneLevels = next;
  emit();
}

export function clearZoneState(): void {
  if (selectedZone === '' && zoneLevels.size === 0) return;
  selectedZone = '';
  zoneLevels = new Map();
  emit();
}

// ── History filters ───────────────────────────────────────────────────────
// The history view's two client-side filters. They narrow rows the API already
// returned, so they are view state — but they live here because the footer's
// "Restablecer filtros" control belongs to the shell, and the control and the
// view have to agree on what a reset means. The snapshot object is cached so
// `useSyncExternalStore` sees a stable reference between changes.
let historyQuery = '';
let historyStatus = '';
let historyFilters = { query: '', status: '' };

function publishHistoryFilters(): void {
  historyFilters = { query: historyQuery, status: historyStatus };
  emit();
}

export function getHistoryFilters(): { query: string; status: string } {
  return historyFilters;
}

export function setHistoryQuery(query: string): void {
  if (query === historyQuery) return;
  historyQuery = query;
  publishHistoryFilters();
}

export function setHistoryStatus(status: string): void {
  if (status === historyStatus) return;
  historyStatus = status;
  publishHistoryFilters();
}

// What the footer control does: clear both filters. A no-op when they are already
// clear, so a reset does not wake every view for nothing.
export function resetHistoryFilters(): void {
  if (historyQuery === '' && historyStatus === '') return;
  historyQuery = '';
  historyStatus = '';
  publishHistoryFilters();
}

export function useHistoryFilters(): { query: string; status: string } {
  return useSyncExternalStore(subscribeZoneState, getHistoryFilters, getHistoryFilters);
}

export function useSelectedZone(): string {
  return useSyncExternalStore(subscribeZoneState, getSelectedZone, getSelectedZone);
}

export function useZoneLevels(): ReadonlyMap<string, RiskLevel> {
  return useSyncExternalStore(subscribeZoneState, getZoneLevels, getZoneLevels);
}

export function useSessionState(): SessionState {
  return useSyncExternalStore(subscribeSession, getSessionSnapshot, getSessionSnapshot);
}

// Called once from `main.tsx`. An explicit binding beats a module-load side
// effect: the coupling between "the session ended" and "the private view is
// empty" is then visible where the application starts.
export function bindZoneStateToSession(): () => void {
  return subscribeSession(() => {
    // Any state other than an established session means the private view is
    // empty: without a token there is no zone the page is describing, and the
    // previous session's colours must not survive on screen.
    if (getSessionSnapshot().status !== 'signed-in') {
      clearZoneState();
      // A filter is part of the private view too: a sign-out must not leave a
      // search term or a status narrowing the next session's rows.
      resetHistoryFilters();
    }
  });
}
