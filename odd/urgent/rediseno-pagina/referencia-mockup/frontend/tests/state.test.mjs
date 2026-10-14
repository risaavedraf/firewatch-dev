import test from 'node:test';
import assert from 'node:assert/strict';
import { createInitialState, reducer } from '../src/core/state.js';
import { sectorRisk, freshness, filteredHistory } from '../src/core/selectors.js';
import { routes, allowedRoute } from '../src/core/router.js';

const admin = () => ({ ...createInitialState(), role: 'admin', scenario: 'future' });
const apply = (state, type, payload = {}) => reducer(state, { type, ...payload });

test('route membership rejects inherited names, retires mock routes and preserves role redirects', () => {
  for (const role of ['viewer', 'admin']) {
    const state = { ...createInitialState(), role, route: 'history' };
    for (const route of ['toString', '__proto__', 'constructor', 'hasOwnProperty', 'unknown-route', 'models', 'prevention']) {
      assert.equal(allowedRoute(route, role), 'overview', `${role}: ${route}`);
      assert.equal(apply(state, 'NAVIGATE', { route }).route, 'overview');
    }
    assert.equal(allowedRoute('history', role), 'history');
    assert.equal(allowedRoute('sources', role), role === 'admin' ? 'sources' : 'overview');
  }
  assert.deepEqual(Object.keys(routes), ['overview', 'history', 'sources', 'jobs', 'audit', 'access']);
});

test('retired mock views are gone and every remaining route still renders', async () => {
  for (const retired of ['models', 'prevention']) {
    await assert.rejects(import(`../src/views/${retired}.js`), { code: 'ERR_MODULE_NOT_FOUND' });
  }
  const { shell } = await import('../src/components/shell.js');
  for (const scenario of ['current', 'future']) {
    for (const role of ['viewer', 'admin']) {
      for (const route of Object.keys(routes)) {
        if (allowedRoute(route, role) !== route) continue;
        const state = { ...createInitialState(), scenario, role, route };
        const view = await import(`../src/views/${route}.js`);
        const markup = shell(state, view[route](state));
        assert.ok(markup.includes('<h1>'), `${route}/${role}/${scenario}`);
        assert.doesNotMatch(markup, /\bundefined\b|\bNaN\b|\[object Object\]/, `${route}/${role}/${scenario}`);
        if (route === 'sources') assert.doesNotMatch(markup, /\b(?:248|64|31)\b/, 'No fictitious source counts');
      }
    }
  }
});

test('retired mock state, fixtures and write transitions are absent', () => {
  const state = createInitialState();
  for (const key of ['models', 'observedFires', 'actions', 'activeModel', 'previousModel']) assert.equal(key in state, false, key);
  const future = admin();
  for (const type of ['APPROVE_MODEL', 'REVERT_MODEL', 'CREATE_ACTION', 'UPDATE_ACTION', 'ADD_EVIDENCE', 'REVIEW_EVIDENCE', 'REEVALUATE']) {
    const payload = { id: 'action-01', title: 'x', owner: 'y', sectorId: 's1', priority: 'high', deadline: '2026-01-17', status: 'closed', reference: 'x', sufficiency: 'sufficient', reason: 'x' };
    assert.strictEqual(apply(future, type, payload), future, type);
  }
});

test('current demo never classifies a sector; future simulation is explicit', () => {
  const state = createInitialState();
  for (const sector of state.sectors) assert.equal(sectorRisk(state, sector), 'not_evaluated');
  const future = apply(state, 'SET_SCENARIO', { scenario: 'future' });
  assert.equal(future.scenario, 'future');
  assert.equal(sectorRisk(future, future.sectors[0]), 'high');
  assert.equal(sectorRisk(future, future.sectors[3]), 'not_evaluated');
});

test('freshness uses expiry against frozen clock, independent from risk', () => {
  assert.equal(freshness({ expiresAt: '2026-01-15T14:59:00Z' }), 'stale');
  assert.equal(freshness({ expiresAt: '2026-01-15T15:00:00Z' }), 'stale');
  assert.equal(freshness({ expiresAt: '2026-01-15T15:01:00Z' }), 'current');
  assert.equal(freshness({ expiresAt: null }), 'unknown');
});

test('history combines scenario, search, and freshness; detail selection validates id', () => {
  let state = createInitialState();
  assert.equal(filteredHistory(state).length, 2);
  state = apply(state, 'SET_FILTER', { key: 'freshnessFilter', value: 'stale' });
  assert.deepEqual(filteredHistory(state).map(row => row.id), ['demo-01']);
  state = apply(state, 'SET_FILTER', { key: 'query', value: 'absent' });
  assert.equal(filteredHistory(state).length, 0);
  state = apply(state, 'SELECT_HISTORY', { id: 'b104' });
  assert.equal(state.selectedHistoryId, null);
  state = apply(state, 'SELECT_HISTORY', { id: 'demo-01' });
  assert.equal(state.selectedHistoryId, 'demo-01');
});

test('viewer cannot mutate admin state even by directly calling reducer', () => {
  const state = createInitialState();
  for (const type of ['REQUEST_JOB', 'ADVANCE_JOB', 'REVIEW_SOURCE', 'IMPORT_SOURCE']) {
    const next = apply(state, type, { id: 'weather' });
    for (const key of ['jobs', 'sources', 'audit']) assert.deepEqual(next[key], state[key]);
    assert.match(next.feedback, /administrador/i);
  }
  assert.equal(allowedRoute('sources', 'viewer'), 'overview');
  assert.equal(allowedRoute('sources', 'admin'), 'sources');
  assert.equal(allowedRoute('missing', 'admin'), 'overview');
  assert.equal(allowedRoute('history', 'viewer'), 'history');
});

test('current scenario blocks mock writes but allows synthetic job lifecycle', () => {
  const state = { ...admin(), scenario: 'current' };
  const blocked = apply(state, 'REVIEW_SOURCE', { id: 'weather' });
  assert.equal(blocked.sources[0].quality, 'pending');
  assert.match(blocked.feedback, /no disponible/i);
  let running = apply(state, 'REQUEST_JOB');
  const id = running.jobs[0].id;
  assert.equal(running.jobs[0].status, 'queued');
  running = apply(running, 'ADVANCE_JOB', { id });
  assert.equal(running.jobs[0].status, 'published');
  const batch = running.history.find(row => row.id === running.jobs[0].batchId);
  assert.equal(batch.risk, 'not_evaluated');
  assert.equal(batch.model, 'demo-fixture-v1');
  assert.equal(apply(running, 'ADVANCE_JOB', { id }).history.length, running.history.length);
});

test('sources stay declarative descriptors without invented counts or observed fires', () => {
  const state = createInitialState();
  assert.deepEqual(state.sources.map(source => source.id), ['weather', 'territory']);
  for (const source of state.sources) {
    for (const removed of ['count', 'missing', 'duplicates', 'coverage', 'time', 'checks']) assert.equal(removed in source, false, `${source.id}: ${removed}`);
  }
  let reviewed = apply(admin(), 'REVIEW_SOURCE', { id: 'weather' });
  assert.equal(reviewed.sources[0].quality, 'reviewed');
  assert.equal(reviewed.audit.length, 2);
  reviewed = apply(reviewed, 'IMPORT_SOURCE', { id: 'weather' });
  assert.equal(reviewed.sources[0].quality, 'pending');
  assert.equal(reviewed.audit.length, 3);
  assert.deepEqual(reviewed.sources.map(source => source.id), ['weather', 'territory']);
});

test('role/scenario changes normalize routes; reset restores independent fixtures', () => {
  let state = apply(admin(), 'NAVIGATE', { route: 'jobs' });
  assert.equal(state.route, 'jobs');
  state = apply(state, 'SET_ROLE', { role: 'viewer' });
  assert.equal(state.route, 'overview');
  state = apply(state, 'SET_SCENARIO', { scenario: 'unknown' });
  assert.equal(state.scenario, 'future');
  state = apply(state, 'RESET');
  assert.equal(state.role, 'viewer');
  assert.equal(state.scenario, 'current');
  assert.equal(state.audit.length, 1);
  state.sectors[0].name = 'mutated';
  assert.equal(createInitialState().sectors[0].name, 'Cordón del Arrayán');
});

test('publication uses the declared fixture reference without rewriting history or mutating prior state', () => {
  const initial = admin();
  const original = structuredClone(initial);
  let state = apply(initial, 'REQUEST_JOB');
  state = apply(state, 'ADVANCE_JOB', { id: state.jobs[0].id });
  assert.equal(state.history[0].model, 'scenario-v1');
  assert.equal(state.history.find(row => row.id === 'b104').model, 'scenario-v1');
  assert.deepEqual(initial, original);
  assert.equal(state.audit[0].actor, 'Administrador simulado');
  assert.equal(state.audit[0].time, '2026-01-15T15:00:00Z');
});

test('invalid and alternate input cannot alter unknown entities or publish across scenarios', () => {
  const state = admin();
  assert.deepEqual(apply(state, 'REVIEW_SOURCE', { id: 'unknown' }).sources, state.sources);
  assert.equal(apply(state, 'SET_ROLE', { role: 'root' }).role, 'admin');
  assert.equal(apply(state, 'SELECT_SECTOR', { id: 'unknown' }).sectorId, 's1');
  assert.equal(apply(state, 'SET_FILTER', { key: 'freshnessFilter', value: 'unknown' }).freshnessFilter, 'all');
  let queued = apply(state, 'REQUEST_JOB');
  const id = queued.jobs[0].id;
  queued = apply(queued, 'SET_SCENARIO', { scenario: 'current' });
  assert.equal(apply(queued, 'ADVANCE_JOB', { id }).jobs[0].status, 'queued');
});
