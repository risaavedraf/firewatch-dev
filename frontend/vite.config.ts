/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The page origin is load-bearing, not a preference: Keycloak's registered
// valid redirect URI (`http://localhost:4173/`) and the gateway's `WEB_ORIGIN`
// both name the `localhost` spelling. `127.0.0.1:4173` is a different origin to
// a browser, so both the dev server and the preview server pin this exact
// origin and fail instead of silently moving to another port.
export default defineConfig({
  plugins: [react()],
  server: { host: 'localhost', port: 4173, strictPort: true },
  preview: { host: 'localhost', port: 4173, strictPort: true },
  build: {
    outDir: 'dist',
    // No source maps in the served bundle: the app is a public client and the
    // served tree carries no server-only code, but the map would still publish
    // reviewer-facing comments to every visitor.
    sourcemap: false,
    target: 'es2022',
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./tests/setup.ts'],
    include: ['tests/**/*.test.{ts,tsx}'],
    restoreMocks: true,
  },
});
