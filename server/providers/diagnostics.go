package providers

// diagnostics.go — implements DiagnosticsProvider using go-phpcs analysis and
// style rules from github.com/ayanozturk/go-php-parser (local: go-phpcs).

import (
	"go-phpcs/analyse"
	goplexer "go-phpcs/lexer"
	goparser "go-phpcs/parser"
	"go-phpcs/style"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// DiagnosticsProvider produces LSP diagnostics by running go-phpcs analysis
// and style rules against the document text.
type DiagnosticsProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *DiagnosticsProvider) Analyse(uri, text string) []lsp.Diagnostic {
	filename := uriToFilename(uri)

	// Parse with go-phpcs parser to get AST nodes.
	l := goplexer.New(text)
	parser := goparser.New(l, false)
	nodes := parser.Parse()

	var diags []lsp.Diagnostic

	// Run analysis rules (assignment-in-condition, empty statements, etc.)
	for _, issue := range analyse.RunAnalysisRules(filename, nodes) {
		sev := lsp.DiagSeverityWarning
		diags = append(diags, lsp.Diagnostic{
			Range:    lineColToRange(issue.Line, issue.Column),
			Severity: &sev,
			Code:     issue.Code,
			Source:   "phpls",
			Message:  issue.Message,
		})
	}

	// Run style rules (PSR-1/PSR-12 etc.) — "all" runs every registered rule.
	for _, issue := range style.RunSelectedRules(filename, []byte(text), nodes, []string{"all"}) {
		sev := lsp.DiagSeverityWarning
		if issue.Type == style.Error {
			sev = lsp.DiagSeverityError
		}
		diags = append(diags, lsp.Diagnostic{
			Range:    lineColToRange(issue.Line, issue.Column),
			Severity: &sev,
			Code:     issue.Code,
			Source:   "phpls",
			Message:  issue.Message,
		})
	}

	// Surface parse errors from go-phpcs as error diagnostics.
	for _, errMsg := range parser.Errors() {
		sev := lsp.DiagSeverityError
		diags = append(diags, lsp.Diagnostic{
			Range:    lineColToRange(0, 0),
			Severity: &sev,
			Source:   "phpls",
			Message:  errMsg,
		})
	}

	return diags
}

// lineColToRange converts a 1-based line/column from go-phpcs to an LSP Range.
// The range covers only the start position; the editor highlights the word.
func lineColToRange(line, col int) lsp.Range {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	pos := lsp.Position{
		Line:      uint32(line - 1),
		Character: uint32(col - 1),
	}
	return lsp.Range{Start: pos, End: pos}
}

// uriToFilename strips the file:// scheme for display in issue messages.
func uriToFilename(uri string) string {
	const prefix = "file://"
	if len(uri) > len(prefix) {
		return uri[len(prefix):]
	}
	return uri
}
