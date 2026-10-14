import { readdir, readFile, stat } from 'node:fs/promises';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import assert from 'node:assert/strict';
import { createInitialState, reducer } from '../src/core/state.js';
import { routes, allowedRoute } from '../src/core/router.js';
import { shell } from '../src/components/shell.js';

const root = fileURLToPath(new URL('../', import.meta.url));
async function filesIn(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const groups = await Promise.all(entries.map(entry => entry.isDirectory() ? filesIn(path.join(directory, entry.name)) : [path.join(directory, entry.name)]));
  return groups.flat();
}

const files = await filesIn(root);
const code = files.filter(file => /\.(?:js|mjs)$/.test(file));
for (const file of code) {
  const result = spawnSync(process.execPath, ['--check', file], { encoding: 'utf8' });
  assert.equal(result.status, 0, `${path.relative(root, file)}: ${result.stderr}`);
  const source = await readFile(file, 'utf8');
  for (const match of source.matchAll(/(?:from\s*|import\s*)['"](\.[^'"]+)['"]/g)) {
    const imported = path.resolve(path.dirname(file), match[1]);
    assert.ok(imported.startsWith(root), 'Import must remain in frontend');
    assert.ok((await stat(imported)).isFile(), `Missing import: ${match[1]}`);
  }
}
const html = await readFile(path.join(root, 'index.html'), 'utf8');
const developmentCopy = /\b(?:fictici[oa]s?|simulad[oa]s?|simulaci[oó]n|demo|demostraci[oó]n|fixtures?|maqueta|prototipo|ilustrativ[oa]s?|sint[eé]tic[oa]s?|escenario|inferencia|predicciones)\b|producto futuro|no sistema operativo|sin uso operativo|no son|no hay probabilidad validada|no reduce el riesgo|sin evaluación disponible/i;
function assertProductCopy(markup, context) {
  const text = markup.replace(/<[^>]*>/g, ' ');
  const accessibleCopy = [...markup.matchAll(/\b(?:aria-label|title|alt|content|placeholder)="([^"]*)"/g)].map(match => match[1]).join(' ');
  assert.doesNotMatch(`${text} ${accessibleCopy}`, developmentCopy, context);
}
assertProductCopy(html, 'Browser entry copy');
const entry = await readFile(path.join(root, 'src/main.js'), 'utf8');
assert.match(entry, /let state = \{ \.\.\.createInitialState\(\), scenario: 'future' \}/, 'Browser starts in representative fixture');
assert.match(entry, /if \(action.type === 'RESET'\) state = \{ \.\.\.state, scenario: 'future' \}/, 'Browser reset retains representative presentation');
assert.ok(html.includes('<link rel="icon" href="data:,">'), 'Explicit empty favicon avoids an implicit HTTP request');
for (const match of html.matchAll(/(?:src|href)="(\.\/[^"#]+)"/g)) assert.ok((await stat(path.join(root, match[1]))).isFile());
const runtimeFiles = files.filter(file => /\.(?:html|css)$/.test(file) || file.includes(`${path.sep}src${path.sep}`));
for (const file of runtimeFiles) {
  const source = await readFile(file, 'utf8');
  assert.doesNotMatch(source, /(?:src|href)\s*=\s*["'](?:https?:|\/\/)|@import\s|url\(\s*["']?(?:https?:|\/\/)/i, 'No external assets');
  const withoutEmptyFavicon = file === path.join(root, 'index.html') ? source.replace('<link rel="icon" href="data:,">', '') : source;
  assert.doesNotMatch(withoutEmptyFavicon, /(?:src|href)\s*=\s*["'](?:data:|javascript:)|url\(\s*["']?(?:data:|javascript:)/i, 'Only the exact empty favicon may use a data URL');
  assert.doesNotMatch(source, /\son[a-z]+\s*=/i, 'No inline event handlers');
  assert.doesNotMatch(source, /\b(?:fetch|XMLHttpRequest|WebSocket)\s*\(/, 'No real network integration');
}

let renders = 0;
for (const scenario of ['current', 'future']) {
  for (const role of ['viewer', 'admin']) {
    for (const route of Object.keys(routes)) {
      if (allowedRoute(route, role) !== route) continue;
       const state = { ...createInitialState(), scenario, role, route };
      const view = await import(`../src/views/${route}.js`);
      const rendered = shell(state, view[route](state));
      assertProductCopy(rendered, `${route}/${role}/${scenario}`);
      assert.doesNotMatch(rendered, /class="mockup-strip"|data-command="SET_SCENARIO"/, 'No development controls or strip');
      for (const row of state.history.filter(row => row.scenario === scenario)) {
        if (route === 'history') assertProductCopy(shell(state, view[route]({ ...state, selectedHistoryId: row.id })), `History detail ${row.id}`);
      }
      if (route === 'overview') {
        for (const sector of state.sectors) assertProductCopy(view[route]({ ...state, sectorId: sector.id }), `Sector detail ${sector.id}`);
      }
      assert.ok(rendered.includes('<h1>') && rendered.includes('id="content"'));
      assert.doesNotMatch(rendered, /\bundefined\b|\bNaN\b|\[object Object\]/);
      assert.doesNotMatch(rendered, /\son[a-z]+\s*=|(?:src|href)="(?:https?:|javascript:)/i);
      const ids = [...rendered.matchAll(/\sid="([^"]+)"/g)].map(match => match[1]);
      assert.equal(new Set(ids).size, ids.length, `Duplicate IDs in ${route}`);
      assert.equal((rendered.match(/<form\b/g) ?? []).length, (rendered.match(/<\/form>/g) ?? []).length);
      renders++;
    }
  }
}
for (const scenario of ['current', 'future']) {
  for (const role of ['viewer', 'admin']) {
    let state = { ...createInitialState(), scenario, role };
    const actions = [
      { type: 'SET_ROLE', role }, { type: 'SET_SCENARIO', scenario },
      { type: 'NAVIGATE', route: role === 'admin' ? 'jobs' : 'history' },
      { type: 'REVIEW_SOURCE', id: 'weather' }, { type: 'IMPORT_SOURCE', id: 'weather' },
      { type: 'SELECT_SECTOR', id: 's2' },
      { type: 'SET_FILTER', key: 'query', value: 'Roble' },
      { type: 'REQUEST_JOB' }
    ];
    for (const action of actions) {
      state = reducer(state, action);
      assertProductCopy(state.feedback, `Feedback ${action.type}/${role}/${scenario}`);
    }
    for (const action of [
      { type: 'NAVIGATE', route: 'unknown-route' },
      { type: 'ADVANCE_JOB', id: 'unknown' }, { type: 'REVIEW_SOURCE', id: 'unknown' },
      { type: 'IMPORT_SOURCE', id: 'unknown' }, { type: 'SELECT_SECTOR', id: 'unknown' },
      { type: 'SET_FILTER', key: 'query', value: 'x'.repeat(101) },
      { type: 'SELECT_HISTORY', id: 'b104' }
    ]) assertProductCopy(reducer(state, action).feedback, `Alternate feedback ${action.type}/${role}/${scenario}`);
    const queued = state.jobs.find(job => job.status === 'queued' && job.scenario === scenario);
    state = reducer(state, { type: 'ADVANCE_JOB', id: queued?.id ?? 'unknown' });
    assertProductCopy(state.feedback, 'Publication feedback');
    for (const route of Object.keys(routes).filter(route => allowedRoute(route, role) === route)) {
      const view = await import(`../src/views/${route}.js`);
      assertProductCopy(view[route](state), `Updated ${route}/${role}/${scenario}`);
    }
    assertProductCopy(reducer(state, { type: 'RESET' }).feedback, 'Reset feedback');
  }
}
let unsafe = { ...createInitialState(), scenario: 'future', role: 'admin' };
const attack = '<img src=x onerror="alert(1)">';
unsafe = reducer(unsafe, { type: 'SET_FILTER', key: 'query', value: attack });
const { history } = await import('../src/views/history.js');
const renderedHistory = history(unsafe);
assert.ok(renderedHistory.includes('&lt;img'), 'Injected text must be escaped');
assert.ok(!renderedHistory.includes(attack));
assert.doesNotMatch(renderedHistory, /href="javascript:/i);
for (const route of Object.keys(routes)) {
  const view = await import(`../src/views/${route}.js`);
  assert.ok(!view[route](unsafe).includes(attack), `${route} must escape user-derived text`);
}
console.log(`PASS: ${code.length} syntax checks, local imports/assets, ${renders} view/role/scenario templates, product copy/details/action feedback, injection escaping.`);
const browsers = ['playwright', '@playwright/test', 'puppeteer'].filter(name => {
  try { import.meta.resolve(name); return true; } catch { return false; }
});
console.log(browsers.length ? `Browser packages detected: ${browsers.join(', ')}. Browser verification requires the parent harness.` : 'Browser availability: no installed Playwright/Puppeteer package; visual and interactive browser verification pending.');
