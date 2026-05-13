/**
 * FoldingProvider — textDocument/foldingRange
 *
 * Syntax-tree driven folding (more reliable than indent-based).
 *
 * Folding ranges for:
 *   - Class / function / method bodies
 *   - Control structures (if, while, for, foreach, switch, try)
 *   - PHPDoc and block comments
 *   - use declaration groups
 *   - Heredoc / nowdoc
 *   - Custom #region / #endregion markers
 *   - Array / match expression bodies
 */

import { FoldingRange, FoldingRangeKind } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class FoldingProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument): FoldingRange[] {
    // TODO:
    //   1. Walk the cached AST for doc.uri.
    //   2. Emit a FoldingRange for every compound statement node.
    //   3. Detect #region / #endregion comment pairs.
    //   4. Detect contiguous use-declaration groups.
    //   5. Detect PHPDoc and multi-line block comments.

    void doc;
    return [];
  }
}
