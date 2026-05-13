/**
 * CodeActionProvider — textDocument/codeAction + codeAction/resolve
 *
 * Available actions:
 *
 *   quickfix:
 *     - Import symbol (add `use` declaration to resolve P1010/undefined class)
 *     - Create missing method stub
 *     - Add missing `return` statement
 *
 *   refactor:
 *     - Implement all abstract methods (generate method stubs)
 *     - Add PHPDoc block (auto-generated with inferred types)
 *     - Extract method
 *     - Convert to named argument
 *
 *   source.organizeImports:
 *     - Sort and deduplicate use declarations
 *     - Remove unused use declarations
 */

import {
  CodeAction,
  CodeActionContext,
  CodeActionKind,
  Range,
  TextEdit,
  WorkspaceEdit,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { ServerConfig } from '../config.js';

export class CodeActionProvider {
  constructor(
    private readonly indexer: WorkspaceIndexer,
    private readonly config: ServerConfig,
  ) {}

  provide(doc: TextDocument, range: Range, context: CodeActionContext): CodeAction[] {
    const actions: CodeAction[] = [];

    // Quick-fix: import undefined symbol
    for (const diag of context.diagnostics) {
      if (diag.code === 'P1010') {
        // TODO: Extract the symbol name from the diagnostic message,
        // find candidates in the index, and offer `use` declaration imports.
      }
    }

    // Source: organise imports
    actions.push({
      title: 'Organise use declarations',
      kind: CodeActionKind.SourceOrganizeImports,
      // resolved lazily
      data: { uri: doc.uri, action: 'organiseImports' },
    });

    // Refactor: Add PHPDoc
    actions.push({
      title: 'Add PHPDoc block',
      kind: CodeActionKind.Refactor,
      data: { uri: doc.uri, action: 'addPhpDoc', range },
    });

    void context;
    return actions;
  }

  resolve(action: CodeAction): CodeAction {
    // TODO: Materialise the WorkspaceEdit for each action kind.
    return action;
  }
}
