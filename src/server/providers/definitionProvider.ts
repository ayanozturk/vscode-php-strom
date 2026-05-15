/**
 * DefinitionProvider — textDocument/definition
 *
 * Navigates from a symbol reference to its declaration.
 * When multiple definitions exist (e.g. constructor + class) all are returned
 * and the client decides how to present them.
 */

import { Location, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { resolveSymbolsAtPosition } from './symbolResolver.js';

export class DefinitionProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Location[] {
    const symbols = resolveSymbolsAtPosition(this.indexer.symbolIndex, doc, position);
    return symbols.map((s) => ({
      uri: s.uri,
      range: s.range,
    }));
  }
}
