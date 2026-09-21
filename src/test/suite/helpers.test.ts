import assert from 'node:assert/strict';
import {
  formatBytes,
  formatDuration,
  formatRate,
  isPotentialPhpConflict,
  mergeExcludePatterns,
} from '../../client/helpers.js';

export async function runHelperTests(): Promise<void> {
  assert.equal(formatDuration(0), '0 ms');
  assert.equal(formatDuration(-5), '0 ms');
  assert.equal(formatDuration(Number.NaN), '0 ms');
  assert.equal(formatDuration(999), '999 ms');
  assert.equal(formatDuration(1_500), '1.50s');
  assert.equal(formatDuration(12_500), '12.5s');

  assert.equal(formatBytes(0), '0 B');
  assert.equal(formatBytes(-1), '0 B');
  assert.equal(formatBytes(Number.NaN), '0 B');
  assert.equal(formatBytes(512), '512 B');
  assert.equal(formatBytes(2_048), '2.0 KB');
  assert.equal(formatBytes(1_572_864), '1.5 MB');
  assert.equal(formatBytes(2 * 1024 * 1024 * 1024), '2.0 GB');

  assert.equal(formatRate(0), '0');
  assert.equal(formatRate(-3), '0');
  assert.equal(formatRate(Number.PositiveInfinity), '0');
  assert.equal(formatRate(12.6), '13');

  assert.deepEqual(
    mergeExcludePatterns(
      ['**/.git/**', '**/node_modules/**'],
      ['**/vendor/**', '  '],
      ['**/node_modules/**', '**/cache/**'],
    ),
    ['**/.git/**', '**/node_modules/**', '**/vendor/**', '**/cache/**'],
  );
  assert.deepEqual(mergeExcludePatterns([], ['  trimmed/**  ']), ['trimmed/**']);

  assert.equal(
    isPotentialPhpConflict({ id: 'vscode.php-language-features', packageJSON: {} }),
    true,
  );
  assert.equal(
    isPotentialPhpConflict({
      id: 'publisher.php-helper',
      packageJSON: {
        activationEvents: ['onLanguage:php'],
        contributes: { languages: [{ id: 'php' }] },
      },
    }),
    true,
  );
  assert.equal(
    isPotentialPhpConflict({
      id: 'publisher.php-helper',
      packageJSON: {
        activationEvents: ['onStartupFinished'],
        contributes: { languages: [{ id: 'php' }] },
      },
    }),
    false,
  );
  assert.equal(
    isPotentialPhpConflict({
      id: 'publisher.python',
      packageJSON: {
        activationEvents: ['onLanguage:php'],
        contributes: { languages: [{ id: 'python' }] },
      },
    }),
    false,
  );
  assert.equal(
    isPotentialPhpConflict({
      id: 'publisher.broken',
      packageJSON: {
        activationEvents: 'onLanguage:php',
        contributes: { languages: 'php' as unknown as Array<{ id?: string }> },
      },
    }),
    false,
  );
}
