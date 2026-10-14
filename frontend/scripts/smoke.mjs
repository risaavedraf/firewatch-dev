import assert from 'node:assert/strict';
import http from 'node:http';
import { createServer } from './serve.mjs';

// The development server for the real page. It is not the mockup server: it
// serves exactly the three page assets and points the browser at the gateway
// API and the identity provider, which live on other origins.
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

// Returns the body of the first at-rule whose prelude matches, matching braces so
// a block that nests other rules is sliced at its real closing brace.
function extractAtRuleBlock(css, prelude) {
  const match = prelude.exec(css);
  if (!match) return null;
  const start = match.index + match[0].length;
  let depth = 1;
  for (let index = start; index < css.length; index += 1) {
    if (css[index] === '{') depth += 1;
    else if (css[index] === '}') {
      depth -= 1;
      if (depth === 0) return css.slice(start, index);
    }
  }
  return null;
}

// Splits a declaration block into rules. The checked media query holds only flat
// rules, so a split on braces is enough and stays deterministic.
function rulesIn(block) {
  return block
    .split('}')
    .map(part => part.trim())
    .filter(part => part.includes('{'))
    .map(part => {
      const brace = part.indexOf('{');
      return {
        selectors: part.slice(0, brace).split(',').map(selector => selector.trim()).filter(Boolean),
        declarations: part.slice(brace + 1)
      };
    });
}

// Tags whose rules inside a media query set display:none in any form.
function hiddenSelectors(block) {
  return rulesIn(block)
    .filter(rule => /(^|;)\s*display\s*:\s*none/.test(rule.declarations))
    .flatMap(rule => rule.selectors);
}

// Slices one element out of the served markup by balancing its own tag name, so
// containment claims are checked against the real nesting and not a substring.
function extractElement(html, openTag) {
  const start = html.indexOf(openTag);
  if (start === -1) return null;
  const tagName = openTag.slice(1).split(/[\s>]/)[0];
  const tags = new RegExp(`<${tagName}\\b|</${tagName}\\s*>`, 'gi');
  tags.lastIndex = start;
  let depth = 0;
  let match;
  while ((match = tags.exec(html)) !== null) {
    if (match[0][1] === '/') {
      depth -= 1;
      if (depth === 0) return html.slice(start, tags.lastIndex);
    } else {
      depth += 1;
    }
  }
  return null;
}

try {
  for (const [url, mime, content] of [
    ['/', 'text/html', 'id="zone-select"'],
    ['/proposal.css', 'text/css', '.zone-shape'],
    ['/proposal.js', 'text/javascript', 'firewatch.pkce.transaction']
  ]) {
    const result = await request(url);
    assert.equal(result.status, 200, url);
    assert.ok(result.headers['content-type'].startsWith(mime), `${url}: ${result.headers['content-type']}`);
    assert.ok(result.body.includes(content), `${url} missing ${content}`);
    assert.equal(result.headers['x-content-type-options'], 'nosniff');
    assert.equal(result.headers['referrer-policy'], 'no-referrer');

    // The page now talks to two other origins, so connect-src must name them.
    // It must not stay at 'none', and it must not be a wildcard.
    const csp = result.headers['content-security-policy'];
    assert.ok(!/connect-src 'none'/.test(csp), `${url}: connect-src still 'none'`);
    assert.ok(/connect-src http:\/\/localhost:8080 http:\/\/localhost:8081/.test(csp), `${url}: ${csp}`);
    assert.ok(!/connect-src[^;]*\*/.test(csp), `${url}: connect-src must not be a wildcard`);
    assert.match(csp, /default-src 'self'/);
    assert.match(csp, /script-src 'self'(?! 'unsafe-inline')/);
  }

  const head = await request('/proposal.js', 'HEAD');
  assert.equal(head.status, 200);
  assert.equal(head.body, '');
  assert.ok(Number(head.headers['content-length']) > 0);

  // The retired mockup surfaces must not be reachable any more.
  for (const url of ['/src/main.js', '/src/core/state.js', '/src/views/sources.js', '/styles/app.css', '/styles/tokens.css']) {
    const result = await request(url);
    assert.equal(result.status, 404, `${url}: ${result.status}`);
  }

  assert.equal((await request('/package.json')).status, 404);
  assert.equal((await request('/scripts/serve.mjs')).status, 404);
  assert.equal((await request('/scripts/smoke.mjs')).status, 404);
  assert.equal((await request('/', 'POST')).status, 405);

  for (const url of ['/../backend/README.md', '/%2e%2e/backend/README.md', '/proposal.css/../index.html', '/%2fproposal.js', '/%5cproposal.js', '/proposal\\js', '/%00index.html', '/%zz', '//index.html', '/%252e%252e/backend/README.md']) {
    const result = await request(url);
    assert.ok([400, 404].includes(result.status), `${url}: ${result.status}`);
    assert.ok(['Invalid path', 'Not found'].includes(result.body), 'No filesystem path leakage');
  }

  assert.equal((await request('/proposal.js?cache=1')).status, 200);

  // Mobile logout reachability. This is a structural check on the served
  // stylesheet and the served markup, not a rendered-layout check: whether the
  // card actually paints at 390px still needs a real browser and stays pending.
  const stylesheet = (await request('/proposal.css')).body;
  const page = (await request('/')).body;

  const mobileBlock = extractAtRuleBlock(stylesheet, /@media\s*\(\s*max-width\s*:\s*720px\s*\)\s*\{/);
  assert.ok(mobileBlock, 'proposal.css must declare a max-width:720px media query');

  const hiddenInMobile = hiddenSelectors(mobileBlock);
  for (const selector of hiddenInMobile) {
    assert.ok(
      !/\.sidebar-bottom\b/.test(selector) && !/\.session-card\b/.test(selector) && !/#logout\b/.test(selector),
      `720px media query must not hide the session container or the logout control: ${selector}`
    );
  }
  assert.ok(
    hiddenInMobile.some(selector => /\.health-card\b/.test(selector)),
    '720px media query must hide the health card instead of the session card'
  );

  const sidebarBottom = extractElement(page, '<div class="sidebar-bottom">');
  assert.ok(sidebarBottom, 'index.html must keep the sidebar-bottom container');
  const sessionCard = extractElement(sidebarBottom, '<div class="session-card">');
  assert.ok(sessionCard, 'index.html must keep the session card inside sidebar-bottom');
  assert.ok(sidebarBottom.includes('health-card'), 'the health card must stay a sibling of the session card in sidebar-bottom');

  const logoutButton = extractElement(sessionCard, '<button id="logout"');
  assert.ok(logoutButton, 'index.html must keep the logout control inside the session card');
  assert.match(logoutButton, />\s*Cerrar sesión\s*</, 'the logout control must keep its Spanish label');
  assert.match(logoutButton, /\shidden\b/, 'the logout control must keep its hidden attribute');
  assert.equal(page.match(/id="logout"/g).length, 1, 'the logout control must exist exactly once, with no duplicate for mobile');

  console.log('PASS: loopback HTTP serves the three page assets with MIME and CSP naming both external origins, HEAD/405/404, explicit allowlist, retired mockup paths absent, raw/encoded traversal confined, and the 720px media query hides the health card while keeping the session card with its single logout control.');
} finally {
  await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  console.log('Loopback smoke server closed; no background process retained.');
}
