// Flat ESLint config (ESLint 10 + typescript-eslint 8). Non-type-checked
// `recommended` is used deliberately: it needs no TS program, keeping lint
// fast and light for the RAM-constrained CI/Compose stack.
const js = require('@eslint/js');
const tseslint = require('typescript-eslint');

module.exports = tseslint.config(
  { ignores: ['dist/', 'node_modules/', 'coverage/', 'eslint.config.js'] },
  {
    files: ['src/**/*.ts', 'test/**/*.ts'],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      parserOptions: { ecmaVersion: 2022, sourceType: 'module' },
    },
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
  },
);
