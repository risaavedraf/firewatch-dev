import assert from 'node:assert/strict';
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from './serve.mjs';

// Loopback check of the server that serves the *built* application. It asserts
// the serving contract only: allowlist, methods, MIME, CSP, headers, traversal
// confinement and the absence of the retired assets. Assertions about the
// rendered markup (the single logout control, the 720px media query that hides
// the health card, the session card inside the sidebar) are no longer checkable
// here because the shell is rendered by React; they live in the component tests
// under tests/ instead.
const distEntry = fileURLToPath(new URL('../dist/index.html', import.meta.url));
try {
  await readFile(distEntry, 'utf8');
} catch {
  console.error('FAIL: dist/index.html is missing. Run `npm run build` before `npm run smoke`.');
  process.exit(1);
}

const server = createServer();
await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
const port = server.address().port;
function request(url, method = 'GET') {
  return new Promise((resolve, reject) => {
    const call = http.request({ hostname: '127.0.0.1', port, path: url, method }, response => {
      let body = '';
      response.setEncoding('utf8');
      response.on('data', chunk => { body += chunk; });
      response.on('end', () => resolve({ status: response.statusCode, headers: response.headers, body }));
    });
    call.on('error', reject);
    call.setTimeout(5000, () => call.destroy(new Error('HTTP smoke timeout')));
    call.end();
  });
}

// The exact files the built entry document references, plus the document itself.
// Everything else must be unreachable: that is the allowlist guarantee the
// retired three-asset server provided and this one has to keep.
const entry = await readFile(distEntry, 'utf8');
const referenced = [...entry.matchAll(/(?:src|href)="\/([^"]+)"/g)].map(match => match[1]);
assert.ok(referenced.length >= 1, 'the entry document must reference at least one built asset');
assert.ok(referenced.some(file => /^assets\/.+\.js$/.test(file)), `the entry document must reference a built script: ${referenced.join(', ')}`);
// A stylesheet is not asserted as present: the design-system unit (N3) adds one,
// and the loop below validates the MIME type of every referenced asset, so a
// stylesheet that ships is checked without this list having to predict it.

// Every document origin the app calls must stay confined to the two registered
// external origins.
function assertCsp(headers, label) {
  const csp = headers['content-security-policy'];
  assert.ok(csp, `${label}: missing CSP`);
  assert.ok(/connect-src http:\/\/localhost:8080 http:\/\/localhost:8081/.test(csp), `${label}: ${csp}`);
  assert.ok(!/connect-src 'none'/.test(csp), `${label}: connect-src still 'none'`);
  assert.ok(!/connect-src[^;]*\*/.test(csp), `${label}: connect-src must not be a wildcard`);
  assert.match(csp, /default-src 'self'/);
  assert.match(csp, /script-src 'self'(?! 'unsafe-inline')/);
  assert.match(csp, /object-src 'none'/);
  assert.match(csp, /frame-ancestors 'none'/);
}

try {
  const page = await request('/');
  assert.equal(page.status, 200, '/');
  assert.ok(page.headers['content-type'].startsWith('text/html'), `/: ${page.headers['content-type']}`);
  assert.ok(page.body.includes('<div id="root">'), 'the served document must expose the React mount point');
  assert.equal(page.headers['x-content-type-options'], 'nosniff');
  assert.equal(page.headers['referrer-policy'], 'no-referrer');
  assert.equal(page.headers['cache-control'], 'no-store');
  assertCsp(page.headers, '/');
  assert.equal(page.body.includes('/proposal.js'), false, 'the retired vanilla script must not be referenced');
  assert.equal(page.body.includes('/proposal.css'), false, 'the retired vanilla stylesheet must not be referenced');

  for (const file of referenced) {
    const asset = await request(`/${file}`);
    assert.equal(asset.status, 200, `/${file}: ${asset.status}`);
    assert.equal(asset.headers['x-content-type-options'], 'nosniff');
    assert.equal(asset.headers['referrer-policy'], 'no-referrer');
    assertCsp(asset.headers, `/${file}`);
    if (file.endsWith('.js')) assert.ok(asset.headers['content-type'].startsWith('text/javascript'), `/${file}: ${asset.headers['content-type']}`);
    if (file.endsWith('.css')) assert.ok(asset.headers['content-type'].startsWith('text/css'), `/${file}: ${asset.headers['content-type']}`);
  }

  const script = referenced.find(file => file.endsWith('.js'));
  const head = await request(`/${script}`, 'HEAD');
  assert.equal(head.status, 200);
  assert.equal(head.body, '');
  assert.ok(Number(head.headers['content-length']) > 0);

  assert.equal((await request('/index.html')).status, 200, 'the entry document is itself allowlisted');
  assert.equal((await request('/')).status, 200);
  assert.equal((await request(`/${referenced[0]}?cache=1`)).status, 200);

  // The retired vanilla page and the mockup surfaces must not be reachable.
  for (const url of ['/proposal.js', '/proposal.css', '/src/main.js', '/src/core/state.js', '/src/views/sources.js', '/styles/app.css', '/styles/tokens.css']) {
    assert.equal((await request(url)).status, 404, `${url} must be unreachable`);
  }

  // Sources, tooling and directories are not served, and there is no listing.
  for (const url of ['/package.json', '/vite.config.ts', '/scripts/serve.mjs', '/scripts/smoke.mjs', '/src/main.tsx', '/node_modules/react/index.js', '/assets/', '/assets', '/legacy/proposal.js']) {
    assert.equal((await request(url)).status, 404, `${url} must be unreachable`);
  }

  assert.equal((await request('/', 'POST')).status, 405);
  assert.equal((await request('/assets/', 'POST')).status, 405);

  for (const url of ['/../backend/README.md', '/%2e%2e/backend/README.md', '/assets/../index.html', '/%2findex.html', '/%5cindex.html', '/index\\html', '/%00index.html', '/%zz', '//index.html', '/%252e%252e/backend/README.md']) {
    const result = await request(url);
    assert.ok([400, 404].includes(result.status), `${url}: ${result.status}`);
    assert.ok(['Invalid path', 'Not found'].includes(result.body), 'No filesystem path leakage');
  }

  console.log(`PASS: loopback HTTP serves the built application (${referenced.length} referenced assets) with MIME, allowlist derived from the entry document, CSP naming both external origins and no wildcard, HEAD/405/404, no directory listing, raw/encoded traversal confined, and the retired vanilla and mockup paths unreachable.`);
} finally {
  await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  console.log('Loopback smoke server closed; no background process retained.');
}
