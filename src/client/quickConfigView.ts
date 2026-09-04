import * as vscode from 'vscode';

const CONFIG_SECTION = 'phpstrom';

// Mirrors the Level0-Level9 scheme already used by server diagnostic codes
// (e.g. Level6.MissingReturnType, Level7.MethodUnion, Level8.MethodNonObject).
const ANALYSIS_LEVELS = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9] as const;
type AnalysisLevel = typeof ANALYSIS_LEVELS[number];
const ANALYSIS_LEVEL_SETTING = 'diagnostics.analysis.level';
const DEFAULT_ANALYSIS_LEVEL: AnalysisLevel = 9;
const ANALYSIS_LEVEL_DESCRIPTIONS: Record<AnalysisLevel, string> = {
  0: 'Basic checks: unknown classes/methods/functions, invocation & language errors',
  1: 'Adds possibly-undefined variable checks',
  2: 'Adds method existence, visibility & PHPDoc consistency checks',
  3: 'Adds throw-type checks',
  4: 'Dead code checks (not yet implemented)',
  5: 'Argument type checks (not yet implemented)',
  6: 'Flags missing type hints (params, return, properties, generics)',
  7: 'Flags partially-invalid calls on union types',
  8: 'Flags method/property access on nullable types',
  9: 'Strictest: cautious handling of mixed types (not yet implemented)',
};

const WORKSPACE_SCAN_ON_START_SETTING = 'diagnostics.workspaceScanOnStart';
const DEFAULT_WORKSPACE_SCAN_ON_START = false;

// Mirrors the phpstrom.environment.phpVersion setting's enum/descriptions.
const PHP_VERSIONS = ['auto', '8.2', '8.3', '8.4', '8.5'] as const;
type PhpVersion = typeof PHP_VERSIONS[number];
const PHP_VERSION_SETTING = 'environment.phpVersion';
const DEFAULT_PHP_VERSION: PhpVersion = 'auto';
const PHP_VERSION_DESCRIPTIONS: Record<PhpVersion, string> = {
  auto: 'Detect from composer.json, then fall back to 8.3',
  '8.2': 'PHP 8.2',
  '8.3': 'PHP 8.3',
  '8.4': 'PHP 8.4',
  '8.5': 'PHP 8.5',
};

type QuickConfigNode =
  | { kind: 'analysisLevel'; id: string }
  | { kind: 'workspaceScanOnStart'; id: string }
  | { kind: 'phpVersion'; id: string };

type PickOption<T> = {
  label: string;
  value: T;
  detail: string;
};

export class QuickConfigTreeProvider implements vscode.TreeDataProvider<QuickConfigNode>, vscode.Disposable {
  private readonly treeDataEmitter = new vscode.EventEmitter<QuickConfigNode | undefined | void>();
  private readonly analysisLevelChangeEmitter = new vscode.EventEmitter<AnalysisLevel>();
  private readonly configListener: vscode.Disposable;

  readonly onDidChangeTreeData = this.treeDataEmitter.event;
  readonly onDidChangeAnalysisLevel = this.analysisLevelChangeEmitter.event;

  constructor() {
    // Picks up edits made via the quick pick, the Settings UI, or settings.json directly.
    this.configListener = vscode.workspace.onDidChangeConfiguration((event) => {
      const analysisLevelChanged = event.affectsConfiguration(`${CONFIG_SECTION}.${ANALYSIS_LEVEL_SETTING}`);
      const otherChanged =
        event.affectsConfiguration(`${CONFIG_SECTION}.${WORKSPACE_SCAN_ON_START_SETTING}`) ||
        event.affectsConfiguration(`${CONFIG_SECTION}.${PHP_VERSION_SETTING}`);

      if (!analysisLevelChanged && !otherChanged) {
        return;
      }

      this.treeDataEmitter.fire();
      if (analysisLevelChanged) {
        this.analysisLevelChangeEmitter.fire(this.getAnalysisLevel());
      }
    });
  }

  dispose(): void {
    this.configListener.dispose();
  }

  getAnalysisLevel(): AnalysisLevel {
    return vscode.workspace.getConfiguration(CONFIG_SECTION).get<AnalysisLevel>(ANALYSIS_LEVEL_SETTING, DEFAULT_ANALYSIS_LEVEL);
  }

  getWorkspaceScanOnStart(): boolean {
    return vscode.workspace
      .getConfiguration(CONFIG_SECTION)
      .get<boolean>(WORKSPACE_SCAN_ON_START_SETTING, DEFAULT_WORKSPACE_SCAN_ON_START);
  }

  getPhpVersion(): PhpVersion {
    return vscode.workspace.getConfiguration(CONFIG_SECTION).get<PhpVersion>(PHP_VERSION_SETTING, DEFAULT_PHP_VERSION);
  }

  // Not yet wired to the analyzer; the setting is persisted but has no effect on diagnostics yet.
  async promptAnalysisLevel(): Promise<void> {
    const current = this.getAnalysisLevel();
    const picked = await this.pickValue(
      'PHP Strom: Analysis Level',
      'Select how strict PHP Strom analysis should be (0 = basic, 9 = strictest)',
      ANALYSIS_LEVELS.map((level) => ({ label: `Level ${level}`, value: level, detail: ANALYSIS_LEVEL_DESCRIPTIONS[level] })),
      current,
    );

    if (picked === undefined || picked === current) {
      return;
    }

    await this.updateSetting(ANALYSIS_LEVEL_SETTING, picked);
  }

  async promptWorkspaceScanOnStart(): Promise<void> {
    const current = this.getWorkspaceScanOnStart();
    const picked = await this.pickValue(
      'PHP Strom: Analyse on Start',
      'Scan every workspace PHP file for diagnostics when the server starts',
      [
        { label: 'Enabled', value: true, detail: 'Scan the whole workspace as soon as the server starts' },
        {
          label: 'Disabled',
          value: false,
          detail: 'Only analyse opened or saved files; use PHP Strom: Refresh Problems Scan for a full scan',
        },
      ],
      current,
    );

    if (picked === undefined || picked === current) {
      return;
    }

    await this.updateSetting(WORKSPACE_SCAN_ON_START_SETTING, picked);
  }

  async promptPhpVersion(): Promise<void> {
    const current = this.getPhpVersion();
    const picked = await this.pickValue(
      'PHP Strom: PHP Version',
      'Select the PHP version used for analysis and stubs',
      PHP_VERSIONS.map((version) => ({
        label: version === 'auto' ? 'Auto' : `PHP ${version}`,
        value: version,
        detail: PHP_VERSION_DESCRIPTIONS[version],
      })),
      current,
    );

    if (picked === undefined || picked === current) {
      return;
    }

    await this.updateSetting(PHP_VERSION_SETTING, picked);
  }

  // Saved as a User setting so it's a real, editable preference (not workspace-local hidden state).
  private async updateSetting(setting: string, value: unknown): Promise<void> {
    await vscode.workspace.getConfiguration(CONFIG_SECTION).update(setting, value, vscode.ConfigurationTarget.Global);
  }

  private async pickValue<T>(
    title: string,
    placeholder: string,
    options: PickOption<T>[],
    current: T,
  ): Promise<T | undefined> {
    const items = options.map((option) => ({
      ...option,
      description: option.value === current ? 'Current' : undefined,
    }));

    const quickPick = vscode.window.createQuickPick<(typeof items)[number]>();
    quickPick.title = title;
    quickPick.placeholder = placeholder;
    quickPick.items = items;
    quickPick.activeItems = items.filter((item) => item.value === current);

    const picked = await new Promise<(typeof items)[number] | undefined>((resolve) => {
      quickPick.onDidAccept(() => resolve(quickPick.selectedItems[0]));
      quickPick.onDidHide(() => resolve(undefined));
      quickPick.show();
    });
    quickPick.dispose();

    return picked?.value;
  }

  getTreeItem(element: QuickConfigNode): vscode.TreeItem {
    switch (element.kind) {
      case 'analysisLevel': {
        const level = this.getAnalysisLevel();
        const item = new vscode.TreeItem('Analysis Level', vscode.TreeItemCollapsibleState.None);
        item.id = element.id;
        item.description = `Level ${level}`;
        item.tooltip = `${ANALYSIS_LEVEL_DESCRIPTIONS[level]}\nClick to change`;
        item.iconPath = new vscode.ThemeIcon('gear');
        item.command = {
          command: 'phpstrom.quickConfig.selectAnalysisLevel',
          title: 'Select Analysis Level',
        };
        item.contextValue = 'analysisLevel';
        return item;
      }
      case 'workspaceScanOnStart': {
        const enabled = this.getWorkspaceScanOnStart();
        const item = new vscode.TreeItem('Analyse on Start', vscode.TreeItemCollapsibleState.None);
        item.id = element.id;
        item.description = enabled ? 'Enabled' : 'Disabled';
        item.tooltip = 'Scan every workspace PHP file for diagnostics when the server starts\nClick to change';
        item.iconPath = new vscode.ThemeIcon('rocket');
        item.command = {
          command: 'phpstrom.quickConfig.selectWorkspaceScanOnStart',
          title: 'Select Analyse on Start',
        };
        item.contextValue = 'workspaceScanOnStart';
        return item;
      }
      case 'phpVersion': {
        const version = this.getPhpVersion();
        const item = new vscode.TreeItem('PHP Version', vscode.TreeItemCollapsibleState.None);
        item.id = element.id;
        item.description = version === 'auto' ? 'Auto' : version;
        item.tooltip = `${PHP_VERSION_DESCRIPTIONS[version]}\nClick to change`;
        item.iconPath = new vscode.ThemeIcon('versions');
        item.command = {
          command: 'phpstrom.quickConfig.selectPhpVersion',
          title: 'Select PHP Version',
        };
        item.contextValue = 'phpVersion';
        return item;
      }
    }
  }

  getChildren(element?: QuickConfigNode): QuickConfigNode[] {
    if (element) {
      return [];
    }

    return [
      { kind: 'analysisLevel', id: 'quickConfig.analysisLevel' },
      { kind: 'workspaceScanOnStart', id: 'quickConfig.workspaceScanOnStart' },
      { kind: 'phpVersion', id: 'quickConfig.phpVersion' },
    ];
  }
}
