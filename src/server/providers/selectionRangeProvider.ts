/**
 * SelectionRangeProvider — textDocument/selectionRange
 *
 * Syntax-tree driven expand/shrink selection (Shift+Alt+→ / Shift+Alt+←).
 *
 * Expansion sequence example (cursor on variable name):
 *   variable name → variable expression → assignment → statement → block → method body → class body → file
 */

import { SelectionRange, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class SelectionRangeProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, positions: Position[]): SelectionRange[] {
    // TODO:
    //   For each position:
    //     1. Find the deepest AST node covering the position.
    //     2. Build a chain of SelectionRange objects walking up to the root.

    return positions.map(() => ({
      range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } },
    }));
  }
}
