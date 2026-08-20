import * as path from 'node:path';
import { runTests } from '@vscode/test-electron';

async function main(): Promise<void> {
  const extensionDevelopmentPath = path.resolve(__dirname, '../..');
  const extensionTestsPath = path.resolve(__dirname, './suite');

  await runTests({
    version: '1.89.1',
    extensionDevelopmentPath,
    extensionTestsPath,
    launchArgs: ['--disable-extensions'],
  });
}

void main().catch((error: unknown) => {
  console.error('Failed to run the VS Code extension smoke test:', error);
  process.exitCode = 1;
});
