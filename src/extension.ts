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

type PhpExtensionConflict = {
  id: string;
  label: string;
};

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  outputChannel = vscode.window.createOutputChannel('PHP Strom');
  context.subscriptions.push(outputChannel);

  await warnAboutConflictingPhpExtensions();

  await startClient(context);

  // Commands
  context.subscriptions.push(
    vscode.commands.registerCommand('phpls.restartServer', async () => {
      await stopClient();
      await startClient(context);
    }),

    vscode.commands.registerCommand('phpls.clearCache', async () => {
      await stopClient();
      await startClient(context, true);
    }),

    vscode.commands.registerCommand('phpls.indexWorkspace', () => {
      client?.sendNotification('phpls/indexWorkspace');
    }),

    vscode.commands.registerCommand('phpls.showOutputChannel', () => {
      outputChannel.show();
    }),

    vscode.commands.registerCommand('phpls.showDetectedPhpExtensions', () => {
      showDetectedPhpExtensions();
    }),
  );

  // Reload server on configuration change
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration('phpls')) {
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
  const config = vscode.workspace.getConfiguration('phpls');
  if (!config.get<boolean>('enable', true)) {
    outputChannel.appendLine('[phpls] Extension disabled via configuration.');
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
    'phpls',
    'PHP Strom',
    serverOptions,
    clientOptions,
  );

  // Indexing progress
  client.onNotification('phpls/indexingStarted', () => {
    outputChannel.appendLine('[phpls] Indexing workspace…');
    vscode.window.withProgress(
      {
        location: vscode.ProgressLocation.Notification,
        title: 'PHP Strom',
        cancellable: false,
      },
      (progress) =>
        new Promise<void>((resolve) => {
          progress.report({ message: 'Indexing workspace…' });
          let lastPct = 0;

          const progressSub = client!.onNotification(
            'phpls/indexingProgress',
            (params: { done: number; total: number }) => {
              const pct = params.total > 0 ? Math.round((params.done / params.total) * 100) : 0;
              const increment = pct - lastPct;
              lastPct = pct;
              progress.report({
                increment,
                message: `Indexing files… ${params.done} / ${params.total}`,
              });
            },
          );

          const doneSub = client!.onNotification(
            'phpls/indexingFinished',
            (params: { symbolCount: number }) => {
              progress.report({ increment: 100 - lastPct, message: 'Done' });
              progressSub.dispose();
              doneSub.dispose();
              outputChannel.appendLine(
                `[phpls] Indexing complete — ${params.symbolCount.toLocaleString()} symbols`,
              );
              resolve();
            },
          );
        }),
    );
  });

  await client.start();
}

async function stopClient(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}

async function resolveServerBinary(context: vscode.ExtensionContext): Promise<string> {
  const platform = os.platform();
  const arch = os.arch();
  const binaryName = platform === 'win32' ? 'phpls.exe' : 'phpls';
  const bundledBinary = context.asAbsolutePath(path.join('bin', `${platform}-${arch}`, binaryName));

  if (await pathExists(bundledBinary)) {
    await ensureExecutable(bundledBinary, platform);
    return bundledBinary;
  }

  const legacyBinary = context.asAbsolutePath(path.join('bin', binaryName));
  if (await pathExists(legacyBinary)) {
    outputChannel.appendLine(
      `[phpls] Falling back to legacy server binary layout for ${platform}-${arch}.`,
    );
    await ensureExecutable(legacyBinary, platform);
    return legacyBinary;
  }

  const supportedTargets = ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'win32-arm64', 'win32-x64'];
  throw new Error(
    `[phpls] No bundled server binary for ${platform}-${arch}. Supported targets: ${supportedTargets.join(', ')}`,
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
  return vscode.workspace.getConfiguration('phpls') as unknown as Record<string, unknown>;
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
    if (id === 'phpls.phpls') {
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
  outputChannel.appendLine('[phpls] Potentially conflicting PHP extensions detected:');
  for (const conflict of conflicts) {
    outputChannel.appendLine(`[phpls]   - ${conflict.label} (${conflict.id})`);
  }
  outputChannel.appendLine('[phpls] VS Code merges navigation results from all enabled definition providers.');
}

function showDetectedPhpExtensions(conflicts = getPotentialPhpExtensionConflicts()): void {
  outputChannel.show(true);
  if (conflicts.length === 0) {
    outputChannel.appendLine('[phpls] No potentially conflicting PHP extensions detected.');
    return;
  }
  logPhpExtensionConflicts(conflicts);
}
