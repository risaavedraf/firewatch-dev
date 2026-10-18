import { expect, test } from 'vitest';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import { preventiveAction, preventiveActions, type Failure, type Outcome } from '../../src/core/api';
import { completeLogin, resetSession } from '../../src/core/session';

import { createCryptoStub, createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// The shapes below are what the running service actually emits, taken from its own
// structs (`prevention/internal/repository/store.go`): `assignee` and `due_date`
// are `*string`, so an unassigned action answers `null`, and `action_history.id`
// is an `int64`, so it arrives as a JSON number.
//
// The browser check found what no unit test had: the transport rejected a real 200
// list and a real 201 create as `invalid_shape`, so the view showed no list and
// opened no detail. These cases pin the real payload, and the last one pins that
// the guard was loosened for the int64 and not for any shape at all.

const ACTIONS_URL = `${gatewayOrigin}/v1/prevention/actions`;
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;

const UNASSIGNED = {
  id: 'b22314ca-6dd8-489c-a265-2990244cb78a',
  community_code: '05101',
  title: 'T17 preventive action',
  description: '',
  priority: '',
  status: 'propuesta',
  assignee: null,
  due_date: null,
  created_by: 'demo-admin',
  created_at: '2026-10-18T15:56:21.588946Z',
  updated_at: '2026-10-18T15:56:21.588946Z',
};

function detail(history: unknown[]): Record<string, unknown> {
  return {
    ...UNASSIGNED,
    status: 'asignada',
    assignee: 'equipo-prevencion',
    due_date: '2026-11-01',
    updated_at: '2026-10-18T15:57:00.100000Z',
    history,
  };
}

const REAL_HISTORY = [
  {
    id: 1,
    action_id: UNASSIGNED.id,
    from_status: 'propuesta',
    to_status: 'asignada',
    actor_sub: 'demo-admin',
    note: 'asignada al equipo',
    created_at: '2026-10-18T15:57:00.100000Z',
  },
  {
    id: 2,
    action_id: UNASSIGNED.id,
    from_status: 'asignada',
    to_status: 'en_curso',
    actor_sub: 'demo-admin',
    note: '',
    created_at: '2026-10-18T15:58:00.200000Z',
  },
];

let fetchStub: ReturnType<typeof createFetchStub>;
let storageStub: ReturnType<typeof createStorageStub>;

async function connect(): Promise<void> {
  fetchStub = createFetchStub();
  storageStub = createStorageStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  vi.stubGlobal('crypto', createCryptoStub());
  resetSession();
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'verifier' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: jwt({ realm_access: { roles: ['viewer'] } }), id_token: 'id-token' });
  await login;
  fetchStub.calls.length = 0;
}

function expectFailure<T>(outcome: Outcome<T>): Failure {
  if (outcome.ok) throw new Error('expected a failure outcome');
  return outcome.failure;
}

test('a real list with null assignee and null due_date is accepted', async () => {
  await connect();

  const read = preventiveActions();
  await flush();
  expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1);
  fetchStub.answer(ACTIONS_URL, [UNASSIGNED]);

  const outcome = await read;
  expect(outcome.ok).toBe(true);
  if (outcome.ok) {
    expect(outcome.data[0].assignee).toBeNull();
    expect(outcome.data[0].status).toBe('propuesta');
  }
});

test('a real detail whose history carries int64 ids is accepted', async () => {
  await connect();

  const read = preventiveAction(UNASSIGNED.id);
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${UNASSIGNED.id}`, detail(REAL_HISTORY));

  const outcome = await read;
  expect(outcome.ok).toBe(true);
  if (outcome.ok) {
    expect(outcome.data.history.map(entry => entry.id)).toEqual([1, 2]);
    expect(outcome.data.assignee).toBe('equipo-prevencion');
  }
});

test('a history id the service cannot produce is still refused', async () => {
  await connect();

  const read = preventiveAction(UNASSIGNED.id);
  await flush();
  fetchStub.answer(`${ACTIONS_URL}/${UNASSIGNED.id}`, detail([{ ...REAL_HISTORY[0], id: '1' }]));

  // A string here is a contract change, not a tolerance: it must keep failing
  // closed instead of rendering a row the server never sent.
  expect(expectFailure(await read).reason).toBe('invalid_shape');
});
