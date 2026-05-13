/**
 * FormattingProvider
 *
 * textDocument/formatting      — format whole document
 * textDocument/rangeFormatting — format a selection
 *
 * Targets PSR-12 / PER Coding Style by default.
 * Brace style is configurable via phpls.format.braceStyle.
 *
 * Supports mixed PHP/HTML/CSS/JS files.
 */

import { TextEdit, Range, FormattingOptions } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { ServerConfig } from '../config.js';

export class FormattingProvider {
  constructor(private readonly config: ServerConfig) {}

  format(doc: TextDocument, options: FormattingOptions): TextEdit[] {
    // TODO:
    //   1. Detect PHP, HTML, CSS, JS regions.
    //   2. Run the PHP formatter over PHP regions (PSR-12 / PER).
    //   3. Delegate HTML, CSS, JS regions to an embedded formatter.
    //   4. Merge edits and return as a single list of TextEdits.
    //
    //   Recommended library: js-beautify for HTML/CSS/JS.
    //   For PHP: implement a custom formatter or wrap php-cs-fixer via process.

    void doc;
    void options;
    return [];
  }

  formatRange(doc: TextDocument, range: Range, options: FormattingOptions): TextEdit[] {
    // TODO: Limit formatting to the provided range.
    void doc;
    void range;
    void options;
    return [];
  }
}
