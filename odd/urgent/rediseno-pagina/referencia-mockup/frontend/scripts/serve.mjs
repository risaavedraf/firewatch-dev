import http from 'node:http';
import { readFile, realpath, stat } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('../', import.meta.url));
const publicFiles = new Set([
  'index.html', 'styles/tokens.css', 'styles/app.css', 'src/main.js',
  'src/core/state.js', 'src/core/selectors.js', 'src/core/router.js', 'src/data/fixtures.js',
  'src/components/shell.js', 'src/components/ui.js', 'src/components/terrain-map.js',
  ...['overview', 'history', 'sources', 'jobs', 'audit', 'access'].map(view => `src/views/${view}.js`)
]);
const mime = { '.html': 'text/html; charset=utf-8', '.css': 'text/css; charset=utf-8', '.js': 'text/javascript; charset=utf-8' };

function safeRelativePath(target) {
  const raw = target.split('?')[0];
  if (!raw.startsWith('/') || /%2f|%5c/i.test(raw)) return null;
  let decoded;
  try { decoded = decodeURIComponent(raw); } catch { return null; }
  if (decoded.includes('\\') || decoded.includes('\0') || decoded.split('/').some(segment => segment === '..' || segment === '.')) return null;
  return decoded === '/' ? 'index.html' : decoded.slice(1);
}

// Design-only local server: explicit asset allowlist, no directory listing or SPA fallback.
export function createServer() {
  return http.createServer(async (request, response) => {
    const send = (status, message, headers = {}) => {
      response.writeHead(status, { 'Content-Type': 'text/plain; charset=utf-8', 'X-Content-Type-Options': 'nosniff', ...headers });
      response.end(request.method === 'HEAD' ? undefined : message);
    };
    if (!['GET', 'HEAD'].includes(request.method)) return send(405, 'Method not allowed', { Allow: 'GET, HEAD' });
    const relative = safeRelativePath(request.url ?? '');
    if (relative === null) return send(400, 'Invalid path');
    if (!publicFiles.has(relative)) return send(404, 'Not found');
    try {
      const canonicalRoot = await realpath(root);
      const filename = await realpath(path.join(root, relative));
      const resolvedRelative = path.relative(canonicalRoot, filename);
      if (resolvedRelative.startsWith(`..${path.sep}`) || resolvedRelative === '..' || path.isAbsolute(resolvedRelative)) return send(404, 'Not found');
      const info = await stat(filename);
      if (!info.isFile()) return send(404, 'Not found');
      const body = await readFile(filename);
      response.writeHead(200, {
        'Content-Type': mime[path.extname(relative)], 'Content-Length': body.length,
        'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff',
        'Content-Security-Policy': "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
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
  server.on('error', () => { console.error('Unable to start local server. Check port 4173.'); process.exitCode = 1; });
  server.listen(4173, '127.0.0.1', () => console.log('FireWatch mockup: http://127.0.0.1:4173 (Ctrl+C to stop)'));
}
