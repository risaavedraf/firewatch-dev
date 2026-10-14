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
    if (getSessionSnapshot().status !== 'signed-in') clearZoneState();
  });
}
