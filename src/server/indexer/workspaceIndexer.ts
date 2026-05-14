/**
 * WorkspaceIndexer
 *
 * Orchestrates workspace-wide PHP file discovery, parsing, and symbol indexing.
 * Handles incremental re-indexing when files are opened, changed, or deleted.
 */

import * as path from 'path';
import * as fs from 'fs';
import { Connection } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';
import { URI } from 'vscode-uri';
import glob from 'fast-glob';

import { ServerConfig } from '../config.js';
import { phpParser, PhpFileAst } from '../parser/phpParser.js';
import { SymbolIndex } from './symbolIndex.js';

export class WorkspaceIndexer {
  private readonly index: SymbolIndex = new SymbolIndex();
  /** Parsed AST cache, keyed by URI */
  private readonly astCache = new Map<string, PhpFileAst>();

  constructor(
    private readonly connection: Connection,
    private readonly config: ServerConfig,
  ) {}

  // ─── Public API ────────────────────────────────────────────────────────────

  get symbolIndex(): SymbolIndex {
    return this.index;
  }

  getAst(uri: string): PhpFileAst | undefined {
    return this.astCache.get(uri);
  }

  async indexWorkspace(): Promise<void> {
    const workspaceFolders = await this.connection.workspace.getWorkspaceFolders();
    if (!workspaceFolders) return;

    const roots = workspaceFolders.map((f) => URI.parse(f.uri).fsPath);

    const patterns = this.config.files.associations.map((a) => a.replace(/^\*\*\//, ''));
    const ignore = this.config.files.exclude;

    this.index.clear();
    this.astCache.clear();

    let indexed = 0;
    for (const root of roots) {
      const files = await glob(patterns, {
        cwd: root,
        ignore,
        absolute: true,
        onlyFiles: true,
      });

      for (const file of files) {
        try {
          const stat = fs.statSync(file);
          if (stat.size > this.config.files.maxSize) continue;
          const text = fs.readFileSync(file, 'utf-8');
          const uri = URI.file(file).toString();
          const ast = phpParser.parse(uri, text, 0);
          this.astCache.set(uri, ast);
          this.index.addFile(ast);
          indexed++;
        } catch {
          // skip unreadable files
        }
      }
    }

    this.connection.console.log(`[phpstrom] Indexed ${indexed} files (${this.index.size} symbols).`);
  }

  onDocumentOpen(doc: TextDocument): void {
    this.indexDocument(doc);
  }

  onDocumentChange(doc: TextDocument): void {
    this.indexDocument(doc);
  }

  onDocumentClose(uri: string): void {
    // Keep the index entry; it still represents the on-disk state.
    // A future enhancement could re-read the file from disk here.
    void uri;
  }

  // ─── Internals ─────────────────────────────────────────────────────────────

  private indexDocument(doc: TextDocument): void {
    const existing = this.astCache.get(doc.uri);
    let ast: PhpFileAst;
    if (existing) {
      ast = phpParser.parseIncremental(existing, doc.getText(), doc.version);
    } else {
      ast = phpParser.parse(doc.uri, doc.getText(), doc.version);
    }
    this.astCache.set(doc.uri, ast);
    this.index.addFile(ast);
  }
}
