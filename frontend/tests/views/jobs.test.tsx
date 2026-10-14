import { act, fireEvent, render } from '@testing-library/react';

import { clearZoneState, selectZone } from '../../src/app/store';
import { gatewayOrigin, issuer, pkceTransactionKey, timing } from '../../src/config';
import { completeLogin, resetSession } from '../../src/core/session';
import { JobsView } from '../../src/views/JobsView';

import { createCryptoStub, createFetchStub, flush, jwt } from '../core/harness';

// N8b — the processing view. The retired page created the job with a fresh
// `Idempotency-Key`, polled it with a bounded attempt cap and a hard deadline,
// and only reported completion when the published batch correlated zone, job and
// batch. These tests pin that contract to the React view, plus the two honesty
// rules the user kept by name: the served synthetic-scenario catalogue and the
// refusal to invent one.

const JOBS_URL = `${gatewayOrigin}/v1/jobs`;
const JOB_URL = `${gatewayOrigin}/v1/jobs/job`;
const SCENARIOS_URL = `${gatewayOrigin}/v1/risk/synthetic-scenarios`;
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;

function latestUrl(zone: string): string {
  return `${gatewayOrigin}/v1/risk/latest?zone_id=${zone}`;
}

function historyUrl(zone: string): string {
  return `${gatewayOrigin}/v1/risk/history?zone_id=${zone}`;
}

function batch(zone = '05101', id = 'batch', jobId = 'job') {
  return {
    zone_id: zone,
    id,
    job_id: jobId,
    demo: true,
    risk_level: 'alto',
    factors: [{ name: 'temperatura', known: true, value: 30, contribution: 0.2, note: '' }],
    rationale: 'Regla académica',
    policy: { policy_id: 'academic-rule-v1', params_hash: 'hash', parameters: { weight: '0.2' } },
    notice: 'Sintético',
  };
}

const CATALOGUE = [
  { id: 'mild', description: 'Perfil suave' },
  { id: 'dry-windy', description: 'Seco y ventoso' },
];

let fetchStub: ReturnType<typeof createFetchStub>;
let keyCounter = 0;

beforeEach(() => {
  keyCounter = 0;
  vi.useFakeTimers();
  fetchStub = createFetchStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  // `randomUUID` is the only crypto call this view makes; the rest of the stub
  // keeps the WebCrypto surface the session module may touch.
  vi.stubGlobal('crypto', { ...createCryptoStub(), randomUUID: () => `key-${(keyCounter += 1)}` });
  resetSession();
  clearZoneState();
  sessionStorage.clear();
});

afterEach(() => {
  resetSession();
  clearZoneState();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

async function connect(): Promise<void> {
  const token = jwt({ realm_access: { roles: ['admin'] } });
  sessionStorage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'v' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: token });
  await login;
  fetchStub.calls.length = 0;
}

function element<T extends HTMLElement = HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (!node) throw new Error(`missing #${id}`);
  return node as T;
}

// Every stub answer resolves a promise whose continuation updates React state,
// so the resolution and the flush that consumes it happen inside `act`.
async function answer(url: string, body: unknown, status = 200): Promise<void> {
  await act(async () => {
    fetchStub.answer(url, body, status);
    await flush();
  });
}

async function mount(zone = '05101') {
  act(() => selectZone(zone));
  const result = render(<JobsView />);
  await flush();
  return result;
}

async function serveCatalogue(items = CATALOGUE): Promise<void> {
  await flush();
  await answer(SCENARIOS_URL, items);
}

async function tick(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

function idempotencyKey(call: { init: RequestInit }): string {
  const headers = call.init.headers as Record<string, string>;
  return headers['Idempotency-Key'];
}

test('the section keeps its identity, the admin-only statement and the unimplemented note', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  const section = document.querySelector('section#jobs');
  expect(section).not.toBeNull();
  expect(section).toHaveAttribute('data-view', 'jobs');
  expect(element('jobs-title')).toHaveTextContent('Procesamiento asíncrono');

  for (const id of ['scenario-select', 'scenario-note', 'create-job', 'job-status', 'job-state', 'job-id', 'job-requested', 'refresh-job']) {
    expect(document.getElementById(id)).not.toBeNull();
  }

  // The action is administrative, and the view says so before the button.
  expect(section).toHaveTextContent(/ACCIÓN ADMINISTRATIVA/);
  expect(section).toHaveTextContent(/requiere permisos de administrador/);

  // The "not implemented" declaration stays inside the view: publishing,
  // manual advance and re-evaluation have no endpoint.
  expect(section).toHaveTextContent(/Publicar lote/i);
  expect(section).toHaveTextContent(/NO IMPLEMENTADO/i);
});

test('requesting a batch posts the selected zone with a fresh idempotency key per creation', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  const first = fetchStub.callsFor(JOBS_URL);
  expect(first).toHaveLength(1);
  expect(first[0].init.method).toBe('POST');
  expect(JSON.parse(first[0].init.body as string)).toEqual({ zone_id: '05101' });
  expect(idempotencyKey(first[0])).toBe('key-1');

  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });

  fireEvent.click(element('create-job'));
  await flush();
  const second = fetchStub.callsFor(JOBS_URL);
  expect(second).toHaveLength(2);
  expect(idempotencyKey(second[1])).not.toBe(idempotencyKey(second[0]));
});

test('a published job confirms only the correlated zone, job and batch', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });
  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(1);

  await answer(JOB_URL, { id: 'job', zone_id: '05101', state: 'published', batch_id: 'batch' });
  await answer(latestUrl('05101'), batch('05101', 'batch', 'job'));
  await answer(historyUrl('05101'), [batch('05101', 'batch', 'job')]);

  expect(element('job-status')).toHaveTextContent(/verificado/);
  expect(element('job-id')).toHaveTextContent('job');
});

test('a latest or history response for another batch cannot report the job as verified', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'published', batch_id: 'batch' });
  await answer(latestUrl('05101'), batch('05101', 'older', 'other'));
  await answer(historyUrl('05101'), [batch('05101', 'older', 'other')]);

  expect(element('job-status')).not.toHaveTextContent(/verificado/);
});

test('the selector lists the served catalogue and posts scenario_id only when one is chosen', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  const select = element<HTMLSelectElement>('scenario-select');
  expect(select).not.toBeDisabled();
  expect([...select.options].map(option => option.value)).toEqual(['', 'mild', 'dry-windy']);
  expect(fetchStub.callsFor(SCENARIOS_URL)).toHaveLength(1);

  // Legacy mode: the field must be OMITTED, never sent empty, because the server
  // rejects an explicit null/empty scenario_id with 400.
  fireEvent.click(element('create-job'));
  await flush();
  const legacyBody = JSON.parse(fetchStub.callsFor(JOBS_URL)[0].init.body as string);
  expect(legacyBody).toEqual({ zone_id: '05101' });
  expect(Object.hasOwn(legacyBody, 'scenario_id')).toBe(false);
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });

  // Selected scenario: the chosen id travels with the request.
  fireEvent.change(select, { target: { value: 'dry-windy' } });
  fireEvent.click(element('create-job'));
  await flush();
  const bodies = fetchStub.callsFor(JOBS_URL).map(call => JSON.parse(call.init.body as string));
  expect(bodies).toHaveLength(2);
  expect(bodies[1]).toEqual({ zone_id: '05101', scenario_id: 'dry-windy' });
});

test('a refused catalogue is never invented: the selector disables and the copy is honest', async () => {
  await connect();
  await mount();
  await answer(SCENARIOS_URL, { code: 'forbidden' }, 403);

  const select = element<HTMLSelectElement>('scenario-select');
  expect(select).toBeDisabled();
  expect([...select.options].map(option => option.value)).toEqual(['']);
  expect(element('scenario-note')).toHaveTextContent(/denegó el permiso para listar escenarios \(403\)/);
  expect(element('scenario-note')).toHaveTextContent(/fixture analítico heredado/);
});

test('a failed catalogue read falls back to the honest copy instead of a profile', async () => {
  await connect();
  await mount();
  await answer(SCENARIOS_URL, { code: 'unavailable' }, 500);

  expect(element<HTMLSelectElement>('scenario-select')).toBeDisabled();
  expect(element('scenario-note')).toHaveTextContent(/No fue posible cargar el catálogo de escenarios/);
});

test('the deadline stops a slow cycle before the attempt cap and reports an unconfirmed completion', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });

  // A 2.5 s read latency keeps every transport read inside its own 10 s bound
  // while the cycle as a whole crosses the 65 s view deadline after ~15 polls,
  // well before the 30-attempt cap. The view must stop and report the expiry.
  const queued = { id: 'job', zone_id: '05101', state: 'queued' };
  while (
    fetchStub.callsFor(JOB_URL).length < timing.maxPollAttempts &&
    !/agotado/.test(element('job-status').textContent ?? '')
  ) {
    await tick(2500);
    await answer(JOB_URL, queued);
    await tick(timing.pollIntervalMs);
  }

  const polls = fetchStub.callsFor(JOB_URL).length;
  expect(element('job-status')).toHaveTextContent(/agotado/);
  expect(element('job-status')).not.toHaveTextContent(/publicado/i);
  expect(element('job-status')).not.toHaveTextContent(/bajo/i);
  expect(element('refresh-job')).not.toBeDisabled();
  // The deadline, not the attempt cap, stopped the cycle, and no correlated
  // batch read was claimed.
  expect(polls).toBeLessThan(timing.maxPollAttempts);
  expect(fetchStub.callsFor(latestUrl('05101'))).toHaveLength(0);
});

test('manual refresh restarts a bounded poll and discards a job from another zone', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });
  await answer(JOB_URL, { id: 'job', zone_id: '05101', state: 'queued' });

  fireEvent.click(element('refresh-job'));
  await flush();
  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(2);

  await answer(JOB_URL, { id: 'job', zone_id: '05102', state: 'published', batch_id: 'wrong' });
  expect(element('job-status')).toHaveTextContent(/otro trabajo o zona/);
});

test('queued responses schedule at most maxPollAttempts polls', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });

  const queued = { id: 'job', zone_id: '05101', state: 'queued' };
  for (let attempt = 0; attempt < timing.maxPollAttempts; attempt += 1) {
    await answer(JOB_URL, queued);
    if (attempt < timing.maxPollAttempts - 1) await tick(timing.pollIntervalMs);
  }

  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(timing.maxPollAttempts);
  expect(element('job-status')).toHaveTextContent(/agotó/);
});

test('a create or poll answer for a superseded zone is discarded', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  act(() => selectZone('05102'));
  await answer(JOBS_URL, { id: 'old', zone_id: '05101', state: 'published', batch_id: 'old-batch' });
  expect(element('job-id')).not.toHaveTextContent('old');

  // A job for the new zone, then a selection move while its poll is in flight:
  // the late answer must not chain a correlated read for the old zone.
  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05102', state: 'queued' });
  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(1);
  act(() => selectZone('05101'));
  await answer(JOB_URL, { id: 'job', zone_id: '05102', state: 'published', batch_id: 'late' });
  expect(fetchStub.callsFor(latestUrl('05102'))).toHaveLength(0);
  expect(element('job-id')).not.toHaveTextContent('job');
});

test('a 403 reads as a permission denial, not as a missing service', async () => {
  await connect();
  await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { code: 'forbidden' }, 403);

  expect(element('job-status')).toHaveTextContent(/denegó el permiso para esta acción \(403\)/);
});

test('logout resets the selector and stops polling', async () => {
  await connect();
  await mount();
  await serveCatalogue();
  fireEvent.change(element<HTMLSelectElement>('scenario-select'), { target: { value: 'mild' } });

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });

  act(() => resetSession());
  await flush();

  const select = element<HTMLSelectElement>('scenario-select');
  expect(select).toBeDisabled();
  expect([...select.options].map(option => option.value)).toEqual(['']);
  expect(select.value).toBe('');
  expect(element('job-id')).toHaveTextContent('—');

  await tick(timing.pollIntervalMs);
  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(1);
});

test('unmounting stops the poll timer', async () => {
  await connect();
  const { unmount } = await mount();
  await serveCatalogue();

  fireEvent.click(element('create-job'));
  await flush();
  await answer(JOBS_URL, { id: 'job', zone_id: '05101', state: 'queued' });
  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(1);

  unmount();
  await tick(timing.pollIntervalMs * 2);
  expect(fetchStub.callsFor(JOB_URL)).toHaveLength(1);
});
