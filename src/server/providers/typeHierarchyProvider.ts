/**
 * TypeHierarchyProvider
 *
 * textDocument/typeHierarchyPrepare
 * typeHierarchy/supertypes
 * typeHierarchy/subtypes
 *
 * Builds a navigable type hierarchy (class tree) for classes, interfaces,
 * traits, and enums. Used to understand inheritance chains at a glance.
 */

import {
  TypeHierarchyItem,
  SymbolKind,
  Position,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class TypeHierarchyProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  prepare(doc: TextDocument, position: Position): TypeHierarchyItem[] | null {
    // TODO:
    //   Resolve the class-like symbol at position and return a
    //   TypeHierarchyItem for it.

    void doc;
    void position;
    return null;
  }

  supertypes(item: TypeHierarchyItem): TypeHierarchyItem[] | null {
    // TODO:
    //   Look up the symbol for item.data (FQN).
    //   Return TypeHierarchyItem[] for each parent class and implemented interface.

    void item;
    return null;
  }

  subtypes(item: TypeHierarchyItem): TypeHierarchyItem[] | null {
    // TODO:
    //   Walk the index for all classes that extend or implement item.data.

    void item;
    return null;
  }
}
