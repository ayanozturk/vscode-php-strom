import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import * as vscode from 'vscode';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from 'vscode-languageclient/node';
import {
  openDiagnosticNode,
  ProjectDiagnosticsTreeProvider,
} from './client/diagnosticsView.js';

let client: LanguageClient | undefined;
let outputChannel: vscode.OutputChannel;
let indexingStatusItem: vscode.StatusBarItem;
let indexingHideTimer: NodeJS.Timeout | undefined;
let analysisStatusItem: vscode.StatusBarItem;
let analysisHideTimer: NodeJS.Timeout | undefined;
const pendingAnalysisUris = new Map<string, number>();
let diagnosticsTreeProvider: ProjectDiagnosticsTreeProvider;

type PhpExtensionConflict = {
  id: string;
  label: string;
};

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  outputChannel = vscode.window.createOutputChannel('PHP Strom');
  context.subscriptions.push(outputChannel);
  diagnosticsTreeProvider = new ProjectDiagnosticsTreeProvider();
  const diagnosticsTreeView = vscode.window.createTreeView('phpstromProblems', {
    treeDataProvider: diagnosticsTreeProvider,
    showCollapseAll: true,
  });
  diagnosticsTreeProvider.setView(diagnosticsTreeView);
  context.subscriptions.push(diagnosticsTreeView);
  indexingStatusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  indexingStatusItem.name = 'PHP Strom Indexing';
  indexingStatusItem.command = 'phpstrom.showOutputChannel';
  indexingStatusItem.hide();
  context.subscriptions.push(indexingStatusItem);
  analysisStatusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 99);
  analysisStatusItem.name = 'PHP Strom Analysis';
  analysisStatusItem.command = 'phpstrom.showOutputChannel';
  analysisStatusItem.hide();
  context.subscriptions.push(analysisStatusItem);

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

    vscode.commands.registerCommand('phpstrom.refreshProblemsScan', () => {
      client?.sendNotification('phpstrom/scanWorkspaceDiagnostics');
    }),

    vscode.commands.registerCommand('phpstrom.problems.showListView', () => {
      diagnosticsTreeProvider.setViewMode('list');
    }),

    vscode.commands.registerCommand('phpstrom.problems.showTreeView', () => {
      diagnosticsTreeProvider.setViewMode('tree');
    }),

    vscode.commands.registerCommand('phpstrom.problems.setFilePathFilter', async () => {
      await diagnosticsTreeProvider.promptFilePathFilter();
    }),

    vscode.commands.registerCommand('phpstrom.problems.clearFilePathFilter', () => {
      diagnosticsTreeProvider.clearFilePathFilter();
    }),

    vscode.commands.registerCommand('phpstrom.problems.enableRegexFilter', () => {
      diagnosticsTreeProvider.setFilePathFilterRegexEnabled(true);
    }),

    vscode.commands.registerCommand('phpstrom.problems.disableRegexFilter', () => {
      diagnosticsTreeProvider.setFilePathFilterRegexEnabled(false);
    }),

    vscode.commands.registerCommand('phpstrom.problems.enableErrorsOnlyFilter', () => {
      diagnosticsTreeProvider.setErrorsOnlyFilterEnabled(true);
    }),

    vscode.commands.registerCommand('phpstrom.problems.disableErrorsOnlyFilter', () => {
      diagnosticsTreeProvider.setErrorsOnlyFilterEnabled(false);
    }),

    vscode.commands.registerCommand('phpstrom.showOutputChannel', () => {
      outputChannel.show();
    }),

    vscode.commands.registerCommand('phpstrom.showDetectedPhpExtensions', () => {
      showDetectedPhpExtensions();
    }),

    vscode.commands.registerCommand('phpstrom.problems.openDiagnostic', async (node) => {
      await openDiagnosticNode(node);
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

    vscode.workspace.onDidSaveTextDocument((document) => {
      if (document.languageId !== 'php' || document.uri.scheme !== 'file') {
        return;
      }

      const uri = document.uri.toString();
      pendingAnalysisUris.set(uri, (pendingAnalysisUris.get(uri) ?? 0) + 1);
      showAnalysisStatus(`$(sync~spin) PHP Strom: Analysing ${path.basename(document.fileName)}`, `PHP Strom is analysing ${document.fileName}`);
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
    documentSelector: [
      { scheme: 'file', language: 'php', pattern: '**/*.php' },
      { scheme: 'file', language: 'php', pattern: '**/*.phar' },
    ],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher('**/*.{php,phar}'),
    },
    initializationOptions: {
      storagePath: context.storageUri?.fsPath,
      globalStoragePath: context.globalStorageUri.fsPath,
      extensionPath: context.extensionPath,
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
      handleDiagnostics: (uri, diagnostics, next) => {
        next(uri, diagnostics);
        diagnosticsTreeProvider.updateDiagnostics(uri, diagnostics);

        const text = diagnostics.length > 0
          ? `$(warning) PHP Strom: ${diagnostics.length.toLocaleString()} diagnostics updated`
          : '$(check) PHP Strom: Analysis updated';
        finishAnalysis(uri.toString(), text, 'PHP Strom finished publishing diagnostics for the saved file');
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
    (params: {
      fileCount: number;
      filesDiscovered: number;
      symbolCount: number;
      linesScanned: number;
      bytesScanned: number;
      durationMs: number;
      filesPerSecond: number;
      linesPerSecond: number;
    }) => {
      clearIndexingHideTimer();
      indexingStatusItem.text = `$(check) PHP Strom: ${params.fileCount.toLocaleString()} files in ${formatDuration(params.durationMs)}`;
      indexingStatusItem.tooltip = [
        'PHP Strom finished indexing the workspace',
        `${params.fileCount.toLocaleString()} / ${params.filesDiscovered.toLocaleString()} files indexed`,
        `${params.linesScanned.toLocaleString()} LOC scanned`,
        `${params.symbolCount.toLocaleString()} symbols indexed`,
      ].join('\n');
      indexingStatusItem.show();
      outputChannel.appendLine(
        `[phpstrom] Indexing complete — ${params.fileCount.toLocaleString()} / ${params.filesDiscovered.toLocaleString()} files, ${params.linesScanned.toLocaleString()} LOC, ${formatBytes(params.bytesScanned)}, ${params.symbolCount.toLocaleString()} symbols in ${formatDuration(params.durationMs)} (${formatRate(params.filesPerSecond)} files/sec, ${formatRate(params.linesPerSecond)} LOC/sec)`,
      );
      indexingHideTimer = setTimeout(() => {
        indexingStatusItem.hide();
        indexingHideTimer = undefined;
      }, 3000);
    },
  );

  client.onNotification(
    'phpstrom/saveAnalysisFinished',
    (params: { uri: string; published: boolean }) => {
      finishAnalysis(
        params.uri,
        '$(check) PHP Strom: Analysis updated',
        'PHP Strom finished analysing the saved file',
      );
    },
  );

  client.onNotification('phpstrom/workspaceDiagnosticsStarted', () => {
    diagnosticsTreeProvider.beginWorkspaceScan();
    outputChannel.appendLine('[phpstrom] Scanning project diagnostics…');
  });

  client.onNotification(
    'phpstrom/workspaceDiagnosticsFinished',
    (params: { filesWithDiagnostics: number; totalDiagnostics: number; capped: boolean; applied: boolean }) => {
      const summary = {
        totalDiagnostics: params.totalDiagnostics,
        capped: params.capped,
      };
      if (params.applied) {
        diagnosticsTreeProvider.finishWorkspaceScan(summary);
      } else {
        diagnosticsTreeProvider.cancelWorkspaceScan(summary);
      }
      outputChannel.appendLine(
        !params.applied
          ? `[phpstrom] Project diagnostics scan reached its safety limit and was not applied (${params.totalDiagnostics.toLocaleString()} diagnostics collected)`
          : `[phpstrom] Project diagnostics scan complete — ${params.filesWithDiagnostics.toLocaleString()} file(s) with diagnostics`,
      );
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

function clearAnalysisHideTimer(): void {
  if (analysisHideTimer) {
    clearTimeout(analysisHideTimer);
    analysisHideTimer = undefined;
  }
}

function showAnalysisStatus(text: string, tooltip: string, hideAfterMs?: number): void {
  clearAnalysisHideTimer();
  analysisStatusItem.text = text;
  analysisStatusItem.tooltip = tooltip;
  analysisStatusItem.show();

  if (hideAfterMs !== undefined) {
    analysisHideTimer = setTimeout(() => {
      analysisStatusItem.hide();
      analysisHideTimer = undefined;
    }, hideAfterMs);
  }
}

function finishAnalysis(uri: string, text: string, tooltip: string): void {
  const pendingCount = pendingAnalysisUris.get(uri);
  if (pendingCount === undefined) {
    return;
  }

  if (pendingCount > 1) {
    pendingAnalysisUris.set(uri, pendingCount - 1);
    return;
  }

  pendingAnalysisUris.delete(uri);
  if (pendingAnalysisUris.size > 0) {
    return;
  }

  showAnalysisStatus(text, tooltip, 2500);
}

async function stopClient(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
  diagnosticsTreeProvider.clear();
  clearIndexingHideTimer();
  clearAnalysisHideTimer();
  pendingAnalysisUris.clear();
  indexingStatusItem.hide();
  analysisStatusItem.hide();
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

function formatDuration(durationMs: number): string {
  if (!Number.isFinite(durationMs) || durationMs < 0) {
    return '0 ms';
  }
  if (durationMs < 1000) {
    return `${Math.round(durationMs).toLocaleString()} ms`;
  }
  return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 2 : 1)}s`;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  const decimals = unitIndex === 0 ? 0 : 1;
  return `${value.toFixed(decimals)} ${units[unitIndex]}`;
}

function formatRate(value: number): string {
  if (!Number.isFinite(value) || value < 0) {
    return '0';
  }
  return Math.round(value).toLocaleString();
}

function getConfiguration(): Record<string, unknown> {
  const config = vscode.workspace.getConfiguration('phpstrom');
  const defaultFilesExclude = ['**/.git/**', '**/node_modules/**', '**/vendor/**/{Tests,tests}/**'];
  const filesExclude = mergeExcludePatterns(
    defaultFilesExclude,
    config.get<string[]>('files.exclude', []),
    enabledWorkspaceExcludePatterns(),
  );

  return {
    enable: config.get<boolean>('enable', true),
    environment: {
      phpVersion: config.get<string>('environment.phpVersion', 'auto'),
      phpVersionOverride: config.get<string>('environment.phpVersionOverride', ''),
      includePaths: config.get<string[]>('environment.includePaths', []),
      documentRoot: config.get<string>('environment.documentRoot', ''),
    },
    files: {
      associations: config.get<string[]>('files.associations', ['**/*.php', '**/*.phtml', '**/*.phar']),
      exclude: filesExclude,
      maxSize: config.get<number>('files.maxSize', 1_000_000),
    },
    stubs: config.get<string[]>('stubs', []),
    diagnostics: {
      enable: config.get<boolean>('diagnostics.enable', true),
      run: config.get<string>('diagnostics.run', 'onType'),
      workspaceScanOnStart: config.get<boolean>('diagnostics.workspaceScanOnStart', false),
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

function enabledWorkspaceExcludePatterns(): string[] {
  const filesConfig = vscode.workspace.getConfiguration('files');
  const configured = filesConfig.get<Record<string, boolean | { when?: string }>>('exclude', {});
  const patterns: string[] = [];

  for (const [pattern, value] of Object.entries(configured)) {
    if (value === false) {
      continue;
    }
    patterns.push(pattern);
  }

  return patterns;
}

function mergeExcludePatterns(...groups: readonly string[][]): string[] {
  const merged = new Set<string>();
  for (const group of groups) {
    for (const pattern of group) {
      const trimmed = pattern.trim();
      if (trimmed) {
        merged.add(trimmed);
      }
    }
  }
  return [...merged];
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
