/**
 * ImplementationProvider — textDocument/implementation
 *
 * Finds all concrete implementations of:
 *   - An interface (all classes that implement it)
 *   - An abstract class (all concrete sub-classes)
 *   - An abstract or interface method (all overriding methods)
 */

import { Location, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class ImplementationProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Location[] {
    // TODO:
    //   1. Resolve the symbol at position (class / method).
    //   2. Search the type hierarchy tree for all sub-types.
    //   3. For method references: find all sub-type methods with the same name.
    //   4. Return Location[] for each implementation.

    void doc;
    void position;
    return [];
  }
}
