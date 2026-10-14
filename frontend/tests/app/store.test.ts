import { issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, getSessionSnapshot, resetSession } from '../../src/core/session';
import {
  bindZoneStateToSession,
  clearZoneState,
  getSelectedZone,
  getZoneLevels,
  rememberZoneLevel,
  selectZone,
  subscribeZoneState,
} from '../../src/app/store';

import { createCryptoStub, createFetchStub, createStorageStub, createWindowStub, flush, jwt } from '../core/harness';

// The shared state exists so that moving between views does not change which zone
// the page is describing, and so the end of a session cannot leave the previous
// session's colours on screen. Both are invisible in a single-view test, which is
// why they are asserted here.
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;

let fetchStub: ReturnType<typeof createFetchStub>;
let storageStub: ReturnType<typeof createStorageStub>;

function setup(): void {
  fetchStub = createFetchStub();
  storageStub = createStorageStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  vi.stubGlobal('crypto', createCryptoStub());
  vi.stubGlobal('window', createWindowStub().window);
  resetSession();
  clearZoneState();
}

async function signIn(): Promise<void> {
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'verifier' }));
  const callback = completeLogin('?code=c&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, {
    access_token: jwt({ realm_access: { roles: ['admin'] } }),
    id_token: jwt({ sub: 'u' }),
  });
  await callback;
}

beforeEach(setup);

test('selecting a zone notifies readers once and reads back', () => {
  let notifications = 0;
  const unsubscribe = subscribeZoneState(() => {
    notifications += 1;
  });

  selectZone('05109');
  expect(getSelectedZone()).toBe('05109');
  expect(notifications).toBe(1);

  // Selecting the same zone again is not a change and must not wake every view.
  selectZone('05109');
  expect(notifications).toBe(1);

  selectZone('05101');
  expect(notifications).toBe(2);
  unsubscribe();
});

test('a remembered level survives moving the selection away, and null clears it', () => {
  selectZone('05109');
  rememberZoneLevel('05109', 'alto');
  selectZone('05101');

  expect(getSelectedZone()).toBe('05101');
  expect(getZoneLevels().get('05109')).toBe('alto');

  // A later read that finds no published batch must remove the old colour rather
  // than keep claiming it.
  rememberZoneLevel('05109', null);
  expect(getZoneLevels().has('05109')).toBe(false);
});

test('readers are not notified when there is nothing to clear', () => {
  let notifications = 0;
  const unsubscribe = subscribeZoneState(() => {
    notifications += 1;
  });

  clearZoneState();
  expect(notifications).toBe(0);
  unsubscribe();
});

test('ending a real session clears the selection and every remembered colour', async () => {
  const unbind = bindZoneStateToSession();

  selectZone('05109');
  rememberZoneLevel('05109', 'alto');

  await signIn();
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect(getSelectedZone()).toBe('05109');

  // A logout, a 401 or a refused callback all end the session the same way: the
  // private view must stop describing the previous session's zone.
  resetSession();

  expect(getSelectedZone()).toBe('');
  expect(getZoneLevels().size).toBe(0);
  unbind();
});
