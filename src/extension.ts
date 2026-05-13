import * as path from 'path';
import * as os from 'os';
import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;
let outputChannel: vscode.OutputChannel;

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

  const binaryName = os.platform() === 'win32' ? 'phpls.exe' : 'phpls';
  const serverBinary = context.asAbsolutePath(path.join('bin', binaryName));

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

function getConfiguration(): Record<string, unknown> {
  return vscode.workspace.getConfiguration('phpls') as unknown as Record<string, unknown>;
}

async function warnAboutConflictingPhpExtensions(): Promise<void> {
  const conflicts = vscode.extensions.all.filter((ext) => {
    if (!ext.isActive && !ext.packageJSON?.activationEvents) {
      return false;
    }

    const id = ext.id.toLowerCase();
    if (id === 'phpls.phpls') {
      return false;
    }

    if (id === 'vscode.php-language-features') {
      return true;
    }

    const contributesLanguages = Array.isArray(ext.packageJSON?.contributes?.languages)
      ? ext.packageJSON.contributes.languages
      : [];

    return contributesLanguages.some((language: { id?: string }) => language?.id === 'php');
  });

  if (conflicts.length === 0) {
    return;
  }

  const labels = conflicts.map((ext) => ext.packageJSON?.displayName ?? ext.id).join(', ');
  outputChannel.appendLine(
    `[phpls] Conflicting PHP language providers detected: ${labels}. VS Code merges definition results from all enabled providers.`,
  );

  const selection = await vscode.window.showWarningMessage(
    'PHP Strom detected other enabled PHP language extensions. Cmd+Click definitions will be merged across providers until those extensions are disabled for this workspace.',
    'Open Extensions',
  );

  if (selection === 'Open Extensions') {
    await vscode.commands.executeCommand('workbench.view.extensions');
  }
}
