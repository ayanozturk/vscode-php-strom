/**
 * DeclarationProvider — textDocument/declaration
 *
 * Navigates to the *initial* declaration of a symbol in a type hierarchy.
 * For example, invoking on an overriding method navigates to the abstract or
 * interface declaration rather than the overriding method itself.
 */

import { Location, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class DeclarationProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Location[] {
    // TODO:
    //   1. Resolve the symbol at position.
    //   2. Walk up the type hierarchy to find the root declaration.
    //   3. Return the location of the root declaration.

    void doc;
    void position;
    return [];
  }
}
