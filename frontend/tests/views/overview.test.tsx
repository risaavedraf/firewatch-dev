// N8b — the territory overview: the consolidated metrics, the real territorial
// map and the latest published batch with its evidence.
//
// The suite drives the view through the accessible surface plus the contracted
// identifiers, stubs `globalThis.fetch` and exercises the validity bound with
// fake timers. Every test title names the legacy readback assertion it ports.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { gatewayOrigin, issuer, pkceTransactionKey, routes } from '../../src/config';
import { clearZoneState, getSelectedZoneLabel } from '../../src/app/store';
import { completeLogin, getSessionSnapshot, resetSession } from '../../src/core/session';
import type { RiskBatch, ZoneCollection, ZoneFeature } from '../../src/types/api';
import { OverviewView } from '../../src/views/OverviewView';
import { createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const ZONES_URL = `${gatewayOrigin}${routes.zones}`;
const latestUrl = (zone: string): string => `${gatewayOrigin}${routes.latest}?zone_id=${zone}`;
const historyUrl = (zone: string): string => `${gatewayOrigin}${routes.history}?zone_id=${zone}`;
const MODELS_URL = `${gatewayOrigin}${routes.models}`;

// Two stubs are shared by the whole file and rebuilt before every test, so no
// test can inherit a pending request or a previous session from another.
let fetchStub: ReturnType<typeof createFetchStub>;
let storageStub: ReturnType<typeof createStorageStub>;

function prepare(): void {
  fetchStub = createFetchStub();
  storageStub = createStorageStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  resetSession();
  clearZoneState();
}

beforeEach(prepare);

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

// --- Fixtures --------------------------------------------------------------
//
// The geometry is schematic and intentionally shared: these tests are about the
// view contract, not about the projection, which the map unit already covers.

function ring(minLon = -71.7, minLat = -33.1, size = 0.1): ZoneFeature['geometry'] {
  return {
    type: 'Polygon',
    coordinates: [
      [
        [minLon, minLat],
        [minLon + size, minLat],
        [minLon + size, minLat + size],
        [minLon, minLat],
      ],
    ],
  };
}

function feature(code: string, name: string, province = 'Valparaíso'): ZoneFeature {
  return { type: 'Feature', properties: { code, name, province }, geometry: ring() };
}

function catalogue(features: ZoneFeature[]): ZoneCollection {
  return { type: 'FeatureCollection', features };
}

function catalogueOfSize(size: number): ZoneCollection {
  const features: ZoneFeature[] = [];
  for (let index = 0; index < size; index += 1) {
    const code = String(50000 + index);
    features.push(feature(code, `Comuna ${code}`));
  }
  return catalogue(features);
}

function batch(zone = '05101', id = 'batch-1', job = 'job-1', overrides: Partial<RiskBatch> = {}): RiskBatch {
  return {
    zone_id: zone,
    id,
    job_id: job,
    demo: true,
    risk_level: 'alto',
    factors: [{ name: 'temperatura', known: true, value: 30, contribution: 0.2, note: '' }],
    rationale: 'Regla académica',
    policy: { policy_id: 'academic-rule-v1', params_hash: 'hash', parameters: { weight: '0.2' } },
    notice: 'Sintético',
    ...overrides,
  };
}

// --- Driving the view ------------------------------------------------------

function element(id: string): HTMLElement {
  const node = document.getElementById(id);
  if (node === null) throw new Error(`missing DOM ${id}`);
  return node;
}

function textOf(id: string): string {
  return element(id).textContent ?? '';
}

async function settle(): Promise<void> {
  await act(async () => {
    await flush();
  });
}

async function signIn(): Promise<void> {
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'v' }));
  const login = completeLogin('?code=c&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: jwt({ realm_access: { roles: ['viewer'] } }), id_token: jwt({ sub: 'u' }) });
  await login;
  fetchStub.calls.length = 0;
}

async function openSignedIn(): Promise<void> {
  await signIn();
  render(<OverviewView />);
  await settle();
}

async function openSignedOut(): Promise<void> {
  render(<OverviewView />);
  await settle();
}

async function serveCatalogue(collection: ZoneCollection): Promise<void> {
  fetchStub.answer(ZONES_URL, collection);
  await settle();
}

async function serveZone(
  zone: string,
  latestBody: unknown,
  latestStatus = 200,
  historyBody: unknown = [],
  historyStatus = 200,
): Promise<void> {
  fetchStub.answer(latestUrl(zone), latestBody, latestStatus);
  fetchStub.answer(historyUrl(zone), historyBody, historyStatus);
  await settle();
}

async function choose(zone: string): Promise<void> {
  fireEvent.change(screen.getByRole('combobox', { name: 'Zona a consultar' }), { target: { value: zone } });
  await settle();
}

function latestCallCount(): number {
  return fetchStub.calls.filter(call => call.url.includes(routes.latest)).length;
}

function historyCallCount(): number {
  return fetchStub.calls.filter(call => call.url.includes(routes.history)).length;
}

function mapIsHidden(): boolean {
  return element('map-svg').hasAttribute('hidden');
}

function selector(): HTMLSelectElement {
  return element('zone-select') as HTMLSelectElement;
}

async function logOut(): Promise<void> {
  act(() => {
    resetSession();
  });
  await settle();
}

// --- Contract surface ------------------------------------------------------

test('the view keeps the overview identity, every contracted identifier and the coverage note', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone('05101', null, 404);

  const section = document.getElementById('overview');
  expect(section).not.toBeNull();
  expect(section).toHaveAttribute('data-view', 'overview');

  for (const id of [
    'zone-count',
    'metric-batches',
    'metric-high',
    'risk-pill',
    'risk-status',
    'risk-level',
    'zone-code',
    'zone-name',
    'zone-province',
    'batch-origin',
    'batch-notice',
    'batch-validity',
    'risk-factors',
    'risk-rationale',
    'risk-policy',
    'data-status',
    'model-version',
    'valid-until',
    'batch-id',
    'input-scope-badge',
  ]) {
    expect(document.getElementById(id), `missing DOM ${id}`).not.toBeNull();
  }

  // The "Cobertura por sector" limitation stays inside this view.
  expect(screen.getByText(/Cobertura por sector/)).toBeInTheDocument();
});

test('without a session the metrics stay unknown and the map states the connection requirement', async () => {
  await openSignedOut();

  expect(textOf('zone-count')).toBe('—');
  expect(textOf('metric-batches')).toBe('—');
  expect(textOf('metric-high')).toBe('0');
  expect(textOf('risk-pill')).toBe('SIN CONSULTAR');
  expect(textOf('risk-status')).toMatch(/Conecta un token autorizado/);
  expect(mapIsHidden()).toBe(true);
  expect(textOf('map-empty')).toBe('Conecta un token autorizado para cargar las zonas analizadas.');
});

test('zone count derives from the actual /zones feature length, never a copied constant', async () => {
  await openSignedIn();
  await serveCatalogue(catalogueOfSize(38));
  expect(textOf('zone-count')).toBe('38');
  expect(textOf('zone-count')).not.toBe('39');

  cleanup();
  prepare();
  await openSignedIn();
  await serveCatalogue(catalogueOfSize(3));
  expect(textOf('zone-count')).toBe('3');
});

test('metric cards report the real catalogue, history and read-zone counts', async () => {
  await openSignedIn();

  // Nothing was asked yet: the catalogue count is explicitly unknown, not zero.
  expect(textOf('zone-count')).toBe('—');

  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca')]));
  expect(textOf('zone-count')).toBe('2');

  // The auto-selected zone's history is still in flight, so the metric is unset.
  expect(textOf('metric-batches')).toBe('—');
  expect(textOf('metric-high')).toBe('0');

  // An empty history is a real zero, never a blank.
  await serveZone('05101', null, 404, []);
  expect(textOf('metric-batches')).toBe('0');

  await choose('05102');
  await serveZone('05102', batch('05102'), 200, [batch('05102'), batch('05102', 'second', 'job2')]);
  expect(textOf('metric-batches')).toBe('2');
  expect(textOf('metric-high')).toBe('1');

  await logOut();
  expect(textOf('metric-high')).toBe('0');
  expect(textOf('metric-batches')).toBe('—');
  expect(textOf('zone-count')).toBe('No informado');
});

test('the catalogue the view loads is the only source of the shared chip name', async () => {
  await openSignedIn();
  // Before any catalogue arrives there is no name to report, not a provisional one.
  expect(getSelectedZoneLabel()).toBe('');

  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca')]));
  // The auto-selected first code is named with the same label the map selector
  // shows, and only because the served catalogue carried it.
  expect(getSelectedZoneLabel()).toBe('Valparaíso (05101)');
});

test('a healthy session still renders the live catalogue count and keeps selection working', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca'), feature('05201', 'Juan Fernández')]));
  expect(textOf('zone-count')).toBe('3');

  // The map shape is the accessible selection surface, not just the select.
  fireEvent.click(screen.getByRole('button', { name: 'Casablanca (05102)' }));
  await settle();
  expect(selector().value).toBe('05102');

  await serveZone('05102', null, 404, []);
  expect(textOf('risk-pill')).toBe('SIN LOTE');
});

test('selection reads latest and history; a 404 is absence, never low risk', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca')]));
  await serveZone('05101', null, 404, []);

  await choose('05102');
  expect(latestCallCount()).toBe(2);
  expect(historyCallCount()).toBe(2);

  await serveZone('05102', null, 404, []);
  expect(textOf('risk-pill')).toBe('SIN LOTE');
  expect(textOf('risk-level')).toBe('Sin lote publicado');
  expect(textOf('risk-level')).not.toBe('BAJO');
  expect(textOf('metric-batches')).toBe('0');
  expect(element('risk-factors').children).toHaveLength(0);
});

test('wrong-zone and superseded latest responses cannot render; catalogue geometry stays unchanged', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca')]));
  await serveZone('05101', batch('05101', 'first'), 200, [batch('05101', 'first')]);

  const before = element('map-svg').querySelectorAll('path').length;

  // A to B to A: the first A answer is superseded by the second A request.
  await choose('05102');
  await choose('05101');
  fetchStub.answer(latestUrl('05102'), batch('05102', 'obsolete'));
  fetchStub.answer(historyUrl('05102'), [batch('05102', 'obsolete')]);
  fetchStub.answer(latestUrl('05101'), batch('05101', 'current'));
  fetchStub.answer(historyUrl('05101'), [batch('05101', 'current')]);
  await settle();
  expect(textOf('batch-id')).toBe('current');
  expect(textOf('batch-id')).not.toBe('obsolete');

  // A batch the server computed for another zone is discarded as a whole.
  await choose('05102');
  await serveZone('05102', batch('05101', 'wrong-zone'), 200, []);
  expect(textOf('risk-status')).toMatch(/otra zona/);
  expect(textOf('risk-pill')).toBe('NO DISPONIBLE');
  expect(textOf('batch-id')).not.toBe('wrong-zone');
  expect(element('map-svg').querySelectorAll('path').length).toBe(before);
});

test('an unreadable batch body renders as explicitly unavailable, never as a plausible level', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  // `rationale` is required by the boundary guard, so this body is rejected and
  // must never be shown as a severity the API did not state.
  await serveZone('05101', { ...batch('05101'), rationale: undefined }, 200, []);

  expect(textOf('risk-pill')).toBe('NO DISPONIBLE');
  expect(textOf('risk-status')).toMatch(/ilegible/);
  expect(textOf('batch-id')).toBe('Sin consultar');
  expect(textOf('risk-level')).not.toMatch(/BAJO|MEDIO|ALTO/);
});

test('unknown level, false demo discrepancy, loading and logout clear prior details', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca')]));
  await serveZone('05101', null, 404, []);

  await choose('05102');
  await serveZone('05102', batch('05102', 'b', 'j', { demo: false, risk_level: 'desconocido' }), 200, [batch('05102')]);
  expect(textOf('risk-pill')).toBe('INFORMACIÓN INSUFICIENTE');
  expect(textOf('risk-level')).toBe('INFORMACIÓN INSUFICIENTE');
  expect(textOf('batch-origin')).toMatch(/discrepante/);
  expect(textOf('batch-notice')).toMatch(/no riesgo real observado/);

  // A new selection clears the previous batch's details while the read is in flight.
  await choose('05101');
  expect(element('risk-factors').children).toHaveLength(0);
  expect(textOf('risk-policy')).toBe('Sin consultar');
  expect(textOf('risk-rationale')).toBe('Sin consultar');

  await logOut();
  expect(textOf('zone-code')).toBe('No informado');
  expect(element('risk-factors').children).toHaveLength(0);
  expect(textOf('risk-policy')).toBe('Sin consultar');
  expect(textOf('risk-pill')).toBe('SIN CONSULTAR');
});

test('lot validity from published_at/valid_until is separate from the synthetic recency notice', async () => {
  const future = new Date(Date.now() + 86_400_000).toISOString();
  const past = new Date(Date.now() - 86_400_000).toISOString();

  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone('05101', batch('05101', 'b', 'j', { published_at: past, valid_until: future }), 200, [batch('05101')]);
  expect(textOf('batch-validity')).toMatch(/Vigente/i);
  expect(textOf('batch-validity')).toMatch(/publicado/i);
  expect(textOf('batch-notice')).toMatch(/no riesgo real observado/i);
  expect(textOf('batch-notice')).not.toMatch(/vigente|vencida/i);

  cleanup();
  prepare();
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone('05101', batch('05101', 'b', 'j', { published_at: past, valid_until: past }), 200, [batch('05101')]);
  expect(textOf('batch-validity')).toMatch(/Vencida/i);
  expect(textOf('batch-notice')).toMatch(/no riesgo real observado/i);
});

test('batch validity label re-evaluates on its own bounded timer, stops at or before valid_until and logout clears it', async () => {
  vi.useFakeTimers();
  const start = new Date('2026-01-01T00:00:00.000Z');
  vi.setSystemTime(start);

  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso'), feature('05102', 'Casablanca'), feature('05103', 'Quilpué')]));

  const windowMs = 3000;
  const published = new Date(start.getTime() - 1000).toISOString();
  const until = new Date(start.getTime() + windowMs).toISOString();
  await serveZone('05101', batch('05101', 'b', 'j', { published_at: published, valid_until: until }), 200, [batch('05101')]);
  expect(textOf('batch-validity')).toMatch(/Vigente/);

  // The flip is scheduled no later than valid_until; the skew may only expire
  // the claim early, never keep it alive past the declared bound.
  await act(async () => {
    vi.advanceTimersByTime(windowMs);
  });
  expect(textOf('batch-validity')).toMatch(/Vencida/);
  expect(textOf('batch-validity')).not.toMatch(/Vigente/);
  expect(Date.now()).toBeLessThanOrEqual(Date.parse(until) + 1);

  // A new selection arms a fresh bound, then logout clears the label for good.
  await choose('05102');
  await serveZone('05102', batch('05102', 'b2', 'j2', { published_at: published, valid_until: new Date(Date.now() + windowMs).toISOString() }), 200, [batch('05102')]);
  expect(textOf('batch-validity')).toMatch(/Vigente/);

  await logOut();
  expect(textOf('batch-validity')).toBe('Sin consultar');
  await act(async () => {
    vi.advanceTimersByTime(60_000);
  });
  expect(textOf('batch-validity')).toBe('Sin consultar');
});

test('a 401 reset clears a live validity bound instead of keeping the claim', async () => {
  vi.useFakeTimers();
  const start = new Date('2026-01-01T00:00:00.000Z');
  vi.setSystemTime(start);

  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone('05101', batch('05101', 'b', 'j', { published_at: start.toISOString(), valid_until: new Date(start.getTime() + 20_000).toISOString() }), 200, [batch('05101')]);
  expect(textOf('batch-validity')).toMatch(/Vigente/);

  // The transport resets the session on 401; the view must drop the bound.
  act(() => {
    resetSession();
  });
  await settle();
  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(textOf('batch-validity')).toBe('Sin consultar');
  await act(async () => {
    vi.advanceTimersByTime(30_000);
  });
  expect(textOf('batch-validity')).toBe('Sin consultar');
});

test('synthetic warning independent of notice; unknown factor never displays zero', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone(
    '05101',
    batch('05101', 'b', 'j', {
      notice: '',
      rationale: '<img onerror=evil>',
      factors: [{ name: 'viento', known: false, note: 'ausente', value: 0, contribution: 0 }],
    }),
    200,
    [batch('05101')],
  );

  expect(textOf('batch-origin')).toMatch(/Sintético/);
  expect(textOf('batch-notice')).toMatch(/no riesgo real observado/);
  expect(textOf('batch-notice')).toMatch(/ausente o inválido/);
  // The rationale is reproduced as text, never interpreted as markup.
  expect(textOf('risk-rationale')).toBe('<img onerror=evil>');
  expect(element('risk-factors').children[0].textContent).not.toMatch(/0/);
});

test.each([
  ['missing', undefined],
  ['malformed', {}],
])('demo mandatory warning with %s notice', async (_label, notice) => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  const incoming = { ...batch('05101'), notice };
  await serveZone('05101', incoming, 200, [incoming]);
  expect(textOf('batch-origin')).toMatch(/Sintético/);
  expect(textOf('batch-notice')).toMatch(/no riesgo real observado/);
  expect(textOf('batch-notice')).toMatch(/ausente o inválido/);
  expect(textOf('batch-notice')).not.toContain('[object Object]');
});

test('factor bars use the real contribution and a factor without data draws no bar', async () => {
  await openSignedIn();
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone(
    '05101',
    batch('05101', 'b', 'j', {
      factors: [
        { name: 'temperatura', known: true, value: 30, contribution: 0.5, note: '' },
        { name: 'humedad', known: true, value: 10, contribution: 0.25, note: '' },
        { name: 'viento', known: false, value: 0, contribution: 0, note: 'sin dato' },
      ],
    }),
    200,
    [batch('05101')],
  );

  const items = element('risk-factors').children;
  expect(items).toHaveLength(3);
  const trackOf = (item: Element): HTMLElement | null => item.querySelector('.factor-track');
  const widthOf = (item: Element): string => (trackOf(item)?.firstElementChild as HTMLElement).style.width;
  expect(widthOf(items[0])).toBe('100%');
  expect(widthOf(items[1])).toBe('50%');
  expect(trackOf(items[2])).toBeNull();
});

test('superseded /v1/risk/zones after a 401 reset cannot repopulate map, count, selector or risk status', async () => {
  await openSignedIn();
  // The catalogue read is in flight when the 401 arrives.
  fetchStub.answer(ZONES_URL, { code: 'unauthorized' }, 401);
  await settle();

  expect(getSessionSnapshot().status).toBe('signed-out');
  expect(textOf('zone-count')).toBe('No informado');
  const clearedRisk = textOf('risk-status');
  // The catalogue read carried the bearer, so this is a rejected token, not a
  // first visit: the panel must name the rejection (S2).
  expect(clearedRisk).toMatch(/rechazó el token/i);
  expect(mapIsHidden()).toBe(true);
  expect(selector().disabled).toBe(true);

  // No late read may chain a selection, and no zone read may have been issued.
  expect(textOf('risk-status')).toBe(clearedRisk);
  expect(latestCallCount()).toBe(0);
  expect(historyCallCount()).toBe(0);
});

test('superseded /v1/risk/zones after logout cannot repopulate state nor chain latest/history reads', async () => {
  await openSignedIn();
  await logOut();
  const clearedRisk = textOf('risk-status');

  // The response lands after the session closed: it owns nothing.
  fetchStub.answer(ZONES_URL, catalogue([feature('05101', 'Valparaíso')]));
  await settle();

  expect(textOf('zone-count')).toBe('No informado');
  expect(textOf('metric-batches')).toBe('—');
  expect(mapIsHidden()).toBe(true);
  expect(selector().disabled).toBe(true);
  expect(textOf('risk-status')).toBe(clearedRisk);
  expect(latestCallCount()).toBe(0);
  expect(historyCallCount()).toBe(0);
});

// S2 — a rejected token is the visible consequence of the model change: the
// map's empty message and the risk panel name the rejection instead of telling
// the operator they never connected.
test('a token-bearing 401 names the rejection in the map and the risk panel (S2)', async () => {
  await openSignedIn();
  fetchStub.answer(ZONES_URL, { code: 'unauthorized' }, 401);
  await settle();

  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: 'rejected' });
  expect(textOf('map-empty')).not.toBe('Conecta un token autorizado para cargar las zonas analizadas.');
  expect(textOf('map-empty')).toMatch(/rechazó el token/i);
  expect(textOf('risk-status')).toMatch(/rechazó el token/i);
});

// S6 — the reason belongs to the rejected session only: once the session is
// closed (the state `signOut` publishes) the anonymous copy is the correct one.
test('closing a rejected session returns to the anonymous copy (S6)', async () => {
  await openSignedIn();
  fetchStub.answer(ZONES_URL, { code: 'unauthorized' }, 401);
  await settle();
  expect(getSessionSnapshot().reason).toBe('rejected');

  await logOut();

  expect(getSessionSnapshot()).toEqual({ status: 'signed-out', roles: null, reason: null });
  expect(textOf('map-empty')).toBe('Conecta un token autorizado para cargar las zonas analizadas.');
  expect(textOf('risk-status')).toBe('Conecta un token autorizado para consultar la API.');
});

// T13 — the active model version comes from the model registry (`GET /v1/models`,
// an OBJECT ENVELOPE `{models: [...]}`) and is labelled with the `synthetic` mark
// the service declares. An undeclared mark is stated as undeclared, never assumed
// to be non-synthetic; an unreachable or wrongly-shaped registry is stated, never
// replaced with a plausible version.
test('shows the active model version from the {models} envelope with its declared synthetic mark (T13)', async () => {
  await openSignedIn();

  fetchStub.answer(MODELS_URL, {
    models: [
      {
        version: 'academic-rule-v1',
        kind: 'rule',
        params_hash: 'e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7',
        approved_at: null,
        metrics: { official: false, calibrated_against_observed_fires: false },
        active: false,
      },
      { version: 'modelo-sintetico', kind: 'rule', params_hash: null, approved_at: null, metrics: { synthetic: true }, active: true },
    ],
  });
  await settle();

  expect(textOf('model-active-version')).toBe('modelo-sintetico');
  expect(textOf('model-synthetic')).toBe('SINTÉTICO');
  expect(textOf('model-status')).toMatch(/sintétic/i);
});

test('the real default active row declares no synthetic mark, and a wrong envelope is rejected (T13)', async () => {
  await openSignedIn();
  // The exact default seed: academic-rule-v1 active, firewatch-rule-v2 and
  // demo-fixture-v1 registered but inactive. Neither academic-rule-v1 nor
  // firewatch-rule-v2 declares `synthetic`, so the honest answer is "not declared".
  fetchStub.answer(MODELS_URL, {
    models: [
      {
        version: 'academic-rule-v1',
        kind: 'rule',
        params_hash: 'e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7',
        approved_at: null,
        metrics: { official: false, calibrated_against_observed_fires: false },
        active: true,
      },
      {
        version: 'firewatch-rule-v2',
        kind: 'rule',
        params_hash: '59259c4c0c470e5a0d512fe09f6c748c38fd29f9bc5d760d49754555c66e699b',
        approved_at: null,
        metrics: { official: false, calibrated_against_observed_fires: false, aggregation: 'dominant_factor' },
        active: false,
      },
      { version: 'demo-fixture-v1', kind: 'placeholder', params_hash: null, approved_at: null, metrics: { official: false, synthetic: true }, active: false },
    ],
  });
  await settle();

  expect(textOf('model-active-version')).toBe('academic-rule-v1');
  expect(textOf('model-synthetic')).toBe('NO DECLARADO');
  expect(textOf('model-status')).toMatch(/no declara/i);

  cleanup();
  prepare();
  await openSignedIn();
  // The bare array was the assumed shape and is NOT what the service serves; it
  // must render as unavailable rather than as a parsed registry.
  fetchStub.answer(MODELS_URL, [{ version: 'academic-rule-v1', active: true }]);
  await settle();
  expect(textOf('model-active-version')).toBe('No informada');
  expect(textOf('model-synthetic')).toBe('NO DISPONIBLE');
});

// T2/S1 — the panel must present the registry version and the version that really
// classified the newest batch as two different facts, and it must state when an
// activation applies: to publications whose classifier resolution happens after
// the call, never to one already in flight. The sentence this replaces claimed the
// running classifier was fixed by MODEL_ID at startup and that activating a
// version moved only the registry, which was exactly the defect.
test('separates the registry version from the batch version and states when an activation applies (T2)', async () => {
  await openSignedIn();
  fetchStub.answer(MODELS_URL, {
    models: [
      {
        version: 'firewatch-rule-v2',
        kind: 'rule',
        params_hash: '59259c4c0c470e5a0d512fe09f6c748c38fd29f9bc5d760d49754555c66e699b',
        approved_at: '2026-10-18T12:00:00Z',
        metrics: { official: false, activation: { at: '2026-10-18T12:00:00Z' } },
        active: true,
      },
      {
        version: 'academic-rule-v1',
        kind: 'rule',
        params_hash: 'e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7',
        approved_at: null,
        metrics: { official: false, calibrated_against_observed_fires: false },
        active: false,
      },
    ],
  });
  await settle();
  // The batch was resolved before the activation, so it still names v1 while the
  // registry already presents v2. That difference is legitimate and the panel must
  // show both instead of presenting one as the other.
  await serveCatalogue(catalogue([feature('05101', 'Valparaíso')]));
  await serveZone('05101', batch('05101', 'b', 'j', { model_version: 'academic-rule-v1' }), 200, [
    batch('05101', 'b', 'j', { model_version: 'academic-rule-v1' }),
  ]);

  expect(textOf('model-active-version')).toBe('firewatch-rule-v2');
  expect(textOf('model-batch-version')).toBe('academic-rule-v1');

  const note = textOf('model-panel-note');
  expect(note).toMatch(/registro/i);
  expect(note).toMatch(/lote/i);
  expect(note).toMatch(/después de esa llamada/i);
  expect(note).toMatch(/en curso/i);
  expect(note).toMatch(/MODEL_ID/);
  expect(note).toMatch(/sembrada como activa/i);
  expect(note).not.toMatch(/fija su versión con MODEL_ID al arrancar, así que/i);
  expect(note).not.toMatch(/no el clasificador que está en uso/i);
  expect(note).not.toMatch(/recrear el servicio/i);
});
