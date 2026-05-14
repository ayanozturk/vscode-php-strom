/**
 * DocumentLinksProvider — textDocument/documentLink
 *
 * Clickable links within PHP source files:
 *   - require / require_once / include / include_once paths
 *   - @see annotations referencing local files
 *
 * Respects phpstrom.environment.documentRoot for resolving document-root-relative
 * paths (e.g. $_SERVER['DOCUMENT_ROOT'] . '/path/to/file.php').
 */

import { DocumentLink } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';
import { URI } from 'vscode-uri';
import * as path from 'path';
import * as fs from 'fs';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { ServerConfig } from '../config.js';

export class DocumentLinksProvider {
  constructor(
    private readonly indexer: WorkspaceIndexer,
    private readonly config: ServerConfig,
  ) {}

  provide(doc: TextDocument): DocumentLink[] {
    const links: DocumentLink[] = [];

    // TODO:
    //   1. Walk include/require expression nodes in the AST.
    //   2. Resolve the target path relative to the document or documentRoot.
    //   3. Verify the file exists and return a DocumentLink with target URI.
    //   4. Parse @see PHPDoc tags for file references and emit links.

    void doc;
    return links;
  }

  private resolvePath(fromUri: string, includePath: string): string | undefined {
    const fromDir = path.dirname(URI.parse(fromUri).fsPath);
    const documentRoot = this.config.environment.documentRoot;

    const candidates = [
      path.resolve(fromDir, includePath),
      documentRoot ? path.resolve(documentRoot, includePath) : null,
    ].filter(Boolean) as string[];

    return candidates.find((p) => fs.existsSync(p));
  }
}
