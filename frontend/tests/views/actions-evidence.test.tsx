import { act, render, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, resetSession } from '../../src/core/session';
import { ActionsView } from '../../src/views/ActionsView';

import { createCryptoStub, createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// S2 / T3 — the preventive-evidence, human-review and synthetic-reassessment half
// of the actions view. The transport and the session are stubbed exactly as the
// rest of the suite; the view is exercised end to end: the admin-only evidence
// form, the human decision controls, the acceptance guard and the honest,
// retryable rendering of a reassessment link.

const ACTIONS_URL = `${gatewayOrigin}/v1/prevention/actions`;
const JOBS_URL = `${gatewayOrigin}/v1/jobs`;
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const ADMIN_TOKEN = jwt({ realm_access: { roles: ['admin'] } });
const VIEWER_TOKEN = jwt({ realm_access: { roles: ['viewer'] } });

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

async function connect(token: string): Promise<void> {
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'verifier' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: token, id_token: 'id-token' });
  await login;
  fetchStub.calls.length = 0;
}

function actionFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'action-1',
    community_code: 'valparaiso-demo',
    title: 'Despeje de vegetación',
    description: 'Franja de 30 m junto al camino',
    priority: 'alta',
    status: 'enviada',
    assignee: 'equipo-1',
    due_date: '2026-11-01',
    created_by: 'actor-1',
    created_at: '2026-10-17T10:00:00Z',
    updated_at: '2026-10-17T10:00:00Z',
    ...overrides,
  };
}

function evidenceFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 5,
    action_id: 'action-1',
    criterion: 'Acta de la junta y fotografías de la faja',
    references: ['https://example.org/acta'],
    created_by: 'admin-1',
    created_at: '2026-10-18T12:00:00Z',
    ...overrides,
  };
}

function detailFixture(sufficient: boolean, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    ...actionFixture(overrides),
    history: [
      {
        id: 1,
        action_id: 'action-1',
        from_status: 'propuesta',
        to_status: 'asignada',
        actor_sub: 'actor-1',
        note: 'alta inicial',
        created_at: '2026-10-17T10:00:00Z',
      },
    ],
    evidence: [evidenceFixture()],
    evidence_reviews: sufficient
      ? [
          {
            id: 1,
            action_id: 'action-1',
            evidence_id: 5,
            decision: 'suficiente',
            reviewer_sub: 'reviewer-1',
            note: 'acta revisada',
            reviewed_at: '2026-10-18T12:05:00Z',
          },
        ]
      : [],
    reassessments: [],
    evidence_sufficient: sufficient,
  };
}

async function answerList(body: unknown, status = 200, requestId = ''): Promise<void> {
  await act(async () => {
    fetchStub.answer(ACTIONS_URL, body, status, requestId);
    await flush();
  });
}

async function answerDetail(body: unknown, status = 200, requestId = ''): Promise<void> {
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1`, body, status, requestId);
    await flush();
  });
}

function text(id: string): string {
  return document.getElementById(id)?.textContent ?? '';
}

async function openAdminDetail(detail: Record<string, unknown>): Promise<void> {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detail);
}

beforeEach(setup);

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the evidence form posts the criterion and references to the action', async () => {
  await openAdminDetail(detailFixture(false));

  await userEvent.type(document.getElementById('evidence-criterion') as HTMLElement, 'Acta y fotografías');
  await userEvent.type(
    document.getElementById('evidence-references') as HTMLElement,
    'https://example.org/acta{enter}https://example.org/foto',
  );
  await userEvent.click(document.getElementById('record-evidence') as HTMLButtonElement);

  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/evidence`)).toHaveLength(1));
  const post = fetchStub.callsFor(`${ACTIONS_URL}/action-1/evidence`)[0];
  expect(JSON.parse(String(post.init.body))).toEqual({
    criterion: 'Acta y fotografías',
    references: ['https://example.org/acta', 'https://example.org/foto'],
  });

  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/evidence`, evidenceFixture(), 201);
    await flush();
  });
  expect(text('actions-notice')).toMatch(/evidencia registrada/i);
  // Recording new evidence empties the form and re-reads the server detail.
  expect((document.getElementById('evidence-criterion') as HTMLTextAreaElement).value).toBe('');
});

test('the evidence form refuses a non-http reference before calling the API', async () => {
  await openAdminDetail(detailFixture(false));

  await userEvent.type(document.getElementById('evidence-criterion') as HTMLElement, 'Acta');
  await userEvent.type(document.getElementById('evidence-references') as HTMLElement, 'ftp://example.org/acta');
  await userEvent.click(document.getElementById('record-evidence') as HTMLButtonElement);

  expect(text('actions-notice')).toMatch(/no es una referencia válida/i);
  expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/evidence`)).toHaveLength(0);
});

test('a viewer sees the evidence panels read-only', async () => {
  await connect(VIEWER_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture(true));

  expect((document.getElementById('record-evidence') as HTMLButtonElement).disabled).toBe(true);
  expect((document.getElementById('review-suficiente') as HTMLButtonElement).disabled).toBe(true);
  expect((document.getElementById('request-reassessment') as HTMLButtonElement).disabled).toBe(true);
  // The read still works: the snapshot and the sufficient decision are visible.
  expect(text('action-evidence-rows')).toContain('Acta de la junta');
  expect(text('action-evidence-review-rows')).toContain('Suficiente');
});

test('acceptance is disabled with the reason until the server reports sufficient evidence', async () => {
  await openAdminDetail(detailFixture(false));

  const accept = document.querySelector('[data-transition-to="aceptada"]') as HTMLButtonElement;
  expect(accept.disabled).toBe(true);
  expect(accept.getAttribute('aria-describedby')).toBe('actions-acceptance-note');
  expect(text('actions-acceptance-note')).toMatch(/suficiencia/i);
  expect(text('actions-sufficiency')).toMatch(/insuficiente/i);
});

test('acceptance is enabled once the server reports sufficient evidence', async () => {
  await openAdminDetail(detailFixture(true));

  const accept = document.querySelector('[data-transition-to="aceptada"]') as HTMLButtonElement;
  expect(accept.disabled).toBe(false);
  expect(document.getElementById('actions-acceptance-note')).toBeNull();
  expect(text('actions-sufficiency')).toMatch(/suficiente/i);
});

test('a human review posts the decision and note to the current snapshot', async () => {
  await openAdminDetail(detailFixture(false));

  await userEvent.type(document.getElementById('review-note') as HTMLElement, 'revisado por la brigada');
  await userEvent.click(document.getElementById('review-suficiente') as HTMLButtonElement);

  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/evidence/5/reviews`)).toHaveLength(1));
  const post = fetchStub.callsFor(`${ACTIONS_URL}/action-1/evidence/5/reviews`)[0];
  expect(JSON.parse(String(post.init.body))).toEqual({ decision: 'suficiente', note: 'revisado por la brigada' });

  await act(async () => {
    fetchStub.answer(
      `${ACTIONS_URL}/action-1/evidence/5/reviews`,
      {
        id: 2,
        action_id: 'action-1',
        evidence_id: 5,
        decision: 'suficiente',
        reviewer_sub: 'reviewer-2',
        note: 'revisado por la brigada',
        reviewed_at: '2026-10-18T12:06:00Z',
      },
      201,
    );
    await flush();
  });
  expect(text('actions-notice')).toMatch(/decisión registrada/i);
});

test('a 409 insufficient_evidence is rendered with its code and request id', async () => {
  await openAdminDetail(detailFixture(true));

  await userEvent.click(document.querySelector('[data-transition-to="aceptada"]') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/transitions`, { code: 'insufficient_evidence' }, 409, 'req-insufficient');
    await flush();
  });

  expect(text('actions-notice')).toMatch(/409/);
  expect(text('actions-notice')).toMatch(/insufficient_evidence/);
  expect(text('actions-notice')).toMatch(/req-insufficient/);
  expect(text('actions-notice')).toMatch(/no cambió/i);
});

test('a synthetic reassessment links the real published batch and states the disclaimer', async () => {
  await openAdminDetail(detailFixture(true));

  expect(text('action-reassessment-disclaimer')).toMatch(/sintétic/i);
  expect(text('action-reassessment-disclaimer')).toMatch(/no es prueba causal/i);

  await userEvent.click(document.getElementById('request-reassessment') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(JOBS_URL)).toHaveLength(1));
  const create = fetchStub.callsFor(JOBS_URL)[0];
  // Only the commune travels: no batch, version or risk level is ever sent.
  expect(JSON.parse(String(create.init.body))).toEqual({ zone_id: 'valparaiso-demo' });

  await act(async () => {
    fetchStub.answer(JOBS_URL, { id: 'job-1', zone_id: 'valparaiso-demo', state: 'requested' }, 201);
    await flush();
  });
  await waitFor(() => expect(fetchStub.callsFor(`${JOBS_URL}/job-1`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${JOBS_URL}/job-1`, {
      id: 'job-1',
      zone_id: 'valparaiso-demo',
      state: 'published',
      batch_id: 'batch-1',
    });
    await flush();
  });

  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/reassessments`)).toHaveLength(1));
  const link = fetchStub.callsFor(`${ACTIONS_URL}/action-1/reassessments`)[0];
  expect(JSON.parse(String(link.init.body))).toEqual({ job_id: 'job-1' });

  await act(async () => {
    fetchStub.answer(
      `${ACTIONS_URL}/action-1/reassessments`,
      {
        id: 1,
        action_id: 'action-1',
        job_id: 'job-1',
        batch_id: 'batch-1',
        version: 'academic-rule-v1',
        risk_level: 'medio',
        zone_id: 'valparaiso-demo',
        synthetic: true,
        causal_proof: false,
        batch_published_at: '2026-10-18T12:10:00Z',
        linked_by: 'admin-1',
        linked_at: '2026-10-18T12:11:00Z',
      },
      201,
    );
    await flush();
  });

  await waitFor(() => expect(text('reassessment-status')).toMatch(/vinculada al lote batch-1/i));
  // The detail is re-read after the link so the table reflects the server.
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`).length).toBeGreaterThanOrEqual(2));
  await answerDetail(detailFixture(true));
  await answerList([actionFixture()]);
});

test('a failed reassessment link keeps a retryable state and never claims association', async () => {
  await openAdminDetail(detailFixture(true));

  await userEvent.click(document.getElementById('request-reassessment') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(JOBS_URL)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(JOBS_URL, { id: 'job-1', zone_id: 'valparaiso-demo', state: 'requested' }, 201);
    await flush();
  });
  await waitFor(() => expect(fetchStub.callsFor(`${JOBS_URL}/job-1`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${JOBS_URL}/job-1`, {
      id: 'job-1',
      zone_id: 'valparaiso-demo',
      state: 'published',
      batch_id: 'batch-1',
    });
    await flush();
  });
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/reassessments`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/reassessments`, { code: 'reassessment_unavailable' }, 409, 'req-409');
    await flush();
  });

  await waitFor(() => expect(document.getElementById('retry-reassessment')).not.toBeNull());
  expect(text('reassessment-status')).toMatch(/reintentar/i);
  expect(text('reassessment-status')).toMatch(/no se registró ninguna asociación/i);
  expect(text('reassessment-status')).not.toMatch(/vinculada al lote/i);

  // The retry reuses the same job: it never requests a second one.
  await userEvent.click(document.getElementById('retry-reassessment') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(`${JOBS_URL}/job-1`).length).toBeGreaterThanOrEqual(2));
  expect(fetchStub.callsFor(JOBS_URL)).toHaveLength(1);
});

// ── Originating-action fences ─────────────────────────────────────────────
//
// A reassessment cycle belongs to the action that started it. Selecting another
// action must clear the local cycle, the retry must never send the old job to
// the new action, and a late response for the old action must never render on
// the new one. These are local-observation rules only: the server association for
// the originating action is never cancelled.

async function answerDetailFor(id: string, body: unknown, status = 200, requestId = ''): Promise<void> {
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/${id}`, body, status, requestId);
    await flush();
  });
}

async function answerJob(id: string, body: unknown, status = 200, requestId = ''): Promise<void> {
  await act(async () => {
    fetchStub.answer(`${JOBS_URL}/${id}`, body, status, requestId);
    await flush();
  });
}

async function openAction(id: string, detail: Record<string, unknown>): Promise<void> {
  await userEvent.click(document.querySelector(`[data-action-id="${id}"]`) as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/${id}`)).toHaveLength(1));
  await answerDetailFor(id, detail);
}

test('a reassessment cycle is cleared when another action is selected and can never link its job to that action', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture({ id: 'action-a' }), actionFixture({ id: 'action-b', title: 'Charla' })]);

  // A starts a reassessment whose polling fails, leaving a retryable retained job.
  await openAction('action-a', detailFixture(true, { id: 'action-a' }));
  await userEvent.click(document.getElementById('request-reassessment') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(JOBS_URL)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(JOBS_URL, { id: 'job-a', zone_id: 'valparaiso-demo', state: 'requested' }, 201);
    await flush();
  });
  await waitFor(() => expect(fetchStub.callsFor(`${JOBS_URL}/job-a`)).toHaveLength(1));
  await answerJob('job-a', { code: 'storage_unavailable' }, 503, 'req-503');
  expect(document.getElementById('retry-reassessment')).not.toBeNull();

  // Selecting B must clear the cycle. If any retry control still leaks through,
  // clicking it must never post A's job to B. The retry polls the originating
  // job, so the poll is answered as published to expose the mutation boundary.
  await openAction('action-b', detailFixture(true, { id: 'action-b' }));
  const retry = document.getElementById('retry-reassessment');
  if (retry !== null) {
    await userEvent.click(retry as HTMLButtonElement);
    await flush();
    await act(async () => {
      fetchStub.answer(`${JOBS_URL}/job-a`, {
        id: 'job-a',
        zone_id: 'valparaiso-demo',
        state: 'published',
        batch_id: 'batch-a',
      });
      await flush();
    });
  }
  expect(fetchStub.calls.some(call => call.url === `${ACTIONS_URL}/action-b/reassessments`)).toBe(false);
  expect(text('reassessment-status')).toBe('');
  expect(document.getElementById('retry-reassessment')).toBeNull();
});

test('a late link response for the originating action never changes the newly selected action', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture({ id: 'action-a' }), actionFixture({ id: 'action-b', title: 'Charla' })]);

  // A reaches the link request (a real published batch). The request stays
  // unanswered, so the late response can be delivered after B is selected.
  await openAction('action-a', detailFixture(true, { id: 'action-a' }));
  await userEvent.click(document.getElementById('request-reassessment') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(JOBS_URL)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(JOBS_URL, { id: 'job-a', zone_id: 'valparaiso-demo', state: 'requested' }, 201);
    await flush();
  });
  await waitFor(() => expect(fetchStub.callsFor(`${JOBS_URL}/job-a`)).toHaveLength(1));
  await answerJob('job-a', { id: 'job-a', zone_id: 'valparaiso-demo', state: 'published', batch_id: 'batch-a' });
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-a/reassessments`)).toHaveLength(1));
  // The link request names the originating action and its job, never another one.
  const link = fetchStub.callsFor(`${ACTIONS_URL}/action-a/reassessments`)[0];
  expect(JSON.parse(String(link.init.body))).toEqual({ job_id: 'job-a' });

  await openAction('action-b', detailFixture(true, { id: 'action-b' }));
  const detailReadsBefore = fetchStub.callsFor(`${ACTIONS_URL}/action-b`).length;

  // The late success is for A: it must not be rendered on B nor refresh B.
  await act(async () => {
    fetchStub.answer(
      `${ACTIONS_URL}/action-a/reassessments`,
      {
        id: 1,
        action_id: 'action-a',
        job_id: 'job-a',
        batch_id: 'batch-a',
        version: 'academic-rule-v1',
        risk_level: 'medio',
        zone_id: 'valparaiso-demo',
        synthetic: true,
        causal_proof: false,
        batch_published_at: '2026-10-18T12:10:00Z',
        linked_by: 'admin-1',
        linked_at: '2026-10-18T12:11:00Z',
      },
      201,
    );
    await flush();
  });

  expect(text('reassessment-status')).not.toMatch(/vinculada al lote/i);
  expect(text('reassessment-status')).toBe('');
  expect(fetchStub.callsFor(`${ACTIONS_URL}/action-b`)).toHaveLength(detailReadsBefore);
  expect(fetchStub.calls.some(call => call.url === `${ACTIONS_URL}/action-b/reassessments`)).toBe(false);
});
