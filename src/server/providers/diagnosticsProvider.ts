/**
 * DiagnosticsProvider — textDocument/publishDiagnostics
 *
 * Runs static analysis on open PHP documents and publishes diagnostics via
 * the LSP connection.  Analysis categories:
 *
 *   Free tier (always active):
 *     - Syntax errors (from parser)
 *     - Undefined classes / interfaces / traits / enums
 *     - Undefined functions
 *     - Undefined constants
 *     - Undefined variables (basic scope analysis)
 *     - Type mismatch diagnostics (configurable strictness)
 *     - Deprecated symbol usage
 *     - Missing return statements
 *     - Duplicate symbol declarations
 *
 *   Additional checks (enabled via settings):
 *     - strict_types enforcement
 *     - PHPDoc type mismatch
 *     - @disregard suppression
 */

import {
  Connection,
  Diagnostic,
  DiagnosticSeverity,
  Range,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from '../indexer/workspaceIndexer.js';
import { ServerConfig } from '../config.js';

export class DiagnosticsProvider {
  constructor(
    private readonly indexer: WorkspaceIndexer,
    private readonly config: ServerConfig,
    private readonly connection: Connection,
  ) {}

  validate(doc: TextDocument): void {
    if (!this.config.diagnostics.enable) {
      this.connection.sendDiagnostics({ uri: doc.uri, diagnostics: [] });
      return;
    }

    const diagnostics = this.analyse(doc);
    this.connection.sendDiagnostics({ uri: doc.uri, diagnostics });
  }

  private analyse(doc: TextDocument): Diagnostic[] {
    const ast = this.indexer.getAst(doc.uri);
    if (!ast) return [];

    const diagnostics: Diagnostic[] = [];

    // 1. Parser-reported syntax errors
    for (const err of ast.errors) {
      diagnostics.push({
        severity: DiagnosticSeverity.Error,
        range: lspRange(err.range),
        message: err.message,
        source: 'phpstrom',
        code: 'P0001',
      });
    }

    // TODO: Implement remaining diagnostic checks:
    //   2. Undefined symbol resolution (walk class/function/constant references)
    //   3. Type-checking via TypeInferrer
    //   4. Undefined variable detection (scope analysis)
    //   5. Deprecated symbol detection (check @deprecated PHPDoc tags)
    //   6. Missing return type analysis
    //   7. strict_types enforcement when config.diagnostics.strictTypes is set
    //   8. @disregard annotation suppression

    // Apply per-file exclusion rules from config.diagnostics.exclude
    return this.applyExclusions(doc.uri, diagnostics);
  }

  private applyExclusions(uri: string, diagnostics: Diagnostic[]): Diagnostic[] {
    const exclusions = this.config.diagnostics.exclude;
    if (Object.keys(exclusions).length === 0) return diagnostics;

    // TODO: Match URI against glob patterns and filter out excluded codes.
    return diagnostics;
  }
}

function lspRange(r: {
  start: { line: number; column: number };
  end: { line: number; column: number };
}): Range {
  return {
    start: { line: r.start.line, character: r.start.column },
    end: { line: r.end.line, character: r.end.column },
  };
}
