/**
 * InlayHintsProvider — textDocument/inlayHint
 *
 * Inline annotations rendered in the editor (enabled by default):
 *   - Parameter names : argument labels for positional function/method calls
 *   - Parameter types : inferred types for un-typed closure parameters
 *   - Return types    : inferred return type for un-typed functions/methods
 *
 * Each hint type is individually toggle-able via phpstrom.inlayHints.* settings.
 */

import { InlayHint, InlayHintKind, Range } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { ServerConfig } from '../config.js';

export class InlayHintsProvider {
  constructor(
    private readonly indexer: WorkspaceIndexer,
    private readonly config: ServerConfig,
  ) {}

  provide(doc: TextDocument, range: Range): InlayHint[] {
    const hints: InlayHint[] = [];
    const ih = this.config.inlayHints;

    // TODO:
    //   1. Walk function/method call expressions in the range.
    //      For each argument: emit a parameter-name hint (if enabled).
    //   2. Walk closure declarations in the range.
    //      For each untyped param: infer type and emit hint (if enabled).
    //   3. Walk function/method declarations in the range.
    //      For untyped return: infer type and emit hint (if enabled).

    void doc;
    void range;
    void ih;
    return hints;
  }
}
