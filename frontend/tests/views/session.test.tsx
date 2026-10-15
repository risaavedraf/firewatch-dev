import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { act, render } from '@testing-library/react';

import { issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, getSessionSnapshot, resetSession } from '../../src/core/session';
import { PERMISSION_MATRIX, SessionView } from '../../src/views/SessionView';

import {
  createCryptoStub,
  createFetchStub,
  createStorageStub,
  createWindowStub,
  flush,
  jwt,
} from '../core/harness';

// N8b — the session + permissions view (S13/S17). Two rules this suite exists to
// protect:
//   1. no client-side role gating: the view renders the same controls for every
//      role and there is no role selector, switcher or picker anywhere;
//   2. the decoded role is a token FACT, not an authorization: it comes from
//      `realm_access.roles`, its signature is not verified, and the view says so.
// An absent, unreadable or role-less token shows an explicit unavailable state,
// never a guessed role.
//
// The transport, storage and crypto are stubbed exactly as `tests/core/harness.ts`
// establishes; the real `window` stays in place because this is a DOM render.

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;

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

beforeEach(() => {
  setup();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the view carries the session and permissions panels with every legacy identifier', () => {
  const { container } = render(<SessionView />);

  const section = container.querySelector('section#session') as HTMLElement | null;
  expect(section).not.toBeNull();
  expect(section).toHaveAttribute('data-view', 'session');

  // These are the twelve identifiers the retired harness resolved by id; losing
  // any of them is a silent regression, not a cosmetic change.
  for (const id of [
    'session',
    'login',
    'auth-status',
    'permissions',
    'permissions-title',
    'session-roles-badge',
    'session-roles-status',
    'session-roles',
    'session-role-note',
    'permissions-matrix-note',
    'permissions-rows',
    'permissions-unimplemented',
  ]) {
    expect(document.getElementById(id), id).not.toBeNull();
  }
});

test('the sign-in control starts the Keycloak round trip and states the token stays in memory', async () => {
  render(<SessionView />);

  const login = document.getElementById('login') as HTMLButtonElement | null;
  expect(login).not.toBeNull();
  expect(login).toHaveTextContent('Iniciar sesión con Keycloak');

  // The note the user must be able to read: the access token lives in memory and
  // no cookie is sent. It is copied verbatim from `legacy/index.html`.
  const notes = [...document.querySelectorAll('.auth-panel .panel-note')].map(node => node.textContent ?? '');
  expect(notes).toContain(
    'El token de acceso permanece solo en memoria mientras esta página está abierta. No se guardan tokens ni se envían cookies.',
  );
  expect(document.getElementById('auth-status')).not.toBeNull();

  // The redirect target is stubbed for this click only: the view must hand the
  // browser to the issuer, and no other test in this file navigates.
  const windowStub = createWindowStub();
  vi.stubGlobal('window', windowStub.window);
  act(() => {
    login!.click();
  });
  await vi.waitFor(() => {
    expect(storageStub.writes).toHaveLength(1);
  });
  expect(windowStub.assign).toHaveBeenCalledTimes(1);
  expect(String(windowStub.assign.mock.calls[0][0]).startsWith(issuer)).toBe(true);

  // The ONLY value written to storage is the short-lived PKCE transaction: no
  // token, no profile, no cookie jar.
  expect(storageStub.writes[0].key).toBe(pkceTransactionKey);
  const transaction = JSON.parse(storageStub.writes[0].value) as { state: string; verifier: string };
  expect(typeof transaction.state).toBe('string');
  expect(typeof transaction.verifier).toBe('string');
  expect(getSessionSnapshot().status).toBe('signing-in');
  expect([...storageStub.snapshot().values()].some(value => value.includes('access_token'))).toBe(false);
});

test('the decoded role is shown verbatim as an unverified token fact', async () => {
  await connect(jwt({ realm_access: { roles: ['viewer', 'admin'] } }));

  render(<SessionView />);

  expect(document.getElementById('session-roles')).toHaveTextContent('viewer, admin');
  expect(document.getElementById('session-roles-badge')).toHaveTextContent('TOKEN LEÍDO');

  // The note must survive verbatim: the role is read from the token claim and the
  // signature is NOT verified by the page.
  const note = document.getElementById('session-role-note')?.textContent ?? '';
  expect(note).toMatch(/realm_access\.roles/);
  expect(note).toMatch(/no verifica su firma/i);
  expect(note).toMatch(/autorización/i);

  const status = document.getElementById('session-roles-status')?.textContent ?? '';
  expect(status).toMatch(/decodific/i);
  expect(status).toMatch(/sin verificar la firma/i);
  expect(status).toMatch(/servidor/i);
  expect(status).toMatch(/única autoridad/i);
});

test('an absent token shows an explicit unavailable state without guessing a role', () => {
  render(<SessionView />);

  expect(document.getElementById('session-roles-badge')).toHaveTextContent('SIN SESIÓN');
  expect(document.getElementById('session-roles')).toHaveTextContent('No disponible');
  const text = document.getElementById('session-roles-status')?.textContent ?? '';
  expect(text).toMatch(/sin token/i);
  expect(text).not.toMatch(/\bviewer\b|\badmin\b/);
});

test('unreadable or role-less tokens show an explicit unavailable state and never a guessed role', async () => {
  const cases: ReadonlyArray<[string, string]> = [
    ['not a JWT at all', 'test'],
    ['two segments only', 'aaaa.bbbb'],
    ['payload is not decodable base64url', `${jwt({ alg: 'none' }).split('.')[0]}.@@@@.firma`],
    ['roles is a bare string, not an array', jwt({ realm_access: { roles: 'admin' } })],
    ['roles is an empty array', jwt({ realm_access: { roles: [] } })],
    ['roles holds no usable string', jwt({ realm_access: { roles: ['', 42, '  '] } })],
    ['no realm_access claim', jwt({ sub: 'x' })],
  ];

  for (const [label, token] of cases) {
    resetSession();
    await connect(token);
    expect(getSessionSnapshot(), label).toEqual({ status: 'signed-in', roles: null });

    const { unmount } = render(<SessionView />);
    expect(document.getElementById('session-roles')?.textContent, label).toBe('No informado');
    expect(document.getElementById('session-roles-badge')?.textContent, label).toBe('NO INFORMADO');
    const text = `${document.getElementById('session-roles')?.textContent ?? ''} ${
      document.getElementById('session-roles-status')?.textContent ?? ''
    }`;
    expect(text, label).toMatch(/realm_access\.roles/);
    expect(text, label).not.toMatch(/\bviewer\b|\badmin\b/);
    unmount();
  }
});

test('the decoded role is display-only: a viewer and an admin token render the same controls', async () => {
  resetSession();
  await connect(jwt({ realm_access: { roles: ['viewer'] } }));
  const beforeViewer = fetchStub.calls.length;
  const viewer = render(<SessionView />);
  const viewerButtons = [...document.querySelectorAll('button')];
  const viewerRows = document.querySelectorAll('#permissions-rows tr').length;
  // The view itself issues no request a role could have influenced.
  expect(fetchStub.calls.length).toBe(beforeViewer);
  viewer.unmount();

  resetSession();
  await connect(jwt({ realm_access: { roles: ['viewer', 'admin'] } }));
  const beforeAdmin = fetchStub.calls.length;
  render(<SessionView />);
  const adminButtons = [...document.querySelectorAll('button')];
  const adminRows = document.querySelectorAll('#permissions-rows tr').length;
  expect(fetchStub.calls.length).toBe(beforeAdmin);

  // Role may not add, remove, hide or disable a single control or matrix row.
  expect(adminButtons.map(button => button.textContent)).toEqual(viewerButtons.map(button => button.textContent));
  expect(adminButtons.map(button => button.disabled)).toEqual(viewerButtons.map(button => button.disabled));
  expect(adminButtons.map(button => button.hidden)).toEqual(viewerButtons.map(button => button.hidden));
  expect(adminRows).toBe(viewerRows);
});

test('logout returns the role panel to the explicit unavailable state', async () => {
  await connect(jwt({ realm_access: { roles: ['admin'] } }));
  render(<SessionView />);
  expect(document.getElementById('session-roles')).toHaveTextContent('admin');

  act(() => {
    resetSession();
  });

  expect(document.getElementById('session-roles')).toHaveTextContent('No disponible');
  expect(document.getElementById('session-roles-badge')).toHaveTextContent('SIN SESIÓN');
  expect(document.getElementById('session-roles-status')?.textContent).toMatch(/sin token/i);
});

// Every matrix row must cite a REAL server-side guard by `path:line`. The test
// holds its own expectation of what each cited line must contain, so a stale or
// invented citation fails here instead of being asserted on screen. This is the
// port of the legacy assertion
// `the permissions matrix documents the real server guards and marks what it cannot verify`.
const GUARD_LINES: Record<string, RegExp> = {
  'backend/services/gateway/internal/handler/gateway.go:39': /mux\.Handle\("\/v1\/audit"/,
  'backend/services/gateway/internal/handler/gateway.go:80': /health\/ready/,
  'backend/services/gateway/internal/handler/gateway.go:102': /auth := strings\.Fields/,
  'backend/services/gateway/internal/handler/gateway.go:108': /verifier\.Verify\(ctx, auth\[1\]\)/,
  'backend/services/risk/internal/handler/security.go:43': /role == "viewer" \|\| role == "admin"/,
  'backend/services/risk/internal/handler/security.go:48': /fail\(w, 403, "forbidden"\)/,
  'backend/services/risk/internal/handler/api.go:41': /"GET \/v1\/risk\/status"/,
  'backend/services/risk/internal/handler/api.go:80': /"GET \/v1\/risk\/latest"/,
  'backend/services/risk/internal/handler/api.go:81': /"GET \/v1\/risk\/history"/,
  'backend/services/risk/internal/handler/api.go:82': /"GET \/v1\/risk\/zones"/,
  'backend/services/risk/internal/handler/api.go:83': /"GET \/v1\/risk\/synthetic-scenarios"/,
  'backend/services/risk/internal/handler/api.go:88': /"GET \/v1\/risk\/sources"/,
  'backend/services/risk/internal/handler/api.go:89': /return Authorize\(verifier, mux\)/,
  'backend/services/jobs/internal/handler/security.go:44': /role == "admin"/,
  'backend/services/jobs/internal/handler/security.go:49': /fail\(w, 403, "forbidden"\)/,
  'backend/services/jobs/internal/handler/api.go:51': /"GET \/v1\/jobs\/status"/,
  'backend/services/jobs/internal/handler/api.go:52': /"POST \/v1\/jobs"/,
  'backend/services/jobs/internal/handler/api.go:135': /"GET \/v1\/jobs\/\{id\}"/,
  'backend/services/jobs/internal/handler/api.go:152': /"GET \/v1\/audit"/,
  'backend/services/jobs/internal/handler/api.go:160': /return Authorize\(verifier, mux\)/,
};

test('the permissions matrix documents the real server guards and marks what it cannot verify', () => {
  render(<SessionView />);

  const rows = [...document.querySelectorAll('#permissions-rows tr')];
  expect(rows.length).toBe(PERMISSION_MATRIX.length);
  expect(rows.length).toBeGreaterThanOrEqual(8);

  const cited = new Set<string>();
  let unverified = 0;
  for (const row of rows) {
    const cells = [...row.querySelectorAll('td')];
    expect(cells).toHaveLength(4);
    expect(cells[0].textContent?.trim().length ?? 0).toBeGreaterThan(0);

    const cell = cells[3].textContent ?? '';
    if (cell.startsWith('No verificado')) {
      unverified += 1;
      continue;
    }
    for (const ref of cell.split(' · ')) {
      expect(GUARD_LINES[ref], `uncited or unknown guard reference: ${ref}`).toBeDefined();
      cited.add(ref);
      const [file, line] = ref.split(':');
      const source = readFileSync(resolve(process.cwd(), '..', file), 'utf8').split('\n');
      expect(source[Number(line) - 1] ?? '', ref).toMatch(GUARD_LINES[ref]);
    }
  }
  expect(unverified).toBeGreaterThanOrEqual(1);
  expect([...cited].sort()).toEqual(Object.keys(GUARD_LINES).sort());

  const rowsWith = (text: string): Element[] =>
    rows.filter(row => (row.querySelector('td')?.textContent ?? '').includes(text));
  const auditRow = rowsWith('/v1/audit');
  expect(auditRow).toHaveLength(1);
  const auditCells = [...auditRow[0].querySelectorAll('td')];
  expect(auditCells[1].textContent).toMatch(/^No\b/);
  expect(auditCells[1].textContent).toMatch(/403/);
  expect(auditCells[2].textContent).toMatch(/^Sí(\s|$)/);

  const latestRows = rowsWith('/v1/risk/latest');
  expect(latestRows).toHaveLength(1);
  const latestCells = [...latestRows[0].querySelectorAll('td')];
  expect(latestCells[1].textContent).toMatch(/^Sí(\s|$)/);

  const jobsRow = rowsWith('POST /v1/jobs');
  expect(jobsRow).toHaveLength(1);
  const jobsCells = [...jobsRow[0].querySelectorAll('td')];
  expect(jobsCells[1].textContent).toMatch(/^No\b/);
  expect(jobsCells[2].textContent).toMatch(/^Sí(\s|$)/);
});

test('no role selector, switcher or picker is ported anywhere in the view', () => {
  const { container } = render(<SessionView />);

  // The retired mockup let the operator pick a role. That selector is not ported:
  // a page cannot grant a permission, so a selector would be a false boundary.
  expect(container.querySelectorAll('select')).toHaveLength(0);
  expect(container.querySelectorAll('input')).toHaveLength(0);
  expect(container.querySelectorAll('[id^="role"], [id^="rol-"]')).toHaveLength(0);
  expect(/role-select|role-switch|role-picker|perfil-select|selector de rol/i.test(container.innerHTML)).toBe(false);

  // The panel states the omission instead of leaving it implicit.
  expect(document.getElementById('permissions-unimplemented')?.textContent).toMatch(
    /no portado a propósito/i,
  );

  // The matrix note names the real guards and the `ruta:línea` citation format.
  const matrixNote = document.getElementById('permissions-matrix-note')?.textContent ?? '';
  expect(matrixNote).toMatch(/ruta:línea/);
  expect(matrixNote).toMatch(/risk\/internal\/handler\/security\.go:43/);
  expect(matrixNote).toMatch(/jobs\/internal\/handler\/security\.go:44/);
});

// F3 — the sign-in control must answer to the session it starts. `Shell.tsx:153`
// already makes the logout control state-aware (`hidden={!signedIn}`), but the
// sign-in button carried no state at all: an authenticated operator still saw an
// enabled "Iniciar sesión con Keycloak" whose only effect was to restart the whole
// OAuth flow. Both controls now read the same `session.status`, so the pair can
// never disagree about whether a session exists.
//
// "The session ended" is asserted through `resetSession()` because `signOut()`
// funnels through it (`src/core/session.ts`) before it redirects; that the
// redirect itself happens is already covered by `tests/core/session.test.ts`.
test('the sign-in control is disabled while a session is established and enabled again once it ends', async () => {
  render(<SessionView />);
  const before = document.getElementById('login') as HTMLButtonElement | null;
  expect(before).not.toBeNull();
  expect(before?.disabled).toBe(false);

  await connect(jwt({ realm_access: { roles: ['viewer'] } }));
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect((document.getElementById('login') as HTMLButtonElement).disabled).toBe(true);

  await act(async () => {
    resetSession();
  });
  expect(getSessionSnapshot().status).toBe('signed-out');
  expect((document.getElementById('login') as HTMLButtonElement).disabled).toBe(false);
});
