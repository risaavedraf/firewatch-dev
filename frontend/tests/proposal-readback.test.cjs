const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const html = fs.readFileSync(`${__dirname}/../index.html`, 'utf8');
const ids = new Set([...html.matchAll(/id="([^"]+)"/g)].map(m => m[1]));
const source = fs.readFileSync(`${__dirname}/../proposal.js`, 'utf8');
// The gateway API now lives on its own origin; the page is served from :4173.
// Every gateway call must be absolute, while `/token` still resolves against the
// Keycloak issuer.
const GATEWAY_ORIGIN = 'http://localhost:8080';
const ISSUER = 'http://localhost:8081/realms/firewatch';
const tick = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };
function harness(catalogue, catalogueStatus = 200, deferZones = false) {
  const nodes = new Map(), calls = [], pending = [], timers = [];
  class Element {
    constructor(svg = false) { this.svg = svg; this.attributes = new Map(); this.children = []; this.listeners = {}; this.value = ''; this.textContent = ''; this.classes = new Set(); this.classList = { add: (...v) => v.forEach(x => this.classes.add(x)), remove: (...v) => v.forEach(x => this.classes.delete(x)), toggle: (x, on) => on ? this.classes.add(x) : this.classes.delete(x) }; }
    append(...v) { this.children.push(...v); }
    replaceChildren(...v) { this.children = v; }
    get hidden() { return this.svg ? this.hiddenExpando : this.hasAttribute('hidden'); }
    set hidden(v) { if (this.svg) this.hiddenExpando = v; else if (v) this.setAttribute('hidden', ''); else this.removeAttribute('hidden'); }
    setAttribute(k, v) { this.attributes.set(k, String(v)); if (k !== 'hidden') this[k] = v; }
    removeAttribute(k) { this.attributes.delete(k); }
    hasAttribute(k) { return this.attributes.has(k); }
    addEventListener(k, fn) { this.listeners[k] = fn; }
  }
  const node = id => { assert.ok(ids.has(id) || ['.history-panel', '.health-dot'].includes(id), `missing DOM ${id}`); if (!nodes.has(id)) { const el = new Element(id === 'map-svg'); const tag = [...html.matchAll(/<[^>]+id="([^"]+)"[^>]*>/g)].find(m => m[1] === id)?.[0]; if (tag && /\shidden(?:\s|>)/.test(tag)) el.setAttribute('hidden', ''); nodes.set(id, el); } return nodes.get(id); };
  const polygon = { type: 'Polygon', coordinates: [[[-71,-33],[-70,-33],[-70,-32],[-71,-33]]] };
  const zones = catalogue || { type: 'FeatureCollection', features: ['05101','05102'].map(code => ({ properties: { code, name: code }, geometry: code === '05101' ? polygon : { type: 'MultiPolygon', coordinates: [polygon.coordinates] } })) };
  const fetch = (path, options) => {
    calls.push({ path, options });
    if (path === `${GATEWAY_ORIGIN}/health/ready`) return Promise.resolve({ ok: true, status: 204 });
    if (path === `${ISSUER}/protocol/openid-connect/token`) return Promise.resolve({ ok: true, status: 200, json: async () => ({ access_token: 'test' }) });
    if (path === `${GATEWAY_ORIGIN}/v1/risk/zones` && !deferZones) return Promise.resolve({ ok: catalogueStatus < 400, status: catalogueStatus, json: async () => zones });
    return new Promise(resolve => pending.push({ path, resolve }));
  };
  vm.runInNewContext(source, { document: { getElementById: node, querySelector: node, createElement: () => new Element(), createElementNS: () => new Element(true) }, fetch, URL, URLSearchParams, window: { location: { search: '?code=c&state=s', pathname: '/', hash: '', assign() {} }, history: { replaceState() {} } }, sessionStorage: { getItem: () => JSON.stringify({ state: 's', verifier: 'v' }), removeItem() {} }, crypto: { randomUUID: () => 'key' }, setTimeout: (fn, delay) => { fn.delay = delay; timers.push(fn); return timers.length; }, clearTimeout: id => { timers[id - 1] = null; }, AbortController });
  // Calls recorded by the stub are absolute, so a caller may keep passing the
  // relative gateway path and have it resolved against the expected origin.
  // Matching stays exact on the resolved URL, never a weakened substring match.
  function answer(path, body, status = 200) { const resolved = path.startsWith('http') ? path : `${GATEWAY_ORIGIN}${path}`; const i = pending.findIndex(x => x.path === resolved); assert.notEqual(i, -1, `pending ${resolved}`); pending.splice(i, 1)[0].resolve({ ok: status < 400, status, json: async () => body }); }
  const batch = (zone = '05101', id = 'batch', job = 'job') => ({ zone_id: zone, id, job_id: job, demo: true, risk_level: 'alto', factors: [{name: 'temperatura', known: true, value: 30, contribution: 0.2, note: ''}], rationale: 'Regla académica', policy: {policy_id: 'academic-rule-v1', params_hash: 'hash', parameters: {weight: '0.2'}}, notice: 'Sintético', geometry: polygon });
  const scenarioCatalogue = [{ id: 'mild', description: 'Perfil suave' }, { id: 'dry-windy', description: 'Seco y ventoso' }];
  return { node, calls, pending, timers, answer, batch, scenarioCatalogue, async ready() { await tick(); answer('/v1/risk/synthetic-scenarios', scenarioCatalogue); answer('/v1/risk/latest?zone_id=05101', {}, 404); answer('/v1/risk/history?zone_id=05101', []); await tick(); }, async create() { node('create-job').listeners.click(); await tick(); }, async select(code) { node('zone-select').listeners.change({ target: { value: code } }); await tick(); } };
}
test('actual POST uses CUT; published job automatically reads correlated latest and history', async () => {
  const h = harness(); await h.ready(); await h.create();
  assert.equal(JSON.parse(h.calls.find(x => x.path === `${GATEWAY_ORIGIN}/v1/jobs`).options.body).zone_id, '05101');
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'queued' }); await tick();
  assert.ok(h.pending.some(x => x.path === `${GATEWAY_ORIGIN}/v1/jobs/job`));
  const paths = h.node('map-svg').children[0].children.map(x => x.d);
  h.answer('/v1/jobs/job', { id: 'job', zone_id: '05101', state: 'published', batch_id: 'batch' }); await tick();
  h.answer('/v1/risk/latest?zone_id=05101', { ...h.batch(), geometry: { type: 'Point', coordinates: [-71, -33] } }); h.answer('/v1/risk/history?zone_id=05101', [h.batch()]); await tick();
  assert.equal(h.node('batch-id').textContent, 'batch');
  assert.deepEqual(h.node('map-svg').children[0].children.map(x => x.d), paths);
  assert.match(h.node('job-status').textContent, /verificado/);
  assert.match(h.node('risk-status').textContent, /demostración.*no.*riesgo real observado/i);
  assert.match(h.node('risk-status').textContent, /publicación.*observaciones/i);
});
test('selection cancels old create response; 404 remains absence, not low', async () => {
  const h = harness(); await h.ready(); await h.create(); await h.select('05102');
  h.answer('/v1/jobs', { id: 'old', zone_id: '05101', state: 'published', batch_id: 'old-batch' });
  h.answer('/v1/risk/latest?zone_id=05102', {}, 404); h.answer('/v1/risk/history?zone_id=05102', []); await tick();
  assert.notEqual(h.node('job-id').textContent, 'old'); assert.equal(h.node('risk-pill').textContent, 'SIN LOTE');
});
test('wrong CUT and superseded latest responses cannot render; catalogue geometry stays unchanged', async () => {
  const h = harness(); await h.ready(); await h.select('05102'); await h.select('05101');
  h.answer('/v1/risk/latest?zone_id=05102', h.batch('05102')); h.answer('/v1/risk/history?zone_id=05102', []);
  h.answer('/v1/risk/latest?zone_id=05101', h.batch('05102')); h.answer('/v1/risk/history?zone_id=05101', []); await tick();
  assert.notEqual(h.node('batch-id').textContent, 'batch'); assert.match(h.node('risk-status').textContent, /otra zona/);
});
test('older unrelated latest cannot complete newly published job', async () => {
  const h = harness(); await h.ready(); await h.create();
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'published', batch_id: 'batch' }); await tick();
  h.answer('/v1/risk/latest?zone_id=05101', h.batch('05101', 'older', 'other')); h.answer('/v1/risk/history?zone_id=05101', [h.batch('05101', 'older', 'other')]); await tick();
  assert.notEqual(h.node('batch-id').textContent, 'older'); assert.doesNotMatch(h.node('job-status').textContent, /verificado/i);
});
test('deadline fences even hung fetch; manual refresh and wrong job CUT stay honest', async () => {
  const h = harness(); await h.ready(); await h.create();
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'queued' }); await tick();
  h.timers.find(x => x && x.delay === 65000)(); await tick();
  assert.match(h.node('job-status').textContent, /agotado/);
  assert.equal(h.node('refresh-job').disabled, false);
  h.answer('/v1/jobs/job', { id: 'job', zone_id: '05101', state: 'published', batch_id: 'late' }); await tick();
  assert.ok(!h.pending.some(x => x.path.includes('/risk/latest')));
  h.node('refresh-job').listeners.click(); await tick();
  h.answer('/v1/jobs/job', { id: 'job', zone_id: '05102', state: 'published', batch_id: 'wrong' }); await tick();
  assert.match(h.node('job-status').textContent, /otro trabajo o zona/);
});
test('in-flight job GET is fenced on selection; MultiPolygon readback leaves catalogue paths intact', async () => {
  const h = harness(); await h.ready(); await h.create();
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'queued' }); await tick(); await h.select('05102');
  h.answer('/v1/jobs/job', { id: 'job', zone_id: '05101', state: 'published', batch_id: 'late' });
  const paths = h.node('map-svg').children[0].children.map(x => x.d);
  const b = { ...h.batch('05102'), geometry: { type: 'MultiPolygon', coordinates: [] } };
  h.answer('/v1/risk/latest?zone_id=05102', b); h.answer('/v1/risk/history?zone_id=05102', [b]); await tick();
  assert.deepEqual(h.node('map-svg').children[0].children.map(x => x.d), paths);
  await h.create(); h.answer('/v1/jobs', { id: 'new', zone_id: '05102', state: 'published', batch_id: 'new-batch' }); await tick(); await h.select('05101');
  h.answer('/v1/risk/latest?zone_id=05102', h.batch('05102', 'new-batch', 'new')); h.answer('/v1/risk/history?zone_id=05102', [h.batch('05102', 'new-batch', 'new')]);
  h.answer('/v1/risk/latest?zone_id=05101', {}, 404); h.answer('/v1/risk/history?zone_id=05101', []); await tick();
  assert.equal(h.node('risk-pill').textContent, 'SIN LOTE'); assert.notEqual(h.node('batch-id').textContent, 'new-batch');
});
test('superseded same-CUT jobs and forbidden errors cannot report completion', async () => {
  const h = harness(); await h.ready(); await h.create();
  h.answer('/v1/jobs', { id: 'first', zone_id: '05101', state: 'queued' }); await tick(); await h.create();
  h.answer('/v1/jobs/first', { id: 'first', zone_id: '05101', state: 'published', batch_id: 'obsolete' }); h.answer('/v1/jobs', {}, 403); await tick();
  assert.match(h.node('job-status').textContent, /403/); assert.notEqual(h.node('batch-id').textContent, 'obsolete');
});
test('queued responses schedule at most thirty polls', async () => {
  const h = harness(); await h.ready(); await h.create();
  const job = { id: 'job', zone_id: '05101', state: 'queued' };
  h.answer('/v1/jobs', job); await tick();
  for (let i = 0; i < 30; i++) {
    h.answer('/v1/jobs/job', job); await tick();
    const index = h.timers.findIndex(x => x && x.delay === 2000);
    if (i < 29) { assert.notEqual(index, -1); const fn = h.timers[index]; h.timers[index] = null; fn(); await tick(); }
  }
  assert.match(h.node('job-status').textContent, /agotó/); assert.ok(!h.pending.some(x => x.path === `${GATEWAY_ORIGIN}/v1/jobs/job`));
});
test('superseded unauthorized response cannot reset the current selection or session', async () => {
  const h = harness(); await h.ready(); await h.create(); await h.select('05102');
  h.answer('/v1/jobs', {}, 401); h.answer('/v1/risk/latest?zone_id=05102', {}, 404); h.answer('/v1/risk/history?zone_id=05102', []); await tick();
  assert.equal(h.node('session-state').textContent, 'Sesión conectada'); assert.equal(h.node('risk-pill').textContent, 'SIN LOTE');
});

test('map buttons and ficha are real DOM, keyboard and click share selection', async () => {
  const h = harness(); await h.ready();
  const shapes = h.node('map-svg').children[0].children;
  assert.equal(shapes[0].role, 'button');
  shapes[1].listeners.keydown({key: ' ', preventDefault() {}}); await tick();
  assert.equal(h.node('zone-select').value, '05102');
  assert.equal(shapes[1]['aria-pressed'], 'true');
  assert.equal(h.node('zone-code').textContent, '05102');
  assert.equal(h.node('zone-province').textContent, 'No informado');
});
for (const [field, value] of Object.entries({factors: [], rationale: '', demo: 'true', policy: {}, risk_level: 'HIGH'})) {
  test(`malformed ${field} refuses correlated completion and severity`, async () => {
    const h = harness(); await h.ready(); await h.create();
    h.answer('/v1/jobs', {id: 'job', zone_id: '05101', state: 'published', batch_id: 'batch'}); await tick();
    const b = {...h.batch(), [field]: value};
    h.answer('/v1/risk/latest?zone_id=05101', b); h.answer('/v1/risk/history?zone_id=05101', [b]); await tick();
    assert.doesNotMatch(h.node('job-status').textContent, /verificado/);
    assert.ok(!h.node('risk-status').classes.has('risk-good'));
    assert.ok(!h.node('map-svg').children[0].children[0].classes.has('level-alto'));
  });
}
test('synthetic warning independent of notice; unknown factor never displays zero', async () => {
  const h = harness(); await h.ready(); await h.select('05102');
  const b = {...h.batch('05102'), notice: {}, rationale: '<img onerror=evil>', factors: [{name: 'viento', known: false, note: 'ausente', value: 0, contribution: 0}]};
  h.answer('/v1/risk/latest?zone_id=05102', b); h.answer('/v1/risk/history?zone_id=05102', [b]); await tick();
  assert.match(h.node('batch-origin').textContent, /Sintético/);
  assert.match(h.node('batch-notice').textContent, /no riesgo real observado/);
  assert.match(h.node('batch-notice').textContent, /ausente o inválido/);
  assert.equal(h.node('risk-rationale').textContent, '<img onerror=evil>');
  assert.doesNotMatch(h.node('risk-factors').children[0].textContent, /0/);
});

const realCatalogue = JSON.parse(fs.readFileSync(`${__dirname}/../../backend/contracts/gen/geo/valparaiso-comunas.geojson`, 'utf8'));
const apiCatalogue = {type: 'FeatureCollection', features: realCatalogue.features.map(f => ({geometry: f.geometry, properties: {code: f.properties.CUT_COM, name: f.properties.COMUNA, province: f.properties.PROVINCIA}}))};
test('38 real polygons, both island frames, click/Enter/Space select catalogue identities', async () => {
  const h = harness(apiCatalogue);
  assert.equal(h.node('map-svg').hasAttribute('hidden'), true, 'initial/loading mainland remains hidden');
  assert.equal(h.node('map-empty').hidden, false);
  await h.ready();
  assert.equal(h.node('map-svg').hasAttribute('hidden'), false, 'rendered mainland must remove SVG hidden attribute');
  assert.equal(h.node('map-empty').hidden, true);
  const mainland = h.node('map-svg').children[0].children;
  const islands = h.node('map-insets').children.map(f => f.children[0].children[0].children[0]);
  assert.equal(mainland.length + islands.length, 38); assert.equal(islands.length, 2);
  for (const [shape, key] of [[mainland[1], null], [islands[0], 'Enter'], [islands[1], ' ']]) {
    if (key) shape.listeners.keydown({key, preventDefault() {}}); else shape.listeners.click();
    await tick();
    const code = shape['data-zone'];
    assert.equal(h.node('zone-select').value, code);
    assert.equal(shape['aria-pressed'], 'true');
    const f = apiCatalogue.features.find(f => f.properties.code === code);
    assert.equal(h.node('zone-name').textContent, f.properties.name);
    assert.equal(h.node('zone-province').textContent, f.properties.province);
    h.answer(`/v1/risk/latest?zone_id=${code}`, {}, 404); h.answer(`/v1/risk/history?zone_id=${code}`, []); await tick();
  }
});
test('empty and island-only catalogues keep mainland hidden; logout hides rendered mainland again', async () => {
  for (const features of [[], apiCatalogue.features.filter(f => ['05201', '05104'].includes(f.properties.code))]) {
    const h = harness({type: 'FeatureCollection', features}); await tick();
    assert.equal(h.node('map-svg').hasAttribute('hidden'), true);
    assert.equal(h.node('map-empty').hidden, false);
  }
  const failed = harness(undefined, 500); await tick();
  assert.equal(failed.node('map-svg').hasAttribute('hidden'), true);
  assert.equal(failed.node('map-empty').hidden, false);
  assert.equal(failed.node('map-insets').children.length, 0);
  const h = harness(); await h.ready();
  assert.equal(h.node('map-svg').hasAttribute('hidden'), false);
  h.node('logout').listeners.click(); await tick();
  assert.equal(h.node('map-svg').hasAttribute('hidden'), true);
  assert.equal(h.node('map-empty').hidden, false);
  assert.equal(h.node('map-insets').children.length, 0);
});
for (const notice of [undefined, {}]) test(`demo mandatory warning with ${notice === undefined ? 'missing' : 'malformed'} notice`, async () => {
  const h = harness(); await h.ready(); await h.select('05102');
  const b = {...h.batch('05102'), notice};
  h.answer('/v1/risk/latest?zone_id=05102', b); h.answer('/v1/risk/history?zone_id=05102', [b]); await tick();
  assert.match(h.node('batch-origin').textContent, /Synthetic/);
  assert.match(h.node('batch-notice').textContent, /no riesgo real observado/);
  assert.match(h.node('batch-notice').textContent, /ausente o inválido/);
});
test('unknown level, false demo discrepancy, loading and logout clear prior details', async () => {
  const h = harness(); await h.ready(); await h.select('05102');
  const b = {...h.batch('05102'), demo: false, risk_level: 'desconocido'};
  h.answer('/v1/risk/latest?zone_id=05102', b); h.answer('/v1/risk/history?zone_id=05102', [b]); await tick();
  assert.equal(h.node('risk-pill').textContent, 'INFORMACIÓN INSUFICIENTE');
  assert.match(h.node('batch-origin').textContent, /discrepante/);
  assert.match(h.node('batch-notice').textContent, /no riesgo real observado/);
  await h.select('05101');
  assert.equal(h.node('risk-factors').children.length, 0);
  assert.equal(h.node('risk-policy').textContent, 'Sin consultar');
  h.node('logout').listeners.click(); await tick();
  h.answer('/v1/risk/latest?zone_id=05101', h.batch()); h.answer('/v1/risk/history?zone_id=05101', [h.batch()]); await tick();
  assert.equal(h.node('zone-code').textContent, 'No informado');
  assert.equal(h.node('risk-factors').children.length, 0);
  assert.equal(h.node('session-state').textContent, 'Sin conexión');
});
test('A to B to A discards first A response, including details and history', async () => {
  const h = harness(); await h.ready(); await h.select('05101'); await h.select('05102'); await h.select('05101');
  h.answer('/v1/risk/latest?zone_id=05101', {...h.batch(), rationale: 'obsolete'});
  h.answer('/v1/risk/history?zone_id=05101', [h.batch()]);
  h.answer('/v1/risk/latest?zone_id=05102', h.batch('05102'));
  h.answer('/v1/risk/history?zone_id=05102', [h.batch('05102')]); await tick();
  assert.equal(h.node('risk-rationale').textContent, 'Sin consultar');
  h.answer('/v1/risk/latest?zone_id=05101', {...h.batch(), rationale: 'current'});
  h.answer('/v1/risk/history?zone_id=05101', [h.batch()]); await tick();
  assert.equal(h.node('risk-rationale').textContent, 'current');
});
test('valid latest cannot complete with malformed correlated history', async () => {
  const h = harness(); await h.ready(); await h.create();
  h.answer('/v1/jobs', {id: 'job', zone_id: '05101', state: 'published', batch_id: 'batch'}); await tick();
  h.answer('/v1/risk/latest?zone_id=05101', h.batch());
  h.answer('/v1/risk/history?zone_id=05101', [{...h.batch(), factors: [{name: 'viento', known: true, note: '', value: Infinity, contribution: 0}]}]); await tick();
  assert.doesNotMatch(h.node('job-status').textContent, /verificado/);
});

const STALE = () => new Date(Date.now() - 3600000).toISOString();
function provenanceBody(overrides = {}) {
  const provenance = {
    source_id: 'mop-idechile-dpa-comunas',
    provider: 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)',
    endpoint: 'https://rest-sit.mop.gob.cl/query',
    query_sha256: 'a'.repeat(64),
    source_format: 'Esri JSON', derived_format: 'GeoJSON (RFC 7946)',
    retrieved_at: '2026-10-11T18:42:42+00:00', cutoff_at: '2026-10-11T18:42:42+00:00',
    raw_path: 'data/raw/geo/x.esri.json', raw_sha256: 'b'.repeat(64),
    derived_path: 'backend/contracts/gen/geo/valparaiso-comunas.geojson', derived_sha256: 'c'.repeat(64),
    attribution: 'Subsecretaria de Desarrollo Regional y Administrativo (SUBDERE)',
    restriction: 'las publicaciones y productos derivados de esta cartografia deben ser revisadas y validadas por DIFROL.',
    restriction_source: 'Metadatos del servicio INTEROP/SERVICIO_DPA',
    notes: 'No se inventa fecha de corte.',
    record_count: 38, requested_srid: 4326, source_wkid: 5360, expected_record_count: 38,
    record_count_matches_expected: true, complete: true, exceeded_transfer_limit: false,
    ...overrides,
  };
  return {
    geography: {
      provenance,
      source_cutoff_at: null,
      cutoff_semantics: 'retrieval_substitution: stored cutoff_at equals retrieved_at; actual source cutoff is unknown',
      integrity: { algorithm: 'sha256', recorded_sha256: 'c'.repeat(64), computed_sha256: 'c'.repeat(64), matches: true },
    },
    risk_input: { id: 'fixture-sample', display_name: 'Muestra sintética', synthetic: true, license: 'N/A' },
  };
}
function statusBody(overrides = {}) {
  return {
    checked_at: new Date().toISOString(), ready: true, reason: '',
    dependencies: {
      database: { state: 'ready' }, broker: { state: 'ready' },
      dead_letter: { state: 'ready', depth: 0, sampled_at: new Date().toISOString(), age_seconds: 0, max_age_seconds: 120, max_depth: 1 },
    },
    ...overrides,
  };
}

async function answerPipeline(h, sources, risk, jobs) {
  if (sources) h.answer('/v1/risk/sources', sources.body, sources.status || 200);
  if (risk) h.answer('/v1/risk/status', risk.body, risk.status || 200);
  if (jobs) h.answer('/v1/jobs/status', jobs.body, jobs.status || 200);
  await tick();
}

test('provenance panel shows provider, attribution and restriction verbatim with the derived hash', async () => {
  const h = harness(); await h.ready();
  const body = provenanceBody();
  await answerPipeline(h, { body });
  assert.equal(h.node('sources-provider').textContent, body.geography.provenance.provider);
  assert.equal(h.node('sources-attribution').textContent, body.geography.provenance.attribution);
  assert.equal(h.node('sources-restriction').textContent, body.geography.provenance.restriction);
  assert.equal(h.node('sources-record-count').textContent, '38');
  assert.match(h.node('sources-derived-sha256').textContent, /c{8}/);
  assert.match(h.node('sources-integrity').textContent, /coincide/i);
  assert.match(h.node('sources-cutoff').textContent, /sin fecha de corte real/i);
  assert.ok(h.node('sources-status').classes.has('risk-good'));
});

test('provenance fails closed on 503 body, integrity mismatch and malformed payload', async () => {
  const h1 = harness(); await h1.ready();
  await answerPipeline(h1, { body: { code: 'geography_integrity_unavailable' }, status: 503 });
  assert.match(h1.node('sources-status').textContent, /no disponible/i);
  assert.match(h1.node('sources-status').textContent, /geography_integrity_unavailable/);
  assert.equal(h1.node('sources-provider').textContent, 'No disponible');
  assert.ok(!h1.node('sources-status').classes.has('risk-good'));
  const mismatch = provenanceBody(); mismatch.geography.integrity.matches = false;
  const h2 = harness(); await h2.ready();
  await answerPipeline(h2, { body: mismatch });
  assert.equal(h2.node('sources-provider').textContent, 'No disponible');
  assert.ok(!h2.node('sources-status').classes.has('risk-good'));
  const h3 = harness(); await h3.ready();
  await answerPipeline(h3, { body: { geography: {} } });
  assert.equal(h3.node('sources-provider').textContent, 'No disponible');
  assert.ok(!h3.node('sources-status').classes.has('risk-good'));
});

test('zone count derives from the actual /zones feature length, never a copied constant', async () => {
  const h = harness(apiCatalogue); await h.ready();
  assert.equal(h.node('zone-count').textContent, '38');
  assert.notEqual(h.node('zone-count').textContent, '39');
  const small = harness({ type: 'FeatureCollection', features: apiCatalogue.features.slice(0, 3) }); await small.ready();
  assert.equal(small.node('zone-count').textContent, '3');
});

test('lot validity from published_at/valid_until is separate from the synthetic recency notice', async () => {
  const future = new Date(Date.now() + 86400000).toISOString();
  const past = new Date(Date.now() - 86400000).toISOString();
  const h = harness(); await h.ready(); await h.select('05102');
  const live = { ...h.batch('05102'), published_at: past, valid_until: future };
  h.answer('/v1/risk/latest?zone_id=05102', live); h.answer('/v1/risk/history?zone_id=05102', [live]); await tick();
  assert.match(h.node('batch-validity').textContent, /vigente/i);
  assert.match(h.node('batch-validity').textContent, /publicado/i);
  assert.match(h.node('batch-notice').textContent, /no riesgo real observado/i);
  assert.doesNotMatch(h.node('batch-notice').textContent, /vigente|vencida/i);
  const h2 = harness(); await h2.ready(); await h2.select('05102');
  const expired = { ...h2.batch('05102'), published_at: past, valid_until: past };
  h2.answer('/v1/risk/latest?zone_id=05102', expired); h2.answer('/v1/risk/history?zone_id=05102', [expired]); await tick();
  assert.match(h2.node('batch-validity').textContent, /vencida/i);
  assert.match(h2.node('batch-notice').textContent, /no riesgo real observado/i);
});

test('batch validity label re-evaluates on its own bounded timer, stops at or before valid_until and logout clears it', async () => {
  const h = harness(); await h.ready(); await h.select('05102');
  const published = new Date(Date.now() - 1000).toISOString();
  const windowMs = 3000;
  const live = { ...h.batch('05102'), published_at: published, valid_until: new Date(Date.now() + windowMs).toISOString() };
  h.answer('/v1/risk/latest?zone_id=05102', live); h.answer('/v1/risk/history?zone_id=05102', [live]); await tick();
  assert.match(h.node('batch-validity').textContent, /Vigente/);
  assert.ok(!/setInterval\s*\(/.test(source), 'the validity re-evaluation must be setTimeout-only');
  // renderLotValidity clears the previous handle before arming its replacement,
  // so the validity timer is the most recently scheduled one.
  const armed = h.timers.length - 1;
  const timer = h.timers[armed];
  assert.ok(timer && timer.delay > 0, 'a bounded validity timer must be armed for a live batch');
  // Direction: the Vigente -> Vencida flip is scheduled at or before
  // valid_until. A clock-skew margin may only expire the label early, never
  // keep the claim alive past the bound the batch itself declares.
  assert.ok(timer.delay <= windowMs, `flip scheduled after valid_until: ${timer.delay}ms for a ${windowMs}ms window`);

  // Near expiry: whatever the margin, a 250ms window must not keep the Vigente
  // claim standing, and a late flip may never restore it.
  const near = harness(); await near.ready(); await near.select('05102');
  const closing = { ...near.batch('05102'), published_at: published, valid_until: new Date(Date.now() + 250).toISOString() };
  near.answer('/v1/risk/latest?zone_id=05102', closing); near.answer('/v1/risk/history?zone_id=05102', [closing]); await tick();
  const stillArmed = near.timers.findIndex(x => x && x.delay > 0 && x.delay < windowMs);
  if (/Vigente/.test(near.node('batch-validity').textContent)) {
    assert.notEqual(stillArmed, -1, 'a batch claiming Vigente must arm its flip timer');
    assert.ok(near.timers[stillArmed].delay <= 250, `a 250ms window may not schedule its flip at or after the bound; got ${near.timers[stillArmed].delay}ms`);
  } else {
    assert.match(near.node('batch-validity').textContent, /Vencida/);
  }
  await new Promise(resolve => setTimeout(resolve, 310));
  const flip = near.timers[stillArmed];
  if (flip) { near.timers[stillArmed] = null; flip(); await tick(); }
  assert.match(near.node('batch-validity').textContent, /Vencida/);
  assert.doesNotMatch(near.node('batch-validity').textContent, /Vigente/);

  // A newer batch replaces the timer, and logout must leave none live.
  await h.select('05102');
  const newer = { ...h.batch('05102'), published_at: published, valid_until: new Date(Date.now() + 30000).toISOString() };
  h.answer('/v1/risk/latest?zone_id=05102', newer); h.answer('/v1/risk/history?zone_id=05102', [newer]); await tick();
  assert.match(h.node('batch-validity').textContent, /Vigente/);
  const cleanup = h.timers.length - 1;
  assert.ok(h.timers[cleanup] && h.timers[cleanup].delay > 0 && h.timers[cleanup].delay <= 30000, 'the live batch must arm its validity timer again, inside the window');
  h.node('logout').listeners.click(); await tick();
  assert.equal(h.timers[cleanup], null, 'logout must clear the validity timer');
  assert.equal(h.node('batch-validity').textContent, 'Sin consultar');

  // The other two owner paths: a zone change and a 401 reset must each clear
  // the batch validity timer and drop the label.
  const zoned = harness(); await zoned.ready(); await zoned.select('05102');
  const zbatch = { ...zoned.batch('05102'), published_at: published, valid_until: new Date(Date.now() + 20000).toISOString() };
  zoned.answer('/v1/risk/latest?zone_id=05102', zbatch); zoned.answer('/v1/risk/history?zone_id=05102', [zbatch]); await tick();
  const ztimer = zoned.timers.length - 1;
  assert.ok(zoned.timers[ztimer] && zoned.timers[ztimer].delay > 0, 'the live batch must arm its validity timer');
  await zoned.select('05101');
  assert.equal(zoned.timers[ztimer], null, 'a zone change must clear the batch validity timer');
  assert.equal(zoned.node('batch-validity').textContent, 'Sin consultar');

  const reset = harness(); await reset.ready(); await reset.select('05102');
  const rbatch = { ...reset.batch('05102'), published_at: published, valid_until: new Date(Date.now() + 20000).toISOString() };
  reset.answer('/v1/risk/latest?zone_id=05102', rbatch); reset.answer('/v1/risk/history?zone_id=05102', [rbatch]); await tick();
  const rtimer = reset.timers.length - 1;
  assert.ok(reset.timers[rtimer] && reset.timers[rtimer].delay > 0, 'the live batch must arm its validity timer before the 401');
  reset.answer('/v1/risk/status', {}, 401); await tick();
  assert.equal(reset.node('session-state').textContent, 'Sin conexión');
  assert.equal(reset.timers[rtimer], null, 'a 401 reset must clear the batch validity timer');
  assert.equal(reset.node('batch-validity').textContent, 'Sin consultar');
});

test('a service is verified only with ready and a fresh checked_at; stale snapshots are unknown, never green', async () => {
  const h = harness(); await h.ready();
  await answerPipeline(h, undefined, { body: statusBody() }, { body: statusBody() });
  assert.match(h.node('pipeline-risk').textContent, /verificad/i);
  assert.match(h.node('pipeline-jobs').textContent, /verificad/i);
  assert.ok(h.node('pipeline-status').classes.has('risk-good'));
  const stale = statusBody({ checked_at: STALE() });
  const h2 = harness(); await h2.ready();
  await answerPipeline(h2, undefined, { body: stale }, { body: stale });
  assert.match(h2.node('pipeline-risk').textContent, /desconocid/i);
  assert.doesNotMatch(h2.node('pipeline-risk').textContent, /verificad/i);
  assert.ok(!h2.node('pipeline-status').classes.has('risk-good'));
  const noMax = statusBody({ checked_at: STALE() });
  delete noMax.dependencies.dead_letter.max_age_seconds;
  const h3 = harness(); await h3.ready();
  await answerPipeline(h3, undefined, { body: noMax }, { body: noMax });
  assert.match(h3.node('pipeline-risk').textContent, /desconocid/i);
  assert.doesNotMatch(h3.node('pipeline-risk').textContent, /verificad/i);
});

test('503 dead_letter_depth_unknown with age beyond max-age stays unknown even on HTTP 503', async () => {
  const h = harness(); await h.ready();
  const degraded = statusBody({
    ready: false, reason: 'dead_letter_depth_unknown',
    dependencies: {
      database: { state: 'ready' }, broker: { state: 'ready' },
      dead_letter: { state: 'unknown', depth: null, sampled_at: STALE(), age_seconds: 45, max_age_seconds: 6, max_depth: 1 },
    },
  });
  await answerPipeline(h, undefined, { body: degraded, status: 503 }, { body: degraded, status: 503 });
  assert.match(h.node('pipeline-risk').textContent, /desconocid/i);
  assert.doesNotMatch(h.node('pipeline-risk').textContent, /verificad/i);
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
});

test('verified branch requires known queue depth inside the bound, never an unearned claim', async () => {
  // Exact copy the panel must show whenever the depth claim is not earned. The
  // fixture statusBody() is otherwise fully healthy, so any shape below that
  // still reaches this string was rejected on the evidence itself.
  const depthGap = ' · profundidad de cola no acreditada como conocida y dentro de la cota';
  const stateGap = ' · estados de dependencia ausentes o contradictorios con "ready"';
  const unknown = gaps => `Desconocido: el estado no se acredita como saludable (HTTP 200${gaps}).`;
  const verified = 'Verificado: ready con checked_at fresco y profundidad de cola dentro de la cota.';
  const base = statusBody().dependencies;
  const dead = (extra = {}) => ({ state: 'ready', depth: 0, sampled_at: new Date().toISOString(), age_seconds: 0, max_age_seconds: 120, max_depth: 1, ...extra });
  const withDeadLetter = dependencies => statusBody({ dependencies });
  const without = (object, key) => { const copy = { ...object }; delete copy[key]; return copy; };
  const bodyWithoutDependencies = without(statusBody(), 'dependencies');
  const cases = [
    // The backend's own unknown sentinel (DeadLetterDepthUnknown = -1) is
    // deliberately outside the range of a real depth (>= 0).
    ['depth -1 unknown sentinel', withDeadLetter({ ...base, dead_letter: dead({ depth: -1, max_depth: 0 }) }), depthGap],
    ['depth negative', withDeadLetter({ ...base, dead_letter: dead({ depth: -2 }) }), depthGap],
    ['max_depth negative', withDeadLetter({ ...base, dead_letter: dead({ max_depth: -1 }) }), depthGap],
    ['depth null', withDeadLetter({ ...base, dead_letter: dead({ depth: null }) }), depthGap],
    ['max_depth null', withDeadLetter({ ...base, dead_letter: dead({ max_depth: null }) }), depthGap],
    ['depth absent', withDeadLetter({ ...base, dead_letter: without(dead(), 'depth') }), depthGap],
    ['max_depth absent', withDeadLetter({ ...base, dead_letter: without(dead(), 'max_depth') }), depthGap],
    ['depth non-numeric', withDeadLetter({ ...base, dead_letter: dead({ depth: '0' }) }), depthGap],
    ['max_depth non-numeric', withDeadLetter({ ...base, dead_letter: dead({ max_depth: '1' }) }), depthGap],
    ['depth over bound', withDeadLetter({ ...base, dead_letter: dead({ depth: 4 }) }), depthGap],
    ['dependencies absent', bodyWithoutDependencies, depthGap + stateGap],
    ['dependencies empty', withDeadLetter({}), depthGap + stateGap],
    ['dead letter absent', withDeadLetter({ database: { state: 'ready' }, broker: { state: 'ready' } }), depthGap + stateGap],
    ['dead letter degraded with a depth inside the bound', withDeadLetter({ ...base, dead_letter: dead({ state: 'degraded' }) }), stateGap],
    ['dead letter unknown with a numeric depth', withDeadLetter({ ...base, dead_letter: dead({ state: 'unknown' }) }), stateGap],
    ['broker degraded while ready is true', withDeadLetter({ ...base, broker: { state: 'degraded' } }), stateGap],
    ['broker unknown while ready is true', withDeadLetter({ ...base, broker: { state: 'unknown' } }), stateGap],
    ['broker absent', withDeadLetter({ database: { state: 'ready' }, dead_letter: dead() }), stateGap],
    ['database degraded while ready is true', withDeadLetter({ ...base, database: { state: 'degraded' } }), stateGap],
    ['database unknown while ready is true', withDeadLetter({ ...base, database: { state: 'unknown' } }), stateGap],
    // The backend always serializes `dependencies.database` as an object stating
    // ready or degraded (handler.Dependencies in the jobs and risk main.go), so
    // an absent, null, empty or non-object entry is missing evidence: the health
    // claim may never be read as agreement with `ready: true`.
    ['database absent', withDeadLetter(without(base, 'database')), stateGap],
    ['database null', withDeadLetter({ ...base, database: null }), stateGap],
    ['database empty', withDeadLetter({ ...base, database: {} }), stateGap],
    ['database non-object', withDeadLetter({ ...base, database: 'ready' }), stateGap],
  ];
  for (const [label, body, gaps] of cases) {
    const h = harness(); await h.ready();
    await answerPipeline(h, undefined, { body }, { body });
    assert.equal(h.node('pipeline-risk').textContent, unknown(gaps), `${label}: must stay on the explicit unknown presentation`);
    assert.equal(h.node('pipeline-jobs').textContent, unknown(gaps), `${label}: the jobs row is gated the same way`);
    assert.equal(h.node('pipeline-badge').textContent, 'SIN VERIFICAR', `${label}: must not be verified`);
    assert.ok(!h.node('pipeline-status').classes.has('risk-good'), `${label}: must not be green`);
  }
  // Positive control: a fully consistent healthy body still earns the label,
  // including the backend's default bound depth == max_depth == 0 (inclusive).
  for (const healthy of [statusBody(), withDeadLetter({ ...base, dead_letter: dead({ depth: 0, max_depth: 0 }) })]) {
    const green = harness(); await green.ready();
    await answerPipeline(green, undefined, { body: healthy }, { body: healthy });
    assert.equal(green.node('pipeline-risk').textContent, verified);
    assert.equal(green.node('pipeline-jobs').textContent, verified);
    assert.equal(green.node('pipeline-badge').textContent, 'VERIFICADO');
    assert.ok(green.node('pipeline-status').classes.has('risk-good'));
  }
});

test('jobs 403 is unverified and not authorized, never down, and does not corrupt the risk service', async () => {
  const h = harness(); await h.ready();
  await answerPipeline(h, undefined, { body: statusBody() }, { body: { code: 'forbidden' }, status: 403 });
  assert.match(h.node('pipeline-risk').textContent, /verificad/i);
  assert.match(h.node('pipeline-jobs').textContent, /no autorizado para comprobar/i);
  assert.doesNotMatch(h.node('pipeline-jobs').textContent, /ca[ií]do|down/i);
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
});

test('/health/ready 204 alone is labelled gateway-only and leaves the pipeline unverified', async () => {
  const h = harness(); await h.ready();
  assert.match(h.node('readiness-status').textContent, /gateway/i);
  assert.match(h.node('readiness-status').textContent, /204/);
  assert.doesNotMatch(h.node('readiness-status').textContent, /pipeline acreditado/i);
  assert.doesNotMatch(h.node('pipeline-status').textContent, /verificad/i);
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
});

test('logout clears every new panel and cancels the bounded recursive refresh timer', async () => {
  const h = harness(); await h.ready();
  await answerPipeline(h, { body: provenanceBody() }, { body: statusBody() }, { body: statusBody() });
  assert.equal(h.node('sources-provider').textContent, 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)');
  assert.match(h.node('pipeline-risk').textContent, /verificad/i);
  const timerIndex = h.timers.findIndex(x => x && x.delay === 30000);
  assert.notEqual(timerIndex, -1, 'expected a 30s recursive refresh timer');
  assert.ok(!/setInterval\s*\(/.test(source), 'the harness defines no setInterval and the cadence must be setTimeout-only');
  h.node('logout').listeners.click(); await tick();
  assert.equal(h.timers[timerIndex], null, 'logout must cancel the refresh timer');
  assert.equal(h.node('sources-provider').textContent, 'No disponible');
  assert.equal(h.node('sources-status').textContent, 'Conecta un token autorizado para consultar la procedencia.');
  assert.equal(h.node('pipeline-risk').textContent, 'Sin verificar');
  assert.equal(h.node('pipeline-jobs').textContent, 'Sin verificar');
  assert.equal(h.node('zone-count').textContent, 'No informado');
  assert.equal(h.node('batch-validity').textContent, 'Sin consultar');
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
});

test('superseded pipeline responses cannot overwrite newer state nor resurrect green after logout', async () => {
  const h = harness(); await h.ready();
  // Real overlap: the bounded deadline fences the first in-flight cycle and
  // schedules a newer one, so a newer refresh STARTS before the older reads
  // are ever resolved. The earlier version of this test only logged out and
  // answered late, never exercising the overwrite rule it names.
  const deadlineIndex = h.timers.findIndex(x => x && x.delay === 10000);
  assert.notEqual(deadlineIndex, -1, 'expected a bounded pipeline deadline for the in-flight cycle');
  const runDeadline = h.timers[deadlineIndex]; h.timers[deadlineIndex] = null;
  runDeadline(); await tick();
  const nextCycleIndex = h.timers.findIndex(x => x && x.delay === 30000);
  assert.notEqual(nextCycleIndex, -1, 'the timed-out cycle must still schedule the next one');
  const startNewer = h.timers[nextCycleIndex]; h.timers[nextCycleIndex] = null;
  startNewer(); await tick();

  // The older, superseded reads resolve late: they must not paint the panel.
  await answerPipeline(h, { body: provenanceBody() }, { body: statusBody() }, { body: statusBody() });
  assert.equal(h.node('pipeline-risk').textContent, 'Sin verificar');
  assert.notEqual(h.node('sources-provider').textContent, 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)');
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));

  // The newer cycle owns the panel and is allowed to go green.
  await answerPipeline(h, { body: provenanceBody() }, { body: statusBody() }, { body: statusBody() });
  assert.match(h.node('pipeline-risk').textContent, /verificad/i);
  assert.equal(h.node('sources-provider').textContent, 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)');
  assert.ok(h.node('pipeline-status').classes.has('risk-good'));

  // Logout half, kept: a refresh in flight when the session closes must not
  // resurrect a green panel afterwards.
  const inflightIndex = h.timers.findIndex(x => x && x.delay === 30000);
  assert.notEqual(inflightIndex, -1, 'the newer cycle must schedule the next one');
  const startInflight = h.timers[inflightIndex]; h.timers[inflightIndex] = null;
  startInflight(); await tick();
  h.node('logout').listeners.click(); await tick();
  await answerPipeline(h, { body: provenanceBody() }, { body: statusBody() }, { body: statusBody() });
  assert.equal(h.node('pipeline-risk').textContent, 'Sin verificar');
  assert.equal(h.node('sources-provider').textContent, 'No disponible');
  assert.equal(h.node('zone-count').textContent, 'No informado');
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
  assert.equal(h.timers.filter(x => x && x.delay === 10000).length, 0, 'logout must cancel the pipeline deadline');
  assert.equal(h.timers.filter(x => x && x.delay === 30000).length, 0);
});

test('a hung pipeline refresh is bounded, drops the verification claim and still reschedules', async () => {
  const h = harness(); await h.ready();
  const deadlineIndex = h.timers.findIndex(x => x && x.delay === 10000);
  assert.notEqual(deadlineIndex, -1, 'the pipeline cycle must arm a bounded deadline');
  const runDeadline = h.timers[deadlineIndex]; h.timers[deadlineIndex] = null;
  runDeadline(); await tick();
  assert.equal(h.node('pipeline-badge').textContent, 'SIN VERIFICAR');
  assert.equal(h.node('pipeline-risk').textContent, 'Sin verificar');
  assert.equal(h.node('pipeline-jobs').textContent, 'Sin verificar');
  assert.match(h.node('pipeline-status').textContent, /no llegó dentro de la cota/i);
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
  const rescheduled = h.timers.findIndex(x => x && x.delay === 30000);
  assert.notEqual(rescheduled, -1, 'the timed-out cycle must still schedule the next one');
  // The fenced cycle's late reads must not put the verification claim back.
  await answerPipeline(h, { body: provenanceBody() }, { body: statusBody() }, { body: statusBody() });
  assert.equal(h.node('pipeline-risk').textContent, 'Sin verificar');
  assert.notEqual(h.node('sources-provider').textContent, 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)');
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
});

function zonesBody() {
  const polygon = { type: 'Polygon', coordinates: [[[-71,-33],[-70,-33],[-70,-32],[-71,-33]]] };
  return { type: 'FeatureCollection', features: [
    { properties: { code: '05101', name: 'Valparaíso' }, geometry: polygon },
    { properties: { code: '05102', name: 'Casablanca' }, geometry: polygon },
  ] };
}

test('superseded /v1/risk/zones after a 401 reset cannot repopulate map, count, selector or risk status', async () => {
  const h = harness(undefined, 200, true);
  await tick();
  h.answer('/v1/risk/sources', {}, 401); await tick();
  assert.equal(h.node('session-state').textContent, 'Sin conexión');
  const resetMessage = h.node('risk-status').textContent;
  assert.match(resetMessage, /401/);
  h.answer('/v1/risk/zones', zonesBody()); await tick();
  assert.equal(h.node('zone-count').textContent, 'No informado');
  assert.equal(h.node('map-svg').hasAttribute('hidden'), true);
  assert.equal(h.node('map-empty').hidden, false);
  assert.equal(h.node('risk-status').textContent, resetMessage);
  const select = h.node('zone-select');
  assert.equal(select.children.length, 1);
  assert.equal(select.children[0].value, '');
  assert.equal(select.disabled, true);
  assert.ok(!h.pending.some(x => x.path.includes('/v1/risk/latest')));
  assert.ok(!h.pending.some(x => x.path.includes('/v1/risk/history')));
});

test('superseded /v1/risk/zones after logout cannot repopulate state nor chain latest/history reads', async () => {
  const h = harness(undefined, 200, true);
  await tick();
  h.node('logout').listeners.click(); await tick();
  const resetMessage = h.node('risk-status').textContent;
  h.answer('/v1/risk/zones', zonesBody()); await tick();
  assert.equal(h.node('zone-count').textContent, 'No informado');
  assert.equal(h.node('map-svg').hasAttribute('hidden'), true);
  assert.equal(h.node('map-empty').hidden, false);
  assert.equal(h.node('risk-status').textContent, resetMessage);
  assert.equal(h.node('zone-select').disabled, true);
  assert.ok(!h.pending.some(x => x.path.includes('/v1/risk/latest')));
  assert.ok(!h.pending.some(x => x.path.includes('/v1/risk/history')));
});

test('a healthy session still renders the live catalogue count and keeps selection working', async () => {
  const h = harness(apiCatalogue); await h.ready();
  assert.equal(h.node('zone-count').textContent, '38');
  const shape = h.node('map-insets').children[0].children[0].children[0].children[0];
  shape.listeners.keydown({ key: 'Enter', preventDefault() {} }); await tick();
  assert.equal(h.node('zone-select').value, shape['data-zone']);
  h.answer(`/v1/risk/latest?zone_id=${shape['data-zone']}`, {}, 404);
  h.answer(`/v1/risk/history?zone_id=${shape['data-zone']}`, []); await tick();
  assert.equal(h.node('risk-pill').textContent, 'SIN LOTE');
});

test('every consumed gateway path is absolute on the gateway origin and /token keeps the Keycloak issuer', async () => {
  const h = harness(); await h.ready(); await h.create();
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'queued' }); await tick();
  for (const path of [
    '/health/ready',
    '/v1/risk/zones',
    '/v1/risk/latest?zone_id=05101',
    '/v1/risk/history?zone_id=05101',
    '/v1/risk/sources',
    '/v1/risk/status',
    '/v1/jobs/status',
    '/v1/jobs',
    '/v1/jobs/job',
  ]) {
    assert.ok(h.calls.some(x => x.path === `${GATEWAY_ORIGIN}${path}`), `expected absolute gateway call ${GATEWAY_ORIGIN}${path}`);
  }
  const tokenCalls = h.calls.filter(x => x.path.includes('/token'));
  assert.equal(tokenCalls.length, 1, 'exactly one token exchange is performed');
  assert.equal(tokenCalls[0].path, `${ISSUER}/protocol/openid-connect/token`);
  assert.ok(!tokenCalls[0].path.startsWith(GATEWAY_ORIGIN), 'the token call must not resolve against the gateway origin');
});

test('scenario selector lists the served catalogue and posts scenario_id only when one is chosen', async () => {
  const h = harness(); await h.ready();
  const select = h.node('scenario-select');
  assert.equal(select.disabled, false, 'a connected session enables the scenario selector');
  assert.deepEqual(select.children.map(option => option.value), ['', 'mild', 'dry-windy']);
  assert.equal(h.calls.filter(x => x.path === `${GATEWAY_ORIGIN}/v1/risk/synthetic-scenarios`).length, 1, 'the catalogue is fetched once per session');

  // Legacy mode: no scenario chosen. The field must be OMITTED, never sent empty,
  // because the server rejects an explicit null/empty scenario_id with 400.
  await h.create();
  const legacy = JSON.parse(h.calls.find(x => x.path === `${GATEWAY_ORIGIN}/v1/jobs`).options.body);
  assert.deepEqual(legacy, { zone_id: '05101' });
  assert.equal(Object.hasOwn(legacy, 'scenario_id'), false, 'the legacy body must omit scenario_id');
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'queued' }); await tick();

  // Selected scenario: the chosen id travels with the request.
  select.value = 'dry-windy';
  await h.create();
  const bodies = h.calls.filter(x => x.path === `${GATEWAY_ORIGIN}/v1/jobs`).map(x => JSON.parse(x.options.body));
  assert.equal(bodies.length, 2);
  assert.deepEqual(bodies[1], { zone_id: '05101', scenario_id: 'dry-windy' });
});

test('logout resets the scenario selector to the legacy-only disabled state', async () => {
  const h = harness(); await h.ready();
  h.node('scenario-select').value = 'mild';
  h.node('logout').listeners.click(); await tick();
  const select = h.node('scenario-select');
  assert.equal(select.disabled, true);
  assert.deepEqual(select.children.map(option => option.value), ['']);
  assert.equal(select.value, '');
});

// Collects every drawn zone shape by its data-zone attribute, whatever the
// nesting inside the SVG, so a colour claim is checked against the real element.
function shapesByZone(node) {
  const found = new Map();
  const walk = current => {
    if (!current || typeof current !== 'object') return;
    if (current['data-zone'] && current.classList) found.set(current['data-zone'], current);
    for (const child of current.children || []) walk(child);
  };
  walk(node);
  return found;
}

test('a zone with a published batch keeps its colour after the selection moves away', async () => {
  const h = harness(); await h.ready();
  const shapes = () => shapesByZone(h.node('map-svg'));

  await h.select('05102');
  h.answer('/v1/risk/latest?zone_id=05102', { ...h.batch('05102'), risk_level: 'medio' });
  h.answer('/v1/risk/history?zone_id=05102', []); await tick();
  assert.ok(shapes().get('05102').classes.has('level-medio'), '05102 is coloured from its published batch');

  await h.select('05101');
  h.answer('/v1/risk/latest?zone_id=05101', { ...h.batch('05101'), risk_level: 'alto' });
  h.answer('/v1/risk/history?zone_id=05101', []); await tick();
  assert.ok(shapes().get('05101').classes.has('level-alto'), '05101 is coloured from its published batch');

  // The whole point: moving the selection back must NOT grey out 05101, which
  // still has a published batch. Only the outline marks the selection.
  await h.select('05102');
  h.answer('/v1/risk/latest?zone_id=05102', { ...h.batch('05102'), risk_level: 'medio' });
  h.answer('/v1/risk/history?zone_id=05102', []); await tick();
  assert.ok(shapes().get('05101').classes.has('level-alto'), 'a deselected zone must keep its colour');
  assert.ok(shapes().get('05102').classes.has('level-medio'));
  assert.ok(shapes().get('05102').classes.has('zone-selected'), 'the selection is still shown by the outline');
});

test('a zone with no published batch stays neutral and logout clears every colour', async () => {
  const h = harness(); await h.ready();
  // 404 from latest is absence, not low: the comuna stays grey, never invented.
  assert.ok(shapesByZone(h.node('map-svg')).get('05101').classes.has('zone-neutral'));

  await h.select('05102');
  h.answer('/v1/risk/latest?zone_id=05102', { ...h.batch('05102'), risk_level: 'alto' });
  h.answer('/v1/risk/history?zone_id=05102', []); await tick();
  assert.ok(shapesByZone(h.node('map-svg')).get('05102').classes.has('level-alto'));

  h.node('logout').listeners.click(); await tick();
  assert.ok(!shapesByZone(h.node('map-svg')).get('05102').classes.has('level-alto'), 'logout must clear the map colours');
});

// The sidebar mirrors the page sections as in-page anchors. The active marker is
// presentation: it must follow the section the user actually opened, never claim
// a selection nobody made, and add no request of its own.
test('sidebar marks the section actually opened and marks nothing else', async () => {
  const h = harness();
  await h.ready();
  assert.ok(h.node('nav-overview').classes.has('is-active'), 'the opening section is marked');
  assert.equal(h.node('nav-overview').attributes.get('aria-current'), 'true');
  const before = h.calls.length;
  h.node('nav-history').listeners.click();
  assert.equal(h.calls.length, before, 'marking a section issues no request');
  assert.ok(h.node('nav-history').classes.has('is-active'), 'the clicked section is marked');
  assert.ok(!h.node('nav-overview').classes.has('is-active'), 'only one section stays marked');
  assert.equal(h.node('nav-overview').attributes.get('aria-current'), undefined, 'aria-current does not linger on the old link');
});

// The metric cards summarise the page from data it already holds: the served
// catalogue, the history it read for the selected zone, and the levels this
// session actually read. No card may invent a number, and every one clears with
// the closed session.
test('metric cards report the real catalogue, history and read-zone counts', async () => {
  const h = harness();
  await h.ready();
  assert.equal(h.node('zone-count').textContent, '2', 'the catalogue size comes from the served features');
  assert.equal(h.node('metric-batches').textContent, '0', 'an empty history is a real zero, not a blank');
  assert.equal(h.node('metric-high').textContent, '0', 'nothing read yet means no counted level');
  await h.select('05102');
  h.answer('/v1/risk/latest?zone_id=05102', h.batch('05102'));
  h.answer('/v1/risk/history?zone_id=05102', [h.batch('05102'), h.batch('05102', 'second', 'job2')]);
  await tick();
  assert.equal(h.node('metric-batches').textContent, '2', 'the history length is the one actually returned');
  assert.equal(h.node('metric-high').textContent, '1', 'only the zone whose latest batch this session read is counted');
  h.node('logout').listeners.click();
  await tick();
  assert.equal(h.node('metric-high').textContent, '0', 'a closed session counts no level');
  assert.equal(h.node('metric-batches').textContent, '\u2014', 'a closed session shows no history length');
});

// The history panel reads one zone at a time. Search and the status filter work
// ONLY on the rows the API already returned for that zone: they never issue a
// read, never reach another zone, and the badge keeps reporting what the API
// returned while the result count reports what is actually visible.
test('history search and status filter narrow only the rows already returned for the zone', async () => {
  const h = harness(); await h.ready(); await h.select('05102');
  const rows = [
    { ...h.batch('05102', 'lote-uno', 'j1'), requested_at: '2026-01-01T00:00:00Z', data_status: 'sufficient' },
    { ...h.batch('05102', 'lote-dos', 'j2'), requested_at: '2026-02-02T00:00:00Z', data_status: 'insufficient' },
  ];
  h.answer('/v1/risk/latest?zone_id=05102', rows[0]);
  h.answer('/v1/risk/history?zone_id=05102', rows);
  await tick();
  assert.equal(h.node('history-rows').children.length, 2, 'both returned rows are shown');
  assert.equal(h.node('history-count').textContent, '2', 'the badge counts what the API returned');
  assert.equal(h.node('history-visible-count').textContent, '2 de 2');
  const historyReads = h.calls.filter(x => x.path.includes('/v1/risk/history')).length;
  h.node('history-status-filter').listeners.change({ target: { value: 'insufficient' } });
  assert.equal(h.node('history-rows').children.length, 1, 'the status filter narrows the visible rows');
  assert.equal(h.node('history-visible-count').textContent, '1 de 2');
  assert.equal(h.node('history-count').textContent, '2', 'filtering never rewrites what the API returned');
  assert.equal(h.calls.filter(x => x.path.includes('/v1/risk/history')).length, historyReads, 'filtering issues no new read');
  h.node('history-query').listeners.input({ target: { value: 'lote-uno' } });
  assert.equal(h.node('history-rows').children.length, 0, 'searching inside the narrowed rows finds nothing');
  assert.equal(h.calls.filter(x => x.path.includes('/v1/risk/history')).length, historyReads, 'searching issues no new read');
  h.node('reset-filters').listeners.click();
  assert.equal(h.node('history-rows').children.length, 2, 'resetting filters restores every returned row');
  assert.equal(h.node('history-visible-count').textContent, '2 de 2');
  assert.equal(h.node('history-query').value, '', 'the reset control clears the query it owns');
});

// T6 — the activity register. `/v1/audit` is admin-only, accepts NO query
// parameter and the service bounds the answer itself, so the panel renders
// exactly the five fields it carries (`actor_id`, `action`, `job_id`,
// `trace_id`, `occurred_at`) and labels the retired mockup's missing columns as
// not implemented instead of inventing a display name, a detail or a severity.
test('audit panel reads the parameterless admin endpoint once per session and renders the five real fields verbatim', async () => {
  const h = harness(); await h.ready();
  const reads = h.calls.filter(x => x.path.startsWith(`${GATEWAY_ORIGIN}/v1/audit`));
  assert.equal(reads.length, 1, 'a connected session reads /v1/audit exactly once');
  assert.equal(reads[0].path, `${GATEWAY_ORIGIN}/v1/audit`, 'the endpoint takes no query parameters');
  assert.equal(reads[0].options.headers.Authorization, 'Bearer test', 'the read is authenticated');
  h.answer('/v1/audit', [
    { actor_id: 'actor-opaco-1', action: 'job.requested', job_id: 'job-1', trace_id: 'trace-1', occurred_at: '2026-01-01T00:00:00Z' },
    { actor_id: 'actor-opaco-2', action: 'batch.published', job_id: 'job-2', trace_id: 'trace-2', occurred_at: '2026-01-02T00:00:00Z' },
  ]); await tick();
  assert.equal(h.node('audit-rows').children.length, 2, 'one row per returned event');
  assert.deepEqual(
    h.node('audit-rows').children[0].children.map(cell => cell.textContent),
    ['actor-opaco-1', 'job.requested', 'job-1', 'trace-1', '2026-01-01T00:00:00Z'],
    'every real field is shown verbatim, in its own cell');
  assert.equal(h.node('audit-count').textContent, '2', 'the badge reports the returned length');
  assert.equal(h.node('audit-badge').textContent, 'LEÍDO');
  assert.equal(h.node('audit-table').hasAttribute('hidden'), false, 'a non-empty register shows its table');
  assert.ok(h.node('audit-status').classes.has('risk-good'));
  // Nothing the endpoint does not provide is rendered: no severity, no display
  // name, no Spanish gloss of the action enum.
  const status = h.node('audit-status').textContent;
  assert.doesNotMatch(status, /severidad|nombre para mostrar|riesgo/i);
});

test('audit 403 renders an explicit admin-only reason instead of hiding the panel', async () => {
  const h = harness(); await h.ready();
  h.answer('/v1/audit', { code: 'forbidden', request_id: 'req-9' }, 403); await tick();
  const status = h.node('audit-status').textContent;
  assert.equal(h.node('audit-badge').textContent, 'SIN PERMISO');
  assert.match(status, /403/);
  assert.match(status, /administrador/i);
  assert.match(status, /forbidden/);
  assert.match(status, /req-9/);
  assert.equal(h.node('audit-rows').children.length, 0);
  assert.equal(h.node('audit-table').hasAttribute('hidden'), true, 'a denied read shows no event table');
  assert.equal(h.node('audit-count').textContent, '\u2014');
  assert.ok(!h.node('audit-status').classes.has('risk-good'));
});

test('audit payload fails closed unless every entry states an action and a time, and an omitted identifier is labelled in place', async () => {
  const notArray = harness(); await notArray.ready();
  notArray.answer('/v1/audit', { code: 'unexpected' }); await tick();
  assert.equal(notArray.node('audit-badge').textContent, 'NO DISPONIBLE');
  assert.match(notArray.node('audit-status').textContent, /unexpected/);
  assert.equal(notArray.node('audit-rows').children.length, 0);
  assert.equal(notArray.node('audit-table').hasAttribute('hidden'), true);

  const incomplete = harness(); await incomplete.ready();
  incomplete.answer('/v1/audit', [{ actor_id: 'a', job_id: 'j', trace_id: 't' }]); await tick();
  assert.equal(incomplete.node('audit-badge').textContent, 'NO DISPONIBLE');
  assert.equal(incomplete.node('audit-rows').children.length, 0, 'an entry without action or occurred_at is not an event and nothing is invented');

  const partial = harness(); await partial.ready();
  partial.answer('/v1/audit', [{ actor_id: 'a', action: 'cualquier.valor', job_id: 'j', trace_id: '', occurred_at: '2026-01-01T00:00:00Z' }]); await tick();
  assert.deepEqual(
    partial.node('audit-rows').children[0].children.map(cell => cell.textContent),
    ['a', 'cualquier.valor', 'j', 'No informado', '2026-01-01T00:00:00Z'],
    'an action outside the documented enum is shown verbatim and an omitted identifier is labelled, never filled in');
});

test('logout clears the audit panel, a fenced reply cannot repaint it, and the manual refresh re-reads the same endpoint', async () => {
  const h = harness(); await h.ready();
  // The initial read is in flight: the control is disabled until it settles, so
  // an answer can never land after a newer one.
  assert.equal(h.node('audit-refresh').disabled, true);
  h.answer('/v1/audit', [{ actor_id: 'a', action: 'job.requested', job_id: 'j', trace_id: 't', occurred_at: '2026-01-01T00:00:00Z' }]); await tick();
  assert.equal(h.node('audit-rows').children.length, 1);
  assert.equal(h.node('audit-refresh').disabled, false, 'a settled register leaves the refresh control available');

  h.node('audit-refresh').listeners.click(); await tick();
  assert.equal(h.calls.filter(x => x.path === `${GATEWAY_ORIGIN}/v1/audit`).length, 2, 'the refresh re-reads the same parameterless endpoint');
  assert.equal(h.node('audit-badge').textContent, 'CONSULTANDO');
  h.answer('/v1/audit', []); await tick();
  assert.equal(h.node('audit-count').textContent, '0', 'an empty register is a real zero');
  assert.equal(h.node('audit-rows').children.length, 0);

  // Close the session with a read in flight: the late reply owns nothing.
  h.node('audit-refresh').listeners.click(); await tick();
  h.node('logout').listeners.click(); await tick();
  assert.equal(h.node('audit-refresh').disabled, true, 'a closed session cannot re-read the register');
  h.answer('/v1/audit', [{ actor_id: 'late', action: 'batch.published', job_id: 'j', trace_id: 't', occurred_at: '2026-01-03T00:00:00Z' }]); await tick();
  assert.equal(h.node('audit-rows').children.length, 0, 'a fenced reply must not repaint the closed panel');
  assert.equal(h.node('audit-badge').textContent, 'SIN CONSULTAR');
  assert.equal(h.node('audit-count').textContent, '\u2014');
  assert.equal(h.node('audit-table').hasAttribute('hidden'), true);
  assert.equal(h.node('audit-status').textContent, 'Conecta un token autorizado para consultar el registro de actividad.');
});
