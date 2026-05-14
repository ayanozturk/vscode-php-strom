import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;
let outputChannel: vscode.OutputChannel;
let indexingStatusItem: vscode.StatusBarItem;
let indexingHideTimer: NodeJS.Timeout | undefined;

type PhpExtensionConflict = {
  id: string;
  label: string;
};

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  outputChannel = vscode.window.createOutputChannel('PHP Strom');
  context.subscriptions.push(outputChannel);
  indexingStatusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  indexingStatusItem.name = 'PHP Strom Indexing';
  indexingStatusItem.command = 'phpstrom.showOutputChannel';
  indexingStatusItem.hide();
  context.subscriptions.push(indexingStatusItem);

  await warnAboutConflictingPhpExtensions();

  await startClient(context);

  // Commands
  context.subscriptions.push(
    vscode.commands.registerCommand('phpstrom.restartServer', async () => {
      await stopClient();
      await startClient(context);
    }),

    vscode.commands.registerCommand('phpstrom.clearCache', async () => {
      await stopClient();
      await startClient(context, true);
    }),

    vscode.commands.registerCommand('phpstrom.indexWorkspace', () => {
      client?.sendNotification('phpstrom/indexWorkspace');
    }),

    vscode.commands.registerCommand('phpstrom.showOutputChannel', () => {
      outputChannel.show();
    }),

    vscode.commands.registerCommand('phpstrom.showDetectedPhpExtensions', () => {
      showDetectedPhpExtensions();
    }),
  );

  // Reload server on configuration change
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration('phpstrom')) {
        client?.sendNotification('workspace/didChangeConfiguration', {
          settings: getConfiguration(),
        });
      }
    }),
  );
}

export async function deactivate(): Promise<void> {
  await stopClient();
}

async function startClient(context: vscode.ExtensionContext, clearCache = false): Promise<void> {
  const config = vscode.workspace.getConfiguration('phpstrom');
  if (!config.get<boolean>('enable', true)) {
    outputChannel.appendLine('[phpstrom] Extension disabled via configuration.');
    return;
  }

  const serverBinary = await resolveServerBinary(context);

  const serverOptions: ServerOptions = {
    command: serverBinary,
    args: [],
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: 'file', language: 'php' }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher('**/*.{php,phtml,phar}'),
    },
    initializationOptions: {
      storagePath: context.storageUri?.fsPath,
      globalStoragePath: context.globalStorageUri.fsPath,
      clearCache,
      settings: getConfiguration(),
    },
    outputChannel,
    traceOutputChannel: outputChannel,
    middleware: {
      // Intercept completions to optionally trigger signature help after accepting
      provideCompletionItem: async (document, position, context_, token, next) => {
        const items = await next(document, position, context_, token);
        return items;
      },
    },
  };

  client = new LanguageClient(
    'phpstrom',
    'PHP Strom',
    serverOptions,
    clientOptions,
  );

  // Indexing progress
  client.onNotification('phpstrom/indexingStarted', () => {
    clearIndexingHideTimer();
    outputChannel.appendLine('[phpstrom] Indexing workspace…');
    indexingStatusItem.text = '$(sync~spin) PHP Strom: Indexing workspace';
    indexingStatusItem.tooltip = 'PHP Strom is indexing the workspace';
    indexingStatusItem.show();
  });

  client.onNotification(
    'phpstrom/indexingProgress',
    (params: { done: number; total: number }) => {
      const progressText = params.total > 0
        ? `$(sync~spin) PHP Strom: ${params.done.toLocaleString()} / ${params.total.toLocaleString()}`
        : '$(sync~spin) PHP Strom: Indexing workspace';
      indexingStatusItem.text = progressText;
      indexingStatusItem.tooltip = `PHP Strom indexing ${params.done.toLocaleString()} of ${params.total.toLocaleString()} files`;
      indexingStatusItem.show();
    },
  );

  client.onNotification(
    'phpstrom/indexingFinished',
    (params: { symbolCount: number }) => {
      clearIndexingHideTimer();
      indexingStatusItem.text = `$(check) PHP Strom: ${params.symbolCount.toLocaleString()} symbols indexed`;
      indexingStatusItem.tooltip = 'PHP Strom finished indexing the workspace';
      indexingStatusItem.show();
      outputChannel.appendLine(
        `[phpstrom] Indexing complete — ${params.symbolCount.toLocaleString()} symbols`,
      );
      indexingHideTimer = setTimeout(() => {
        indexingStatusItem.hide();
        indexingHideTimer = undefined;
      }, 3000);
    },
  );

  await client.start();
}

function clearIndexingHideTimer(): void {
  if (indexingHideTimer) {
    clearTimeout(indexingHideTimer);
    indexingHideTimer = undefined;
  }
}

async function stopClient(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
  clearIndexingHideTimer();
  indexingStatusItem.hide();
}

async function resolveServerBinary(context: vscode.ExtensionContext): Promise<string> {
  const platform = os.platform();
  const arch = os.arch();
  const binaryName = platform === 'win32' ? 'phpstrom.exe' : 'phpstrom';
  const bundledBinary = context.asAbsolutePath(path.join('bin', `${platform}-${arch}`, binaryName));

  if (await pathExists(bundledBinary)) {
    await ensureExecutable(bundledBinary, platform);
    return bundledBinary;
  }

  const legacyBinary = context.asAbsolutePath(path.join('bin', binaryName));
  if (await pathExists(legacyBinary)) {
    outputChannel.appendLine(
      `[phpstrom] Falling back to legacy server binary layout for ${platform}-${arch}.`,
    );
    await ensureExecutable(legacyBinary, platform);
    return legacyBinary;
  }

  const supportedTargets = ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'win32-arm64', 'win32-x64'];
  throw new Error(
    `[phpstrom] No bundled server binary for ${platform}-${arch}. Supported targets: ${supportedTargets.join(', ')}`,
  );
}

async function pathExists(targetPath: string): Promise<boolean> {
  try {
    await fs.access(targetPath);
    return true;
  } catch {
    return false;
  }
}

async function ensureExecutable(targetPath: string, platform: NodeJS.Platform): Promise<void> {
  if (platform === 'win32') {
    return;
  }

  await fs.chmod(targetPath, 0o755);
}

function getConfiguration(): Record<string, unknown> {
  const config = vscode.workspace.getConfiguration('phpstrom');

  return {
    enable: config.get<boolean>('enable', true),
    environment: {
      phpVersion: config.get<string>('environment.phpVersion', '8.3'),
      includePaths: config.get<string[]>('environment.includePaths', []),
      documentRoot: config.get<string>('environment.documentRoot', ''),
    },
    files: {
      associations: config.get<string[]>('files.associations', ['**/*.php', '**/*.phtml', '**/*.phar']),
      exclude: config.get<string[]>('files.exclude', ['**/.git/**', '**/node_modules/**']),
      maxSize: config.get<number>('files.maxSize', 1_000_000),
    },
    stubs: config.get<string[]>('stubs', []),
    diagnostics: {
      enable: config.get<boolean>('diagnostics.enable', true),
      run: config.get<string>('diagnostics.run', 'onType'),
      undefinedSymbols: config.get<boolean>('diagnostics.undefinedSymbols', true),
      undefinedVariables: config.get<boolean>('diagnostics.undefinedVariables', true),
      typeErrors: config.get<boolean>('diagnostics.typeErrors', true),
      strictTypes: config.get<boolean>('diagnostics.strictTypes', false),
      relaxedTypeCheck: config.get<boolean>('diagnostics.relaxedTypeCheck', true),
      noMixedTypeCheck: config.get<boolean>('diagnostics.noMixedTypeCheck', true),
      typeCheckDocumentedTypes: config.get<boolean>('diagnostics.typeCheckDocumentedTypes', false),
      exclude: config.get<Record<string, string[]>>('diagnostics.exclude', {}),
      overrides: config.get<Record<string, unknown>>('diagnostics.overrides', {}),
    },
    completion: {
      insertUseDeclaration: config.get<boolean>('completion.insertUseDeclaration', true),
      fullyQualifyGlobalSymbols: config.get<boolean>('completion.fullyQualifyGlobalSymbols', false),
      triggerParameterHints: config.get<boolean>('completion.triggerParameterHints', true),
      maxItems: config.get<number>('completion.maxItems', 100),
    },
    format: {
      braceStyle: config.get<string>('format.braceStyle', 'per'),
      insertSpaces: config.get<boolean>('format.insertSpaces', true),
      tabSize: config.get<number>('format.tabSize', 4),
    },
    phpdoc: {
      useFullyQualifiedNames: config.get<boolean>('phpdoc.useFullyQualifiedNames', false),
      returnVoid: config.get<boolean>('phpdoc.returnVoid', true),
      textFormat: config.get<string>('phpdoc.textFormat', 'snippet'),
    },
    codeLens: {
      references: {
        enable: config.get<boolean>('codeLens.references.enable', false),
      },
      implementations: {
        enable: config.get<boolean>('codeLens.implementations.enable', false),
      },
      overrides: {
        enable: config.get<boolean>('codeLens.overrides.enable', false),
      },
      parent: {
        enable: config.get<boolean>('codeLens.parent.enable', false),
      },
      usages: {
        enable: config.get<boolean>('codeLens.usages.enable', false),
      },
    },
    inlayHints: {
      parameterNames: {
        enable: config.get<boolean>('inlayHints.parameterNames.enable', true),
      },
      parameterTypes: {
        enable: config.get<boolean>('inlayHints.parameterTypes.enable', true),
      },
      returnTypes: {
        enable: config.get<boolean>('inlayHints.returnTypes.enable', true),
      },
    },
    compatibility: {
      preferPsalmPhpstanPrefixedAnnotations: config.get<boolean>(
        'compatibility.preferPsalmPhpstanPrefixedAnnotations',
        false,
      ),
    },
    telemetry: {
      enable: config.get<boolean>('telemetry.enable', false),
    },
    trace: {
      server: config.get<string>('trace.server', 'off'),
    },
  };
}

async function warnAboutConflictingPhpExtensions(): Promise<void> {
  const conflicts = getPotentialPhpExtensionConflicts();
  const builtInPhpExtensionId = 'vscode.php-language-features';

  if (conflicts.length === 0) {
    return;
  }

  if (conflicts.every((conflict) => conflict.id.toLowerCase() === builtInPhpExtensionId)) {
    return;
  }

  logPhpExtensionConflicts(conflicts);

  const labels = conflicts.map((ext) => ext.label).join(', ');

  const selection = await vscode.window.showWarningMessage(
    `PHP Strom detected other enabled PHP language extensions: ${labels}. Cmd+Click definitions may be merged across providers.`,
    'Show Details',
    'Open Extensions',
  );

  if (selection === 'Show Details') {
    showDetectedPhpExtensions(conflicts);
    return;
  }

  if (selection === 'Open Extensions') {
    await vscode.commands.executeCommand('workbench.view.extensions');
  }
}

function getPotentialPhpExtensionConflicts(): PhpExtensionConflict[] {
  const conflicts: PhpExtensionConflict[] = [];

  for (const ext of vscode.extensions.all) {
    const id = ext.id.toLowerCase();
    if (id === 'aossoftware.phpstrom') {
      continue;
    }

    if (!isPotentialPhpConflict(ext)) {
      continue;
    }

    conflicts.push({
      id: ext.id,
      label: ext.packageJSON?.displayName ?? ext.id,
    });
  }

  conflicts.sort((left, right) => left.label.localeCompare(right.label));
  return conflicts;
}

function isPotentialPhpConflict(ext: vscode.Extension<unknown>): boolean {
  const id = ext.id.toLowerCase();
  const activationEvents = Array.isArray(ext.packageJSON?.activationEvents)
    ? ext.packageJSON.activationEvents
    : [];
  const contributesLanguages = Array.isArray(ext.packageJSON?.contributes?.languages)
    ? ext.packageJSON.contributes.languages
    : [];

  if (id === 'vscode.php-language-features') {
    return true;
  }

  if (!contributesLanguages.some((language: { id?: string }) => language?.id === 'php')) {
    return false;
  }

  return activationEvents.some((event: string) => event === 'onLanguage:php');
}

function logPhpExtensionConflicts(conflicts: PhpExtensionConflict[]): void {
  outputChannel.appendLine('[phpstrom] Potentially conflicting PHP extensions detected:');
  for (const conflict of conflicts) {
    outputChannel.appendLine(`[phpstrom]   - ${conflict.label} (${conflict.id})`);
  }
  outputChannel.appendLine('[phpstrom] VS Code merges navigation results from all enabled definition providers.');
}

function showDetectedPhpExtensions(conflicts = getPotentialPhpExtensionConflicts()): void {
  outputChannel.show(true);
  if (conflicts.length === 0) {
    outputChannel.appendLine('[phpstrom] No potentially conflicting PHP extensions detected.');
    return;
  }
  logPhpExtensionConflicts(conflicts);
}
