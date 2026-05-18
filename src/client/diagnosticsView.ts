import * as vscode from 'vscode';

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

type ProblemChildNode = ProblemFolderNode | ProblemFileNode;

type DiagnosticsViewNode = ProblemTypeNode | ProblemFolderNode | ProblemFileNode | ProblemDiagnosticNode;

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

  readonly onDidChangeTreeData = this.treeDataEmitter.event;

  setView(view: vscode.TreeView<DiagnosticsViewNode>): void {
    this.view = view;
    this.updateViewPresentation();
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
    }
  }

  getChildren(element?: DiagnosticsViewNode): DiagnosticsViewNode[] {
    const groups = this.buildProblemTypeNodes();

    if (!element) {
      return groups;
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
    const groups = this.buildProblemTypeNodes();
    const fileUris = new Set<string>();
    let totalDiagnostics = 0;

    for (const group of groups) {
      totalDiagnostics += group.diagnosticsCount;
      for (const file of collectFiles(group.children)) {
        fileUris.add(file.uri.toString());
      }
    }

    return {
      totalDiagnostics,
      totalFiles: fileUris.size,
      totalProblemTypes: groups.length,
    };
  }

  private buildProblemTypeNodes(): ProblemTypeNode[] {
    const groups = new Map<string, { label: string; files: Map<string, ProblemFileNode> }>();

    for (const [uriString, diagnostics] of this.diagnosticsByUri.entries()) {
      const uri = vscode.Uri.parse(uriString);
      const relativePath = vscode.workspace.asRelativePath(uri, false);

      for (const diagnostic of diagnostics) {
        const groupKey = getDiagnosticGroupKey(diagnostic);
        let group = groups.get(groupKey);
        if (!group) {
          group = {
            label: getDiagnosticGroupLabel(diagnostic),
            files: new Map<string, ProblemFileNode>(),
          };
          groups.set(groupKey, group);
        }

        let file = group.files.get(uriString);
        if (!file) {
          file = {
            kind: 'file',
            id: `file:${groupKey}:${uriString}`,
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
          id: `diagnostic:${groupKey}:${uriString}:${diagnostic.range.start.line}:${diagnostic.range.start.character}:${diagnostic.message}`,
          uri,
          diagnostic,
          label: diagnostic.message,
          description: location,
        });
        file.diagnosticsCount += 1;
        file.maxSeverity = maxSeverity(file.maxSeverity, diagnostic.severity);
      }
    }

    return [...groups.entries()]
      .map(([groupKey, group]): ProblemTypeNode => {
        const files = [...group.files.values()]
          .map((file) => ({
            ...file,
            diagnostics: file.diagnostics.sort(compareDiagnostics),
          }))
          .sort((left, right) => left.relativePath.localeCompare(right.relativePath));
        const children = buildFolderTree(groupKey, files);

        const diagnosticsCount = files.reduce((total, file) => total + file.diagnosticsCount, 0);
        const maxGroupSeverity = files.reduce(
          (severity, file) => maxSeverity(severity, file.maxSeverity),
          vscode.DiagnosticSeverity.Hint,
        );

        return {
          kind: 'problemType',
          id: `problemType:${groupKey}`,
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
}

export async function openDiagnosticNode(node: ProblemDiagnosticNode): Promise<void> {
  const document = await vscode.workspace.openTextDocument(node.uri);
  const editor = await vscode.window.showTextDocument(document, {
    preview: true,
    selection: new vscode.Range(node.diagnostic.range.start, node.diagnostic.range.end),
  });

  editor.revealRange(node.diagnostic.range, vscode.TextEditorRevealType.InCenter);
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
  const code = diagnostic.code;
  if (typeof code === 'string' || typeof code === 'number') {
    return String(code);
  }

  if (code && typeof code === 'object' && 'value' in code) {
    return String(code.value);
  }

  return diagnostic.message.split('\n', 1)[0];
}

function getDiagnosticGroupLabel(diagnostic: vscode.Diagnostic): string {
  const code = diagnostic.code;
  if (typeof code === 'string' || typeof code === 'number') {
    return String(code);
  }

  if (code && typeof code === 'object' && 'value' in code) {
    return String(code.value);
  }

  return diagnostic.message.split('\n', 1)[0] || 'General';
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

function maxSeverity(
  current: vscode.DiagnosticSeverity,
  next: vscode.DiagnosticSeverity | undefined,
): vscode.DiagnosticSeverity {
  if (next === undefined) {
    return current;
  }

  return next < current ? next : current;
}