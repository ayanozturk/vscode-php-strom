/**
 * CompletionProvider
 *
 * LSP textDocument/completion + completionItem/resolve
 *
 * Provides camelCase/underscore-aware completions for:
 *   - Variables in scope
 *   - Class / interface / trait / enum names
 *   - Methods and properties on typed expressions
 *   - Functions and constants (global + namespaced)
 *   - PHP keywords
 *   - PHPDoc tags inside doc-blocks
 *   - Automatic use-declaration insertion
 */

import {
  CompletionItem,
  CompletionItemKind,
  CompletionContext,
  InsertTextFormat,
  Position,
  TextEdit,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { ServerConfig } from '../config.js';

export class CompletionProvider {
  constructor(
    private readonly indexer: WorkspaceIndexer,
    private readonly config: ServerConfig,
  ) {}

  provide(
    doc: TextDocument,
    position: Position,
    _context: CompletionContext | undefined,
  ): CompletionItem[] {
    // TODO: Walk the AST to determine context (inside class body, argument
    // list, use declaration, PHPDoc, etc.) and return relevant completions.
    //
    // Implementation notes:
    //   1. Find the token at `position` in the cached AST.
    //   2. Determine the completion context (after `->`, `::`, `\`, `$`, etc.).
    //   3. For member completions: resolve the type of the LHS expression
    //      and enumerate properties/methods from the symbol index.
    //   4. For type completions: search symbolIndex.search(prefix) and
    //      produce use-declaration edits when insertUseDeclaration is enabled.
    //   5. For variable completions: walk the current scope for declared vars.
    //   6. Sort by relevance (camelCase prefix match score).

    const items: CompletionItem[] = [];

    // Emit keyword completions as a baseline
    for (const kw of PHP_KEYWORDS) {
      items.push({
        label: kw,
        kind: CompletionItemKind.Keyword,
        insertTextFormat: InsertTextFormat.PlainText,
      });
    }

    // Emit workspace symbol completions
    const partial = this.wordBefore(doc, position);
    if (partial.length > 0) {
      const symbols = this.indexer.symbolIndex.search(partial, this.config.completion.maxItems);
      for (const sym of symbols) {
        items.push(symbolToCompletion(sym));
      }
    }

    return items;
  }

  resolve(item: CompletionItem): CompletionItem {
    // TODO: Populate `documentation` and `detail` lazily from the index.
    return item;
  }

  private wordBefore(doc: TextDocument, pos: Position): string {
    const line = doc.getText({
      start: { line: pos.line, character: 0 },
      end: pos,
    });
    const match = /[\w\\]+$/.exec(line);
    return match ? match[0] : '';
  }
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

import { IndexedSymbol } from '../indexer/symbolIndex.js';

function symbolToCompletion(sym: IndexedSymbol): CompletionItem {
  const kindMap: Record<string, CompletionItemKind> = {
    class: CompletionItemKind.Class,
    interface: CompletionItemKind.Interface,
    trait: CompletionItemKind.Module,
    enum: CompletionItemKind.Enum,
    function: CompletionItemKind.Function,
    constant: CompletionItemKind.Constant,
    method: CompletionItemKind.Method,
    property: CompletionItemKind.Field,
  };

  return {
    label: sym.name,
    kind: kindMap[sym.kind] ?? CompletionItemKind.Text,
    detail: sym.fqn,
    documentation: sym.doc,
    insertTextFormat: InsertTextFormat.PlainText,
    data: sym.fqn, // used during resolve
  };
}

const PHP_KEYWORDS = [
  'abstract', 'and', 'array', 'as', 'break', 'callable', 'case', 'catch',
  'class', 'clone', 'const', 'continue', 'declare', 'default', 'do', 'echo',
  'else', 'elseif', 'empty', 'enddeclare', 'endfor', 'endforeach', 'endif',
  'endswitch', 'endwhile', 'enum', 'extends', 'final', 'finally', 'fn', 'for',
  'foreach', 'function', 'global', 'goto', 'if', 'implements', 'include',
  'include_once', 'instanceof', 'insteadof', 'interface', 'isset', 'list',
  'match', 'namespace', 'new', 'null', 'or', 'print', 'private', 'protected',
  'public', 'readonly', 'require', 'require_once', 'return', 'static',
  'switch', 'throw', 'trait', 'try', 'unset', 'use', 'var', 'while', 'xor',
  'yield', 'yield from',
  'true', 'false',
  'int', 'float', 'string', 'bool', 'void', 'never', 'mixed', 'object',
  'self', 'static', 'parent', 'iterable',
];
