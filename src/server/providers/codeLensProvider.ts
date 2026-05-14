/**
 * CodeLensProvider — textDocument/codeLens + codeLens/resolve
 *
 * Lenses rendered inline above declarations (disabled by default):
 *   - References  : "N references" → triggers Find All References
 *   - Implementations : "N implementations" (interfaces / abstract methods)
 *   - Overrides   : "N overrides" (virtual methods)
 *   - Parent      : "overrides ParentClass::method" → go to parent declaration
 *   - Usages      : "N usages" (traits)
 *
 * Each lens is individually toggle-able via phpstrom.codeLens.* settings.
 */

import { CodeLens, Range } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { ServerConfig } from '../config.js';

export class CodeLensProvider {
  constructor(
    private readonly indexer: WorkspaceIndexer,
    private readonly config: ServerConfig,
  ) {}

  provide(doc: TextDocument): CodeLens[] {
    const lenses: CodeLens[] = [];
    const cl = this.config.codeLens;

    // At least one lens type must be enabled
    if (!cl.references && !cl.implementations && !cl.overrides && !cl.parent && !cl.usages) {
      return lenses;
    }

    // TODO:
    //   Walk document symbols and emit CodeLens items.
    //   Store the symbol FQN in CodeLens.data for lazy resolution.

    void doc;
    return lenses;
  }

  resolve(lens: CodeLens): CodeLens {
    // TODO: Compute the reference / implementation counts and set lens.command.
    return lens;
  }
}
