/**
 * HoverProvider — textDocument/hover
 *
 * Returns type info, signature, and PHPDoc documentation for the symbol at
 * the cursor, with links to official PHP manual pages for built-ins.
 */

import { Hover, MarkupKind, Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { resolveSymbolsAtPosition } from './symbolResolver.js';

export class HoverProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(doc: TextDocument, position: Position): Hover | null {
    const symbols = resolveSymbolsAtPosition(this.indexer.symbolIndex, doc, position);
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
