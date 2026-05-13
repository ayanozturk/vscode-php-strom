/**
 * HighlightProvider — textDocument/documentHighlight
 *
 * Highlights all references to the symbol under the cursor in the current file.
 * Read and write contexts are distinguished (DocumentHighlightKind).
 */

import {
  DocumentHighlight,
  DocumentHighlightKind,
  Position,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class HighlightProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): DocumentHighlight[] {
    // TODO:
    //   1. Resolve the symbol at position within the current file's AST.
    //   2. Walk the AST for all references to the same symbol in this file.
    //   3. Classify each reference as Read or Write.
    //   4. Return DocumentHighlight[] with appropriate kinds.

    void doc;
    void position;
    return [];
  }
}
