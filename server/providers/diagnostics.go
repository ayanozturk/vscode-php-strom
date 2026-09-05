package providers

// diagnostics.go — implements DiagnosticsProvider using go-phpcs analysis and
// style rules from github.com/ayanozturk/go-php-parser (local: go-phpcs).

import (
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
	"github.com/ayanozturk/go-php-parser/sharedcache"
	"github.com/ayanozturk/go-php-parser/style"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

const parserDiagnosticCode = "Parser Errors"

var analysisSourceLocks [64]sync.Mutex

func runAnalysisRulesForSource(filename, text string, nodes []ast.Node, ctx *analyse.AnalysisContext) []analyse.AnalysisIssue {
	if isVendoredAnalysisPath(filename) {
		return nil
	}
	lock := &analysisSourceLocks[crc32.ChecksumIEEE([]byte(filename))%uint32(len(analysisSourceLocks))]
	lock.Lock()
	defer lock.Unlock()

	source := []byte(text)
	sharedcache.StoreCachedFileContent(filename, source)
	defer sharedcache.DeleteCachedFileContent(filename)
	defer sharedcache.DeleteCachedLines(source)

	return analyse.RunAnalysisRulesWithContext(filename, nodes, ctx)
}

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
	if method, ok := r.fallback.ResolveMethod(className, methodName); ok {
		return method, true
	}
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

func (r projectFallbackResolver) ResolveOwnMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	if r.project != nil {
		if method, ok := r.project.ResolveOwnMethod(className, methodName); ok {
			return method, true
		}
	}
	return r.fallback.ResolveOwnMethod(className, methodName)
}

func (r projectFallbackResolver) MethodsDeclaredBy(className string) []analyse.ResolvedMethod {
	if r.project != nil {
		if methods := r.project.MethodsDeclaredBy(className); len(methods) > 0 {
			return methods
		}
	}
	return r.fallback.MethodsDeclaredBy(className)
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

func (r projectFallbackResolver) ResolveConstant(className, constantName string) (analyse.ResolvedConstant, bool) {
	if r.project != nil {
		if constant, ok := r.project.ResolveConstant(className, constantName); ok {
			return constant, true
		}
	}
	return r.fallback.ResolveConstant(className, constantName)
}

func (r projectFallbackResolver) ResolveOwnConstant(className, constantName string) (analyse.ResolvedConstant, bool) {
	if r.project != nil {
		if constant, ok := r.project.ResolveOwnConstant(className, constantName); ok {
			return constant, true
		}
	}
	return r.fallback.ResolveOwnConstant(className, constantName)
}

func (r projectFallbackResolver) ResolveFunction(name string) (analyse.ResolvedFunction, bool) {
	if r.project != nil {
		if fn, ok := r.project.ResolveFunction(name); ok {
			return fn, true
		}
	}
	return r.fallback.ResolveFunction(name)
}

func (r projectFallbackResolver) DuplicateClasses(filename string) []analyse.DuplicateSymbol {
	if r.project == nil {
		return nil
	}
	return r.project.DuplicateClasses(filename)
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
	return r.resolveMethodWithTemplates(className, methodName, nil, make(map[string]struct{}))
}

func (r workspaceSymbolResolver) ResolveOwnMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	return r.resolveDirectMethod(className, methodName)
}

func (r workspaceSymbolResolver) MethodsDeclaredBy(className string) []analyse.ResolvedMethod {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return nil
	}
	prefix := strings.ToLower(classSym.FQN + "::")
	var methods []analyse.ResolvedMethod
	for _, sym := range r.idx.GetIndex().AllSymbols() {
		if sym.Kind == indexer.KindMethod && strings.HasPrefix(strings.ToLower(sym.FQN), prefix) {
			method := resolvedMethod(sym)
			if classSym.Kind == indexer.KindInterface {
				method.Abstract = true
			}
			methods = append(methods, method)
		}
	}
	sort.Slice(methods, func(i, j int) bool {
		return strings.ToLower(methods[i].Name) < strings.ToLower(methods[j].Name)
	})
	return methods
}

func (r workspaceSymbolResolver) resolveMethodWithTemplates(className, methodName string, bindings map[string]string, seen map[string]struct{}) (analyse.ResolvedMethod, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedMethod{}, false
	}
	key := strings.ToLower(strings.TrimPrefix(classSym.FQN, `\`))
	if _, exists := seen[key]; exists {
		return analyse.ResolvedMethod{}, false
	}
	seen[key] = struct{}{}
	defer delete(seen, key)
	if method, found := r.resolveDirectMethod(classSym.FQN, methodName); found {
		method.ReturnType = applyTemplateBindings(method.ReturnType, bindings)
		for i := range method.Params {
			method.Params[i].Type = applyTemplateBindings(method.Params[i].Type, bindings)
		}
		return method, true
	}
	parents := append(append([]string(nil), classSym.Extends...), classSym.Implements...)
	for _, parentName := range parents {
		parentSym, parentOK := r.resolveClassSymbol(parentName)
		if !parentOK {
			continue
		}
		parentBindings := map[string]string(nil)
		if relation, relationOK := genericParentRelation(classSym, parentName); relationOK {
			parentBindings = make(map[string]string, len(parentSym.Templates))
			for i, template := range parentSym.Templates {
				if i >= len(relation.TypeArguments) {
					break
				}
				parentBindings[template] = applyTemplateBindings(relation.TypeArguments[i], bindings)
			}
		}
		if method, found := r.resolveMethodWithTemplates(parentSym.FQN, methodName, parentBindings, seen); found {
			return method, true
		}
	}
	return analyse.ResolvedMethod{}, false
}

func genericParentRelation(classSym *indexer.Symbol, parentName string) (indexer.GenericParent, bool) {
	for _, relation := range classSym.GenericParents {
		if strings.EqualFold(strings.TrimPrefix(relation.FQN, `\`), strings.TrimPrefix(parentName, `\`)) {
			return relation, true
		}
	}
	return indexer.GenericParent{}, false
}

func applyTemplateBindings(raw string, bindings map[string]string) string {
	if raw == "" || len(bindings) == 0 {
		return raw
	}
	var out strings.Builder
	for start := 0; start < len(raw); {
		if !isGenericIdentifierByte(raw[start]) {
			out.WriteByte(raw[start])
			start++
			continue
		}
		end := start + 1
		for end < len(raw) && isGenericIdentifierByte(raw[end]) {
			end++
		}
		token := raw[start:end]
		if replacement, ok := bindings[token]; ok {
			out.WriteString(replacement)
		} else {
			out.WriteString(token)
		}
		start = end
	}
	return out.String()
}

func isGenericIdentifierByte(value byte) bool {
	return value == '\\' || value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= 0x80
}

func (r workspaceSymbolResolver) resolveDirectMethod(className, methodName string) (analyse.ResolvedMethod, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedMethod{}, false
	}

	idx := r.idx.GetIndex()
	if sym := idx.GetByFQN(classSym.FQN + "::" + methodName); sym != nil && sym.Kind == indexer.KindMethod {
		method := resolvedMethod(sym)
		if classSym.Kind == indexer.KindInterface {
			method.Abstract = true
		}
		return method, true
	}

	for _, sym := range idx.GetByName(methodName) {
		if sym.Kind != indexer.KindMethod {
			continue
		}
		if !strings.HasPrefix(sym.FQN, classSym.FQN+"::") {
			continue
		}
		if strings.EqualFold(sym.Name, methodName) {
			method := resolvedMethod(sym)
			if classSym.Kind == indexer.KindInterface {
				method.Abstract = true
			}
			return method, true
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

func (r workspaceSymbolResolver) ResolveConstant(className, constantName string) (analyse.ResolvedConstant, bool) {
	for _, candidate := range r.classLineage(className) {
		if constant, ok := r.ResolveOwnConstant(candidate, constantName); ok {
			return constant, true
		}
	}
	return analyse.ResolvedConstant{}, false
}

func (r workspaceSymbolResolver) ResolveOwnConstant(className, constantName string) (analyse.ResolvedConstant, bool) {
	classSym, ok := r.resolveClassSymbol(className)
	if !ok {
		return analyse.ResolvedConstant{}, false
	}
	idx := r.idx.GetIndex()
	if sym := idx.GetByFQN(classSym.FQN + "::" + constantName); sym != nil && sym.Kind == indexer.KindConstant {
		return resolvedConstant(sym), true
	}
	for _, sym := range idx.GetByName(constantName) {
		if sym.Kind == indexer.KindConstant && strings.HasPrefix(sym.FQN, classSym.FQN+"::") && strings.EqualFold(sym.Name, constantName) {
			return resolvedConstant(sym), true
		}
	}
	return analyse.ResolvedConstant{}, false
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

func (r workspaceSymbolResolver) DuplicateClasses(string) []analyse.DuplicateSymbol {
	return nil
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

func resolvedConstant(sym *indexer.Symbol) analyse.ResolvedConstant {
	return analyse.ResolvedConstant{
		Name:           sym.Name,
		DeclaringClass: strings.TrimSuffix(sym.FQN, "::"+sym.Name),
		Type:           sym.Type,
		Visibility:     sym.Visibility,
		Final:          sym.IsFinal,
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

// Forget releases parsed and semantic state retained for a document that is no
// longer open. The cache is shared with hover and definition providers.
func (p *DiagnosticsProvider) Forget(uri string) {
	if p != nil {
		p.cache.forget(uri)
	}
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
	return p.cfg.DiagnosticsExclusions.Filter(filename, p.analyseParsed(uri, filename, text, snapshot.nodes, snapshot.errors))
}

// AnalyseTransient analyses a closed/background document without retaining its
// AST or semantic snapshot in the interactive document cache. Workspace scans
// may cover thousands of files, while the cache is intended for repeated
// diagnostics and semantic queries against open documents.
func (p *DiagnosticsProvider) AnalyseTransient(uri, text string) []lsp.Diagnostic {
	filename := uriToFilename(uri)
	if p.cfg.DiagnosticsExclusions.IgnoresAll(filename) {
		return []lsp.Diagnostic{}
	}
	snapshot := parseSemanticSnapshot(text)
	return p.cfg.DiagnosticsExclusions.Filter(filename, p.analyseParsed("", filename, text, snapshot.nodes, snapshot.errors))
}

func (p *DiagnosticsProvider) AnalyseParsed(uri, text string, nodes []ast.Node, parseErrors []string) []lsp.Diagnostic {
	filename := uriToFilename(uri)
	if p.cfg.DiagnosticsExclusions.IgnoresAll(filename) {
		return []lsp.Diagnostic{}
	}
	return p.cfg.DiagnosticsExclusions.Filter(filename, p.analyseParsed(uri, filename, text, nodes, parseErrors))
}

func (p *DiagnosticsProvider) analyseParsed(cacheKey, filename, text string, nodes []ast.Node, parseErrors []string) []lsp.Diagnostic {
	var diags []lsp.Diagnostic
	suppressions := collectInlineDiagnosticSuppressions(text)
	positions := newSourcePositionMapper(text)

	// Run analysis rules (assignment-in-condition, empty statements, etc.)
	analysisCtx := p.cache.analysisContextForFile(p.idx, cacheKey, filename, text, nodes)
	analysisCtx.PHPVersion = p.cfg.PHPVersion
	analysisCtx.AnalysisLevel = p.cfg.AnalysisLevel
	analysisCtx.DisabledIssueCodes = p.cfg.disabledAnalysisIssueCodes()
	for _, issue := range analyse.FilterIssues(runAnalysisRulesForSource(filename, text, nodes, analysisCtx), p.cfg.DiagnosticsOverrides) {
		sev := lsp.DiagSeverityWarning
		diags = append(diags, lsp.Diagnostic{
			Range:    positions.spanRange(issue.Line, issue.Column, issue.EndLine, issue.EndColumn),
			Severity: &sev,
			Code:     issue.Code,
			Source:   "phpstrom",
			Message:  issue.Message,
		})
	}

	if !p.cfg.DisabledAnalysis.Style {
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
	}

	if !p.cfg.DisabledAnalysis.SyntaxErrors {
		for _, errMsg := range parseErrors {
			sev := lsp.DiagSeverityError
			diags = append(diags, lsp.Diagnostic{
				Range:    parseErrorRange(positions, errMsg),
				Severity: &sev,
				Code:     parserDiagnosticCode,
				Source:   "phpstrom",
				Message:  errMsg,
			})
		}
	}

	return suppressions.filter(diags)
}

func (c Config) disabledAnalysisIssueCodes() map[string]bool {
	d := c.DisabledAnalysis
	disabled := make(map[string]bool)
	add := func(off bool, codes ...string) {
		if !off {
			return
		}
		for _, code := range codes {
			disabled[code] = true
		}
	}
	add(d.UndefinedSymbols, "Level0.Symbols", "Level2.MethodExistence", "Level7.MethodUnion")
	add(d.UndefinedVariables, "Level1.Variables")
	add(d.ClassModel, "Level0.ClassModel")
	add(d.InvalidCalls, "Level0.Invocation")
	add(d.Language, "Level0.Language")
	add(d.TypeErrors,
		"A.RETURN.TYPE", "A.RETURN.VOID", "A.RETURN.NEVER", "A.VOID.PURE",
		"A.PROP.TYPE", "A.ARG.TYPE", "A.ARG.COUNT",
		"Level0.PropertyCallableType",
		"A.ASSIGN.OP.INVALID", "A.BINARY.OP.INVALID",
		"Level2.PHPDocClass", "Level2.PHPDocParamName", "Level2.PHPDocParamType",
		"Level2.PHPDocPropertyType", "Level2.PHPDocReturnType",
		"Level2.PHPDocGenericLessTypes", "Level2.PHPDocGenericMoreTypes", "Level2.PHPDocNotGeneric", "Level2.PHPDocGenericNotSubtype",
		"Level6.MissingGenericType", "Level6.MissingIterableValueType",
		"Level6.MissingParameterType", "Level6.MissingReturnType", "Level6.MissingPropertyType",
		"Level2.MethodNonObject", "Level8.MethodNonObject",
	)
	add(d.MethodVisibility, "Level2.MethodVisibility")
	add(d.ThrowTypes, "Level3.ThrowType")
	add(d.Deprecated, "A.DEPRECATED.CALL")
	add(d.UnreachableCode, "Generic.CodeAnalysis.UnreachableCode")
	add(d.EmptyStatements, "Generic.CodeAnalysis.EmptyStatement")
	add(d.AssignmentInCondition, "Generic.CodeAnalysis.AssignmentInCondition")
	add(d.SideEffects, "PSR1.Files.SideEffects")
	if len(disabled) == 0 {
		return nil
	}
	return disabled
}

func parseErrorRange(positions sourcePositionMapper, message string) lsp.Range {
	lineText, remainder, ok := strings.Cut(strings.TrimPrefix(message, "line "), ":")
	if !ok || !strings.HasPrefix(message, "line ") {
		return positions.pointRange(0, 0)
	}
	columnText, _, ok := strings.Cut(remainder, ":")
	if !ok {
		return positions.pointRange(0, 0)
	}
	line, lineErr := strconv.Atoi(lineText)
	column, columnErr := strconv.Atoi(columnText)
	if lineErr != nil || columnErr != nil {
		return positions.pointRange(0, 0)
	}
	return positions.pointRange(line, column)
}

// lineColToRange retains the legacy point contract for style diagnostics whose
// coordinate sources are not yet uniformly structured parser rune positions.
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

func isVendoredAnalysisPath(path string) bool {
	if path == "" {
		return false
	}
	normalized := strings.ReplaceAll(path, "\\", "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "vendor" {
			return true
		}
	}
	return false
}

// uriToFilename strips the file:// scheme for display in issue messages.
func uriToFilename(uri string) string {
	const prefix = "file://"
	if len(uri) > len(prefix) {
		return uri[len(prefix):]
	}
	return uri
}
