import assert from 'node:assert/strict';
import http from 'node:http';
import { createServer } from './serve.mjs';

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

try {
  for (const [url, mime, content] of [
    ['/', 'text/html', 'id="app"'], ['/styles/tokens.css', 'text/css', '--forest-900'],
    ['/styles/app.css', 'text/css', '.sidebar'], ['/src/main.js', 'text/javascript', 'hashchange'],
    ['/src/core/state.js', 'text/javascript', 'export function reducer'],
    ['/src/views/sources.js', 'text/javascript', 'REVIEW_SOURCE']
  ]) {
    const result = await request(url);
    assert.equal(result.status, 200, url);
    assert.ok(result.headers['content-type'].startsWith(mime));
    assert.ok(result.body.includes(content));
    assert.equal(result.headers['x-content-type-options'], 'nosniff');
    assert.match(result.headers['content-security-policy'], /connect-src 'none'/);
  }
  const head = await request('/src/main.js', 'HEAD');
  assert.equal(head.status, 200);
  assert.equal(head.body, '');
  assert.ok(Number(head.headers['content-length']) > 0);
  const missing = await request('/missing.js');
  assert.equal(missing.status, 404);
  assert.equal(missing.body, 'Not found');
  assert.equal((await request('/', 'POST')).status, 405);
  assert.equal((await request('/package.json')).status, 404);
  assert.equal((await request('/scripts/serve.mjs')).status, 404);
  for (const url of ['/../backend/README.md', '/%2e%2e/backend/README.md', '/src/../index.html', '/src%2fmain.js', '/src%5cmain.js', '/src\\main.js', '/%00index.html', '/%zz', '//index.html', '/%252e%252e/backend/README.md']) {
    const result = await request(url);
    assert.ok([400, 404].includes(result.status), `${url}: ${result.status}`);
    assert.ok(['Invalid path', 'Not found'].includes(result.body), 'No filesystem path leakage');
  }
  assert.equal((await request('/src/main.js?cache=1')).status, 200);
  console.log('PASS: loopback HTTP assets/MIME, HEAD/405/404, explicit allowlist, raw/encoded traversal and slash/backslash confinement.');
} finally {
  await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  console.log('Loopback smoke server closed; no background process retained.');
}
