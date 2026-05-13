/**
 * TypeDefinitionProvider — textDocument/typeDefinition
 *
 * Navigates to the class/interface definition of a typed variable or parameter,
 * rather than the variable declaration itself.
 */

import { Location, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class TypeDefinitionProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Location[] {
    // TODO:
    //   1. Resolve the expression / variable at position.
    //   2. Use TypeInferrer to determine the type(s).
    //   3. Look up each type in the symbol index.
    //   4. Return Location[] for each type definition.

    void doc;
    void position;
    return [];
  }
}
