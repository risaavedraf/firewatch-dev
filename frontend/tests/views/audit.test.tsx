import { act, render, waitFor } from '@testing-library/react';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, resetSession } from '../../src/core/session';
import { AuditView } from '../../src/views/AuditView';

import {
  createCryptoStub,
  createFetchStub,
  createStorageStub,
  flush,
  jwt,
} from '../core/harness';

// N8b — the activity register (S13/S17). `/v1/audit` is a parameterless,
// admin-only endpoint: the view renders exactly the five fields it carries
// (`actor_id`, `action`, `job_id`, `trace_id`, `occurred_at`), states an explicit
// 403 for a non-administrator and fails closed on an entry that does not say WHAT
// happened and WHEN. It is reachable for every role on purpose: a `viewer` must
// arrive and read the server's own refusal, because there is no client-side role
// gating.
//
// The transport, storage and crypto are stubbed exactly as `tests/core/harness.ts`
// establishes; the real `window` stays in place because this is a DOM render.

const AUDIT_URL = `${gatewayOrigin}/v1/audit`;
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const ADMIN_TOKEN = jwt({ realm_access: { roles: ['viewer', 'admin'] } });
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
}

// `answerAudit` resolves the pending `/v1/audit` request inside `act`, so the
// state update the transport settles is flushed before the assertions run.
async function answerAudit(body: unknown, status = 200, requestId = ''): Promise<void> {
  await act(async () => {
    fetchStub.answer(AUDIT_URL, body, status, requestId);
    await flush();
  });
}

function text(id: string): string {
  return document.getElementById(id)?.textContent ?? '';
}

function badgeText(): string {
  return text('audit-badge');
}

beforeEach(() => {
  setup();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the audit section carries every legacy identifier and the five real columns', async () => {
  await connect(ADMIN_TOKEN);
  const { container } = render(<AuditView />);

  const section = container.querySelector('section#audit') as HTMLElement | null;
  expect(section).not.toBeNull();
  expect(section).toHaveAttribute('data-view', 'audit');

  for (const id of [
    'audit',
    'audit-title',
    'audit-badge',
    'audit-status',
    'audit-unimplemented',
    'audit-table',
    'audit-rows',
    'audit-count',
    'audit-refresh',
  ]) {
    expect(document.getElementById(id), id).not.toBeNull();
  }

  // The retired mockup's name/detail/id fields have no backend: the panel says so
  // instead of simulating them.
  expect(text('audit-unimplemented')).toMatch(/nombre del perfil/i);
  expect(text('audit-unimplemented')).toMatch(/no se muestran ni se simulan/i);

  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  const headerCells = [...document.querySelectorAll('#audit-table thead th')].map(cell => cell.textContent);
  expect(headerCells).toEqual([
    'Actor (actor_id)',
    'Acción',
    'Trabajo (job_id)',
    'Traza (trace_id)',
    'Ocurrido (occurred_at)',
  ]);
});

test('the panel reads the parameterless admin endpoint once and renders the five real fields verbatim', async () => {
  await connect(ADMIN_TOKEN);
  render(<AuditView />);

  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  const call = fetchStub.callsFor(AUDIT_URL)[0];
  expect(call.url).toBe(AUDIT_URL);
  expect((call.init.headers as Record<string, string>).Authorization).toBe(`Bearer ${ADMIN_TOKEN}`);

  await answerAudit([
    { actor_id: 'actor-opaco-1', action: 'job.requested', job_id: 'job-1', trace_id: 'trace-1', occurred_at: '2026-01-01T00:00:00Z' },
    { actor_id: 'actor-opaco-2', action: 'batch.published', job_id: 'job-2', trace_id: 'trace-2', occurred_at: '2026-01-02T00:00:00Z' },
  ]);

  const rows = [...document.querySelectorAll('#audit-rows tr')];
  expect(rows).toHaveLength(2);
  expect([...rows[0].querySelectorAll('td')].map(cell => cell.textContent)).toEqual([
    'actor-opaco-1',
    'job.requested',
    'job-1',
    'trace-1',
    '2026-01-01T00:00:00Z',
  ]);
  expect(text('audit-count')).toBe('2');
  expect(badgeText()).toBe('LEÍDO');
  expect(document.getElementById('audit-table')?.hasAttribute('hidden')).toBe(false);
  expect(text('audit-status')).toMatch(/Registro real devuelto por \/v1\/audit/);
  expect((document.getElementById('audit-refresh') as HTMLButtonElement).disabled).toBe(false);

  // Nothing the endpoint does not provide is rendered: no severity, no display
  // name, no Spanish gloss of the action enum.
  expect(text('audit-status')).not.toMatch(/severidad|nombre para mostrar|riesgo/i);
});

test('audit 403 renders an explicit admin-only reason instead of hiding the panel', async () => {
  await connect(VIEWER_TOKEN);
  render(<AuditView />);

  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  // The transport reads the request id from the `X-Request-ID` response header.
  await answerAudit({ code: 'forbidden' }, 403, 'req-9');

  const status = text('audit-status');
  expect(badgeText()).toBe('SIN PERMISO');
  expect(status).toMatch(/403/);
  expect(status).toMatch(/administrador/i);
  expect(status).toMatch(/forbidden/);
  expect(status).toMatch(/req-9/);
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(0);
  expect(document.getElementById('audit-table')?.hasAttribute('hidden')).toBe(true);
  expect(text('audit-count')).toBe('—');
});

test('a viewer reaches the audit view and reads the server 403 instead of being gated by role', async () => {
  await connect(VIEWER_TOKEN);
  const { container } = render(<AuditView />);

  // The view exists and its control is offered to a viewer: the server, not the
  // client, decides the permission.
  expect(container.querySelector('section#audit')).not.toBeNull();
  const refresh = document.getElementById('audit-refresh') as HTMLButtonElement;
  expect(refresh.hidden).toBe(false);

  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  expect((fetchStub.callsFor(AUDIT_URL)[0].init.headers as Record<string, string>).Authorization).toBe(
    `Bearer ${VIEWER_TOKEN}`,
  );

  await answerAudit({ code: 'forbidden' }, 403, 'req-viewer');
  expect(badgeText()).toBe('SIN PERMISO');
  expect(text('audit-status')).toMatch(/403/);
  expect(text('audit-status')).toMatch(/req-viewer/);
  expect(refresh.disabled).toBe(false);
});

test('the register fails closed unless every entry states an action and a time, and an omitted identifier is labelled in place', async () => {
  // A body that is not an array of events is not a register.
  await connect(ADMIN_TOKEN);
  const notArray = render(<AuditView />);
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  await answerAudit({ code: 'unexpected' });
  expect(badgeText()).toBe('NO DISPONIBLE');
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(0);
  expect(document.getElementById('audit-table')?.hasAttribute('hidden')).toBe(true);
  notArray.unmount();

  // An entry without an action and a time is not an event: nothing is invented.
  setup();
  await connect(ADMIN_TOKEN);
  const incomplete = render(<AuditView />);
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  await answerAudit([{ actor_id: 'a', job_id: 'j', trace_id: 't' }]);
  expect(badgeText()).toBe('NO DISPONIBLE');
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(0);
  incomplete.unmount();

  // An omitted identifier is labelled in place, never filled in from another
  // field, and an action outside the documented enum is shown verbatim.
  setup();
  await connect(ADMIN_TOKEN);
  render(<AuditView />);
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  await answerAudit([
    { actor_id: 'a', action: 'cualquier.valor', job_id: 'j', trace_id: '', occurred_at: '2026-01-01T00:00:00Z' },
  ]);
  const cells = [...document.querySelectorAll('#audit-rows tr td')].map(cell => cell.textContent);
  expect(cells).toEqual(['a', 'cualquier.valor', 'j', 'No informado', '2026-01-01T00:00:00Z']);
});

test('the refresh control re-reads the same parameterless endpoint and an empty register is a real zero', async () => {
  await connect(ADMIN_TOKEN);
  render(<AuditView />);
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  await answerAudit([
    { actor_id: 'a', action: 'job.requested', job_id: 'j', trace_id: 't', occurred_at: '2026-01-01T00:00:00Z' },
  ]);
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(1);

  act(() => {
    (document.getElementById('audit-refresh') as HTMLButtonElement).click();
  });
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(2);
  });
  expect(fetchStub.callsFor(AUDIT_URL)[1].url).toBe(AUDIT_URL);
  expect(badgeText()).toBe('CONSULTANDO');

  await answerAudit([]);
  expect(badgeText()).toBe('LEÍDO');
  expect(text('audit-count')).toBe('0');
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(0);
  expect(text('audit-status')).toMatch(/sin eventos/);
});

test('closing the session clears the register and a fenced reply cannot repaint it', async () => {
  await connect(ADMIN_TOKEN);
  render(<AuditView />);
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(1);
  });
  await answerAudit([
    { actor_id: 'a', action: 'job.requested', job_id: 'j', trace_id: 't', occurred_at: '2026-01-01T00:00:00Z' },
  ]);
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(1);

  // Start a read, then close the session while it is in flight.
  act(() => {
    (document.getElementById('audit-refresh') as HTMLButtonElement).click();
  });
  await waitFor(() => {
    expect(fetchStub.callsFor(AUDIT_URL)).toHaveLength(2);
  });
  act(() => {
    resetSession();
  });

  expect(badgeText()).toBe('SIN CONSULTAR');
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(0);
  expect(text('audit-count')).toBe('—');
  expect(document.getElementById('audit-table')?.hasAttribute('hidden')).toBe(true);
  expect((document.getElementById('audit-refresh') as HTMLButtonElement).disabled).toBe(true);

  // The late reply owns nothing: it must not repaint the closed panel.
  await answerAudit([
    { actor_id: 'late', action: 'batch.published', job_id: 'j', trace_id: 't', occurred_at: '2026-01-03T00:00:00Z' },
  ]);
  expect(document.querySelectorAll('#audit-rows tr')).toHaveLength(0);
  expect(badgeText()).toBe('SIN CONSULTAR');
  expect(text('audit-status')).toBe('Conecta un token autorizado para consultar el registro de actividad.');
});
