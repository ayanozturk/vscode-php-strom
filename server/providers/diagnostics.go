package providers

// diagnostics.go — implements DiagnosticsProvider using go-phpcs analysis and
// style rules from github.com/ayanozturk/go-php-parser (local: go-phpcs).

import (
	"go-phpcs/analyse"
	"go-phpcs/style"
	"strings"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

type workspaceSymbolResolver struct {
	idx *indexer.WorkspaceIndexer
}

func (r workspaceSymbolResolver) ClassExists(name string) bool {
	_, ok := r.resolveClassSymbol(name)
	return ok
}

func (r workspaceSymbolResolver) ResolveClass(name string) (analyse.ResolvedClass, bool) {
	sym, ok := r.resolveClassSymbol(name)
	if !ok {
		return analyse.ResolvedClass{}, false
	}
	return analyse.ResolvedClass{
		Name:       sym.FQN,
		Extends:    append([]string(nil), sym.Extends...),
		Implements: append([]string(nil), sym.Implements...),
	}, true
}

func (r workspaceSymbolResolver) ResolveMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedMethod{}, false
	}

	idx := r.idx.GetIndex()
	if sym := idx.GetByFQN(classSym.FQN + "::" + methodName); sym != nil && sym.Kind == indexer.KindMethod {
		return resolvedMethod(sym), true
	}

	for _, sym := range idx.Search(methodName) {
		if sym.Kind != indexer.KindMethod {
			continue
		}
		if !strings.HasPrefix(sym.FQN, classSym.FQN+"::") {
			continue
		}
		if strings.EqualFold(sym.Name, methodName) {
			return resolvedMethod(sym), true
		}
	}

	return analyse.ResolvedMethod{}, false
}

func (r workspaceSymbolResolver) ResolveProperty(className, propertyName string) (analyse.ResolvedProperty, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedProperty{}, false
	}

	idx := r.idx.GetIndex()
	if sym := idx.GetByFQN(classSym.FQN + "::$" + propertyName); sym != nil && sym.Kind == indexer.KindProperty {
		return analyse.ResolvedProperty{Name: sym.Name, Type: sym.Type}, true
	}

	for _, sym := range idx.Search(propertyName) {
		if sym.Kind != indexer.KindProperty {
			continue
		}
		if !strings.HasPrefix(sym.FQN, classSym.FQN+"::$") {
			continue
		}
		if strings.EqualFold(sym.Name, propertyName) {
			return analyse.ResolvedProperty{Name: sym.Name, Type: sym.Type}, true
		}
	}

	return analyse.ResolvedProperty{}, false
}

func (r workspaceSymbolResolver) resolveClassSymbol(name string) (*indexer.Symbol, bool) {
	if r.idx == nil {
		return nil, false
	}

	idx := r.idx.GetIndex()
	for _, candidate := range resolveWorkspaceTypeCandidates(name) {
		sym := idx.GetByFQN(candidate)
		if sym != nil && isClassLikeKind(sym.Kind) {
			return sym, true
		}
	}

	lookup := unqualifiedName(name)
	if lookup == "" {
		return nil, false
	}
	for _, sym := range prioritizeDefinitionMatches(idx.Search(lookup), lookup) {
		if !isClassLikeKind(sym.Kind) {
			continue
		}
		if strings.EqualFold(sym.Name, lookup) {
			return sym, true
		}
	}

	return nil, false
}

func resolveWorkspaceTypeCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}

	seen := make(map[string]struct{}, 2)
	var candidates []string
	appendCandidate := func(value string) {
		value = ensureLeadingSlash(value)
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	appendCandidate(name)
	appendCandidate(strings.TrimPrefix(name, `\`))
	return candidates
}

func resolvedMethod(sym *indexer.Symbol) analyse.ResolvedMethod {
	params := make([]analyse.ResolvedParam, 0, len(sym.Params))
	for _, param := range sym.Params {
		params = append(params, analyse.ResolvedParam{Name: param.Name, Type: param.Type})
	}
	return analyse.ResolvedMethod{
		Name:       sym.Name,
		ReturnType: sym.ReturnType,
		Params:     params,
	}
}

// DiagnosticsProvider produces LSP diagnostics by running go-phpcs analysis
// and style rules against the document text.
type DiagnosticsProvider struct {
	idx   *indexer.WorkspaceIndexer
	cfg   Config
	cache *semanticDocumentCache
}

func (p *DiagnosticsProvider) Analyse(uri, text string) []lsp.Diagnostic {
	filename := uriToFilename(uri)
	snapshot := p.cache.snapshot(uri, text)
	nodes := snapshot.nodes

	var diags []lsp.Diagnostic

	// Run analysis rules (assignment-in-condition, empty statements, etc.)
	analysisCtx := p.cache.analysisContext(p.idx)
	for _, issue := range analyse.FilterIssues(analyse.RunAnalysisRulesWithContext(filename, nodes, analysisCtx), p.cfg.DiagnosticsOverrides) {
		sev := lsp.DiagSeverityWarning
		diags = append(diags, lsp.Diagnostic{
			Range:    lineColToRange(issue.Line, issue.Column),
			Severity: &sev,
			Code:     issue.Code,
			Source:   "phpstrom",
			Message:  issue.Message,
		})
	}

	// Run style rules (PSR-1/PSR-12 etc.) — "all" runs every registered rule.
	for _, issue := range style.FilterIssues(style.RunSelectedRules(filename, []byte(text), nodes, []string{"all"}), p.cfg.DiagnosticsOverrides) {
		sev := lsp.DiagSeverityWarning
		if issue.Type == style.Error {
			sev = lsp.DiagSeverityError
		}
		diags = append(diags, lsp.Diagnostic{
			Range:    lineColToRange(issue.Line, issue.Column),
			Severity: &sev,
			Code:     issue.Code,
			Source:   "phpstrom",
			Message:  issue.Message,
		})
	}

	// Surface parse errors from go-phpcs as error diagnostics.
	for _, errMsg := range snapshot.errors {
		sev := lsp.DiagSeverityError
		diags = append(diags, lsp.Diagnostic{
			Range:    lineColToRange(0, 0),
			Severity: &sev,
			Source:   "phpstrom",
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
