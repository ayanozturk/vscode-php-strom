import * as vscode from 'vscode';

type ProblemCategoryKey = 'style' | 'staticAnalysis';
type DiagnosticTypeFilter = 'all' | 'errors';

type ProblemCategoryNode = {
  kind: 'category';
  id: ProblemCategoryKey;
  label: string;
  diagnosticsCount: number;
  fileCount: number;
  problemTypeCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  children: ProblemTypeNode[];
};

type ProblemTypeNode = {
  kind: 'problemType';
  id: string;
  label: string;
  diagnosticsCount: number;
  fileCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  children: ProblemListFileNode[];
};

type ProblemListFileNode = {
  kind: 'listFile';
  id: string;
  category: ProblemCategoryKey;
  uri: vscode.Uri;
  relativePath: string;
  diagnosticsCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  firstDiagnostic: vscode.Diagnostic;
  problemTypeCount: number;
  label: string;
  description: string;
};

type DiagnosticsViewNode = ProblemCategoryNode | ProblemTypeNode | ProblemListFileNode;

type ViewStats = {
  totalDiagnostics: number;
  totalFiles: number;
  totalCategories: number;
  totalProblemTypes: number;
};

type ScanSummary = {
  totalDiagnostics: number;
  capped: boolean;
};

export class ProjectDiagnosticsTreeProvider implements vscode.TreeDataProvider<DiagnosticsViewNode> {
  private readonly diagnosticsByUri = new Map<string, readonly vscode.Diagnostic[]>();
  private stagedDiagnosticsByUri: Map<string, readonly vscode.Diagnostic[]> | undefined;
  private readonly treeDataEmitter = new vscode.EventEmitter<DiagnosticsViewNode | undefined | void>();
  private view: vscode.TreeView<DiagnosticsViewNode> | undefined;
  private workspaceScanInProgress = false;
  private workspaceScanProgress = { done: 0, total: 0 };
  private lastScanSummary: ScanSummary | undefined;
  private filePathFilter = '';
  private filePathFilterRegex = false;
  private filePathFilterRegexError: string | undefined;
  private diagnosticTypeFilter: DiagnosticTypeFilter = 'all';

  readonly onDidChangeTreeData = this.treeDataEmitter.event;

  setView(view: vscode.TreeView<DiagnosticsViewNode>): void {
    this.view = view;
    this.updateFilterContexts();
    this.updateViewPresentation();
  }

  async promptFilePathFilter(): Promise<void> {
    const input = await vscode.window.showInputBox({
      title: 'Filter Problems by File/Folder',
      prompt: this.filePathFilterRegex
        ? 'Enter a regular expression to match file or folder paths.'
        : 'Enter text to match file or folder paths.',
      placeHolder: this.filePathFilterRegex ? 'e.g. ^src/.+\\.php$' : 'e.g. src/Controllers or User.php',
      value: this.filePathFilter,
      ignoreFocusOut: true,
    });

    if (input === undefined) {
      return;
    }

    this.filePathFilter = input.trim();
    this.updateFilterContexts();
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  clearFilePathFilter(): void {
    if (!this.filePathFilter) {
      return;
    }

    this.filePathFilter = '';
    this.updateFilterContexts();
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  setFilePathFilterRegexEnabled(enabled: boolean): void {
    if (this.filePathFilterRegex === enabled) {
      return;
    }

    this.filePathFilterRegex = enabled;
    this.updateFilterContexts();
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  setErrorsOnlyFilterEnabled(enabled: boolean): void {
    const nextFilter: DiagnosticTypeFilter = enabled ? 'errors' : 'all';
    if (this.diagnosticTypeFilter === nextFilter) {
      return;
    }

    this.diagnosticTypeFilter = nextFilter;
    this.updateFilterContexts();
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  updateDiagnostics(uri: vscode.Uri, diagnostics: readonly vscode.Diagnostic[]): void {
    const target = this.stagedDiagnosticsByUri ?? this.diagnosticsByUri;
    if (diagnostics.length === 0) {
      target.delete(uri.toString());
    } else {
      target.set(uri.toString(), diagnostics);
    }

    this.updateViewPresentation();
    if (!this.workspaceScanInProgress) {
      this.treeDataEmitter.fire();
    }
  }

  beginWorkspaceScan(): void {
    this.workspaceScanInProgress = true;
    this.workspaceScanProgress = { done: 0, total: 0 };
    this.stagedDiagnosticsByUri = new Map<string, readonly vscode.Diagnostic[]>();
    this.lastScanSummary = undefined;
    this.updateViewPresentation();
  }

  finishWorkspaceScan(summary?: ScanSummary): void {
    if (this.stagedDiagnosticsByUri) {
      this.diagnosticsByUri.clear();
      for (const [uri, diagnostics] of this.stagedDiagnosticsByUri.entries()) {
        this.diagnosticsByUri.set(uri, diagnostics);
      }
    }

    this.stagedDiagnosticsByUri = undefined;
    this.workspaceScanInProgress = false;
    this.lastScanSummary = summary;
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  cancelWorkspaceScan(summary?: ScanSummary): void {
    this.stagedDiagnosticsByUri = undefined;
    this.workspaceScanInProgress = false;
    this.lastScanSummary = summary;
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  updateWorkspaceScanProgress(done: number, total: number): void {
    this.workspaceScanProgress = { done, total };
    this.updateViewPresentation();
  }

  clear(): void {
    this.diagnosticsByUri.clear();
    this.stagedDiagnosticsByUri = undefined;
    this.workspaceScanInProgress = false;
    this.updateViewPresentation();
    this.treeDataEmitter.fire();
  }

  getTreeItem(element: DiagnosticsViewNode): vscode.TreeItem {
    switch (element.kind) {
      case 'category': {
        const item = new vscode.TreeItem(
          categoryDisplayLabel(element.id),
          vscode.TreeItemCollapsibleState.Expanded,
        );
        item.id = `category:${element.id}`;
        item.description = `${formatCount(element.diagnosticsCount)} issues • ${formatCount(element.fileCount)} files • ${formatCount(element.problemTypeCount)} types`;
        item.iconPath = iconForCategory(element.id, element.maxSeverity);
        item.tooltip = `${element.label}\n${formatCount(element.diagnosticsCount)} diagnostic${element.diagnosticsCount === 1 ? '' : 's'} across ${formatCount(element.problemTypeCount)} problem type${element.problemTypeCount === 1 ? '' : 's'} and ${formatCount(element.fileCount)} file${element.fileCount === 1 ? '' : 's'}`;
        item.accessibilityInformation = {
          label: `${element.label}, ${formatCount(element.diagnosticsCount)} diagnostics in ${formatCount(element.fileCount)} files`,
          role: 'treeitem',
        };
        return item;
      }
      case 'problemType': {
        const display = formatProblemTypeLabel(element.label);
        const item = new vscode.TreeItem(
          display.label,
          vscode.TreeItemCollapsibleState.Collapsed,
        );
        item.id = element.id;
        item.description = [
          display.context,
          `${formatCount(element.diagnosticsCount)} issues`,
          `${formatCount(element.fileCount)} files`,
        ].filter(Boolean).join(' • ');
        item.iconPath = iconForSeverity(element.maxSeverity);
        item.tooltip = `${element.label}\n${formatCount(element.diagnosticsCount)} diagnostic${element.diagnosticsCount === 1 ? '' : 's'} in ${formatCount(element.fileCount)} file${element.fileCount === 1 ? '' : 's'}`;
        item.accessibilityInformation = {
          label: `${element.label}, ${formatCount(element.diagnosticsCount)} diagnostics in ${formatCount(element.fileCount)} files`,
          role: 'treeitem',
        };
        return item;
      }
      case 'listFile': {
        const item = new vscode.TreeItem(
          element.label,
          vscode.TreeItemCollapsibleState.None,
        );
        item.id = element.id;
        item.resourceUri = element.uri;
        item.description = `${formatCount(element.diagnosticsCount)} issue${element.diagnosticsCount === 1 ? '' : 's'} • ${formatCount(element.problemTypeCount)} type${element.problemTypeCount === 1 ? '' : 's'}`;
        item.iconPath = iconForSeverity(element.maxSeverity);
        item.tooltip = new vscode.MarkdownString([
          `**${element.relativePath}**`,
          '',
          `${formatCount(element.diagnosticsCount)} diagnostic${element.diagnosticsCount === 1 ? '' : 's'}`,
          '',
          `${formatCount(element.problemTypeCount)} problem type${element.problemTypeCount === 1 ? '' : 's'}`,
        ].join('\n'));
        item.accessibilityInformation = {
          label: `${element.relativePath}, ${formatCount(element.diagnosticsCount)} diagnostics across ${formatCount(element.problemTypeCount)} problem types`,
          role: 'treeitem',
        };
        item.command = {
          command: 'phpstrom.problems.openDiagnostic',
          title: 'Open Diagnostic',
          arguments: [element],
        };
        return item;
      }
    }
  }

  getChildren(element?: DiagnosticsViewNode): DiagnosticsViewNode[] {
    const categories = this.buildProblemCategoryNodes();

    if (!element) {
      return categories;
    }

    if (element.kind === 'category') {
      return element.children;
    }

    if (element.kind === 'problemType') {
      return element.children;
    }

    return [];
  }

  private updateFilterContexts(): void {
    void vscode.commands.executeCommand('setContext', 'phpstromProblems.filterActive', this.filePathFilter.length > 0);
    void vscode.commands.executeCommand('setContext', 'phpstromProblems.filterRegex', this.filePathFilterRegex);
    void vscode.commands.executeCommand('setContext', 'phpstromProblems.errorsOnly', this.diagnosticTypeFilter === 'errors');
  }

  private updateViewPresentation(): void {
    if (!this.view) {
      return;
    }

    if (this.workspaceScanInProgress) {
      const filterText = this.formatFilterSummary();
      const progressText = this.workspaceScanProgress.total > 0
        ? ` (${formatCount(this.workspaceScanProgress.done)}/${formatCount(this.workspaceScanProgress.total)} files)`
        : '';
      this.view.message = filterText
        ? `Scanning project diagnostics…${progressText} ${filterText}`
        : `Scanning project diagnostics…${progressText}`;
      this.view.badge = undefined;
      return;
    }

    const stats = this.getStats();
    const filterText = this.formatFilterSummary();
    if (this.filePathFilterRegexError) {
      this.view.message = `Invalid regex filter: ${this.filePathFilterRegexError}${filterText ? ` ${filterText}` : ''}`;
      this.view.badge = undefined;
      return;
    }

    if (stats.totalDiagnostics === 0) {
      this.view.message = this.lastScanSummary?.capped
        ? `Stopped after ${formatCount(this.lastScanSummary.totalDiagnostics)} diagnostics.`
        : 'No PHP Strom diagnostics in the current workspace.';
      if (filterText) {
        this.view.message = `${this.view.message} ${filterText}`;
      }
      this.view.badge = undefined;
      return;
    }

    const suffix = this.lastScanSummary?.capped
      ? ` Scan stopped at ${formatCount(this.lastScanSummary.totalDiagnostics)} diagnostics.`
      : '';
    this.view.message = `${formatCount(stats.totalDiagnostics)} diagnostics in ${formatCount(stats.totalFiles)} file${stats.totalFiles === 1 ? '' : 's'} across ${formatCount(stats.totalProblemTypes)} problem type${stats.totalProblemTypes === 1 ? '' : 's'}.${suffix}${filterText ? ` ${filterText}` : ''}`;
    this.view.badge = {
      value: stats.totalDiagnostics,
      tooltip: 'PHP Strom diagnostics',
    };
  }

  private getStats(): ViewStats {
    const categories = this.buildProblemCategoryNodes();
    const fileUris = new Set<string>();
    let totalDiagnostics = 0;
    let totalProblemTypes = 0;

    for (const category of categories) {
      totalDiagnostics += category.diagnosticsCount;
      totalProblemTypes += category.problemTypeCount;
      for (const child of category.children) {
        for (const uri of collectProblemTypeFileUris(child.children)) {
          fileUris.add(uri);
        }
      }
    }

    return {
      totalDiagnostics,
      totalFiles: fileUris.size,
      totalCategories: categories.length,
      totalProblemTypes,
    };
  }

  private buildProblemCategoryNodes(): ProblemCategoryNode[] {
    const matchesPath = this.createPathFilterPredicate();
    const matchesDiagnostic = this.createDiagnosticTypeFilterPredicate();
    return this.buildListCategoryNodes(matchesPath, matchesDiagnostic);
  }

  private buildListCategoryNodes(
    matchesPath: (relativePath: string) => boolean,
    matchesDiagnostic: (diagnostic: vscode.Diagnostic) => boolean,
  ): ProblemCategoryNode[] {
    const categories = new Map<ProblemCategoryKey, {
      label: string;
      problemTypes: Map<string, {
        label: string;
        files: Map<string, {
          uri: vscode.Uri;
          relativePath: string;
          diagnosticsCount: number;
          maxSeverity: vscode.DiagnosticSeverity;
          firstDiagnostic: vscode.Diagnostic;
        }>;
      }>;
    }>();

    for (const [uriString, diagnostics] of this.diagnosticsByUri.entries()) {
      const uri = vscode.Uri.parse(uriString);
      const relativePath = vscode.workspace.asRelativePath(uri, false);

      if (!matchesPath(relativePath)) {
        continue;
      }

      for (const diagnostic of diagnostics) {
        if (!matchesDiagnostic(diagnostic)) {
          continue;
        }

        const categoryInfo = getDiagnosticCategory(diagnostic);
        let category = categories.get(categoryInfo.key);
        if (!category) {
          category = { label: categoryInfo.label, problemTypes: new Map() };
          categories.set(categoryInfo.key, category);
        }

        const groupKey = getDiagnosticGroupKey(diagnostic);
        let problemType = category.problemTypes.get(groupKey);
        if (!problemType) {
          problemType = {
            label: getDiagnosticGroupLabel(diagnostic),
            files: new Map(),
          };
          category.problemTypes.set(groupKey, problemType);
        }

        let file = problemType.files.get(uriString);
        if (!file) {
          file = {
            uri,
            relativePath,
            diagnosticsCount: 0,
            maxSeverity: vscode.DiagnosticSeverity.Hint,
            firstDiagnostic: diagnostic,
          };
          problemType.files.set(uriString, file);
        }

        file.diagnosticsCount += 1;
        file.maxSeverity = maxSeverity(file.maxSeverity, diagnostic.severity);
        if (compareDiagnosticPositions(diagnostic, file.firstDiagnostic) < 0) {
          file.firstDiagnostic = diagnostic;
        }
      }
    }

    return [...categories.entries()]
      .map(([categoryKey, category]): ProblemCategoryNode => {
        const problemTypes = buildListProblemTypeNodes(categoryKey, category.problemTypes);
        const fileUris = new Set<string>();
        const diagnosticsCount = problemTypes.reduce((total, problemType) => total + problemType.diagnosticsCount, 0);
        const maxCategorySeverity = problemTypes.reduce(
          (severity, file) => maxSeverity(severity, file.maxSeverity),
          vscode.DiagnosticSeverity.Hint,
        );

        for (const problemType of problemTypes) {
          for (const file of problemType.children) {
            fileUris.add(file.uri.toString());
          }
        }

        return {
          kind: 'category',
          id: categoryKey,
          label: category.label,
          diagnosticsCount,
          fileCount: fileUris.size,
          problemTypeCount: problemTypes.length,
          maxSeverity: maxCategorySeverity,
          children: problemTypes,
        };
      })
      .sort((left, right) => categorySortOrder(left.id) - categorySortOrder(right.id));
  }

  private createPathFilterPredicate(): (relativePath: string) => boolean {
    this.filePathFilterRegexError = undefined;
    const filter = this.filePathFilter;
    if (!filter) {
      return () => true;
    }

    if (!this.filePathFilterRegex) {
      const normalizedFilter = filter.toLocaleLowerCase();
      return (relativePath: string) => relativePath.toLocaleLowerCase().includes(normalizedFilter);
    }

    let expression: RegExp;
    try {
      expression = new RegExp(filter, 'i');
    } catch (error) {
      this.filePathFilterRegexError = error instanceof Error ? error.message : 'Invalid pattern';
      return () => false;
    }

    return (relativePath: string) => expression.test(relativePath);
  }

  private createDiagnosticTypeFilterPredicate(): (diagnostic: vscode.Diagnostic) => boolean {
    if (this.diagnosticTypeFilter !== 'errors') {
      return () => true;
    }

    return (diagnostic: vscode.Diagnostic) => diagnostic.severity === vscode.DiagnosticSeverity.Error;
  }

  private formatFilterSummary(): string {
    const segments: string[] = [];
    if (this.diagnosticTypeFilter === 'errors') {
      segments.push('Type: errors only');
    }

    if (this.filePathFilter) {
      const mode = this.filePathFilterRegex ? 'regex' : 'text';
      segments.push(`Filter (${mode}): ${this.filePathFilter}`);
    }

    return segments.join(' • ');
  }
}

function buildListProblemTypeNodes(
  categoryKey: ProblemCategoryKey,
  groups: Map<string, {
    label: string;
    files: Map<string, {
      uri: vscode.Uri;
      relativePath: string;
      diagnosticsCount: number;
      maxSeverity: vscode.DiagnosticSeverity;
      firstDiagnostic: vscode.Diagnostic;
    }>;
  }>,
): ProblemTypeNode[] {
  return [...groups.entries()]
    .map(([groupKey, group]): ProblemTypeNode => {
      const files = [...group.files.entries()]
        .map(([uriString, file]): ProblemListFileNode => ({
          kind: 'listFile',
          id: `listFile:${categoryKey}:${groupKey}:${uriString}`,
          category: categoryKey,
          uri: file.uri,
          relativePath: file.relativePath,
          diagnosticsCount: file.diagnosticsCount,
          maxSeverity: file.maxSeverity,
          firstDiagnostic: file.firstDiagnostic,
          problemTypeCount: 1,
          label: file.relativePath,
          description: `${file.diagnosticsCount}`,
        }))
        .sort(compareListFiles);

      const diagnosticsCount = files.reduce((total, file) => total + file.diagnosticsCount, 0);
      const maxGroupSeverity = files.reduce(
        (severity, file) => maxSeverity(severity, file.maxSeverity),
        vscode.DiagnosticSeverity.Hint,
      );

      return {
        kind: 'problemType',
        id: `problemType:${categoryKey}:${groupKey}:list`,
        label: group.label,
        diagnosticsCount,
        fileCount: files.length,
        maxSeverity: maxGroupSeverity,
        children: files,
      };
    })
    .sort((left, right) => {
      if (right.diagnosticsCount !== left.diagnosticsCount) {
        return right.diagnosticsCount - left.diagnosticsCount;
      }

      return left.label.localeCompare(right.label);
    });
}

export async function openDiagnosticNode(node: ProblemListFileNode): Promise<void> {
  const document = await vscode.workspace.openTextDocument(node.uri);
  const diagnostic = node.firstDiagnostic;
  const editor = await vscode.window.showTextDocument(document, {
    preview: true,
    selection: new vscode.Range(diagnostic.range.start, diagnostic.range.end),
  });

  editor.revealRange(diagnostic.range, vscode.TextEditorRevealType.InCenter);
}

function compareListFiles(left: ProblemListFileNode, right: ProblemListFileNode): number {
  if (right.diagnosticsCount !== left.diagnosticsCount) {
    return right.diagnosticsCount - left.diagnosticsCount;
  }

  const pathComparison = left.relativePath.localeCompare(right.relativePath);
  if (pathComparison !== 0) {
    return pathComparison;
  }

  return compareDiagnosticPositions(left.firstDiagnostic, right.firstDiagnostic);
}

function compareDiagnosticPositions(left: vscode.Diagnostic, right: vscode.Diagnostic): number {
  if (left.range.start.line !== right.range.start.line) {
    return left.range.start.line - right.range.start.line;
  }

  if (left.range.start.character !== right.range.start.character) {
    return left.range.start.character - right.range.start.character;
  }

  return left.message.localeCompare(right.message);
}

function collectProblemTypeFileUris(children: ProblemListFileNode[]): string[] {
  return children.map((child) => child.uri.toString());
}

function getDiagnosticGroupKey(diagnostic: vscode.Diagnostic): string {
  const code = diagnosticCodeString(diagnostic);
  if (code) {
    return code;
  }

  return diagnostic.message.split('\n', 1)[0];
}

function getDiagnosticGroupLabel(diagnostic: vscode.Diagnostic): string {
  const code = diagnosticCodeString(diagnostic);
  if (code) {
    return code;
  }

  return diagnostic.message.split('\n', 1)[0] || 'General';
}

function diagnosticCodeString(diagnostic: vscode.Diagnostic): string | undefined {
  const code = diagnostic.code;
  if (typeof code === 'string' || typeof code === 'number') {
    return String(code);
  }

  if (code && typeof code === 'object' && 'value' in code) {
    return String(code.value);
  }

  return undefined;
}

function getDiagnosticCategory(diagnostic: vscode.Diagnostic): { key: ProblemCategoryKey; label: string } {
  const code = diagnosticCodeString(diagnostic);
  if (isStyleDiagnosticCode(code)) {
    return { key: 'style', label: 'Style Issues' };
  }

  return { key: 'staticAnalysis', label: 'Static Analysis Issues' };
}

function isStyleDiagnosticCode(code: string | undefined): boolean {
  if (!code) {
    return false;
  }

  return code.startsWith('PSR')
    || code.startsWith('Squiz.')
    || code.startsWith('Generic.Formatting.')
    || code.startsWith('Generic.WhiteSpace.');
}

function categorySortOrder(category: ProblemCategoryKey): number {
  switch (category) {
    case 'style':
      return 0;
    case 'staticAnalysis':
      return 1;
  }
}

function formatCount(count: number): string {
  return count.toLocaleString();
}

function categoryDisplayLabel(category: ProblemCategoryKey): string {
  switch (category) {
    case 'style':
      return 'Style';
    case 'staticAnalysis':
      return 'Static Analysis';
  }
}

function formatProblemTypeLabel(label: string): { label: string; context?: string } {
  const segments = label.split('.');
  if (segments.length < 2) {
    return { label };
  }

  const visibleLabel = segments[segments.length - 1];
  if (/^[A-Z0-9_]+$/.test(visibleLabel) && visibleLabel.length <= 8) {
    return { label };
  }

  const context = segments.slice(0, -1).join('.');

  return {
    label: visibleLabel,
    context,
  };
}

function iconForSeverity(severity: vscode.DiagnosticSeverity): vscode.ThemeIcon {
  switch (severity) {
    case vscode.DiagnosticSeverity.Error:
      return new vscode.ThemeIcon('error', new vscode.ThemeColor('problemsErrorIcon.foreground'));
    case vscode.DiagnosticSeverity.Warning:
      return new vscode.ThemeIcon('warning', new vscode.ThemeColor('problemsWarningIcon.foreground'));
    case vscode.DiagnosticSeverity.Information:
      return new vscode.ThemeIcon('info', new vscode.ThemeColor('problemsInfoIcon.foreground'));
    case vscode.DiagnosticSeverity.Hint:
    default:
      return new vscode.ThemeIcon('lightbulb');
  }
}

function iconForCategory(category: ProblemCategoryKey, severity: vscode.DiagnosticSeverity): vscode.ThemeIcon {
  switch (category) {
    case 'style':
      return new vscode.ThemeIcon('symbol-color', new vscode.ThemeColor('charts.purple'));
    case 'staticAnalysis':
      return iconForSeverity(severity);
  }
}

function maxSeverity(
  current: vscode.DiagnosticSeverity,
  next: vscode.DiagnosticSeverity | undefined,
): vscode.DiagnosticSeverity {
  if (next === undefined) {
    return current;
  }

  return next < current ? next : current;
}
