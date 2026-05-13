/**
 * RenameProvider
 *
 * textDocument/rename         — rename symbol across workspace
 * textDocument/prepareRename  — validate and return rename range
 *
 * Handles:
 *   - Class / interface / trait / enum renames (+ file rename via PSR-4)
 *   - Method and property renames across the type hierarchy
 *   - Variable renames within scope
 *   - Namespace renames (updates all FQNs and use declarations)
 *   - Alias-only renames when the symbol is imported with `use … as`
 */

import {
  WorkspaceEdit,
  TextEdit,
  Range,
  Position,
  PrepareRenameResult,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class RenameProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  prepare(doc: TextDocument, position: Position): PrepareRenameResult | null {
    // TODO:
    //   1. Find the symbol at position.
    //   2. Return null (error) for symbols that cannot be renamed
    //      (e.g. built-in PHP functions, external vendor symbols).
    //   3. Return the range of the symbol name token.

    void doc;
    void position;
    return null;
  }

  provide(doc: TextDocument, position: Position, newName: string): WorkspaceEdit | null {
    // TODO:
    //   1. Resolve all references to the symbol (same as ReferencesProvider).
    //   2. Build TextEdit[] for each reference file.
    //   3. If renaming a class and PSR-4 mapping can be inferred, include
    //      a document rename instruction via WorkspaceEdit.documentChanges.
    //   4. Update use declarations to match the new name.

    void doc;
    void position;
    void newName;
    return null;
  }
}
