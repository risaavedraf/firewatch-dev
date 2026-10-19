import { afterEach, beforeEach, expect, test } from 'vitest';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import {
  createPreventiveAction,
  modelVersions,
  preventiveAction,
  preventiveActions,
  sources,
  transitionPreventiveAction,
  type Failure,
  type Outcome,
} from '../../src/core/api';
import { completeLogin, resetSession } from '../../src/core/session';
import type { PreventiveAction, PreventiveActionDetail } from '../../src/types/api';

import { createCryptoStub, createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// S3 — the transport half of the frozen `prevention` contract
// (`odd/tasks/presentation-client-ready.md` §«Contrato congelado»): the four
// routes, the same absolute gateway origin and bearer as every other read, and
// the `{"code": ..., "request_id": ...}` error format the other services use.

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const ACTIONS_URL = `${gatewayOrigin}/v1/prevention/actions`;
const MODELS_URL = `${gatewayOrigin}/v1/models`;
const SOURCES_URL = `${gatewayOrigin}/v1/risk/sources`;

let fetchStub: ReturnType<typeof createFetchStub>;
let storageStub: ReturnType<typeof createStorageStub>;

function setup(): void {
  fetchStub = createFetchStub();
  storageStub = createStorageStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  vi.stubGlobal('crypto', createCryptoStub());
  resetSession();
}

async function connect(token = jwt({ realm_access: { roles: ['admin'] } })): Promise<string> {
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'verifier' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: token, id_token: 'id-token' });
  await login;
  fetchStub.calls.length = 0;
  return token;
}

function expectFailure<T>(outcome: Outcome<T>): Failure {
  if (outcome.ok) throw new Error('expected a failure outcome');
  return outcome.failure;
}

function action(overrides: Record<string, unknown> = {}): PreventiveAction {
  return {
    id: 'action-1',
    community_code: '05101',
    title: 'Despeje de vegetación',
    description: '',
    priority: 'alta',
    status: 'propuesta',
    assignee: 'equipo-1',
    due_date: '2026-11-01',
    created_by: 'actor-1',
    created_at: '2026-10-17T10:00:00Z',
    updated_at: '2026-10-17T10:00:00Z',
    ...overrides,
  };
}

function detail(): PreventiveActionDetail {
  return {
    ...action(),
    history: [
      {
        id: 1,
        action_id: 'action-1',
        from_status: null,
        to_status: 'propuesta',
        actor_sub: 'actor-1',
        note: 'alta',
        created_at: '2026-10-17T10:00:00Z',
      },
    ],
    evidence: [],
    evidence_reviews: [],
    reassessments: [],
    evidence_sufficient: false,
  };
}

beforeEach(setup);

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the list route carries the bearer and only the filters that were chosen', async () => {
  const token = await connect();

  const filtered = preventiveActions({ communityCode: '05101', status: 'propuesta' });
  await flush();
  expect(fetchStub.callsFor(`${ACTIONS_URL}?community_code=05101&status=propuesta`)).toHaveLength(1);
  const call = fetchStub.callsFor(`${ACTIONS_URL}?community_code=05101&status=propuesta`)[0];
  expect((call.init.headers as Record<string, string>).Authorization).toBe(`Bearer ${token}`);
  expect(call.init.credentials).toBe('omit');
  fetchStub.answer(`${ACTIONS_URL}?community_code=05101&status=propuesta`, [action()]);
  expect(await filtered).toEqual({ ok: true, data: [action()] });

  // A second list read of the same kind only starts once the first has settled:
  // the transport discards a superseded answer, so two concurrent lists would
  // leave the earlier one unanswerable by design.
  const unfiltered = preventiveActions();
  await flush();
  expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1);
  fetchStub.answer(ACTIONS_URL, []);
  expect(await unfiltered).toEqual({ ok: true, data: [] });
});

test('the list fails closed on a non-array or an entry without identity', async () => {
  await connect();

  const notArray = preventiveActions();
  await flush();
  fetchStub.answer(ACTIONS_URL, { actions: [] });
  expect(expectFailure(await notArray).kind).toBe('malformed');

  const brokenEntry = preventiveActions();
  await flush();
  fetchStub.answer(ACTIONS_URL, [action({ id: '' })]);
  expect(expectFailure(await brokenEntry).kind).toBe('malformed');
});

test('the detail route returns the action with its history', async () => {
  await connect();

  const read = preventiveAction('action-1');
  await flush();
  expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1);
  fetchStub.answer(`${ACTIONS_URL}/action-1`, detail());
  expect(await read).toEqual({ ok: true, data: detail() });
});

test('the detail route rejects a history that is missing or not an array', async () => {
  await connect();

  const missing = preventiveAction('action-1');
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/action-1`, action());
  expect(expectFailure(await missing).kind).toBe('malformed');

  const brokenHistory = preventiveAction('action-1');
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/action-1`, { ...action(), history: [{ from_status: 'propuesta' }] });
  expect(expectFailure(await brokenHistory).kind).toBe('malformed');
});

test('creation posts only the fields it was given and accepts a 201', async () => {
  await connect();

  const created = createPreventiveAction({
    communityCode: '05101',
    title: 'Charla preventiva',
    description: 'Actividad con la junta de vecinos',
    priority: 'media',
    assignee: '',
    dueDate: '',
  });
  await flush();

  const call = fetchStub.callsFor(ACTIONS_URL).find(candidate => candidate.init.method === 'POST');
  expect(call).toBeDefined();
  expect((call?.init.headers as Record<string, string>)['Content-Type']).toBe('application/json');
  expect(JSON.parse(String(call?.init.body))).toEqual({
    community_code: '05101',
    title: 'Charla preventiva',
    description: 'Actividad con la junta de vecinos',
    priority: 'media',
  });

  fetchStub.answer(ACTIONS_URL, action({ status: 'propuesta' }), 201);
  const outcome = await created;
  expect(outcome.ok).toBe(true);
  if (outcome.ok) expect(outcome.data.status).toBe('propuesta');
});

test('a transition posts {to, note} and returns the new version', async () => {
  await connect();

  const outcome = transitionPreventiveAction('action-1', { to: 'asignada', note: 'asignada al equipo' });
  await flush();
  const transitionUrl = `${ACTIONS_URL}/action-1/transitions`;
  const call = fetchStub.callsFor(transitionUrl)[0];
  expect(call.init.method).toBe('POST');
  expect(JSON.parse(String(call.init.body))).toEqual({ to: 'asignada', note: 'asignada al equipo' });

  fetchStub.answer(transitionUrl, action({ status: 'asignada' }));
  const settled = await outcome;
  expect(settled.ok).toBe(true);
  if (settled.ok) expect(settled.data.status).toBe('asignada');
});

test('an invalid transition surfaces the 409 code and the request id untouched', async () => {
  await connect();

  const outcome = transitionPreventiveAction('action-1', { to: 'aceptada', note: '' });
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/action-1/transitions`, { code: 'invalid_transition' }, 409, 'req-409');
  const failure = expectFailure(await outcome);
  expect(failure.status).toBe(409);
  expect(failure.reason).toBe('invalid_transition');
  expect(failure.requestId).toBe('req-409');
});

test('a 422 invalid input is surfaced with its code', async () => {
  await connect();

  const outcome = createPreventiveAction({ communityCode: '', title: '' });
  await flush();
  fetchStub.answer(ACTIONS_URL, { code: 'invalid_input' }, 422, 'req-422');
  const failure = expectFailure(await outcome);
  expect(failure.status).toBe(422);
  expect(failure.reason).toBe('invalid_input');
});

test('a 403 is a stated permission denial and a 401 closes the session', async () => {
  await connect();

  const denied = preventiveActions();
  await flush();
  fetchStub.answer(ACTIONS_URL, { code: 'forbidden' }, 403, 'req-403');
  const forbidden = expectFailure(await denied);
  expect(forbidden.kind).toBe('forbidden');
  expect(forbidden.requestId).toBe('req-403');

  const refused = preventiveActions();
  await flush();
  fetchStub.answer(ACTIONS_URL, { code: 'unauthorized' }, 401, 'req-401');
  const unauthorized = expectFailure(await refused);
  expect(unauthorized.kind).toBe('unauthorized');
  expect(unauthorized.requestId).toBe('req-401');
});

test('the model versions read accepts the real {models} envelope and rejects a malformed list or entry', async () => {
  await connect();
  const read = modelVersions();
  await flush();
  expect(fetchStub.callsFor(MODELS_URL)).toHaveLength(1);
  fetchStub.answer(MODELS_URL, {
    models: [
      { version: 'academic-rule-v1', kind: 'rule', params_hash: null, approved_at: null, metrics: {}, active: true },
      { version: 'demo-fixture-v1', kind: 'placeholder', params_hash: null, approved_at: null, metrics: { synthetic: true }, active: false },
    ],
  });
  const outcome = await read;
  expect(outcome.ok).toBe(true);
  if (outcome.ok) expect(outcome.data).toHaveLength(2);

  // The bare array is the shape that was assumed and is NOT what the service
  // serves; it must be rejected so a wrong shape can never render as data.
  const bareArray = modelVersions();
  await flush();
  fetchStub.answer(MODELS_URL, [{ version: 'academic-rule-v1', active: true }]);
  expect(expectFailure(await bareArray).kind).toBe('malformed');

  const missingList = modelVersions();
  await flush();
  fetchStub.answer(MODELS_URL, { versions: [] });
  expect(expectFailure(await missingList).kind).toBe('malformed');

  const malformedEntry = modelVersions();
  await flush();
  fetchStub.answer(MODELS_URL, { models: [{ version: 'academic-rule-v1' }, 'not-an-entry'] });
  expect(expectFailure(await malformedEntry).kind).toBe('malformed');
});

test('the sources read carries the per-source cycle array through untouched', async () => {
  await connect();
  const body = {
    geography: {
      provenance: {
        provider: 'SUBDERE',
        attribution: 'Atribución',
        restriction: 'Restricción',
        record_count: 38,
        derived_sha256: 'c'.repeat(64),
      },
      source_cutoff_at: null,
      cutoff_semantics: 'retrieval_substitution',
      integrity: { algorithm: 'sha256', matches: true },
    },
    risk_input: { synthetic: true, display_name: 'Perfil', id: 'academic' },
    sources: [
      {
        source_id: 'fixture',
        last_run_at: '2026-01-01T11:45:00Z',
        next_run_at: '2026-01-01T12:00:00Z',
        complete: true,
        age_seconds: 900,
      },
    ],
  };
  const read = sources();
  await flush();
  fetchStub.answer(SOURCES_URL, body);
  const outcome = await read;
  expect(outcome.ok).toBe(true);
  if (outcome.ok) expect(outcome.data.sources).toEqual(body.sources);
});
