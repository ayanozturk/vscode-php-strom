const typescriptEslint = require('@typescript-eslint/eslint-plugin');

module.exports = [
  {
    // Keep lint aligned with the production TypeScript build. The active
    // language server is the Go implementation under server/.
    ignores: ['dist/**', 'node_modules/**', 'out/**', 'src/server/**'],
  },
  ...typescriptEslint.configs['flat/recommended'],
];
