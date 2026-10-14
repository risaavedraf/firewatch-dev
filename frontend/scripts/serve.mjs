import http from 'node:http';
import { readFile, realpath, stat } from 'node:fs/promises';
import { fileURLToPath, URL } from 'node:url';
import path from 'node:path';
import process from 'node:process';
import console from 'node:console';

const distRoot = fileURLToPath(new URL('../dist/', import.meta.url));
const mime = { '.html': 'text/html; charset=utf-8', '.css': 'text/css; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.json': 'application/json; charset=utf-8', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' };

// The page talks to two other origins and the build must not be allowed to reach
// any other. `connect-src` names exactly the gateway and the identity provider:
// neither a wildcard nor `'none'`, and the two spellings are the registered ones.
const CONTENT_SECURITY_POLICY = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src http://localhost:8080 http://localhost:8081; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'";

function safeRelativePath(target) {
  const raw = target.split('?')[0];
  if (!raw.startsWith('/') || /%2f|%5c/i.test(raw)) return null;
  let decoded;
  try { decoded = decodeURIComponent(raw); } catch { return null; }
  if (decoded.includes('\\') || decoded.includes('\0') || decoded.split('/').some(segment => segment === '..' || segment === '.')) return null;
  return decoded === '/' ? 'index.html' : decoded.slice(1);
}

// The allowlist is the exact set of files the built entry document references,
// plus the document itself. Vite hashes asset filenames, so a fixed list of
// names is impossible and an "assets/" directory allowlist would serve anything
// dropped there. Reading the entry document keeps the guarantee the retired
// three-asset server had: only what the page actually loads is reachable.
let allowlistCache = null;
async function allowlist() {
  const entryPath = path.join(distRoot, 'index.html');
  let entryInfo;
  try {
    entryInfo = await stat(entryPath);
  } catch {
    throw Object.assign(new Error('The build is missing: run `npm run build` before starting the server.'), { code: 'ENOENT_BUILD' });
  }
  if (allowlistCache?.mtimeMs === entryInfo.mtimeMs) return allowlistCache.set;
  const html = await readFile(entryPath, 'utf8');
  const set = new Set(['index.html']);
  for (const match of html.matchAll(/(?:src|href)="\/([^"]+)"/g)) set.add(match[1]);
  allowlistCache = { mtimeMs: entryInfo.mtimeMs, set };
  return set;
}

// Development server for the built application: explicit allowlist derived from
// the entry document, no directory listing and no SPA fallback. Routing is
// hash-based, so a request for an unknown path is a mistake and answers 404
// instead of quietly serving the shell. The gateway API and the identity
// provider live on other origins and the page calls them directly, so nothing is
// proxied here.
//
// Open it as http://localhost:4173, never as http://127.0.0.1:4173. The OIDC
// redirect URI and the gateway's allowed origin are both registered for the
// `localhost` origin, so the two spellings are different origins to the browser.
export function createServer() {
  return http.createServer(async (request, response) => {
    const send = (status, message, headers = {}) => {
      response.writeHead(status, { 'Content-Type': 'text/plain; charset=utf-8', 'X-Content-Type-Options': 'nosniff', ...headers });
      response.end(request.method === 'HEAD' ? undefined : message);
    };
    if (!['GET', 'HEAD'].includes(request.method)) return send(405, 'Method not allowed', { Allow: 'GET, HEAD' });
    const relative = safeRelativePath(request.url ?? '');
    if (relative === null) return send(400, 'Invalid path');
    let allowedSet;
    try {
      allowedSet = await allowlist();
    } catch {
      return send(503, 'The application build is missing. Run `npm run build` and reload.');
    }
    if (!allowedSet.has(relative)) return send(404, 'Not found');
    try {
      const canonicalRoot = await realpath(distRoot);
      const filename = await realpath(path.join(distRoot, relative));
      const resolvedRelative = path.relative(canonicalRoot, filename);
      if (resolvedRelative.startsWith(`..${path.sep}`) || resolvedRelative === '..' || path.isAbsolute(resolvedRelative)) return send(404, 'Not found');
      const info = await stat(filename);
      if (!info.isFile()) return send(404, 'Not found');
      const body = await readFile(filename);
      response.writeHead(200, {
        'Content-Type': mime[path.extname(relative)] ?? 'application/octet-stream', 'Content-Length': body.length,
        // `no-store` is the policy the retired server already sent. Hashed assets
        // would normally be cacheable; switching is a serving-policy decision,
        // not a bundler default, so it stays unchanged here.
        'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
        'Content-Security-Policy': CONTENT_SECURITY_POLICY,
        'Referrer-Policy': 'no-referrer'
      });
      response.end(request.method === 'HEAD' ? undefined : body);
    } catch {
      send(404, 'Not found');
    }
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const server = createServer();
  server.on('error', () => { console.error('Unable to start the page server. Check port 4173.'); process.exitCode = 1; });
  server.listen(4173, '127.0.0.1', () => console.log('FireWatch page: http://localhost:4173 (Ctrl+C to stop)'));
}
