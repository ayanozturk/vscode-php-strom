import assert from 'node:assert/strict';
import * as vscode from 'vscode';
import { QuickConfigTreeProvider } from '../../client/quickConfigView.js';

export async function runQuickConfigViewTests(): Promise<void> {
  const provider = new QuickConfigTreeProvider();

  try {
    const children = provider.getChildren();
    assert.equal(children.length, 3);
    assert.deepEqual(
      children.map((child) => child.kind),
      ['analysisLevel', 'workspaceScanOnStart', 'phpVersion'],
    );
    assert.deepEqual(provider.getChildren(children[0]), []);

    const analysisLevel = provider.getAnalysisLevel();
    assert.ok(analysisLevel >= 0 && analysisLevel <= 9);

    const analysisItem = provider.getTreeItem(children[0]!);
    assert.equal(analysisItem.label, 'Analysis Level');
    assert.equal(analysisItem.description, `Level ${analysisLevel}`);
    assert.equal(analysisItem.contextValue, 'analysisLevel');
    assert.equal(analysisItem.command?.command, 'phpstrom.quickConfig.selectAnalysisLevel');
    assert.equal(analysisItem.collapsibleState, vscode.TreeItemCollapsibleState.None);

    const scanEnabled = provider.getWorkspaceScanOnStart();
    const scanItem = provider.getTreeItem(children[1]!);
    assert.equal(scanItem.label, 'Analyse on Start');
    assert.equal(scanItem.description, scanEnabled ? 'Enabled' : 'Disabled');
    assert.equal(scanItem.contextValue, 'workspaceScanOnStart');
    assert.equal(scanItem.command?.command, 'phpstrom.quickConfig.selectWorkspaceScanOnStart');

    const phpVersion = provider.getPhpVersion();
    const phpItem = provider.getTreeItem(children[2]!);
    assert.equal(phpItem.label, 'PHP Version');
    assert.equal(phpItem.description, phpVersion === 'auto' ? 'Auto' : phpVersion);
    assert.equal(phpItem.contextValue, 'phpVersion');
    assert.equal(phpItem.command?.command, 'phpstrom.quickConfig.selectPhpVersion');

    let treeChanged = false;
    const treeSub = provider.onDidChangeTreeData(() => {
      treeChanged = true;
    });
    let levelEvent: number | undefined;
    const levelSub = provider.onDidChangeAnalysisLevel((level) => {
      levelEvent = level;
    });

    const config = vscode.workspace.getConfiguration('phpstrom');
    const previousLevel = config.get<number>('diagnostics.analysis.level');
    const nextLevel = previousLevel === 8 ? 7 : 8;
    await config.update('diagnostics.analysis.level', nextLevel, vscode.ConfigurationTarget.Global);
    assert.equal(provider.getAnalysisLevel(), nextLevel);
    assert.equal(treeChanged, true);
    assert.equal(levelEvent, nextLevel);
    await config.update('diagnostics.analysis.level', previousLevel, vscode.ConfigurationTarget.Global);

    treeSub.dispose();
    levelSub.dispose();
  } finally {
    provider.dispose();
  }

  // After dispose, constructing a fresh provider still works (emitters were cleaned up).
  const again = new QuickConfigTreeProvider();
  assert.equal(again.getChildren().length, 3);
  again.dispose();
}
