/**
 * SignatureHelpProvider — textDocument/signatureHelp
 *
 * Provides parameter documentation for function and method calls.
 * Trigger characters: ( , :
 */

import {
  SignatureHelp,
  SignatureInformation,
  ParameterInformation,
  Position,
  SignatureHelpContext,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';

export class SignatureHelpProvider {
  constructor(private readonly indexer: WorkspaceIndexer) {}

  provide(
    doc: TextDocument,
    position: Position,
    context: SignatureHelpContext | undefined,
  ): SignatureHelp | null {
    // TODO:
    //   1. Walk back from position to find the enclosing function/method call.
    //   2. Resolve the callee's symbol from the index.
    //   3. Build SignatureInformation with ParameterInformation for each param.
    //   4. Determine the active parameter index by counting commas.
    //   5. Handle constructor calls (new ClassName(...)).
    //   6. Support multiple overloaded signatures.

    void doc;
    void position;
    void context;
    return null;
  }
}
