import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { routes } from '../../src/app/routes';
import {
  clearZoneState,
  getHistoryFilters,
  rememberZoneCatalogue,
  resetHistoryFilters,
  selectZone,
  setHistoryQuery,
  setHistoryStatus,
} from '../../src/app/store';
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
const appCss = readFileSync(resolve(process.cwd(), 'src', 'styles', 'app.css'), 'utf8');

let fetchStub: ReturnType<typeof createFetchStub>;

function setup(): void {
  fetchStub = createFetchStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('crypto', createCryptoStub());
  window.sessionStorage.clear();
  resetSession();
  clearZoneState();
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

// Finding 2 — the topbar chip used to be a dead literal that contradicted the
// page body. It now mirrors the zone selected in the shared store and names it
// only from the catalogue that was actually loaded.
test('the topbar chip names the selected zone from the loaded catalogue and never invents one (finding: dead #zone-chip-name)', async () => {
  renderShell();
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(1));
  fetchStub.answer(READY_URL, null, 204);

  const chip = () => document.getElementById('zone-chip-name')?.textContent ?? '';
  // No catalogue was loaded and no zone is selected: the chip claims no zone.
  expect(chip()).toBe('Sin zona');

  act(() => {
    rememberZoneCatalogue(new Map([['05101', 'Valparaíso (05101)']]));
    selectZone('05101');
  });
  expect(chip()).toBe('Valparaíso (05101)');

  // A code the loaded catalogue never carried is a zone without a name: the chip
  // falls back to "Sin zona" instead of inventing one.
  act(() => {
    selectZone('05999');
  });
  expect(chip()).toBe('Sin zona');
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

// Finding 1 — the sign-out control must be visible without scrolling the column.
// jsdom has no layout engine, so the geometry (the control inside the sidebar's
// visible box at 1024x768, 1280x720, 1366x768 and 1440x900) is measured in a
// browser and recorded under tmp/e2e-sidebar/. What a suite can pin is the
// structure that makes it true: the session card with the only sign-out control
// belongs to the sidebar's fixed bottom block, never to the navigation that
// absorbs the height the column is missing. Before this fix the whole column
// scrolled, which pushed `#logout` roughly 190-235 px below the fold at those
// four viewports.
test('the sign-out control sits outside the sidebar region that scrolls (finding: clipped #logout)', async () => {
  renderShell();
  await waitFor(() => expect(fetchStub.callsFor(READY_URL)).toHaveLength(1));
  fetchStub.answer(READY_URL, null, 204);
  await waitFor(() => expect(document.querySelector('.health-dot.ready')).not.toBeNull());

  const sidebar = document.querySelector('aside.sidebar');
  const nav = sidebar?.querySelector('nav');
  const bottom = sidebar?.querySelector('.sidebar-bottom');
  const logout = document.getElementById('logout');
  expect(sidebar).not.toBeNull();
  expect(nav).not.toBeNull();
  expect(bottom).not.toBeNull();
  expect(logout).not.toBeNull();

  // The control is part of the bottom block...
  expect(bottom?.contains(logout as Node)).toBe(true);
  // ...which comes after the navigation and is not inside it: the scrolling
  // region cannot clip a control it does not contain.
  expect(nav?.contains(logout as Node)).toBe(false);
  const children = Array.from(sidebar?.children ?? []);
  expect(children.indexOf(bottom as Element)).toBeGreaterThan(children.indexOf(nav as Element));

  // The declared counterpart: the navigation is what yields the space, and it
  // keeps its rows at their natural height when there is room to spare.
  const navRule = appCss.match(/^\.sidebar nav \{[^}]*\}/m);
  expect(navRule).not.toBeNull();
  expect(navRule?.[0]).toMatch(/overflow-y:\s*(?:auto|scroll)/);
  expect(navRule?.[0]).toMatch(/flex:\s*1 1 auto/);
  expect(navRule?.[0]).toMatch(/min-height:\s*0/);
  expect(navRule?.[0]).toMatch(/align-content:\s*start/);

  // The viewport-bounded height this scroll depends on stays bounded on the
  // desktop breakpoints and stays automatic where the layout is static. The
  // bound is the viewport itself: the column no longer sits below the retired
  // demonstration strip, so it reserves that strip's height nowhere.
  const sidebarRule = appCss.match(/^\.sidebar \{[^}]*\}/m);
  expect(sidebarRule?.[0]).toMatch(/height:\s*(?:100vh|calc\(100vh[^;]*\))/);
  const smallViewport = appCss.match(/@media\s*\(max-width:\s*720px\)[\s\S]*?\.sidebar\s*\{[^}]*\}/);
  expect(smallViewport?.[0]).toMatch(/height:\s*auto/);
});
