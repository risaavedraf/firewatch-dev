import { readdirSync, readFileSync, statSync } from 'node:fs';
import { relative, resolve } from 'node:path';

import { clientID, issuer, pkceTransactionKey, redirectURI } from '../../src/config';
import {
  completeLogin,
  decodeRoles,
  getAccessToken,
  getSessionSnapshot,
  hasPendingCallback,
  resetInterruptedLogin,
  resetSession,
  resumePendingLogin,
  signOut,
  startLogin,
  subscribeSession,
} from '../../src/core/session';

import {
  createCryptoStub,
  createFetchStub,
  createStorageStub,
  createWindowStub,
  flush,
  jwt,
  sha256Base64url,
} from './harness';

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const srcRoot = resolve(process.cwd(), 'src');

let fetchStub: ReturnType<typeof createFetchStub>;
let storageStub: ReturnType<typeof createStorageStub>;
let windowStub: ReturnType<typeof createWindowStub>;

function setup(): void {
  fetchStub = createFetchStub();
  storageStub = createStorageStub();
  windowStub = createWindowStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  vi.stubGlobal('crypto', createCryptoStub());
  vi.stubGlobal('window', windowStub.window);
  resetSession();
}

function seedTransaction(state = 's', verifier = 'verifier'): void {
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state, verifier }));
}

async function connect(token: string, idToken = 'id-token'): Promise<void> {
  seedTransaction('s', 'verifier');
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: token, id_token: idToken });
  await login;
}

function collectSourceFiles(directory: string): string[] {
  const found: string[] = [];
  for (const entry of readdirSync(directory)) {
    const path = `${directory}/${entry}`;
    if (statSync(path).isDirectory()) found.push(...collectSourceFiles(path));
    else if (entry.endsWith('.ts') || entry.endsWith('.tsx')) found.push(path);
  }
  return found;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

test('startLogin writes the PKCE transaction once and redirects with an S256 challenge', async () => {
  setup();
  startLogin();
  await vi.waitFor(() => {
    expect(storageStub.writes).toHaveLength(1);
  });
  expect(storageStub.writes[0].key).toBe(pkceTransactionKey);
  const transaction = JSON.parse(storageStub.writes[0].value) as { state: string; verifier: string };
  expect(typeof transaction.state).toBe('string');
  expect(typeof transaction.verifier).toBe('string');

  expect(windowStub.assign).toHaveBeenCalledTimes(1);
  const target = new URL(windowStub.assign.mock.calls[0][0] as string);
  expect(target.href.startsWith(issuer)).toBe(true);
  expect(target.searchParams.get('client_id')).toBe(clientID);
  expect(target.searchParams.get('redirect_uri')).toBe(redirectURI);
  expect(target.searchParams.get('response_type')).toBe('code');
  expect(target.searchParams.get('scope')).toBe('openid');
  expect(target.searchParams.get('state')).toBe(transaction.state);
  expect(target.searchParams.get('code_challenge_method')).toBe('S256');
  expect(target.searchParams.get('code_challenge')).toBe(await sha256Base64url(transaction.verifier));
  expect(getSessionSnapshot().status).toBe('signing-in');
});

test('completeLogin validates the state, keeps the hash and clears the query', async () => {
  setup();
  windowStub.location.hash = '#overview';
  const token = jwt({ realm_access: { roles: ['viewer'] } });
  await connect(token);

  expect(getSessionSnapshot()).toEqual({ status: 'signed-in', roles: ['viewer'], reason: null });
  expect(getAccessToken()).toBe(token);
  expect(windowStub.replaceState).toHaveBeenCalledWith(null, '', '/#overview');
  expect(storageStub.removals).toContain(pkceTransactionKey);
  expect(storageStub.snapshot().size).toBe(0);
  for (const write of storageStub.writes) expect(write.value.includes(token)).toBe(false);

  const tokenCall = fetchStub.callsFor(TOKEN_URL)[0];
  expect(tokenCall.url.startsWith(issuer)).toBe(true);
  expect(tokenCall.url.startsWith('http://localhost:8080')).toBe(false);
  expect(tokenCall.init.method).toBe('POST');
  expect(tokenCall.init.credentials).toBe('omit');
  expect(tokenCall.init.cache).toBe('no-store');
  const form = new URLSearchParams(tokenCall.init.body as string);
  expect(form.get('grant_type')).toBe('authorization_code');
  expect(form.get('client_id')).toBe(clientID);
  expect(form.get('code')).toBe('code-1');
  expect(form.get('code_verifier')).toBe('verifier');
  expect(form.get('redirect_uri')).toBe(redirectURI);
});

test('completeLogin rejects and resets the session when the returned state does not match', async () => {
  setup();
  seedTransaction('expected', 'verifier');

  await expect(completeLogin('?code=code-1&state=other')).rejects.toThrow();

  expect(fetchStub.calls).toHaveLength(0);
  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: null });
  expect(getAccessToken()).toBe('');
  expect(storageStub.removals).toContain(pkceTransactionKey);
  expect(windowStub.replaceState).toHaveBeenCalled();
});

test('completeLogin rejects when the identity provider cancels the login', async () => {
  setup();
  seedTransaction();

  await expect(completeLogin('?error=access_denied&state=s')).rejects.toThrow();

  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(getAccessToken()).toBe('');
  expect(storageStub.removals).toContain(pkceTransactionKey);
});

test('completeLogin rejects and resets the session when the token exchange fails', async () => {
  setup();
  seedTransaction();
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { code: 'invalid_grant' }, 400);

  await expect(login).rejects.toThrow();
  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(getAccessToken()).toBe('');
});

test('completeLogin rejects and resets the session when no usable access token is returned', async () => {
  setup();
  seedTransaction();
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: '' });

  await expect(login).rejects.toThrow();
  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(getAccessToken()).toBe('');
});

test('hasPendingCallback detects a login callback and ignores an ordinary load', () => {
  expect(hasPendingCallback('')).toBe(false);
  expect(hasPendingCallback('?foo=bar')).toBe(false);
  expect(hasPendingCallback('?code=code-1&state=s')).toBe(true);
  expect(hasPendingCallback('?state=s')).toBe(true);
  expect(hasPendingCallback('?error=access_denied')).toBe(true);
});

test('decodeRoles reads realm_access.roles verbatim and drops unusable entries', () => {
  const token = jwt({ realm_access: { roles: [' viewer ', 'admin', 'viewer', '', 42, '  '] } });
  expect(decodeRoles(token)).toEqual(['viewer', 'admin']);
  expect(decodeRoles(jwt({ realm_access: { roles: ['viewer'] } }))).toEqual(['viewer']);
});

test('decodeRoles returns null for an absent, unreadable or role-less token', () => {
  const malformedBase64 = `${Buffer.from('{"alg":"none"}').toString('base64url')}.@@@@.signature`;
  const notJson = `${Buffer.from('{"alg":"none"}').toString('base64url')}.${Buffer.from('no-json').toString('base64url')}.signature`;
  const cases: [string, string][] = [
    ['not a JWT at all', 'test'],
    ['two segments only', 'aaaa.bbbb'],
    ['payload is not decodable base64url', malformedBase64],
    ['payload is not JSON', notJson],
    ['payload is not an object', jwt(['viewer'])],
    ['no realm_access claim', jwt({ sub: 'x' })],
    ['roles is a bare string', jwt({ realm_access: { roles: 'admin' } })],
    ['roles is an empty array', jwt({ realm_access: { roles: [] } })],
    ['roles holds no usable string', jwt({ realm_access: { roles: ['', 42, '  '] } })],
    ['empty token', ''],
  ];
  for (const [label, token] of cases) expect(decodeRoles(token), label).toBeNull();
});

test('decodeRoles round-trips a multi-byte role name through base64url and strict UTF-8', () => {
  const roles = decodeRoles(jwt({ realm_access: { roles: ['visór-de-prueba', 'admin'] } }));
  expect(roles).toEqual(['visór-de-prueba', 'admin']);
  expect(roles?.join(', ')).not.toMatch(/[\uFFFD]/);
});

test('signOut removes the PKCE transaction, resets the session and redirects to the issuer logout', async () => {
  setup();
  const token = jwt({ realm_access: { roles: ['admin'] } });
  await connect(token, 'id-token');
  storageStub.removals.length = 0;

  signOut();

  expect(storageStub.removals).toContain(pkceTransactionKey);
  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: null });
  expect(getAccessToken()).toBe('');
  const target = new URL(windowStub.assign.mock.calls[0][0] as string);
  expect(target.href.startsWith(issuer)).toBe(true);
  expect(target.searchParams.get('id_token_hint')).toBe('id-token');
  expect(target.searchParams.get('client_id')).toBe(clientID);
  expect(target.searchParams.get('post_logout_redirect_uri')).toBe(redirectURI);
});

test('resetSession clears the in-memory tokens and states an unavailable session', async () => {
  setup();
  await connect(jwt({ realm_access: { roles: ['viewer'] } }));

  resetSession();

  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: null });
  expect(getAccessToken()).toBe('');
});

test('getSessionSnapshot is a stable reference and subscription fires once per change', async () => {
  setup();
  const initial = getSessionSnapshot();
  expect(getSessionSnapshot()).toBe(initial);

  const listener = vi.fn();
  const unsubscribe = subscribeSession(listener);
  startLogin();
  expect(listener).toHaveBeenCalledTimes(1);
  expect(getSessionSnapshot().status).toBe('signing-in');
  expect(getSessionSnapshot()).toBe(getSessionSnapshot());

  unsubscribe();
  resetSession();
  expect(listener).toHaveBeenCalledTimes(1);
  // Let the PKCE flow reach the (stubbed) redirect while the URL is still
  // stubbed, so no navigation escapes after the test double is removed.
  await vi.waitFor(() => expect(windowStub.assign).toHaveBeenCalledTimes(1));
});

test('the decoded token payload has exactly one reader in the application', () => {
  const payloadDecoders = collectSourceFiles(srcRoot)
    .filter((file) => {
      const source = readFileSync(file, 'utf8');
      if (!source.includes('realm_access')) return false;
      // A display mention of the claim is not a reader; only a module that
      // carries its own base64url/UTF-8 decoder could turn it into roles.
      return ['atob', 'TextDecoder', 'BASE64URL_ALPHABET', 'Buffer.from'].some((primitive) => source.includes(primitive));
    })
    .map((file) => relative(srcRoot, file).replace(/\\/g, '/'));
  expect(payloadDecoders).toEqual(['core/session.ts']);
});

test('no browser storage or cookie can hold a token', () => {
  const offenders = collectSourceFiles(srcRoot).filter((file) => {
    const source = readFileSync(file, 'utf8');
    return source.includes('localStorage') || source.includes('document.cookie');
  });
  expect(offenders).toEqual([]);
});

// The callback was implemented but never reached by the application: nothing in
// `src/` called it, and the bundle proved it by tree-shaking the token exchange
// out entirely. These two rules are what make the round trip survive in the app
// rather than only in this suite.
test('a pending callback is consumed on startup, and nothing happens without one', async () => {
  setup();

  // No callback in the address bar: no request, and the session stays closed.
  await resumePendingLogin('');
  expect(fetchStub.callsFor(TOKEN_URL)).toHaveLength(0);
  expect(getSessionSnapshot().status).toBe('signed-out');

  // A callback is exchanged exactly once and establishes the session.
  seedTransaction('s', 'verifier');
  const token = jwt({ realm_access: { roles: ['viewer'] } });
  const resumed = resumePendingLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: token, id_token: 'id-token' });
  await resumed;

  expect(fetchStub.callsFor(TOKEN_URL)).toHaveLength(1);
  expect(getAccessToken()).toBe(token);
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect(storageStub.snapshot().has(pkceTransactionKey)).toBe(false);
});

test('a refused callback resolves without throwing and leaves the session closed', async () => {
  setup();
  seedTransaction('s', 'verifier');

  // The identity provider said no. The operator must land on a usable page, not
  // on an unhandled rejection, and no transaction may survive the refusal.
  await expect(resumePendingLogin('?error=access_denied&state=s')).resolves.toBeUndefined();

  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(getAccessToken()).toBe('');
  expect(storageStub.snapshot().has(pkceTransactionKey)).toBe(false);
});

// A page restored from the back/forward cache keeps its in-memory session, so the
// `signing-in` a click left behind would stay in flight forever: the operator
// pressed Back at the identity provider, the redirect that would have completed
// or failed the flow never ran, and the sign-in control is disabled while it is
// in flight. The restore is the one event that can end that state honestly.
test('a restored page ends an interrupted sign-in and never a real session', async () => {
  setup();
  startLogin();
  // The authorization challenge is built asynchronously and writes the PKCE
  // transaction in the process: let it settle before this test hijacks that
  // transaction, or the two writes race and the state check fails for the wrong
  // reason.
  await vi.waitFor(() => {
    expect(storageStub.writes).toHaveLength(1);
  });
  expect(getSessionSnapshot().status).toBe('signing-in');

  resetInterruptedLogin();
  expect(getSessionSnapshot().status).toBe('signed-out');

  // The dangerous half: a restored page that already had a session must keep it.
  // Signing the operator out here would be a worse defect than the latch.
  await connect(jwt({ realm_access: { roles: ['viewer'] } }));
  const established = getAccessToken();
  expect(getSessionSnapshot().status).toBe('signed-in');

  resetInterruptedLogin();
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect(getAccessToken()).toBe(established);
});

// S3 — the rejection reason travels with the session and every legitimate
// transition clears it. `'rejected'` is set by the transport alone, on a 401
// for a request that carried a non-empty bearer; these tests pin the
// transitions the session unit owns: clearing on a new login and notifying a
// reason-only change with the same status.
test('startLogin clears a rejection reason before the round trip begins (S3)', async () => {
  setup();
  resetSession('rejected');
  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: 'rejected' });

  const listener = vi.fn();
  const unsubscribe = subscribeSession(listener);
  startLogin();

  expect(listener).toHaveBeenCalledTimes(1);
  expect(getSessionSnapshot()).toEqual({ status: 'signing-in', roles: null, reason: null });
  unsubscribe();
  // Let the PKCE flow reach the (stubbed) redirect while the URL is still
  // stubbed, so no navigation escapes after the test double is removed.
  await vi.waitFor(() => expect(windowStub.assign).toHaveBeenCalledTimes(1));
});

test('a completed login clears a rejection reason (S3)', async () => {
  setup();
  resetSession('rejected');

  await connect(jwt({ realm_access: { roles: ['viewer'] } }));

  expect(getSessionSnapshot()).toEqual({ status: 'signed-in', roles: ['viewer'], reason: null });
});

test('a reason-only change with the same status notifies subscribers (S3)', () => {
  setup();
  const listener = vi.fn();
  const unsubscribe = subscribeSession(listener);

  resetSession('rejected');
  expect(listener).toHaveBeenCalledTimes(1);

  resetSession();
  expect(listener).toHaveBeenCalledTimes(2);

  // No change at all: publish stays silent.
  resetSession();
  expect(listener).toHaveBeenCalledTimes(2);
  unsubscribe();
});
