import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { routes } from '../../src/app/routes';
import { getHistoryFilters, resetHistoryFilters, setHistoryQuery, setHistoryStatus } from '../../src/app/store';
import { Shell } from '../../src/components/Shell';
import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import { zones } from '../../src/core/api';
import { completeLogin, resetSession } from '../../src/core/session';

import { createCryptoStub, createFetchStub, flush, jwt } from '../core/harness';

// The shell owns four things a view cannot: the gateway availability card, the
// session card, the single logout control and the footer's filter reset. Each is
// asserted here because each is visible on every view.
const READY_URL = `${gatewayOrigin}/health/ready`;
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;

let fetchStub: ReturnType<typeof createFetchStub>;

function setup(): void {
  fetchStub = createFetchStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('crypto', createCryptoStub());
  window.sessionStorage.clear();
  resetSession();
  resetHistoryFilters();
}

function renderShell() {
  return render(
    <Shell route={routes[0]}>
      <p>contenido</p>
    </Shell>,
  );
}

async function signIn(): Promise<void> {
  window.sessionStorage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'verifier' }));
  const callback = completeLogin('?code=c&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, {
    access_token: jwt({ realm_access: { roles: ['admin'] } }),
    id_token: jwt({ sub: 'u' }),
  });
  await callback;
}

beforeEach(setup);

test('the availability card proves gateway availability and says that is all it proves', async () => {
  renderShell();
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(1));
  fetchStub.answer(READY_URL, null, 204);

  await waitFor(() => expect(screen.getByText(/no acredita el pipeline/i)).toBeInTheDocument());
  expect(document.querySelector('.health-dot.ready')).not.toBeNull();
});

test('a refused probe marks the gateway down instead of claiming it', async () => {
  renderShell();
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(1));
  fetchStub.answer(READY_URL, null, 503);

  await waitFor(() => expect(screen.getByText(/Gateway no disponible \(HTTP 503\)/i)).toBeInTheDocument());
  expect(document.querySelector('.health-dot.down')).not.toBeNull();
});

test('the manual refresh re-probes the same endpoint', async () => {
  renderShell();
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(1));
  fetchStub.answer(READY_URL, null, 204);
  await waitFor(() => expect(document.querySelector('.health-dot.ready')).not.toBeNull());

  await userEvent.click(screen.getByRole('button', { name: /actualizar estado/i }));
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(2));
  fetchStub.answer(READY_URL, null, 503);
  await waitFor(() => expect(document.querySelector('.health-dot.down')).not.toBeNull());
});

test('the session card follows the session and the logout control stays single and hidden while signed out', async () => {
  renderShell();
  fetchStub.answer(READY_URL, null, 204);

  expect(screen.getByText('Sin conexión')).toBeInTheDocument();
  expect(screen.getByText('Token requerido para consultar')).toBeInTheDocument();
  // Exactly one control, and it is hidden — never a duplicate for a second layout.
  expect(document.querySelectorAll('#logout')).toHaveLength(1);
  expect(document.getElementById('logout')).toHaveAttribute('hidden');

  await signIn();

  await waitFor(() => expect(screen.getByText('Sesión conectada')).toBeInTheDocument());
  expect(screen.getByText('Permisos evaluados por el servidor')).toBeInTheDocument();
  expect(document.getElementById('logout')).not.toHaveAttribute('hidden');
});

test('the footer control resets the history filters', async () => {
  setHistoryQuery('05501');
  setHistoryStatus('publicado');
  renderShell();
  fetchStub.answer(READY_URL, null, 204);

  await userEvent.click(screen.getByRole('button', { name: /restablecer filtros/i }));

  expect(getHistoryFilters()).toEqual({ query: '', status: '' });
});

// S2 — the sidebar session card must name a rejected token instead of falling
// back to the first-visit copy, and the reason must not survive the session it
// describes. `resetSession` is the first thing `signOut` runs.
test('a rejected token is named in the session card and does not survive logout (S2/S6)', async () => {
  renderShell();
  fetchStub.answer(READY_URL, null, 204);
  await signIn();
  await waitFor(() => expect(screen.getByText('Sesión conectada')).toBeInTheDocument());

  // An authenticated read carries the bearer and comes back 401.
  const read = zones();
  await flush();
  fetchStub.answer(`${gatewayOrigin}/v1/risk/zones`, { code: 'unauthorized' }, 401);
  await act(async () => {
    await read;
  });

  expect(document.getElementById('session-state')).toHaveTextContent('Sin conexión');
  const detail = document.getElementById('session-detail')?.textContent ?? '';
  expect(detail).not.toBe('Token requerido para consultar');
  expect(detail).toMatch(/rechazó el token/i);

  // The reason belongs to that rejected session only.
  act(() => {
    resetSession();
  });
  expect(document.getElementById('session-detail')).toHaveTextContent('Token requerido para consultar');
});

// S5 — `#update-checked` stopped being a fixed literal: it states what the page
// actually knows and never contradicts `#readiness-status`.
test('the update bar states the real review state and never contradicts the availability card (S5)', async () => {
  renderShell();
  // Before the first probe settles the page knows of no review.
  expect(document.getElementById('update-checked')?.textContent ?? '').toMatch(/sin verificar/i);

  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(1));
  fetchStub.answer(READY_URL, null, 204);
  await waitFor(() => expect(document.querySelector('.health-dot.ready')).not.toBeNull());
  const ready = document.getElementById('update-checked')?.textContent ?? '';
  expect(ready).toMatch(/gateway respondió/i);
  expect(ready).not.toMatch(/no respondió/i);
  expect(ready).not.toMatch(/conecta un token/i);

  // A failed re-check must follow the card, not keep claiming success.
  await userEvent.click(screen.getByRole('button', { name: /actualizar estado/i }));
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(2));
  fetchStub.answer(READY_URL, null, 503);
  await waitFor(() => expect(document.querySelector('.health-dot.down')).not.toBeNull());
  const down = document.getElementById('update-checked')?.textContent ?? '';
  expect(down).toMatch(/no respondió/i);
  expect(down).not.toMatch(/disponible/i);
});
