import { issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, getSessionSnapshot, resetSession } from '../../src/core/session';
import {
  bindZoneStateToSession,
  claimZoneBatchAggregateRead,
  clearZoneState,
  getHistoryFilters,
  getSelectedZone,
  getSelectedZoneLabel,
  getZoneLevels,
  rememberZoneCatalogue,
  rememberZoneLevel,
  resetHistoryFilters,
  selectZone,
  setHistoryQuery,
  setHistoryStatus,
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
  resetHistoryFilters();
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

// The shell's chip and the overview share one name, and that name may only come
// from a catalogue this session actually loaded: a selected code the catalogue
// never carried has no label, never a guessed one.
test('a zone label is readable only after the catalogue that carries it was recorded', () => {
  selectZone('05101');
  expect(getSelectedZoneLabel()).toBe('');

  const source = new Map([['05101', 'Valparaíso (05101)']]);
  rememberZoneCatalogue(source);
  expect(getSelectedZoneLabel()).toBe('Valparaíso (05101)');

  // The store keeps its own copy, so a caller that mutates the map it handed in
  // cannot change what the page shows.
  source.set('05101', 'Otro nombre (05101)');
  expect(getSelectedZoneLabel()).toBe('Valparaíso (05101)');

  // The label describes the selected zone only: a code the catalogue never
  // carried falls back to the empty label instead of an invented name.
  selectZone('05999');
  expect(getSelectedZoneLabel()).toBe('');

  // Ending the session drops the catalogue with the rest of the private state.
  selectZone('05101');
  clearZoneState();
  expect(getSelectedZoneLabel()).toBe('');
});

// S4 — the aggregate read of every zone's last batch is bought ONCE per session,
// not once per mount. The claim is what makes "once" true across a view remount,
// and dropping it with the rest of the zone state is what makes the next session
// buy it again instead of showing the ended session's map.
test('the aggregate read is claimed once per session and dropped with the zone state', () => {
  expect(claimZoneBatchAggregateRead()).toBe(true);
  // A remount asks again and is told the session already bought it: returning to
  // the map must not issue the request a second time.
  expect(claimZoneBatchAggregateRead()).toBe(false);
  expect(claimZoneBatchAggregateRead()).toBe(false);

  // The end of the session drops the claim, so the next one reads the aggregate.
  clearZoneState();
  expect(claimZoneBatchAggregateRead()).toBe(true);
  clearZoneState();
});

test('readers are not notified when there is nothing to clear', () => {  let notifications = 0;
  const unsubscribe = subscribeZoneState(() => {
    notifications += 1;
  });

  clearZoneState();
  expect(notifications).toBe(0);
  unsubscribe();
});

test('ending a real session clears the selection, the colours and the filters', async () => {
  const unbind = bindZoneStateToSession();

  selectZone('05109');
  rememberZoneLevel('05109', 'alto');
  setHistoryQuery('05501');
  setHistoryStatus('publicado');

  await signIn();
  expect(getSessionSnapshot().status).toBe('signed-in');
  expect(getSelectedZone()).toBe('05109');

  // A logout, a 401 or a refused callback all end the session the same way: the
  // private view must stop describing the previous session's zone, and a search
  // term must not survive into the next one.
  resetSession();

  expect(getSelectedZone()).toBe('');
  expect(getZoneLevels().size).toBe(0);
  expect(getHistoryFilters()).toEqual({ query: '', status: '' });
  unbind();
});
