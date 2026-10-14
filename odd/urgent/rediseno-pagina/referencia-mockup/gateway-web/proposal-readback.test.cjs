const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const html = fs.readFileSync(`${__dirname}/index.html`, 'utf8');
const ids = new Set([...html.matchAll(/id="([^"]+)"/g)].map(m => m[1]));
const source = fs.readFileSync(`${__dirname}/proposal.js`, 'utf8');
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
    if (path === '/health/ready') return Promise.resolve({ ok: true, status: 204 });
    if (path.includes('/token')) return Promise.resolve({ ok: true, status: 200, json: async () => ({ access_token: 'test' }) });
    if (path === '/v1/risk/zones' && !deferZones) return Promise.resolve({ ok: catalogueStatus < 400, status: catalogueStatus, json: async () => zones });
    return new Promise(resolve => pending.push({ path, resolve }));
  };
  vm.runInNewContext(source, { document: { getElementById: node, querySelector: node, createElement: () => new Element(), createElementNS: () => new Element(true) }, fetch, URL, URLSearchParams, window: { location: { search: '?code=c&state=s', pathname: '/', hash: '', assign() {} }, history: { replaceState() {} } }, sessionStorage: { getItem: () => JSON.stringify({ state: 's', verifier: 'v' }), removeItem() {} }, crypto: { randomUUID: () => 'key' }, setTimeout: (fn, delay) => { fn.delay = delay; timers.push(fn); return timers.length; }, clearTimeout: id => { timers[id - 1] = null; }, AbortController });
  function answer(path, body, status = 200) { const i = pending.findIndex(x => x.path === path); assert.notEqual(i, -1, `pending ${path}`); pending.splice(i, 1)[0].resolve({ ok: status < 400, status, json: async () => body }); }
  const batch = (zone = '05101', id = 'batch', job = 'job') => ({ zone_id: zone, id, job_id: job, demo: true, risk_level: 'alto', factors: [{name: 'temperatura', known: true, value: 30, contribution: 0.2, note: ''}], rationale: 'Regla académica', policy: {policy_id: 'academic-rule-v1', params_hash: 'hash', parameters: {weight: '0.2'}}, notice: 'Sintético', geometry: polygon });
  return { node, calls, pending, timers, answer, batch, async ready() { await tick(); answer('/v1/risk/latest?zone_id=05101', {}, 404); answer('/v1/risk/history?zone_id=05101', []); await tick(); }, async create() { node('create-job').listeners.click(); await tick(); }, async select(code) { node('zone-select').listeners.change({ target: { value: code } }); await tick(); } };
}
test('actual POST uses CUT; published job automatically reads correlated latest and history', async () => {
  const h = harness(); await h.ready(); await h.create();
  assert.equal(JSON.parse(h.calls.find(x => x.path === '/v1/jobs').options.body).zone_id, '05101');
  h.answer('/v1/jobs', { id: 'job', zone_id: '05101', state: 'queued' }); await tick();
  assert.ok(h.pending.some(x => x.path === '/v1/jobs/job'));
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
  assert.match(h.node('job-status').textContent, /agotó/); assert.ok(!h.pending.some(x => x.path === '/v1/jobs/job'));
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

const realCatalogue = JSON.parse(fs.readFileSync(`${__dirname}/../../../../../contracts/gen/geo/valparaiso-comunas.geojson`, 'utf8'));
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
  h.node('logout').listeners.click(); await tick();
  await answerPipeline(h, { body: provenanceBody() }, { body: statusBody() }, { body: statusBody() });
  assert.equal(h.node('pipeline-risk').textContent, 'Sin verificar');
  assert.equal(h.node('sources-provider').textContent, 'No disponible');
  assert.equal(h.node('zone-count').textContent, 'No informado');
  assert.ok(!h.node('pipeline-status').classes.has('risk-good'));
  assert.equal(h.timers.filter(x => x && x.delay === 30000).length, 0);
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
