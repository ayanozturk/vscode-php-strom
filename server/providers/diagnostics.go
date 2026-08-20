package providers

// diagnostics.go — implements DiagnosticsProvider using go-phpcs analysis and
// style rules from github.com/ayanozturk/go-php-parser (local: go-phpcs).

import (
	"strconv"
	"strings"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
	"github.com/ayanozturk/go-php-parser/style"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

const parserDiagnosticCode = "Parser Errors"

type workspaceSymbolResolver struct {
	idx *indexer.WorkspaceIndexer
}

type projectFallbackResolver struct {
	project  *analyse.ProjectIndex
	fallback workspaceSymbolResolver
}

func (r projectFallbackResolver) ClassExists(name string) bool {
	_, ok := r.ResolveClass(name)
	return ok
}

func (r projectFallbackResolver) FunctionExists(name string) bool {
	if r.project != nil && r.project.FunctionExists(name) {
		return true
	}
	return r.fallback.FunctionExists(name)
}

func (r projectFallbackResolver) ConstantExists(name string) bool {
	if r.project != nil && r.project.ConstantExists(name) {
		return true
	}
	return r.fallback.ConstantExists(name)
}

func (r projectFallbackResolver) ResolveClass(name string) (analyse.ResolvedClass, bool) {
	if r.project != nil {
		if class, ok := r.project.ResolveClass(name); ok {
			return class, true
		}
	}
	return r.fallback.ResolveClass(name)
}

func (r projectFallbackResolver) ResolveMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	for _, candidate := range r.classLineage(className) {
		if r.project != nil {
			if method, ok := resolveDirectProjectMethod(r.project, candidate, methodName); ok {
				return method, true
			}
		}
		if method, ok := r.fallback.resolveDirectMethod(candidate, methodName); ok {
			return method, true
		}
	}
	return analyse.ResolvedMethod{}, false
}

func (r projectFallbackResolver) ResolveProperty(className, propertyName string) (analyse.ResolvedProperty, bool) {
	for _, candidate := range r.classLineage(className) {
		if r.project != nil {
			if property, ok := resolveDirectProjectProperty(r.project, candidate, propertyName); ok {
				return property, true
			}
		}
		if property, ok := r.fallback.resolveDirectProperty(candidate, propertyName); ok {
			return property, true
		}
	}
	return analyse.ResolvedProperty{}, false
}

func (r projectFallbackResolver) ResolveFunction(name string) (analyse.ResolvedFunction, bool) {
	if r.project != nil {
		if fn, ok := r.project.ResolveFunction(name); ok {
			return fn, true
		}
	}
	return r.fallback.ResolveFunction(name)
}

func (r projectFallbackResolver) classLineage(className string) []string {
	var out []string
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(name string) {
		key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), `\`))
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, name)
		class, ok := r.ResolveClass(name)
		if !ok {
			return
		}
		for _, parent := range class.Extends {
			walk(parent)
		}
		for _, iface := range class.Implements {
			walk(iface)
		}
		for _, trait := range class.Traits {
			walk(trait)
		}
	}
	walk(className)
	return out
}

func resolveDirectProjectMethod(project *analyse.ProjectIndex, className, methodName string) (analyse.ResolvedMethod, bool) {
	if project == nil {
		return analyse.ResolvedMethod{}, false
	}
	class, ok := project.ResolveClass(className)
	if ok {
		className = class.Name
	}
	methods := project.Methods[resolverIndexKey(className)]
	if methods == nil {
		return analyse.ResolvedMethod{}, false
	}
	method, ok := methods[strings.ToLower(methodName)]
	if !ok {
		return analyse.ResolvedMethod{}, false
	}
	method.DeclaringClass = className
	return method, true
}

func resolveDirectProjectProperty(project *analyse.ProjectIndex, className, propertyName string) (analyse.ResolvedProperty, bool) {
	if project == nil {
		return analyse.ResolvedProperty{}, false
	}
	class, ok := project.ResolveClass(className)
	if ok {
		className = class.Name
	}
	properties := project.Properties[resolverIndexKey(className)]
	if properties == nil {
		return analyse.ResolvedProperty{}, false
	}
	property, ok := properties[strings.ToLower(strings.TrimPrefix(propertyName, "$"))]
	if !ok {
		return analyse.ResolvedProperty{}, false
	}
	return property, true
}

func resolverIndexKey(name string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), `\`))
}

func (r workspaceSymbolResolver) ClassExists(name string) bool {
	_, ok := r.resolveClassSymbol(name)
	return ok
}

func (r workspaceSymbolResolver) FunctionExists(name string) bool {
	fn, ok := r.ResolveFunction(name)
	return ok && fn.Name != ""
}

func (r workspaceSymbolResolver) ConstantExists(name string) bool {
	if r.idx == nil {
		return false
	}
	idx := r.idx.GetIndex()
	for _, candidate := range resolveWorkspaceTypeCandidates(name) {
		if sym := idx.GetByFQN(candidate); sym != nil && sym.Kind == indexer.KindConstant {
			return true
		}
	}
	lookup := unqualifiedName(name)
	for _, sym := range idx.GetByName(lookup) {
		if sym.Kind == indexer.KindConstant && strings.EqualFold(sym.Name, lookup) {
			return true
		}
	}
	return false
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
		Kind:       resolvedClassKind(sym.Kind),
		Final:      sym.IsFinal,
		Abstract:   sym.IsAbstract,
		Readonly:   sym.IsReadonly,
	}, true
}

func (r workspaceSymbolResolver) ResolveMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	for _, candidate := range r.classLineage(className) {
		if method, ok := r.resolveDirectMethod(candidate, methodName); ok {
			return method, true
		}
	}
	return analyse.ResolvedMethod{}, false
}

func (r workspaceSymbolResolver) resolveDirectMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedMethod{}, false
	}

	idx := r.idx.GetIndex()
	if sym := idx.GetByFQN(classSym.FQN + "::" + methodName); sym != nil && sym.Kind == indexer.KindMethod {
		return resolvedMethod(sym), true
	}

	for _, sym := range idx.GetByName(methodName) {
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
	for _, candidate := range r.classLineage(className) {
		if property, ok := r.resolveDirectProperty(candidate, propertyName); ok {
			return property, true
		}
	}
	return analyse.ResolvedProperty{}, false
}

func (r workspaceSymbolResolver) resolveDirectProperty(className, propertyName string) (analyse.ResolvedProperty, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedProperty{}, false
	}

	idx := r.idx.GetIndex()
	if sym := idx.GetByFQN(classSym.FQN + "::$" + propertyName); sym != nil && sym.Kind == indexer.KindProperty {
		return resolvedProperty(sym), true
	}

	for _, sym := range idx.GetByName(propertyName) {
		if sym.Kind != indexer.KindProperty {
			continue
		}
		if !strings.HasPrefix(sym.FQN, classSym.FQN+"::$") {
			continue
		}
		if strings.EqualFold(sym.Name, propertyName) {
			return resolvedProperty(sym), true
		}
	}

	return analyse.ResolvedProperty{}, false
}

func (r workspaceSymbolResolver) classLineage(className string) []string {
	var out []string
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(name string) {
		key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), `\`))
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, name)
		class, ok := r.ResolveClass(name)
		if !ok {
			return
		}
		for _, parent := range class.Extends {
			walk(parent)
		}
		for _, iface := range class.Implements {
			walk(iface)
		}
	}
	walk(className)
	return out
}

func (r workspaceSymbolResolver) ResolveFunction(name string) (analyse.ResolvedFunction, bool) {
	if r.idx == nil {
		return analyse.ResolvedFunction{}, false
	}
	idx := r.idx.GetIndex()
	for _, candidate := range resolveWorkspaceTypeCandidates(name) {
		if sym := idx.GetByFQN(candidate); sym != nil && sym.Kind == indexer.KindFunction {
			return resolvedFunction(sym), true
		}
	}
	lookup := unqualifiedName(name)
	for _, sym := range idx.GetByName(lookup) {
		if sym.Kind == indexer.KindFunction && strings.EqualFold(sym.Name, lookup) {
			return resolvedFunction(sym), true
		}
	}
	return analyse.ResolvedFunction{}, false
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

	// Only fall back to unqualified name lookup when the input is not a
	// fully-qualified name. If the name contains a namespace separator the
	// caller already provided a specific FQN; guessing by short name would
	// match unrelated classes and produce false-positive diagnostics.
	if strings.Contains(name, `\`) {
		return nil, false
	}

	lookup := unqualifiedName(name)
	if lookup == "" {
		return nil, false
	}
	for _, sym := range prioritizeDefinitionMatches(idx.GetByName(lookup), lookup) {
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
		params = append(params, analyse.ResolvedParam{
			Name:       param.Name,
			Type:       param.Type,
			HasDefault: param.HasDefault,
			IsVariadic: param.IsVariadic,
		})
	}
	return analyse.ResolvedMethod{
		Name:           sym.Name,
		DeclaringClass: strings.TrimSuffix(sym.FQN, "::"+sym.Name),
		ReturnType:     sym.ReturnType,
		Params:         params,
		Visibility:     sym.Visibility,
		IsStatic:       sym.IsStatic,
		Abstract:       sym.IsAbstract,
	}
}

func resolvedFunction(sym *indexer.Symbol) analyse.ResolvedFunction {
	params := make([]analyse.ResolvedParam, 0, len(sym.Params))
	for _, param := range sym.Params {
		params = append(params, analyse.ResolvedParam{
			Name:       param.Name,
			Type:       param.Type,
			HasDefault: param.HasDefault,
			IsVariadic: param.IsVariadic,
		})
	}
	return analyse.ResolvedFunction{Name: sym.Name, ReturnType: sym.ReturnType, Params: params}
}

func resolvedProperty(sym *indexer.Symbol) analyse.ResolvedProperty {
	return analyse.ResolvedProperty{
		Name:       sym.Name,
		Type:       sym.Type,
		Visibility: sym.Visibility,
		IsStatic:   sym.IsStatic,
		Readonly:   sym.IsReadonly,
	}
}

func resolvedClassKind(kind indexer.SymbolKind) string {
	switch kind {
	case indexer.KindInterface:
		return "interface"
	case indexer.KindModule:
		return "trait"
	case indexer.KindEnum:
		return "enum"
	default:
		return "class"
	}
}

// DiagnosticsProvider produces LSP diagnostics by running go-phpcs analysis
// and style rules against the document text.
type DiagnosticsProvider struct {
	idx   *indexer.WorkspaceIndexer
	cfg   Config
	cache *semanticDocumentCache
}

func (p *DiagnosticsProvider) IgnoresAll(uri string) bool {
	return p.cfg.DiagnosticsExclusions.IgnoresAll(uriToFilename(uri))
}

func (p *DiagnosticsProvider) Analyse(uri, text string) []lsp.Diagnostic {
	filename := uriToFilename(uri)
	if p.cfg.DiagnosticsExclusions.IgnoresAll(filename) {
		return []lsp.Diagnostic{}
	}
	snapshot := p.cache.snapshot(uri, text)
	return p.cfg.DiagnosticsExclusions.Filter(filename, p.analyseParsed(filename, text, snapshot.nodes, snapshot.errors))
}

func (p *DiagnosticsProvider) AnalyseParsed(uri, text string, nodes []ast.Node, parseErrors []string) []lsp.Diagnostic {
	filename := uriToFilename(uri)
	if p.cfg.DiagnosticsExclusions.IgnoresAll(filename) {
		return []lsp.Diagnostic{}
	}
	return p.cfg.DiagnosticsExclusions.Filter(filename, p.analyseParsed(filename, text, nodes, parseErrors))
}

func (p *DiagnosticsProvider) analyseParsed(filename, text string, nodes []ast.Node, parseErrors []string) []lsp.Diagnostic {
	var diags []lsp.Diagnostic
	suppressions := collectInlineDiagnosticSuppressions(text)

	// Run analysis rules (assignment-in-condition, empty statements, etc.)
	analysisCtx := p.cache.analysisContextForFile(p.idx, filename, text, nodes)
	analysisCtx.PHPVersion = p.cfg.PHPVersion
	analysisCtx.DisabledIssueCodes = p.cfg.disabledAnalysisIssueCodes()
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
	for _, errMsg := range parseErrors {
		sev := lsp.DiagSeverityError
		diags = append(diags, lsp.Diagnostic{
			Range:    parseErrorRange(errMsg),
			Severity: &sev,
			Code:     parserDiagnosticCode,
			Source:   "phpstrom",
			Message:  errMsg,
		})
	}

	return suppressions.filter(diags)
}

func (c Config) disabledAnalysisIssueCodes() map[string]bool {
	disabled := make(map[string]bool)
	if c.DisableUndefinedSymbols {
		disabled["PHPStan.Level0.Symbols"] = true
	}
	if c.DisableUndefinedVariables {
		disabled["PHPStan.Level0.Variables"] = true
	}
	if c.DisableTypeErrors {
		for _, code := range []string{"A.RETURN.TYPE", "A.PROP.TYPE", "A.ARG.TYPE", "A.ARG.COUNT"} {
			disabled[code] = true
		}
	}
	if len(disabled) == 0 {
		return nil
	}
	return disabled
}

func parseErrorRange(message string) lsp.Range {
	lineText, remainder, ok := strings.Cut(strings.TrimPrefix(message, "line "), ":")
	if !ok || !strings.HasPrefix(message, "line ") {
		return lineColToRange(0, 0)
	}
	columnText, _, ok := strings.Cut(remainder, ":")
	if !ok {
		return lineColToRange(0, 0)
	}
	line, lineErr := strconv.Atoi(lineText)
	column, columnErr := strconv.Atoi(columnText)
	if lineErr != nil || columnErr != nil {
		return lineColToRange(0, 0)
	}
	return lineColToRange(line, column)
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

type inlineDiagnosticSuppressions struct {
	ignoreLines map[uint32]struct{}
}

func collectInlineDiagnosticSuppressions(text string) inlineDiagnosticSuppressions {
	suppressions := inlineDiagnosticSuppressions{ignoreLines: make(map[uint32]struct{})}
	for idx, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		lineNo := uint32(idx)
		if strings.Contains(lower, "@phpstan-ignore-line") ||
			strings.Contains(lower, "@psalm-suppress") ||
			strings.Contains(lower, "@phpcs:ignore") {
			suppressions.ignoreLines[lineNo] = struct{}{}
		}
		if strings.Contains(lower, "@phpstan-ignore-next-line") ||
			strings.Contains(lower, "@psalm-suppress-next-line") {
			suppressions.ignoreLines[lineNo+1] = struct{}{}
		}
	}
	return suppressions
}

func (s inlineDiagnosticSuppressions) filter(diags []lsp.Diagnostic) []lsp.Diagnostic {
	if len(s.ignoreLines) == 0 || len(diags) == 0 {
		return diags
	}
	filtered := diags[:0]
	for _, diag := range diags {
		if _, ok := s.ignoreLines[diag.Range.Start.Line]; ok {
			continue
		}
		filtered = append(filtered, diag)
	}
	return filtered
}

// uriToFilename strips the file:// scheme for display in issue messages.
func uriToFilename(uri string) string {
	const prefix = "file://"
	if len(uri) > len(prefix) {
		return uri[len(prefix):]
	}
	return uri
}
