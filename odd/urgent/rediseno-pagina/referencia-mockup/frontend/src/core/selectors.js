import { CLOCK } from '../data/fixtures.js';

export function sectorRisk(state, sector) {
  return state.scenario === 'current' ? 'not_evaluated' : sector.risk;
}

export function freshness(batch) {
  const expiry = Date.parse(batch?.expiresAt);
  if (!Number.isFinite(expiry)) return 'unknown';
  return expiry <= Date.parse(CLOCK) ? 'stale' : 'current';
}

export function filteredHistory(state) {
  const query = state.query.trim().toLocaleLowerCase('es');
  return state.history.filter(row => {
    const sector = state.sectors.find(item => item.id === row.sectorId);
    const text = `${row.id} ${sector?.name ?? ''} ${row.model} ${row.source}`.toLocaleLowerCase('es');
    return row.scenario === state.scenario && text.includes(query)
      && (state.freshnessFilter === 'all' || freshness(row) === state.freshnessFilter);
  }).slice(0, 100);
}

export function selectedSector(state) {
  return state.sectors.find(sector => sector.id === state.sectorId) ?? state.sectors[0];
}

export function latestBatch(state) {
  return state.history.find(row => row.scenario === state.scenario && row.sectorId === state.sectorId);
}

export function canManage(state, futureOnly = true) {
  return state.role === 'admin' && (!futureOnly || state.scenario === 'future');
}
