import { CLOCK, createInitialState } from '../data/fixtures.js';
import { allowedRoute } from './router.js';

export { createInitialState };
// Retired mock surfaces (model approval/reversal and preventive-action lifecycle) were removed
// because they have no backend support and are superseded by the official gateway dashboard.
// Only the documented mockup transitions below remain.
const writes = new Set(['REQUEST_JOB', 'ADVANCE_JOB', 'REVIEW_SOURCE', 'IMPORT_SOURCE']);
const technicalWrites = new Set(['REQUEST_JOB', 'ADVANCE_JOB']);
const text = (value, max = 160) => typeof value === 'string' && value.trim().length <= max ? value.trim() : '';
const fail = (state, feedback) => ({ ...state, feedback });

function record(state, action, detail, changes) {
  const sequence = state.sequence + 1;
  return {
    ...state, ...changes, sequence, feedback: `${action}.`,
    audit: [{ id: `event-${sequence}`, actor: 'Administrador simulado', action, detail, time: CLOCK }, ...state.audit].slice(0, 100)
  };
}

function requestJob(state) {
  const job = { id: `job-${state.sequence + 1}`, status: 'queued', scenario: state.scenario, sectorId: state.sectorId, actionId: null, requestedAt: '15:00', batchId: null };
  return record(state, 'Proceso solicitado', job.id, { jobs: [job, ...state.jobs] });
}

export function reducer(state, action) {
  if (action.type === 'RESET') return { ...createInitialState(), feedback: 'Vista restablecida.' };
  if (action.type === 'SET_ROLE' && ['viewer', 'admin'].includes(action.role)) {
    return { ...state, role: action.role, route: allowedRoute(state.route, action.role), feedback: 'Perfil actualizado.' };
  }
  if (action.type === 'SET_SCENARIO' && ['current', 'future'].includes(action.scenario)) {
    return { ...state, scenario: action.scenario, query: '', freshnessFilter: 'all', selectedHistoryId: null, feedback: 'Vista actualizada.' };
  }
  if (action.type === 'NAVIGATE') return { ...state, route: allowedRoute(action.route, state.role), feedback: '' };
  if (action.type === 'SELECT_SECTOR' && state.sectors.some(item => item.id === action.id)) return { ...state, sectorId: action.id };
  if (action.type === 'SET_FILTER') {
    if (action.key === 'query') return { ...state, query: text(action.value, 100) };
    if (action.key === 'freshnessFilter' && ['all', 'current', 'stale'].includes(action.value)) return { ...state, freshnessFilter: action.value };
  }
  if (action.type === 'SELECT_HISTORY') {
    if (action.id === null || state.history.some(row => row.id === action.id && row.scenario === state.scenario)) return { ...state, selectedHistoryId: action.id };
  }
  if (!writes.has(action.type)) return state;
  if (state.role !== 'admin') return fail(state, 'Esta acción requiere un perfil administrador de coordinación.');
  if (state.scenario !== 'future' && !technicalWrites.has(action.type)) return fail(state, 'Gestión no disponible para esta vista.');

  if (action.type === 'REQUEST_JOB') return requestJob(state);
  if (action.type === 'ADVANCE_JOB') {
    const job = state.jobs.find(item => item.id === action.id && item.status === 'queued' && item.scenario === state.scenario);
    if (!job) return fail(state, 'El proceso no está en cola para esta vista.');
    const sector = state.sectors.find(item => item.id === job.sectorId) ?? state.sectors[0];
    const batch = {
      id: `batch-${state.sequence + 1}`, sectorId: sector.id, scenario: job.scenario,
      emittedAt: CLOCK, expiresAt: '2026-01-15T16:00:00Z',
      risk: job.scenario === 'current' ? 'not_evaluated' : sector.risk,
      // The retired model-approval workflow no longer selects a version: publications keep the
      // declared fixture reference instead of an approved candidate.
      model: job.scenario === 'current' ? 'demo-fixture-v1' : 'scenario-v1',
      source: 'Publicación manual'
    };
    return record(state, 'Lote publicado', batch.id, {
      history: [batch, ...state.history].slice(0, 100),
      jobs: state.jobs.map(item => item.id === job.id ? { ...item, status: 'published', batchId: batch.id } : item)
    });
  }
  if (['REVIEW_SOURCE', 'IMPORT_SOURCE'].includes(action.type)) {
    if (!state.sources.some(item => item.id === action.id)) return fail(state, 'Fuente no encontrada.');
    const importing = action.type === 'IMPORT_SOURCE';
    // Sources are declarative descriptors without counts: importing records the event and
    // requires a new review, it never fabricates record totals.
    return record(state, importing ? 'Registros incorporados' : 'Revisión de calidad registrada', action.id, {
      sources: state.sources.map(item => item.id === action.id ? { ...item, quality: importing ? 'pending' : 'reviewed' } : item)
    });
  }
  return state;
}
