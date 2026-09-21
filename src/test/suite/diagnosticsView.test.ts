import assert from 'node:assert/strict';
import * as vscode from 'vscode';
import { ProjectDiagnosticsTreeProvider } from '../../client/diagnosticsView.js';

type ViewStub = {
  message: string | undefined;
  badge: { value: number; tooltip: string } | undefined;
};

function createViewStub(): ViewStub & vscode.TreeView<never> {
  return {
    message: undefined,
    badge: undefined,
  } as ViewStub & vscode.TreeView<never>;
}

function diagnostic(
  message: string,
  severity: vscode.DiagnosticSeverity,
  code?: string | number | { value: string | number; target: vscode.Uri },
  line = 0,
  character = 0,
): vscode.Diagnostic {
  const range = new vscode.Range(line, character, line, character + 1);
  const item = new vscode.Diagnostic(range, message, severity);
  if (code !== undefined) {
    item.code = code;
  }
  return item;
}

async function setPathFilter(
  provider: ProjectDiagnosticsTreeProvider,
  value: string | undefined,
): Promise<void> {
  const original = vscode.window.showInputBox;
  (vscode.window as { showInputBox: typeof vscode.window.showInputBox }).showInputBox = async () => value;
  try {
    await provider.promptFilePathFilter();
  } finally {
    (vscode.window as { showInputBox: typeof vscode.window.showInputBox }).showInputBox = original;
  }
}

export async function runDiagnosticsViewTests(): Promise<void> {
  const provider = new ProjectDiagnosticsTreeProvider();
  const view = createViewStub();
  provider.setView(view);

  const styleUri = vscode.Uri.file('/workspace/src/Style.php');
  const analysisUri = vscode.Uri.file('/workspace/src/Analysis.php');
  const otherUri = vscode.Uri.file('/workspace/tests/Other.php');

  provider.updateDiagnostics(styleUri, [
    diagnostic('brace spacing', vscode.DiagnosticSeverity.Warning, 'PSR12.Classes.OpeningBrace'),
    diagnostic('whitespace', vscode.DiagnosticSeverity.Hint, 'Generic.WhiteSpace.ScopeIndent'),
  ]);
  provider.updateDiagnostics(analysisUri, [
    diagnostic('undefined method', vscode.DiagnosticSeverity.Error, 'Level0.UndefinedMethod'),
    diagnostic('dead code', vscode.DiagnosticSeverity.Warning, 'Level4.UnreachableCode', 2, 0),
  ]);
  provider.updateDiagnostics(otherUri, [
    diagnostic('style elsewhere', vscode.DiagnosticSeverity.Information, 'Squiz.WhiteSpace.SuperfluousWhitespace'),
  ]);

  let roots = provider.getChildren();
  assert.equal(roots.length, 2, 'expected style and static-analysis categories');
  assert.equal(roots[0]?.kind, 'category');
  assert.equal(roots[0]?.id, 'style');
  assert.equal(roots[1]?.kind, 'category');
  assert.equal(roots[1]?.id, 'staticAnalysis');

  const styleItem = provider.getTreeItem(roots[0]!);
  assert.equal(styleItem.label, 'Style');
  assert.equal(styleItem.collapsibleState, vscode.TreeItemCollapsibleState.Expanded);
  assert.match(String(styleItem.description), /issues/);

  const styleTypes = provider.getChildren(roots[0]!);
  assert.ok(styleTypes.length >= 1);
  assert.equal(styleTypes[0]?.kind, 'problemType');
  const problemTypeItem = provider.getTreeItem(styleTypes[0]!);
  assert.equal(problemTypeItem.collapsibleState, vscode.TreeItemCollapsibleState.Collapsed);

  const styleFiles = provider.getChildren(styleTypes[0]!);
  assert.ok(styleFiles.length >= 1);
  assert.equal(styleFiles[0]?.kind, 'listFile');
  const fileItem = provider.getTreeItem(styleFiles[0]!);
  assert.equal(fileItem.collapsibleState, vscode.TreeItemCollapsibleState.None);
  assert.equal(fileItem.command?.command, 'phpstrom.problems.openDiagnostic');
  assert.deepEqual(provider.getChildren(styleFiles[0]!), []);

  await setPathFilter(provider, 'Other.php');
  roots = provider.getChildren();
  assert.equal(roots.length, 1);
  assert.equal(roots[0]?.id, 'style');
  assert.equal(roots[0]?.diagnosticsCount, 1);

  provider.setFilePathFilterRegexEnabled(true);
  await setPathFilter(provider, 'Style\\.php$');
  roots = provider.getChildren();
  assert.equal(roots.length, 1);
  assert.equal(roots[0]?.id, 'style');
  const filteredFiles = roots.flatMap((category) =>
    provider.getChildren(category).flatMap((problemType) =>
      provider.getChildren(problemType).map((file) => {
        assert.equal(file.kind, 'listFile');
        return file.kind === 'listFile' ? file.relativePath : '';
      }),
    ),
  );
  assert.ok(filteredFiles.every((path) => path.endsWith('Style.php')));

  await setPathFilter(provider, '(');
  roots = provider.getChildren();
  assert.equal(roots.length, 0, 'invalid regex filter should match nothing');
  assert.match(String(view.message), /Invalid regex filter/);

  provider.setFilePathFilterRegexEnabled(false);
  provider.clearFilePathFilter();
  provider.setErrorsOnlyFilterEnabled(true);
  roots = provider.getChildren();
  assert.equal(roots.length, 1);
  assert.equal(roots[0]?.id, 'staticAnalysis');
  assert.equal(roots[0]?.diagnosticsCount, 1);
  assert.match(String(view.message), /Type: errors only/);

  provider.setErrorsOnlyFilterEnabled(false);
  provider.setErrorsOnlyFilterEnabled(false); // no-op path
  provider.clearFilePathFilter(); // no-op path

  // Staging: live updates go to staged map and must not replace the tree until finish.
  const beforeScan = provider.getChildren().map((node) => node.diagnosticsCount);
  provider.beginWorkspaceScan();
  assert.match(String(view.message), /Scanning project diagnostics/);
  provider.updateWorkspaceScanProgress(2, 10);
  assert.match(String(view.message), /2\/10/);

  provider.updateDiagnostics(styleUri, []);
  provider.updateDiagnostics(analysisUri, [
    diagnostic('fresh error', vscode.DiagnosticSeverity.Error, 'Level0.UndefinedClass'),
  ]);
  assert.deepEqual(
    provider.getChildren().map((node) => node.diagnosticsCount),
    beforeScan,
    'tree should keep previous diagnostics while scan is staging',
  );

  provider.finishWorkspaceScan({ totalDiagnostics: 1, capped: false });
  roots = provider.getChildren();
  assert.equal(roots.length, 1);
  assert.equal(roots[0]?.id, 'staticAnalysis');
  assert.equal(roots[0]?.diagnosticsCount, 1);
  assert.match(String(view.message), /1 diagnostic/);
  assert.equal(view.badge?.value, 1);

  // Cancel must discard staged diagnostics and keep the previously finished set.
  provider.beginWorkspaceScan();
  provider.updateDiagnostics(otherUri, [
    diagnostic('should not apply', vscode.DiagnosticSeverity.Warning, 'PSR1.Files.SideEffects'),
  ]);
  provider.cancelWorkspaceScan({ totalDiagnostics: 99, capped: true });
  roots = provider.getChildren();
  assert.equal(roots.length, 1);
  assert.equal(roots[0]?.diagnosticsCount, 1);
  assert.match(String(view.message), /Scan stopped at 99/);

  // clear() must drop diagnostics and capped-scan residue.
  provider.clear();
  assert.deepEqual(provider.getChildren(), []);
  assert.equal(view.message, 'No PHP Strom diagnostics in the current workspace.');
  assert.equal(view.badge, undefined);

  provider.beginWorkspaceScan();
  provider.updateDiagnostics(analysisUri, [
    diagnostic('capped only', vscode.DiagnosticSeverity.Error, 'Level0.UndefinedMethod'),
  ]);
  provider.cancelWorkspaceScan({ totalDiagnostics: 50, capped: true });
  assert.match(String(view.message), /Stopped after 50/);
  provider.clear();
  assert.equal(
    view.message,
    'No PHP Strom diagnostics in the current workspace.',
    'clear must reset lastScanSummary so capped messaging does not stick',
  );
}
