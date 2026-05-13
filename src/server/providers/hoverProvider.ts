/**
 * HoverProvider — textDocument/hover
 *
 * Returns type info, signature, and PHPDoc documentation for the symbol at
 * the cursor, with links to official PHP manual pages for built-ins.
 */

import { Hover, MarkupKind, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class HoverProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Hover | null {
    // TODO:
    //   1. Resolve the symbol at position (same as definition).
    //   2. Build a markdown string: signature + PHPDoc + PHP manual link.
    //   3. For built-in functions: link to https://www.php.net/manual/en/function.{name}.php

    const word = wordAt(doc, position);
    if (!word) return null;

    const symbols = this.indexer.symbolIndex.getByName(word);
    if (symbols.length === 0) return null;

    const sym = symbols[0];
    const lines: string[] = [];

    lines.push(`\`\`\`php\n${sym.fqn}\n\`\`\``);
    if (sym.doc) lines.push(sym.doc);

    return {
      contents: {
        kind: MarkupKind.Markdown,
        value: lines.join('\n\n'),
      },
    };
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
