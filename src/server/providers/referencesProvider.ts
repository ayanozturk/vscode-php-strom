/**
 * ReferencesProvider — textDocument/references
 *
 * Finds all references to a symbol across the workspace.
 * Reference resolution is aware of type hierarchies — references to a
 * base-class method also include overrides and implementations.
 */

import { Location, Position, ReferenceContext } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class ReferencesProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(
    doc: TextDocument,
    position: Position,
    context: ReferenceContext,
  ): Location[] {
    // TODO:
    //   1. Resolve the symbol at position.
    //   2. Walk all indexed ASTs looking for references to this FQN.
    //   3. Include the declaration itself if context.includeDeclaration is true.
    //   4. For methods: include overrides and implementations in sub-types.
    //   5. For traits: include all classes that use the trait.

    void doc;
    void position;
    void context;
    return [];
  }
}
