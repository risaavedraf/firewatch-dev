import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { act, render, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, getSessionSnapshot, resetSession } from '../../src/core/session';
import { ActionsView } from '../../src/views/ActionsView';

import { createCryptoStub, createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// S3 — the actions table marks the row whose detail is open. jsdom applies no
// stylesheet, so the rule is read from the file the browser receives.
const appCss = readFileSync(resolve(process.cwd(), 'src', 'styles', 'app.css'), 'utf8');

// S3 / T12 — the preventive-action view against the frozen contract. The
// transport and the session are stubbed exactly as the rest of the suite; the
// view itself is exercised end to end: list, detail with its history, creation,
// role-gated transitions and the honest rendering of the server's refusals.

const ACTIONS_URL = `${gatewayOrigin}/v1/prevention/actions`;
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
    community_code: '05101',
    title: 'Despeje de vegetación',
    description: 'Franja de 30 m junto al camino',
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

function detailFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    ...actionFixture(overrides),
    history: [
      {
        id: 1,
        action_id: 'action-1',
        from_status: null,
        to_status: 'propuesta',
        actor_sub: 'actor-1',
        note: 'alta inicial',
        created_at: '2026-10-17T10:00:00Z',
      },
      {
        id: 2,
        action_id: 'action-1',
        from_status: 'propuesta',
        to_status: 'asignada',
        actor_sub: 'admin-1',
        note: 'asignada al equipo',
        created_at: '2026-10-17T11:00:00Z',
      },
    ],
    evidence: [],
    evidence_reviews: [],
    reassessments: [],
    evidence_sufficient: false,
  };
}

async function answerList(body: unknown, status = 200, requestId = ''): Promise<void> {  await act(async () => {
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

beforeEach(() => {
  setup();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the view carries its own identifiers and offers nothing before a session exists', async () => {
  const { container } = render(<ActionsView />);

  const section = container.querySelector('section#actions') as HTMLElement | null;
  expect(section).not.toBeNull();
  expect(section).toHaveAttribute('data-view', 'actions');

  for (const id of ['actions-title', 'actions-badge', 'actions-status', 'actions-rows']) {
    expect(document.getElementById(id), id).not.toBeNull();
  }
  expect(fetchStub.calls).toHaveLength(0);
  expect(text('actions-badge')).toBe('SIN CONSULTAR');
  expect(text('actions-status')).toMatch(/conecta un token autorizado/i);
  expect(document.getElementById('actions-rows')?.querySelectorAll('tr')).toHaveLength(0);
  // The creation control exists but is disabled: the server, not a hidden panel,
  // is what decides.
  expect((document.getElementById('create-action') as HTMLButtonElement).disabled).toBe(true);
});

test('a signed-in admin reads the list with only the filters that are set', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);

  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  const listCall = fetchStub.callsFor(ACTIONS_URL)[0];
  expect((listCall.init.headers as Record<string, string>).Authorization).toBe(`Bearer ${ADMIN_TOKEN}`);
  await answerList([actionFixture(), actionFixture({ id: 'action-2', status: 'en_curso', title: 'Charla' })]);

  const rows = [...(document.getElementById('actions-rows')?.querySelectorAll('tr') ?? [])];
  expect(rows).toHaveLength(2);
  expect(rows[0]).toHaveTextContent('Despeje de vegetación');
  expect(rows[0]).toHaveTextContent('05101');
  expect(rows[0]).toHaveTextContent('Propuesta');
  expect(rows[1]).toHaveTextContent('En curso');
  expect(text('actions-badge')).toBe('LEÍDO');
  expect((document.getElementById('create-action') as HTMLButtonElement).disabled).toBe(false);
});

test('opening an action reads its detail and renders the history row by row', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);

  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture());

  expect(text('action-detail-title')).toContain('Despeje de vegetación');
  expect(text('action-detail-fields')).toContain('equipo-1');
  expect(text('action-detail-status')).toMatch(/historial/i);

  const historyRows = [...(document.getElementById('action-history-rows')?.querySelectorAll('tr') ?? [])];
  expect(historyRows).toHaveLength(2);
  expect(historyRows[0]).toHaveTextContent('propuesta');
  expect(historyRows[0]).toHaveTextContent('alta inicial');
  expect(historyRows[1]).toHaveTextContent('asignada');
  expect(historyRows[1]).toHaveTextContent('admin-1');
});

test('the creation form posts the fields it was given and reports the created action', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([]);

  await userEvent.type(document.getElementById('create-community') as HTMLElement, '05101');
  await userEvent.type(document.getElementById('create-title') as HTMLElement, 'Charla preventiva');
  await userEvent.type(document.getElementById('create-description') as HTMLElement, 'Con la junta de vecinos');
  await userEvent.type(document.getElementById('create-priority') as HTMLElement, 'media');
  await userEvent.type(document.getElementById('create-assignee') as HTMLElement, 'equipo-2');

  await userEvent.click(document.getElementById('create-action') as HTMLButtonElement);
  await waitFor(() => {
    expect(fetchStub.calls.some(call => call.url === ACTIONS_URL && call.init.method === 'POST')).toBe(true);
  });

  const post = fetchStub.calls.find(call => call.url === ACTIONS_URL && call.init.method === 'POST')!;
  expect(JSON.parse(String(post.init.body))).toEqual({
    community_code: '05101',
    title: 'Charla preventiva',
    description: 'Con la junta de vecinos',
    priority: 'media',
    assignee: 'equipo-2',
  });

  await act(async () => {
    fetchStub.answer(ACTIONS_URL, actionFixture({ id: 'action-9', title: 'Charla preventiva' }), 201);
    await flush();
  });
  // The list is re-read after a create so the screen reflects the server, not a
  // local guess about where the new row belongs.
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL).length).toBeGreaterThanOrEqual(2));
  await answerList([actionFixture({ id: 'action-9', title: 'Charla preventiva' })]);
  expect(text('actions-notice')).toMatch(/creada/i);
  expect(text('actions-rows')).toContain('Charla preventiva');
});

test('an admin sees enabled transitions and applying one posts {to, note}', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture({ status: 'propuesta' }));

  const toAsignada = document.querySelector('[data-transition-to="asignada"]') as HTMLButtonElement;
  expect(toAsignada.disabled).toBe(false);
  // Only the valid successors of `propuesta` are offered.
  expect([...document.querySelectorAll('[data-transition-to]')].map(node => node.getAttribute('data-transition-to'))).toEqual([
    'asignada',
    'cancelada',
  ]);

  await userEvent.type(document.getElementById('transition-note') as HTMLElement, 'asignada al equipo');
  await userEvent.click(toAsignada);

  await waitFor(() => {
    expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)).toHaveLength(1);
  });
  const post = fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)[0];
  expect(JSON.parse(String(post.init.body))).toEqual({ to: 'asignada', note: 'asignada al equipo' });

  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/transitions`, actionFixture({ status: 'asignada' }));
    await flush();
  });
  // The history is authored by the server, so the detail is re-read after a
  // transition instead of being patched locally.
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`).length).toBeGreaterThanOrEqual(2));
  await answerDetail(detailFixture({ status: 'asignada' }));
  expect(text('actions-notice')).toMatch(/transición aplicada/i);
});

test('a viewer gets the same view with the transitions disabled and the reason stated', async () => {
  await connect(VIEWER_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture());

  // The view exists and the detail is readable: the declared role gates only the
  // affordance, never the read.
  expect(text('action-detail-title')).toContain('Despeje de vegetación');
  for (const button of document.querySelectorAll('[data-transition-to]')) {
    expect((button as HTMLButtonElement).disabled).toBe(true);
  }
  expect((document.getElementById('create-action') as HTMLButtonElement).disabled).toBe(true);
  const note = text('actions-role-note');
  expect(note).toMatch(/admin/i);
  expect(note).toMatch(/servidor/i);
});

test('a 409 invalid transition is rendered with its code and request id, and does not change the status', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture({ status: 'en_curso' }));

  await userEvent.click(document.querySelector('[data-transition-to="enviada"]') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/transitions`, { code: 'invalid_transition' }, 409, 'req-409');
    await flush();
  });

  expect(text('actions-notice')).toMatch(/409/);
  expect(text('actions-notice')).toMatch(/invalid_transition/);
  expect(text('actions-notice')).toMatch(/req-409/);
  expect(text('actions-notice')).toMatch(/no cambió/i);
});

test('an unknown target state surfaces the frozen 422 invalid_transition_request code', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture({ status: 'propuesta' }));

  await userEvent.click(document.querySelector('[data-transition-to="asignada"]') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/transitions`, { code: 'invalid_transition_request' }, 422, 'req-422');
    await flush();
  });

  expect(text('actions-notice')).toMatch(/422/);
  expect(text('actions-notice')).toMatch(/invalid_transition_request/);
  expect(text('actions-notice')).toMatch(/req-422/);
  expect(text('actions-notice')).toMatch(/no cambió/i);
});

test('a newly created action is shown with an empty history, because creation is not a transition', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([]);

  await userEvent.type(document.getElementById('create-community') as HTMLElement, '05101');
  await userEvent.type(document.getElementById('create-title') as HTMLElement, 'Acción nueva');
  await userEvent.click(document.getElementById('create-action') as HTMLButtonElement);
  await waitFor(() => expect(fetchStub.calls.some(call => call.url === ACTIONS_URL && call.init.method === 'POST')).toBe(true));
  await act(async () => {
    fetchStub.answer(ACTIONS_URL, actionFixture({ id: 'action-new', title: 'Acción nueva' }), 201);
    await flush();
  });

  // The create flow opens the created action; its detail carries no history row.
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-new`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-new`, {
      ...actionFixture({ id: 'action-new' }),
      history: [],
      evidence: [],
      evidence_reviews: [],
      reassessments: [],
      evidence_sufficient: false,
    });
    await flush();
  });
  expect(document.getElementById('action-history-rows')?.querySelectorAll('tr')).toHaveLength(0);
  expect(text('action-detail-status')).toMatch(/0 movimiento/i);
});

test('a 403 on the list is stated as a permission denial with its request id', async () => {
  await connect(VIEWER_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList({ code: 'forbidden' }, 403, 'req-403');

  expect(text('actions-badge')).toBe('SIN PERMISO');
  expect(text('actions-status')).toMatch(/403/);
  expect(text('actions-status')).toMatch(/req-403/);
  expect(text('create-action')).not.toBe('');
});

test('a 401 is rendered with its request id and closes the session', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList({ code: 'unauthorized' }, 401, 'req-401');

  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: 'rejected' });
  expect(text('actions-notice') || text('actions-status')).toMatch(/req-401/);
});

test('a malformed list fails closed instead of showing partial rows', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([{ id: 'action-1', title: 'sin comunidad' }]);

  expect(text('actions-badge')).toBe('NO DISPONIBLE');
  expect(document.getElementById('actions-rows')?.querySelectorAll('tr')).toHaveLength(0);
});

// S3 — the row whose detail is open must be visibly marked, and the rule that
// marks it must sit after the plain hover tint: both carry the same specificity,
// so only a later rule keeps the mark while the pointer is over the row. The
// mark is visual only — the table gains no ARIA state its role does not support.
test('the selected actions row carries is-selected and the stylesheet marks it', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture(), actionFixture({ id: 'action-2', title: 'Charla vecinal' })]);

  expect(document.querySelectorAll('#actions-rows tr.is-selected')).toHaveLength(0);

  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture());

  const selected = document.querySelectorAll('#actions-rows tr.is-selected');
  // Exactly one row is marked, and it is the one whose detail is open.
  expect(selected).toHaveLength(1);
  expect(selected[0]).toHaveTextContent('Despeje de vegetación');
  // A visual mark only: the row keeps its table semantics.
  expect(selected[0]).not.toHaveAttribute('aria-selected');

  // The rule that paints the mark, and its place in the sheet.
  const hoverAt = appCss.indexOf('tbody tr:hover');
  const selectedAt = appCss.indexOf('tbody tr.is-selected');
  expect(hoverAt).toBeGreaterThan(-1);
  expect(selectedAt).toBeGreaterThan(hoverAt);
  const rule = appCss.slice(selectedAt, appCss.indexOf('}', selectedAt));
  expect(rule).toMatch(/background:\s*var\(--soft\)/);
  // The accent itself is painted on the first cell: `border-collapse: collapse`
  // drops a box-shadow declared on the row.
  expect(appCss).toMatch(
    /tbody tr\.is-selected td:first-child\s*\{[^}]*box-shadow:\s*inset 3px 0 0 var\(--forest-700\)/,
  );
});

test('logout clears the list, the detail, the form and the notice', async () => {
  await connect(ADMIN_TOKEN);
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await answerList([actionFixture()]);
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await answerDetail(detailFixture());
  await userEvent.type(document.getElementById('create-title') as HTMLElement, 'Borrador');

  act(() => {
    resetSession();
  });

  await waitFor(() => expect(text('actions-badge')).toBe('SIN CONSULTAR'));
  expect(document.getElementById('actions-rows')?.querySelectorAll('tr')).toHaveLength(0);
  expect(document.getElementById('action-detail')).toBeNull();
  expect((document.getElementById('create-title') as HTMLInputElement).value).toBe('');
  expect(text('actions-notice')).toBe('');
  expect(text('actions-status')).toMatch(/conecta un token autorizado/i);
});

