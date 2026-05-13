/**
 * DefinitionProvider — textDocument/definition
 *
 * Navigates from a symbol reference to its declaration.
 * When multiple definitions exist (e.g. constructor + class) all are returned
 * and the client decides how to present them.
 */

import { Location, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';
import { URI } from 'vscode-uri';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class DefinitionProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Location[] {
    // TODO:
    //   1. Get the AST node at the cursor via indexer.getAst() + phpParser.nodeAt().
    //   2. Resolve the symbol name (handle `->`, `::`, `use` aliases, FQNs).
    //   3. Look up the symbol in indexer.symbolIndex.
    //   4. Return Location[] for each matching declaration.
    //   5. For `new ClassName`, also include the __construct method definition.

    const word = wordAt(doc, position);
    if (!word) return [];

    const symbols = this.indexer.symbolIndex.getByName(word);
    return symbols.map((s) => ({
      uri: s.uri,
      range: s.range,
    }));
  }
}

function wordAt(doc: TextDocument, pos: Position): string {
  const text = doc.getText();
  const offset = doc.offsetAt(pos);
  let start = offset;
  let end = offset;
  while (start > 0 && /\w/.test(text[start - 1])) start--;
  while (end < text.length && /\w/.test(text[end])) end++;
  return text.slice(start, end);
}
