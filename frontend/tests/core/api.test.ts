import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import {
  audit,
  createJob,
  history,
  job,
  jobsStatus,
  latest,
  ready,
  riskStatus,
  scenarios,
  sources,
  zones,
  parseScenarios,
} from '../../src/core/api';
import type { Failure, Outcome } from '../../src/core/api';
import { completeLogin, getAccessToken, getSessionSnapshot, resetSession } from '../../src/core/session';
import type { Job, RiskBatch, SourceProvenance } from '../../src/types/api';

import { createCryptoStub, createFetchStub, createStorageStub, createWindowStub, flush, jwt } from './harness';

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;

const REQUEST_DEADLINE_MS = 10_000;

function expectFailure<T>(outcome: Outcome<T>): Failure {
  if (outcome.ok) throw new Error('expected a failure outcome');
  return outcome.failure;
}

function batch(zone = '05101', id = 'batch-1', jobId = 'job-1'): RiskBatch {
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

function jobFixture(zone = '05101', id = 'job-1'): Job {
  return { id, zone_id: zone, state: 'queued' };
}

function snapshot() {
  return {
    ready: true,
    reason: '',
    checked_at: '2024-01-01T00:00:00Z',
    dependencies: {
      database: { state: 'ready' },
      broker: { state: 'ready' },
      dead_letter: { state: 'ready', max_age_seconds: 30, depth: 0 },
    },
  };
}

function sourceBody(): Record<string, unknown> {
  return {
    geography: {
      provenance: {
        provider: 'SUBDERE',
        attribution: 'Atribución literal',
        restriction: 'Restricción literal',
        record_count: 38,
        derived_sha256: 'c'.repeat(64),
      },
      source_cutoff_at: null,
      cutoff_semantics: 'retrieval_substitution: stored cutoff equals retrieved',
      integrity: { algorithm: 'sha256', recorded_sha256: 'c'.repeat(64), computed_sha256: 'c'.repeat(64), matches: true },
    },
    risk_input: { id: 'academic', display_name: 'Perfil académico', synthetic: true },
  };
}

function prepare() {
  const fetchStub = createFetchStub();
  const storageStub = createStorageStub();
  const windowStub = createWindowStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  vi.stubGlobal('crypto', createCryptoStub());
  vi.stubGlobal('window', windowStub.window);
  return { fetchStub, storageStub, windowStub };
}

async function connectedSession(): Promise<ReturnType<typeof prepare> & { token: string }> {
  const context = prepare();
  const token = jwt({ realm_access: { roles: ['viewer'] } });
  context.storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'v' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  context.fetchStub.answer(TOKEN_URL, { access_token: token });
  await login;
  context.fetchStub.calls.length = 0;
  return { ...context, token };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

test('every gateway read is absolute on the gateway origin with the bearer, omit credentials and no-store', async () => {
  const { fetchStub, token } = await connectedSession();
  const readyRead = ready();
  const zonesRead = zones();
  const latestRead = latest('05101');
  const historyRead = history('05101');
  const sourcesRead = sources();
  const riskRead = riskStatus();
  const jobsRead = jobsStatus();
  const scenariosRead = scenarios();
  const auditRead = audit();
  const jobRead = job('job-1');
  const createRead = createJob({ zoneId: '05101', idempotencyKey: 'key-1' });
  await flush();

  const expected = [
    '/health/ready',
    '/v1/risk/zones',
    '/v1/risk/latest?zone_id=05101',
    '/v1/risk/history?zone_id=05101',
    '/v1/risk/sources',
    '/v1/risk/status',
    '/v1/jobs/status',
    '/v1/risk/synthetic-scenarios',
    '/v1/audit',
    '/v1/jobs/job-1',
    '/v1/jobs',
  ];
  for (const path of expected) {
    expect(fetchStub.calls.map((call) => call.url)).toContain(`${gatewayOrigin}${path}`);
  }
  for (const call of fetchStub.calls) {
    expect(call.init.credentials).toBe('omit');
    expect(call.init.cache).toBe('no-store');
  }
  const readyCall = fetchStub.callsFor(`${gatewayOrigin}/health/ready`)[0];
  expect(readyCall.init.headers).not.toHaveProperty('Authorization');
  const zonesCall = fetchStub.callsFor(`${gatewayOrigin}/v1/risk/zones`)[0];
  expect((zonesCall.init.headers as Record<string, string>).Authorization).toBe(`Bearer ${token}`);

  fetchStub.answer(`${gatewayOrigin}/health/ready`, null, 204);
  fetchStub.answer(`${gatewayOrigin}/v1/risk/zones`, { type: 'FeatureCollection', features: [] });
  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, batch());
  fetchStub.answer(`${gatewayOrigin}/v1/risk/history?zone_id=05101`, []);
  fetchStub.answer(`${gatewayOrigin}/v1/risk/sources`, sourceBody());
  fetchStub.answer(`${gatewayOrigin}/v1/risk/status`, snapshot());
  fetchStub.answer(`${gatewayOrigin}/v1/jobs/status`, snapshot());
  fetchStub.answer(`${gatewayOrigin}/v1/risk/synthetic-scenarios`, []);
  fetchStub.answer(`${gatewayOrigin}/v1/audit`, []);
  fetchStub.answer(`${gatewayOrigin}/v1/jobs/job-1`, jobFixture());
  fetchStub.answer(`${gatewayOrigin}/v1/jobs`, jobFixture());
  await Promise.all([readyRead, zonesRead, latestRead, historyRead, sourcesRead, riskRead, jobsRead, scenariosRead, auditRead, jobRead, createRead]);
});

test('ready accepts only a 2xx gateway answer and never claims the pipeline', async () => {
  const { fetchStub } = await connectedSession();
  const available = ready();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/health/ready`, null, 204);
  expect(await available).toEqual({ ok: true, data: null });

  const unavailable = ready();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/health/ready`, { code: 'down' }, 503, 'req-down');
  const failure = expectFailure(await unavailable);
  expect(failure.kind).toBe('unavailable');
  expect(failure.reason).toBe('down');
  expect(failure.status).toBe(503);
  expect(failure.requestId).toBe('req-down');
});

test('latest reports an absent published batch for a 404 and never a low level', async () => {
  const { fetchStub } = await connectedSession();
  const outcome = latest('05101');
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, { code: 'not_found' }, 404);
  expect(await outcome).toEqual({ ok: true, data: null });
});

test('job reports a missing job distinctly from an absent batch', async () => {
  const { fetchStub } = await connectedSession();
  const outcome = job('job-1');
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/jobs/job-1`, { code: 'not_found' }, 404);
  const failure = expectFailure(await outcome);
  expect(failure.kind).toBe('missing');
  expect(failure.status).toBe(404);
});

test('a shape that fails its guard is malformed and never a partial value', async () => {
  const { fetchStub } = await connectedSession();

  const zonesRead = zones();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/zones`, { type: 'FeatureCollection', features: 'not-an-array' });
  expect(expectFailure(await zonesRead).kind).toBe('malformed');

  const latestRead = latest('05101');
  await flush();
  const broken = batch();
  delete (broken as { policy?: unknown }).policy;
  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, broken);
  expect(expectFailure(await latestRead).kind).toBe('malformed');

  const historyRead = history('05101');
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/history?zone_id=05101`, [batch(), { zone_id: '05101' }]);
  expect(expectFailure(await historyRead).kind).toBe('malformed');

  const auditRead = audit();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/audit`, [{ occurred_at: '2024-01-01T00:00:00Z' }]);
  expect(expectFailure(await auditRead).kind).toBe('malformed');

  const scenariosRead = scenarios();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/synthetic-scenarios`, { not: 'an array' });
  expect(expectFailure(await scenariosRead).kind).toBe('malformed');

  const statusRead = riskStatus();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/status`, { checked_at: '2024-01-01T00:00:00Z', ready: true });
  expect(expectFailure(await statusRead).kind).toBe('malformed');
});

test('malformed JSON is reported as malformed, never as a value', async () => {
  const { fetchStub } = await connectedSession();
  const outcome = zones();
  await flush();
  fetchStub.answerMalformedJson(`${gatewayOrigin}/v1/risk/zones`);
  const failure = expectFailure(await outcome);
  expect(failure.kind).toBe('malformed');
  expect(failure.reason).toBe('malformed_json');
});

test('a batch from another zone is discarded as malformed, never rendered', async () => {
  const { fetchStub } = await connectedSession();

  const latestRead = latest('05101');
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, batch('05102'));
  const latestFailure = expectFailure(await latestRead);
  expect(latestFailure.kind).toBe('malformed');
  expect(latestFailure.reason).toBe('zone_mismatch');

  const historyRead = history('05101');
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/history?zone_id=05101`, [batch('05101'), batch('05102')]);
  const historyFailure = expectFailure(await historyRead);
  expect(historyFailure.kind).toBe('malformed');
  expect(historyFailure.reason).toBe('zone_mismatch');
});

test('401 resets the session and reports unauthorized', async () => {
  const { fetchStub } = await connectedSession();
  const outcome = zones();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/zones`, { code: 'unauthorized' }, 401, 'req-401');
  const failure = expectFailure(await outcome);
  expect(failure).toEqual({ kind: 'unauthorized', reason: 'unauthorized', status: 401, requestId: 'req-401' });
  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(getAccessToken()).toBe('');
});

test('403 is a permission denial, never unavailable, and keeps the session', async () => {
  const { fetchStub } = await connectedSession();
  const outcome = riskStatus();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/status`, { code: 'forbidden' }, 403);
  const failure = expectFailure(await outcome);
  expect(failure.kind).toBe('forbidden');
  expect(failure.status).toBe(403);
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect(getAccessToken()).not.toBe('');
});

test('a superseded response for the same route is discarded instead of rendered', async () => {
  const { fetchStub } = await connectedSession();
  const first = latest('05101');
  const second = latest('05102');
  await flush();

  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05102`, batch('05102', 'new'));
  expect(await second).toEqual({ ok: true, data: batch('05102', 'new') });

  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, batch('05101', 'old'));
  const failure = expectFailure(await first);
  expect(failure.kind).toBe('unavailable');
  expect(failure.reason).toBe('superseded');
});

test('a superseded unauthorized response cannot reset the current session', async () => {
  const { fetchStub } = await connectedSession();
  const first = latest('05101');
  const second = latest('05102');
  await flush();

  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, { code: 'unauthorized' }, 401);
  const failure = expectFailure(await first);
  expect(failure.reason).toBe('superseded');
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect(getAccessToken()).not.toBe('');

  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05102`, batch('05102'));
  expect((await second).ok).toBe(true);
});

test('a response that lands after the view was reset is discarded', async () => {
  const { fetchStub } = await connectedSession();
  const callCountBefore = fetchStub.calls.length;
  const pendingRead = latest('05101');
  await flush();

  resetSession();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/latest?zone_id=05101`, batch('05101'));

  const failure = expectFailure(await pendingRead);
  expect(failure.reason).toBe('superseded');
  expect(fetchStub.calls.length).toBe(callCountBefore + 1);
});

test('a hung read is bounded by the deadline and reports it', async () => {
  const { fetchStub } = await connectedSession();
  vi.useFakeTimers();
  const outcome = latest('05101');
  await flush();
  await vi.advanceTimersByTimeAsync(REQUEST_DEADLINE_MS);
  const failure = expectFailure(await outcome);
  expect(failure.kind).toBe('unavailable');
  expect(failure.reason).toBe('deadline');
  expect(fetchStub.calls).toHaveLength(1);
});

test('an aborted read reports the abort and resolves no data', async () => {
  const { fetchStub } = await connectedSession();
  const controller = new AbortController();
  const outcome = zones(controller.signal);
  await flush();
  expect(fetchStub.calls).toHaveLength(1);
  controller.abort();
  const failure = expectFailure(await outcome);
  expect(failure.kind).toBe('unavailable');
  expect(failure.reason).toBe('aborted');
});

test('createJob posts only the chosen fields with the idempotency header', async () => {
  const { fetchStub } = await connectedSession();

  const legacy = createJob({ zoneId: '05101', idempotencyKey: 'key-1' });
  await flush();
  const legacyCall = fetchStub.callsFor(`${gatewayOrigin}/v1/jobs`)[0];
  expect(legacyCall.init.method).toBe('POST');
  expect(legacyCall.init.headers).toMatchObject({ 'Content-Type': 'application/json', 'Idempotency-Key': 'key-1' });
  expect(JSON.parse(legacyCall.init.body as string)).toEqual({ zone_id: '05101' });
  fetchStub.answer(`${gatewayOrigin}/v1/jobs`, jobFixture());
  expect((await legacy).ok).toBe(true);

  const selected = createJob({ zoneId: '05101', scenarioId: 'dry-windy', idempotencyKey: 'key-2' });
  await flush();
  const selectedCall = fetchStub.callsFor(`${gatewayOrigin}/v1/jobs`)[1];
  expect(selectedCall.init.headers).toMatchObject({ 'Idempotency-Key': 'key-2' });
  expect(JSON.parse(selectedCall.init.body as string)).toEqual({ zone_id: '05101', scenario_id: 'dry-windy' });
  fetchStub.answer(`${gatewayOrigin}/v1/jobs`, jobFixture());
  expect((await selected).ok).toBe(true);

  const empty = createJob({ zoneId: '05101', scenarioId: '', idempotencyKey: 'key-3' });
  await flush();
  const emptyCall = fetchStub.callsFor(`${gatewayOrigin}/v1/jobs`)[2];
  expect(Object.hasOwn(JSON.parse(emptyCall.init.body as string), 'scenario_id')).toBe(false);
  fetchStub.answer(`${gatewayOrigin}/v1/jobs`, jobFixture());
  await empty;
});

test('sources passes the served provenance through and the domain decides its integrity', async () => {
  const { fetchStub } = await connectedSession();

  const okRead = sources();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/sources`, sourceBody());
  const okResult = await okRead;
  if (!okResult.ok) throw new Error('expected a usable provenance');
  const provenance: SourceProvenance = okResult.data;
  expect(provenance.geography.provenance.provider).toBe('SUBDERE');
  expect(provenance.geography.provenance.attribution).toBe('Atribución literal');
  expect(provenance.geography.provenance.restriction).toBe('Restricción literal');
  expect(provenance.geography.provenance.record_count).toBe(38);
  expect(provenance.geography.provenance.derived_sha256).toBe('c'.repeat(64));
  expect(provenance.geography.integrity.matches).toBe(true);
  expect(provenance.geography.cutoff_semantics).toContain('retrieval_substitution');

  // An integrity mismatch is a domain decision, not a transport failure: the
  // payload travels unchanged so the panel can fail closed on it.
  const mismatchBody = sourceBody();
  (mismatchBody.geography as { integrity: { matches: boolean } }).integrity.matches = false;
  const mismatchRead = sources();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/sources`, mismatchBody);
  const mismatchResult = await mismatchRead;
  if (!mismatchResult.ok) throw new Error('the transport must not decide integrity');
  expect(mismatchResult.data.geography.integrity.matches).toBe(false);

  const malformedRead = sources();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/sources`, { geography: {} });
  expect(expectFailure(await malformedRead).kind).toBe('malformed');
});

test('scenarios publishes only usable catalogue entries', () => {
  expect(parseScenarios('not-an-array')).toEqual([]);
  expect(parseScenarios([{ id: 'mild', description: 'Perfil suave' }, { id: '' }, { description: 'sin id' }, null, 42])).toEqual([
    { id: 'mild', description: 'Perfil suave' },
  ]);
  expect(parseScenarios([{ id: 'no-description' }])).toEqual([{ id: 'no-description', description: '' }]);
});
