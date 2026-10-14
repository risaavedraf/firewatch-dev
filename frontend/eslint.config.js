import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import tseslint from 'typescript-eslint';

// `legacy/` is the retired vanilla page, kept only until its assertions are
// ported (N9); it is not linted because it is deleted, not maintained.
export default [
  { ignores: ['dist', 'legacy', 'node_modules', 'coverage'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  reactHooks.configs.flat.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: { ...globals.browser, ...globals.node },
    },
    plugins: { 'react-refresh': reactRefresh },
    rules: {
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
  {
    // `scripts/` are Node programs (the build server and its loopback smoke),
    // not browser code: they legitimately use `process`, `console` and `URL`.
    files: ['scripts/**/*.mjs'],
    languageOptions: { globals: { ...globals.node } },
  },
];
