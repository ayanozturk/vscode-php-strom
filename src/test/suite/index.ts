import assert from 'node:assert/strict';
import * as vscode from 'vscode';

type ConfigurationGroup = {
  title?: string;
  properties?: Record<string, { default?: unknown; type?: string }>;
};

type ExtensionManifest = {
  name?: string;
  publisher?: string;
  contributes?: {
    commands?: Array<{ command?: string }>;
    languages?: Array<{ id?: string }>;
    configuration?: ConfigurationGroup | ConfigurationGroup[];
  };
};

function configurationProperties(manifest: ExtensionManifest): Record<string, { default?: unknown; type?: string }> {
  const configuration = manifest.contributes?.configuration;
  if (!configuration) {
    return {};
  }
  const groups = Array.isArray(configuration) ? configuration : [configuration];
  return Object.assign({}, ...groups.map((group) => group.properties ?? {}));
}

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

  const properties = configurationProperties(manifest);
  const scanOnStart = properties['phpstrom.diagnostics.workspaceScanOnStart'];
  assert.equal(scanOnStart?.default, false, 'expected startup to index symbols without running full diagnostics');

  const analysisLevel = properties['phpstrom.diagnostics.analysis.level'];
  assert.equal(analysisLevel?.type, 'number');
  assert.equal(analysisLevel?.default, 9);

  const analysisToggles = [
    'phpstrom.diagnostics.analysis.syntaxErrors',
    'phpstrom.diagnostics.analysis.undefinedSymbols',
    'phpstrom.diagnostics.analysis.undefinedVariables',
    'phpstrom.diagnostics.analysis.classModel',
    'phpstrom.diagnostics.analysis.invalidCalls',
    'phpstrom.diagnostics.analysis.language',
    'phpstrom.diagnostics.analysis.typeErrors',
    'phpstrom.diagnostics.analysis.methodVisibility',
    'phpstrom.diagnostics.analysis.throwTypes',
    'phpstrom.diagnostics.analysis.deprecated',
    'phpstrom.diagnostics.analysis.unreachableCode',
    'phpstrom.diagnostics.analysis.emptyStatements',
    'phpstrom.diagnostics.analysis.assignmentInCondition',
    'phpstrom.diagnostics.analysis.sideEffects',
    'phpstrom.diagnostics.analysis.style',
  ];
  for (const key of analysisToggles) {
    assert.equal(properties[key]?.type, 'boolean', `expected ${key} to be a checkbox setting`);
  }
  assert.equal(properties['phpstrom.diagnostics.analysis.style']?.default, false);
  assert.equal(properties['phpstrom.diagnostics.analysis.sideEffects']?.default, false);
  assert.equal(properties['phpstrom.inlayHints.parameterTypes.enable']?.default, false);
  assert.equal(properties['phpstrom.inlayHints.returnTypes.enable']?.default, false);
  assert.ok(!('phpstrom.telemetry.enable' in properties), 'expected unused telemetry setting to be removed');
}
