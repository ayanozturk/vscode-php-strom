/**
 * SymbolProvider
 *
 * textDocument/documentSymbol — outline, breadcrumbs, Ctrl+Shift+O
 * workspace/symbol            — Ctrl+T / Go to Symbol in Workspace
 */

import {
  DocumentSymbol,
  SymbolInformation,
  SymbolKind,
  WorkspaceSymbol,
  Position,
  Range,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { IndexedSymbol } from '../indexer/symbolIndex.js';

const SYMBOL_KIND_MAP: Record<string, SymbolKind> = {
  class: SymbolKind.Class,
  interface: SymbolKind.Interface,
  trait: SymbolKind.Module,
  enum: SymbolKind.Enum,
  function: SymbolKind.Function,
  constant: SymbolKind.Constant,
  method: SymbolKind.Method,
  property: SymbolKind.Field,
};

export class SymbolProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provideDocumentSymbols(doc: TextDocument): DocumentSymbol[] {
    // TODO:
    //   Walk the cached AST for `doc.uri` and build a hierarchical
    //   DocumentSymbol tree (class → methods/properties, namespace → classes).

    const symbols = this.indexer.symbolIndex.getByUri(doc.uri);
    // Flattened as a fallback until hierarchical tree is implemented
    return symbols.map((s) => ({
      name: s.name,
      kind: SYMBOL_KIND_MAP[s.kind] ?? SymbolKind.Object,
      range: s.range,
      selectionRange: s.range,
      detail: s.containerFqn,
    }));
  }

  provideWorkspaceSymbols(query: string): WorkspaceSymbol[] {
    const results = this.indexer.symbolIndex.search(query, 100);
    return results.map((s) => ({
      name: s.name,
      kind: SYMBOL_KIND_MAP[s.kind] ?? SymbolKind.Object,
      location: { uri: s.uri, range: s.range },
      containerName: s.containerFqn,
    }));
  }
}
