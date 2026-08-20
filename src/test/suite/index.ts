import assert from 'node:assert/strict';
import * as vscode from 'vscode';

type ExtensionManifest = {
  name?: string;
  publisher?: string;
  contributes?: {
    commands?: Array<{ command?: string }>;
    languages?: Array<{ id?: string }>;
    configuration?: {
      properties?: Record<string, { default?: unknown }>;
    };
  };
};

export async function run(): Promise<void> {
  const extension = vscode.extensions.all.find((candidate) => {
    const manifest = candidate.packageJSON as ExtensionManifest;
    return manifest.publisher === 'AOSSoftware' && manifest.name === 'phpstrom';
  });
  assert.ok(extension, 'expected the PHP Strom development extension to be discoverable');

  const manifest = extension.packageJSON as ExtensionManifest;
  const commands = manifest.contributes?.commands?.map(({ command }) => command) ?? [];
  assert.ok(commands.includes('phpstrom.restartServer'), 'expected restart command contribution');
  assert.ok(commands.includes('phpstrom.indexWorkspace'), 'expected workspace index command contribution');

  const languages = manifest.contributes?.languages?.map(({ id }) => id) ?? [];
  assert.ok(languages.includes('php'), 'expected PHP language contribution');

  const scanOnStart = manifest.contributes?.configuration?.properties?.['phpstrom.diagnostics.workspaceScanOnStart'];
  assert.equal(scanOnStart?.default, true, 'expected full workspace analysis to run after activation');
}
