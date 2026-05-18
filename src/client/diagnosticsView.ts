import * as vscode from 'vscode';

type DiagnosticsViewMode = 'tree' | 'list';
type ProblemCategoryKey = 'style' | 'staticAnalysis';

type ProblemCategoryNode = {
  kind: 'category';
  id: ProblemCategoryKey;
  label: string;
  diagnosticsCount: number;
  fileCount: number;
  problemTypeCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  children: ProblemTypeNode[] | ProblemListFileNode[];
};

type ProblemTypeNode = {
  kind: 'problemType';
  id: string;
  label: string;
  diagnosticsCount: number;
  fileCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  children: ProblemChildNode[];
};

type ProblemFolderNode = {
  kind: 'folder';
  id: string;
  label: string;
  relativePath: string;
  diagnosticsCount: number;
  fileCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  children: ProblemChildNode[];
};

type ProblemFileNode = {
  kind: 'file';
  id: string;
  uri: vscode.Uri;
  relativePath: string;
  diagnosticsCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  diagnostics: ProblemDiagnosticNode[];
};

type ProblemDiagnosticNode = {
  kind: 'diagnostic';
  id: string;
  uri: vscode.Uri;
  diagnostic: vscode.Diagnostic;
  label: string;
  description: string;
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

type ProblemChildNode = ProblemFolderNode | ProblemFileNode;

type DiagnosticNavigationNode = ProblemDiagnosticNode | ProblemListFileNode;

type DiagnosticsViewNode = ProblemCategoryNode | ProblemTypeNode | ProblemFolderNode | ProblemFileNode | ProblemDiagnosticNode | ProblemListFileNode;

type FolderAccumulator = {
  id: string;
  label: string;
  relativePath: string;
  diagnosticsCount: number;
  fileCount: number;
  maxSeverity: vscode.DiagnosticSeverity;
  folders: Map<string, FolderAccumulator>;
  files: ProblemFileNode[];
};

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
  private lastScanSummary: ScanSummary | undefined;
  private viewMode: DiagnosticsViewMode = 'tree';

  readonly onDidChangeTreeData = this.treeDataEmitter.event;

  setView(view: vscode.TreeView<DiagnosticsViewNode>): void {
    this.view = view;
    void this.updateViewModeContext();
    this.updateViewPresentation();
  }

  setViewMode(mode: DiagnosticsViewMode): void {
    if (this.viewMode === mode) {
      return;
    }

    this.viewMode = mode;
    void this.updateViewModeContext();
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
          element.label,
          vscode.TreeItemCollapsibleState.Expanded,
        );
        item.id = `category:${element.id}`;
        item.description = `${element.diagnosticsCount} in ${element.fileCount} file${element.fileCount === 1 ? '' : 's'}`;
        item.iconPath = iconForCategory(element.id, element.maxSeverity);
        item.tooltip = `${element.label}\n${element.diagnosticsCount} diagnostic${element.diagnosticsCount === 1 ? '' : 's'} across ${element.problemTypeCount} problem type${element.problemTypeCount === 1 ? '' : 's'}`;
        return item;
      }
      case 'problemType': {
        const item = new vscode.TreeItem(
          element.label,
          vscode.TreeItemCollapsibleState.Collapsed,
        );
        item.id = element.id;
        item.description = `${element.diagnosticsCount} in ${element.fileCount} file${element.fileCount === 1 ? '' : 's'}`;
        item.iconPath = iconForSeverity(element.maxSeverity);
        item.tooltip = `${element.label}\n${element.diagnosticsCount} diagnostic${element.diagnosticsCount === 1 ? '' : 's'} in ${element.fileCount} file${element.fileCount === 1 ? '' : 's'}`;
        return item;
      }
      case 'file': {
        const item = new vscode.TreeItem(
          element.uri,
          vscode.TreeItemCollapsibleState.Collapsed,
        );
        item.id = element.id;
        item.resourceUri = element.uri;
        item.description = `${element.diagnosticsCount}`;
        item.iconPath = iconForSeverity(element.maxSeverity);
        item.tooltip = `${element.relativePath}\n${element.diagnosticsCount} diagnostic${element.diagnosticsCount === 1 ? '' : 's'}`;
        return item;
      }
      case 'folder': {
        const item = new vscode.TreeItem(
          element.label,
          vscode.TreeItemCollapsibleState.Collapsed,
        );
        item.id = element.id;
        item.description = `${element.diagnosticsCount} in ${element.fileCount} file${element.fileCount === 1 ? '' : 's'}`;
        item.iconPath = new vscode.ThemeIcon('folder');
        item.tooltip = `${element.relativePath}\n${element.diagnosticsCount} diagnostic${element.diagnosticsCount === 1 ? '' : 's'} in ${element.fileCount} file${element.fileCount === 1 ? '' : 's'}`;
        return item;
      }
      case 'diagnostic': {
        const item = new vscode.TreeItem(
          element.label,
          vscode.TreeItemCollapsibleState.None,
        );
        item.id = element.id;
        item.description = element.description;
        item.iconPath = iconForSeverity(element.diagnostic.severity ?? vscode.DiagnosticSeverity.Warning);
        item.tooltip = new vscode.MarkdownString([
          `**${element.label}**`,
          '',
          `${element.description}`,
          '',
          `${vscode.workspace.asRelativePath(element.uri, false)}`,
        ].join('\n'));
        item.command = {
          command: 'phpstrom.problems.openDiagnostic',
          title: 'Open Diagnostic',
          arguments: [element],
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
        item.description = element.description;
        item.iconPath = iconForSeverity(element.maxSeverity);
        item.tooltip = new vscode.MarkdownString([
          `**${element.relativePath}**`,
          '',
          `${element.diagnosticsCount} diagnostic${element.diagnosticsCount === 1 ? '' : 's'}`,
          '',
          `${element.problemTypeCount} problem type${element.problemTypeCount === 1 ? '' : 's'}`,
        ].join('\n'));
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

    if (element.kind === 'folder') {
      return element.children;
    }

    if (element.kind === 'file') {
      return element.diagnostics;
    }

    return [];
  }

  private updateViewModeContext(): Thenable<void> {
    return vscode.commands.executeCommand('setContext', 'phpstromProblems.viewMode', this.viewMode);
  }

  private updateViewPresentation(): void {
    if (!this.view) {
      return;
    }

    if (this.workspaceScanInProgress) {
      this.view.message = 'Scanning project diagnostics…';
      this.view.badge = undefined;
      return;
    }

    const stats = this.getStats();
    if (stats.totalDiagnostics === 0) {
      this.view.message = this.lastScanSummary?.capped
        ? `Stopped after ${this.lastScanSummary.totalDiagnostics.toLocaleString()} diagnostics.`
        : 'No PHP Strom diagnostics in the current workspace.';
      this.view.badge = undefined;
      return;
    }

    const suffix = this.lastScanSummary?.capped
      ? ` Scan stopped at ${this.lastScanSummary.totalDiagnostics.toLocaleString()} diagnostics.`
      : '';
    this.view.message = `${stats.totalDiagnostics} diagnostics in ${stats.totalFiles} file${stats.totalFiles === 1 ? '' : 's'} across ${stats.totalProblemTypes} problem type${stats.totalProblemTypes === 1 ? '' : 's'}.${suffix}`;
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
        if (child.kind === 'listFile') {
          fileUris.add(child.uri.toString());
          continue;
        }

        for (const file of collectFiles(child.children)) {
          fileUris.add(file.uri.toString());
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
    if (this.viewMode === 'list') {
      return this.buildListCategoryNodes();
    }

    const categories = new Map<ProblemCategoryKey, {
      label: string;
      problemTypes: Map<string, { label: string; files: Map<string, ProblemFileNode> }>;
    }>();

    for (const [uriString, diagnostics] of this.diagnosticsByUri.entries()) {
      const uri = vscode.Uri.parse(uriString);
      const relativePath = vscode.workspace.asRelativePath(uri, false);

      for (const diagnostic of diagnostics) {
        const categoryInfo = getDiagnosticCategory(diagnostic);
        let category = categories.get(categoryInfo.key);
        if (!category) {
          category = {
            label: categoryInfo.label,
            problemTypes: new Map<string, { label: string; files: Map<string, ProblemFileNode> }>(),
          };
          categories.set(categoryInfo.key, category);
        }

        const groupKey = getDiagnosticGroupKey(diagnostic);
        const scopedGroupKey = `${categoryInfo.key}:${groupKey}`;
        let group = category.problemTypes.get(groupKey);
        if (!group) {
          group = {
            label: getDiagnosticGroupLabel(diagnostic),
            files: new Map<string, ProblemFileNode>(),
          };
          category.problemTypes.set(groupKey, group);
        }

        let file = group.files.get(uriString);
        if (!file) {
          file = {
            kind: 'file',
            id: `file:${scopedGroupKey}:${uriString}`,
            uri,
            relativePath,
            diagnosticsCount: 0,
            maxSeverity: vscode.DiagnosticSeverity.Hint,
            diagnostics: [],
          };
          group.files.set(uriString, file);
        }

        const location = formatDiagnosticLocation(diagnostic.range.start);
        file.diagnostics.push({
          kind: 'diagnostic',
          id: `diagnostic:${scopedGroupKey}:${uriString}:${diagnostic.range.start.line}:${diagnostic.range.start.character}:${diagnostic.message}`,
          uri,
          diagnostic,
          label: diagnostic.message,
          description: location,
        });
        file.diagnosticsCount += 1;
        file.maxSeverity = maxSeverity(file.maxSeverity, diagnostic.severity);
      }
    }

    return [...categories.entries()]
      .map(([categoryKey, category]): ProblemCategoryNode => {
        const problemTypes = buildProblemTypeNodes(categoryKey, category.problemTypes);
        const fileUris = new Set<string>();
        const diagnosticsCount = problemTypes.reduce((total, problemType) => total + problemType.diagnosticsCount, 0);
        const maxCategorySeverity = problemTypes.reduce(
          (severity, problemType) => maxSeverity(severity, problemType.maxSeverity),
          vscode.DiagnosticSeverity.Hint,
        );

        for (const problemType of problemTypes) {
          for (const file of collectFiles(problemType.children)) {
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

  private buildListCategoryNodes(): ProblemCategoryNode[] {
    const categories = new Map<ProblemCategoryKey, {
      label: string;
      files: Map<string, {
        uri: vscode.Uri;
        relativePath: string;
        diagnosticsCount: number;
        maxSeverity: vscode.DiagnosticSeverity;
        firstDiagnostic: vscode.Diagnostic;
        problemTypes: Set<string>;
      }>;
    }>();

    for (const [uriString, diagnostics] of this.diagnosticsByUri.entries()) {
      const uri = vscode.Uri.parse(uriString);
      const relativePath = vscode.workspace.asRelativePath(uri, false);

      for (const diagnostic of diagnostics) {
        const categoryInfo = getDiagnosticCategory(diagnostic);
        let category = categories.get(categoryInfo.key);
        if (!category) {
          category = { label: categoryInfo.label, files: new Map() };
          categories.set(categoryInfo.key, category);
        }

        let file = category.files.get(uriString);
        if (!file) {
          file = {
            uri,
            relativePath,
            diagnosticsCount: 0,
            maxSeverity: vscode.DiagnosticSeverity.Hint,
            firstDiagnostic: diagnostic,
            problemTypes: new Set(),
          };
          category.files.set(uriString, file);
        }

        file.diagnosticsCount += 1;
        file.maxSeverity = maxSeverity(file.maxSeverity, diagnostic.severity);
        file.problemTypes.add(getDiagnosticGroupKey(diagnostic));
        if (compareDiagnosticPositions(diagnostic, file.firstDiagnostic) < 0) {
          file.firstDiagnostic = diagnostic;
        }
      }
    }

    return [...categories.entries()]
      .map(([categoryKey, category]): ProblemCategoryNode => {
        const files = [...category.files.entries()]
          .map(([uriString, file]): ProblemListFileNode => ({
            kind: 'listFile',
            id: `listFile:${categoryKey}:${uriString}`,
            category: categoryKey,
            uri: file.uri,
            relativePath: file.relativePath,
            diagnosticsCount: file.diagnosticsCount,
            maxSeverity: file.maxSeverity,
            firstDiagnostic: file.firstDiagnostic,
            problemTypeCount: file.problemTypes.size,
            label: file.relativePath,
            description: `${file.diagnosticsCount}`,
          }))
          .sort(compareListFiles);
        const problemTypes = new Set<string>();
        const maxCategorySeverity = files.reduce(
          (severity, file) => maxSeverity(severity, file.maxSeverity),
          vscode.DiagnosticSeverity.Hint,
        );

        for (const file of category.files.values()) {
          for (const problemType of file.problemTypes) {
            problemTypes.add(problemType);
          }
        }

        return {
          kind: 'category',
          id: categoryKey,
          label: category.label,
          diagnosticsCount: files.reduce((total, file) => total + file.diagnosticsCount, 0),
          fileCount: files.length,
          problemTypeCount: problemTypes.size,
          maxSeverity: maxCategorySeverity,
          children: files,
        };
      })
      .sort((left, right) => categorySortOrder(left.id) - categorySortOrder(right.id));
  }
}

function buildProblemTypeNodes(
  categoryKey: ProblemCategoryKey,
  groups: Map<string, { label: string; files: Map<string, ProblemFileNode> }>,
): ProblemTypeNode[] {
  return [...groups.entries()]
    .map(([groupKey, group]): ProblemTypeNode => {
      const scopedGroupKey = `${categoryKey}:${groupKey}`;
      const files = [...group.files.values()]
        .map((file) => ({
          ...file,
          diagnostics: file.diagnostics.sort(compareDiagnostics),
        }))
        .sort((left, right) => left.relativePath.localeCompare(right.relativePath));
      const children = buildFolderTree(scopedGroupKey, files);

      const diagnosticsCount = files.reduce((total, file) => total + file.diagnosticsCount, 0);
      const maxGroupSeverity = files.reduce(
        (severity, file) => maxSeverity(severity, file.maxSeverity),
        vscode.DiagnosticSeverity.Hint,
      );

      return {
        kind: 'problemType',
        id: `problemType:${scopedGroupKey}`,
        label: group.label,
        diagnosticsCount,
        fileCount: files.length,
        maxSeverity: maxGroupSeverity,
        children,
      };
    })
    .sort((left, right) => {
      if (right.diagnosticsCount !== left.diagnosticsCount) {
        return right.diagnosticsCount - left.diagnosticsCount;
      }

      return left.label.localeCompare(right.label);
    });
}

export async function openDiagnosticNode(node: DiagnosticNavigationNode): Promise<void> {
  const document = await vscode.workspace.openTextDocument(node.uri);
  const diagnostic = node.kind === 'listFile' ? node.firstDiagnostic : node.diagnostic;
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

function compareDiagnostics(left: ProblemDiagnosticNode, right: ProblemDiagnosticNode): number {
  if (left.diagnostic.range.start.line !== right.diagnostic.range.start.line) {
    return left.diagnostic.range.start.line - right.diagnostic.range.start.line;
  }

  if (left.diagnostic.range.start.character !== right.diagnostic.range.start.character) {
    return left.diagnostic.range.start.character - right.diagnostic.range.start.character;
  }

  return left.label.localeCompare(right.label);
}

function buildFolderTree(groupKey: string, files: ProblemFileNode[]): ProblemChildNode[] {
  const root: FolderAccumulator = {
    id: `folder:${groupKey}:root`,
    label: '',
    relativePath: '',
    diagnosticsCount: 0,
    fileCount: 0,
    maxSeverity: vscode.DiagnosticSeverity.Hint,
    folders: new Map<string, FolderAccumulator>(),
    files: [],
  };

  for (const file of files) {
    const segments = splitRelativePath(file.relativePath);
    const directorySegments = segments.slice(0, -1);
    let current = root;

    for (const segment of directorySegments) {
      let next = current.folders.get(segment);
      if (!next) {
        const relativePath = current.relativePath ? `${current.relativePath}/${segment}` : segment;
        next = {
          id: `folder:${groupKey}:${relativePath}`,
          label: segment,
          relativePath,
          diagnosticsCount: 0,
          fileCount: 0,
          maxSeverity: vscode.DiagnosticSeverity.Hint,
          folders: new Map<string, FolderAccumulator>(),
          files: [],
        };
        current.folders.set(segment, next);
      }

      next.diagnosticsCount += file.diagnosticsCount;
      next.fileCount += 1;
      next.maxSeverity = maxSeverity(next.maxSeverity, file.maxSeverity);
      current = next;
    }

    current.files.push(file);
  }

  return materializeFolderChildren(root);
}

function materializeFolderChildren(folder: FolderAccumulator): ProblemChildNode[] {
  const folders = [...folder.folders.values()]
    .sort((left, right) => left.relativePath.localeCompare(right.relativePath))
    .map((child): ProblemFolderNode => ({
      kind: 'folder',
      id: child.id,
      label: child.label,
      relativePath: child.relativePath,
      diagnosticsCount: child.diagnosticsCount,
      fileCount: child.fileCount,
      maxSeverity: child.maxSeverity,
      children: materializeFolderChildren(child),
    }));

  const files = [...folder.files].sort((left, right) => left.relativePath.localeCompare(right.relativePath));
  return [...folders, ...files];
}

function collectFiles(children: ProblemChildNode[]): ProblemFileNode[] {
  const files: ProblemFileNode[] = [];

  for (const child of children) {
    if (child.kind === 'file') {
      files.push(child);
      continue;
    }

    files.push(...collectFiles(child.children));
  }

  return files;
}

function splitRelativePath(relativePath: string): string[] {
  return relativePath.split(/[\\/]+/).filter((segment) => segment.length > 0);
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

function formatDiagnosticLocation(position: vscode.Position): string {
  return `Line ${position.line + 1}, Col ${position.character + 1}`;
}

function iconForSeverity(severity: vscode.DiagnosticSeverity): vscode.ThemeIcon {
  switch (severity) {
    case vscode.DiagnosticSeverity.Error:
      return new vscode.ThemeIcon('error');
    case vscode.DiagnosticSeverity.Warning:
      return new vscode.ThemeIcon('warning');
    case vscode.DiagnosticSeverity.Information:
      return new vscode.ThemeIcon('info');
    case vscode.DiagnosticSeverity.Hint:
    default:
      return new vscode.ThemeIcon('lightbulb');
  }
}

function iconForCategory(category: ProblemCategoryKey, severity: vscode.DiagnosticSeverity): vscode.ThemeIcon {
  switch (category) {
    case 'style':
      return new vscode.ThemeIcon('symbol-color');
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
