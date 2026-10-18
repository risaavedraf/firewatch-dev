import { afterEach, beforeEach, expect, test } from 'vitest';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import {
  linkPreventiveReassessment,
  preventiveAction,
  recordPreventiveEvidence,
  reviewPreventiveEvidence,
  type Failure,
  type Outcome,
} from '../../src/core/api';
import { completeLogin, resetSession } from '../../src/core/session';
import type { PreventiveReassessment } from '../../src/types/api';

import { createCryptoStub, createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// S2 transport: the three write routes under the frozen prevention prefix, their
// exact bodies, and the fail-closed validators for the new records. The client
// never sends a batch, version, risk level or reviewer identity.

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const ACTIONS_URL = `${gatewayOrigin}/v1/prevention/actions`;
const ACTION_ID = 'b22314ca-6dd8-489c-a265-2990244cb78a';

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

async function connect(): Promise<string> {
  const token = jwt({ realm_access: { roles: ['admin'] } });
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

function evidence(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 1,
    action_id: ACTION_ID,
    criterion: 'Acta de la junta',
    references: ['https://example.org/acta'],
    created_by: 'admin-1',
    created_at: '2026-10-18T12:00:00Z',
    ...overrides,
  };
}

function review(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 1,
    action_id: ACTION_ID,
    evidence_id: 7,
    decision: 'suficiente',
    reviewer_sub: 'reviewer-1',
    note: 'revisado',
    reviewed_at: '2026-10-18T12:05:00Z',
    ...overrides,
  };
}

function reassessment(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 1,
    action_id: ACTION_ID,
    job_id: '6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44',
    batch_id: 'b1a2c3d4-1111-2222-3333-444455556666',
    version: 'academic-rule-v1',
    risk_level: 'medio',
    zone_id: 'valparaiso-demo',
    synthetic: true,
    causal_proof: false,
    batch_published_at: '2026-10-18T12:10:00Z',
    linked_by: 'admin-1',
    linked_at: '2026-10-18T12:11:00Z',
    ...overrides,
  };
}

beforeEach(setup);

afterEach(() => {
  vi.unstubAllGlobals();
});

test('evidence posts criterion and references with the bearer and no client identity', async () => {
  const token = await connect();
  const outcome = recordPreventiveEvidence(ACTION_ID, {
    criterion: 'Acta de la junta',
    references: ['https://example.org/acta', 'http://example.org/foto'],
  });
  await flush();

  const url = `${ACTIONS_URL}/${ACTION_ID}/evidence`;
  const call = fetchStub.callsFor(url)[0];
  expect(call.init.method).toBe('POST');
  expect((call.init.headers as Record<string, string>).Authorization).toBe(`Bearer ${token}`);
  expect(JSON.parse(String(call.init.body))).toEqual({
    criterion: 'Acta de la junta',
    references: ['https://example.org/acta', 'http://example.org/foto'],
  });

  fetchStub.answer(url, evidence(), 201);
  const settled = await outcome;
  expect(settled.ok).toBe(true);
  if (settled.ok) expect(settled.data.id).toBe(1);
});

test('evidence fails closed on a record without a criterion or references', async () => {
  await connect();
  const missingCriterion = recordPreventiveEvidence(ACTION_ID, { criterion: '', references: ['https://example.org/a'] });
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}/evidence`, evidence({ criterion: '' }), 201);
  expect(expectFailure(await missingCriterion).kind).toBe('malformed');

  const missingReferences = recordPreventiveEvidence(ACTION_ID, { criterion: 'acta', references: [] });
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}/evidence`, evidence({ references: [] }), 201);
  expect(expectFailure(await missingReferences).kind).toBe('malformed');
});

test('a review posts only {decision, note} to the named snapshot and rejects a mismatched evidence id', async () => {
  await connect();
  const outcome = reviewPreventiveEvidence(ACTION_ID, 7, { decision: 'suficiente', note: 'revisado' });
  await flush();

  const url = `${ACTIONS_URL}/${ACTION_ID}/evidence/7/reviews`;
  const call = fetchStub.callsFor(url)[0];
  expect(call.init.method).toBe('POST');
  // The reviewer identity and the date are server-set: the body carries neither.
  expect(JSON.parse(String(call.init.body))).toEqual({ decision: 'suficiente', note: 'revisado' });

  fetchStub.answer(url, review(), 201);
  const settled = await outcome;
  expect(settled.ok).toBe(true);
  if (settled.ok) expect(settled.data.decision).toBe('suficiente');

  const mismatched = reviewPreventiveEvidence(ACTION_ID, 7, { decision: 'suficiente' });
  await flush();
  fetchStub.answer(url, review({ evidence_id: 9 }), 201);
  expect(expectFailure(await mismatched).reason).toBe('evidence_mismatch');
});

test('a stale-snapshot 409 is surfaced with its code and request id untouched', async () => {
  await connect();
  const outcome = reviewPreventiveEvidence(ACTION_ID, 7, { decision: 'suficiente' });
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}/evidence/7/reviews`, { code: 'stale_evidence' }, 409, 'req-409');
  const failure = expectFailure(await outcome);
  expect(failure.status).toBe(409);
  expect(failure.reason).toBe('stale_evidence');
  expect(failure.requestId).toBe('req-409');
});

test('a reassessment posts only the job and derives everything from the server record', async () => {
  await connect();
  const outcome = linkPreventiveReassessment(ACTION_ID, { jobId: '6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44' });
  await flush();

  const url = `${ACTIONS_URL}/${ACTION_ID}/reassessments`;
  const call = fetchStub.callsFor(url)[0];
  expect(call.init.method).toBe('POST');
  expect(JSON.parse(String(call.init.body))).toEqual({ job_id: '6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44' });

  fetchStub.answer(url, reassessment(), 201);
  const settled = await outcome;
  expect(settled.ok).toBe(true);
  if (settled.ok) {
    expect(settled.data.batch_id).toBe('b1a2c3d4-1111-2222-3333-444455556666');
    expect(settled.data.synthetic).toBe(true);
    expect(settled.data.causal_proof).toBe(false);
  }
});

test('a reassessment record that claims causal proof is refused', async () => {
  await connect();
  const outcome = linkPreventiveReassessment(ACTION_ID, { jobId: '6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44' });
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}/reassessments`, reassessment({ causal_proof: true }), 201);
  expect(expectFailure(await outcome).kind).toBe('malformed');
});

test('a reassessment failure is surfaced with its code and request id', async () => {
  await connect();
  const outcome = linkPreventiveReassessment(ACTION_ID, { jobId: '6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44' });
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}/reassessments`, { code: 'reassessment_unavailable' }, 409, 'req-409');
  const failure = expectFailure(await outcome);
  expect(failure.status).toBe(409);
  expect(failure.reason).toBe('reassessment_unavailable');
  expect(failure.requestId).toBe('req-409');
});

test('the detail route requires the S2 collections and the boolean', async () => {
  await connect();
  const base = {
    id: ACTION_ID,
    community_code: '05101',
    title: 'Despeje',
    status: 'enviada',
    history: [],
  };

  const missing = preventiveAction(ACTION_ID);
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}`, base);
  expect(expectFailure(await missing).kind).toBe('malformed');

  const complete = preventiveAction(ACTION_ID);
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${ACTION_ID}`, {
    ...base,
    evidence: [evidence()],
    evidence_reviews: [review()],
    reassessments: [reassessment() as unknown as PreventiveReassessment],
    evidence_sufficient: true,
  });
  const settled = await complete;
  expect(settled.ok).toBe(true);
  if (settled.ok) {
    expect(settled.data.evidence).toHaveLength(1);
    expect(settled.data.evidence_sufficient).toBe(true);
  }
});

test('a client-supplied batch or version is never sent: the body has only job_id', async () => {
  await connect();
  const outcome = linkPreventiveReassessment(ACTION_ID, { jobId: '6f1e7b0a-2f4f-4a3f-9a5b-1d3c7e9f2a44' });
  await flush();
  const url = `${ACTIONS_URL}/${ACTION_ID}/reassessments`;
  const body = JSON.parse(String(fetchStub.callsFor(url)[0].init.body)) as Record<string, unknown>;
  expect(Object.keys(body)).toEqual(['job_id']);
  fetchStub.answer(url, reassessment(), 201);
  await outcome;
});
