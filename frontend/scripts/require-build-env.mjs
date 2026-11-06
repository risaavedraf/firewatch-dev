// Fails a production build that would otherwise ship a bundle pointing at
// localhost. The browser-side guard in src/config.ts stays as the second line of
// defence, but a missing origin must stop the build, not the visitor.
import { existsSync, readFileSync } from 'node:fs';

const REQUIRED = ['VITE_GATEWAY_ORIGIN', 'VITE_ISSUER', 'VITE_REDIRECT_URI'];
// Mirrors how Vite resolves build-time variables: an explicit environment
// variable wins, then .env.production.local, then .env.production.
const FILES = ['.env.production.local', '.env.production'];

function readFileSources() {
  const values = {};
  for (const file of FILES) {
    if (!existsSync(file)) continue;
    for (const line of readFileSync(file, 'utf8').split(/\r?\n/)) {
      const match = /^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$/.exec(line);
      if (!match) continue;
      const value = match[2].replace(/^(['"])(.*)\1$/, '$2');
      if (!(match[1] in values)) values[match[1]] = value;
    }
  }
  return values;
}

const fromFiles = readFileSources();
const missing = REQUIRED.filter((name) => !((process.env[name] ?? fromFiles[name] ?? '').trim()));

if (missing.length > 0) {
  console.error(`FAIL: missing required build variable(s): ${missing.join(', ')}`);
  console.error('A production build must set all three origins.');
  console.error('For the local demo use `npm run build:local`; for a real host see');
  console.error('backend/deploy/prod/README.md.');
  process.exit(1);
}
