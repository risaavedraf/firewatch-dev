import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import console from 'node:console';

// Source gate for the FireWatch app.
//
// It is the deliberate inverse of the retired mockup's checker, which forbade the
// words "demo", "synthetic", "scenario" and "fixtures" and asserted the page made
// no network call at all. The real page must do exactly the opposite: it must
// state the scope of its data and it must call the gateway. So here the honesty
// vocabulary is *required* and the fixture vocabulary is *forbidden*.
//
// Every rule is scoped to what the browser actually receives: `src/**` and
// `index.html`. The scripts, the test suites and the odd/ documents are not
// shipped, and several of them legitimately quote the words this gate forbids —
// including this file, which names them in order to forbid them.
//
// This is a static gate. What can only be observed by rendering (duplicate ids,
// which view a role can reach, fail-closed rendering of a malformed payload) is
// covered by the Vitest suites instead, not asserted here.

const root = fileURLToPath(new URL('../', import.meta.url));
const IGNORED_DIRECTORIES = new Set(['node_modules', 'dist', 'coverage', '.git']);

async function filesIn(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const groups = await Promise.all(
    entries
      .filter(entry => !IGNORED_DIRECTORIES.has(entry.name))
      .map(entry => (entry.isDirectory() ? filesIn(path.join(directory, entry.name)) : [path.join(directory, entry.name)])),
  );
  return groups.flat();
}

function isShipped(relative) {
  return relative === 'index.html' || relative.startsWith(`src${path.sep}`);
}

const files = await filesIn(root);
const shipped = files.filter(file => isShipped(path.relative(root, file)));
assert.ok(shipped.length > 0, 'no shipped source files were found; the gate would pass vacuously');

const sources = new Map();
for (const file of shipped) sources.set(path.relative(root, file), await readFile(file, 'utf8'));

const joined = [...sources.entries()].map(([name, source]) => `${name}\n${source}`).join('\n');

// ── 1. The honesty statement must be present, in the app that ships ────────
const required = [
  ['the synthetic-input scope declaration', /entrada (?:de demostración|sintética)|entorno de demostración/i],
  ['the no-calibration statement', /no calibrad/i],
  ['the not-official / academic-rule statement', /no oficial|regla académica/i],
  ['the CONAF non-substitution statement', /CONAF/],
  ['the SENAPRED non-substitution statement', /SENAPRED/],
  ['the source attribution', /SUBDERE/],
  ['the source restriction', /DIFROL/],
  ['the "not implemented yet" inventory', /NO IMPLEMENTADO/],
  ['the server-decides authorization statement', /el servidor/i],
];
for (const [what, pattern] of required) {
  assert.match(joined, pattern, `missing honesty statement: ${what}`);
}

// ── 2. Invented data must not come back ───────────────────────────────────
// Note what is *not* forbidden here: the word "fixture". The retired mockup's
// checker banned that vocabulary because the mockup was pretending. This page
// names the backend's legacy fixture truthfully — "Sin escenario (fixture
// analítico heredado)" is an honest description of what the server will do — and a
// gate that silenced it would remove a true statement to satisfy a word list.
// What is forbidden is shipping invented data: a fixture module, a frozen clock,
// the mockup's sector identifiers, an alias that invents names, client-side
// random data or a hardcoded demo identity.
const invented = [
  ['a fixture module', /(?:from|require\()\s*['"][^'"]*fixtures?/i],
  ['a frozen clock', /2026-01-15T15:00:00Z/],
  ['the mockup sector identifiers', /\bs[1-4]\b\s*:/],
  ['an alias that invents a display name', /presentationName/],
  ['random or synthetic client-side data', /Math\.random\s*\(/],
  ['a demo identity hardcoded in the client', /demo-admin|demo-viewer/],
];
for (const [what, pattern] of invented) {
  assert.doesNotMatch(joined, pattern, `invented data must not be shipped: ${what}`);
}

// ── 3. The real integration must exist ────────────────────────────────────
for (const route of [
  '/health/ready',
  '/v1/risk/zones',
  '/v1/risk/latest',
  '/v1/risk/history',
  '/v1/risk/sources',
  '/v1/risk/status',
  '/v1/jobs/status',
  '/v1/risk/synthetic-scenarios',
  '/v1/audit',
  '/v1/prevention/actions',
  '/v1/models',
]) {
  assert.ok(joined.includes(route), `the consumed route is missing from the app: ${route}`);
}
assert.match(joined, /\bfetch\s*\(/, 'the app must really call the API');
assert.match(joined, /http:\/\/localhost:8080/, 'every gateway call is absolute against the gateway origin');
assert.match(joined, /http:\/\/localhost:8081\/realms\/firewatch/, 'the issuer is the identity provider, not the gateway');

// ── 4. No client-side authorization boundary ──────────────────────────────
// The client cannot grant permissions, so no route or view may decide anything
// from a role: a `viewer` must be able to open `#audit` and read the server's
// 403. The rule is deliberately about readers and guards rather than about any
// operation on a role array — the decoder itself legitimately filters its own
// list, and a blunt rule would forbid the honest code along with the false
// boundary.
const routes = sources.get(path.join('src', 'app', 'routes.ts'));
assert.ok(routes, 'src/app/routes.ts must exist');
assert.doesNotMatch(routes, /\badmin\b/i, 'no route is marked as admin-only in the client');
assert.match(routes, /audit/, 'the activity register is a declared view for every role');

const sessionPath = path.join('src', 'core', 'session.ts');
const otherReaders = [...sources.entries()].filter(([name, source]) => name !== sessionPath && /decodeRoles/.test(source));
assert.equal(
  otherReaders.length,
  0,
  `the decoded token payload must have exactly one reader (the session store): ${otherReaders.map(([name]) => name).join(', ')}`,
);

for (const [name, source] of sources) {
  assert.doesNotMatch(source, /allowedRoute|requiresRole|requireRole/i, `no client-side route guard: ${name}`);
  // The presentation layer is where a false boundary would appear.
  if (name.startsWith(`src${path.sep}views${path.sep}`) || name.startsWith(`src${path.sep}components${path.sep}`)) {
    assert.doesNotMatch(source, /\broles?\b\s*(?:\?\.)?\s*\.\s*(?:includes|some|filter|every)\b/i, `no view or component may branch on roles: ${name}`);
    assert.doesNotMatch(source, /\broles?\b\s*===?\s*['"]/, `no view or component may compare a role literal: ${name}`);
  }
}

// ── 5. Session and secret hygiene ─────────────────────────────────────────
for (const [name, source] of sources) {
  assert.doesNotMatch(source, /localStorage/, `tokens are never persisted: ${name}`);
  assert.doesNotMatch(source, /dangerouslySetInnerHTML/, `no raw HTML injection: ${name}`);
  assert.doesNotMatch(source, /\bdocument\.cookie\b/, `no cookies are written: ${name}`);
  // The PKCE transaction is the single, deliberate storage exception, and it is
  // keyed by file so this gate points at it if it ever spreads.
  const storageWrites = source.match(/sessionStorage\.\w+/g) ?? [];
  assert.ok(
    storageWrites.every(() => name === path.join('src', 'core', 'session.ts')),
    `sessionStorage is only used for the PKCE transaction: ${name}`,
  );
}
assert.doesNotMatch(joined, /client_secret|BEGIN [A-Z ]*PRIVATE KEY|VITE_[A-Z_]*SECRET/, 'no secret may be bundled');
assert.doesNotMatch(joined, /Basic\s+[A-Za-z0-9+/=]{8,}/, 'no embedded credential');

// ── 6. No external assets, and no inline handlers in the served document ──
const entry = sources.get('index.html');
assert.ok(entry, 'index.html must exist');
assert.doesNotMatch(entry, /\son[a-z]+\s*=/i, 'no inline event handlers');
for (const [name, source] of sources) {
  if (!/\.(?:html|css)$/.test(name)) continue;
  assert.doesNotMatch(source, /(?:src|href)\s*=\s*["'](?:https?:|\/\/)/i, `no external asset: ${name}`);
  assert.doesNotMatch(source, /@import\s+(?:url\()?["']?(?:https?:|\/\/)/i, `no remote stylesheet import: ${name}`);
}

// ── 7. The sidebar must keep the scroll that makes it usable ─────────────────
// This is the one layout property a static gate can honestly protect. The column
// is a fixed-height sticky box, so without a vertical scroll declaration
// everything past the fold — including the session card with the only sign-out
// control — is unreachable rather than merely off screen. Geometry still needs a
// browser: this rule cannot tell whether the controls fit, only that the
// declaration which lets the reader reach them is still there.
const styles = sources.get(path.join('src', 'styles', 'app.css'));
assert.ok(styles, 'src/styles/app.css must exist');
const sidebarRule = styles.match(/^\s*\.sidebar\s*\{[^}]*\}/m);
assert.ok(sidebarRule, 'the .sidebar rule must exist');
assert.match(sidebarRule[0], /position:\s*sticky/, 'the sticky column this rule protects is gone');
assert.match(sidebarRule[0], /overflow-y:\s*(?:auto|scroll)/, 'the sidebar must declare its own vertical scroll');
assert.match(sidebarRule[0], /height:\s*calc\(/, 'the sidebar must keep its bounded height, which is what the scroll is for');
// The mobile breakpoint must keep dropping the fixed height: that is what makes
// the scroll declaration harmless at small widths.
assert.match(
  styles,
  /@media\s*\(max-width:\s*720px\)[\s\S]*?\.sidebar\s*\{[^}]*height:\s*auto/,
  'the 720px breakpoint must keep the sidebar height automatic',
);

console.log(
  `PASS: ${sources.size} shipped source files checked. The honesty statement is present (${required.length} required statements), no fixture, frozen clock, invented identity or alias ships, all eleven consumed routes and both external origins are present, no client-side role guard exists, tokens are never persisted, no secret is bundled, the sidebar still declares its own vertical scroll, and no external asset or inline handler is served.`,
);
