package providers

import (
	"strings"
	"unicode"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

const (
	completionSortMembers  = "0_"
	completionSortSymbols  = "1_"
	completionSortKeywords = "9_"
)

// CompletionProvider produces completion items for a given position.
type CompletionProvider struct {
	idx   *indexer.WorkspaceIndexer
	cfg   Config
	cache *semanticDocumentCache
}

func (p *CompletionProvider) Provide(uri, text string, pos lsp.Position, _ *lsp.CompletionContext) []lsp.CompletionItem {
	if p.idx != nil {
		p.idx.IndexDocument(uri, text)
	}

	word := wordAt(text, pos)
	if access, ok := memberAccessAt(text, pos); ok {
		return p.memberCompletions(uri, text, pos, access, word)
	}

	var items []lsp.CompletionItem
	if word != "" && p.idx != nil {
		for _, sym := range p.idx.GetIndex().Search(word) {
			item := symbolToCompletionItem(sym)
			item.SortText = completionSortSymbols + strings.ToLower(sym.Name)
			items = append(items, item)
			if p.atMaxItems(len(items)) {
				return items
			}
		}
	}
	items = append(items, phpKeywordCompletions(word)...)
	if p.atMaxItems(len(items)) {
		return items[:p.maxItems()]
	}
	return items
}

func (p *CompletionProvider) Resolve(item lsp.CompletionItem) lsp.CompletionItem {
	return item
}

func (p *CompletionProvider) memberCompletions(uri, text string, pos lsp.Position, access memberAccess, prefix string) []lsp.CompletionItem {
	className := p.resolveReceiverClass(uri, text, pos, access)
	if className == "" {
		return nil
	}
	fromThis := access.receiverIsThis()
	var items []lsp.CompletionItem
	for _, sym := range p.classMemberSymbols(className) {
		if !memberMatchesAccess(sym, access.static, fromThis) {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(sym.Name), strings.ToLower(prefix)) {
			continue
		}
		item := symbolToCompletionItem(sym)
		item.SortText = completionSortMembers + memberKindRank(sym.Kind) + strings.ToLower(sym.Name)
		items = append(items, item)
		if p.atMaxItems(len(items)) {
			break
		}
	}
	return items
}

func (p *CompletionProvider) resolveReceiverClass(uri, text string, pos lsp.Position, access memberAccess) string {
	parts := splitObjectChain(access.receiver)
	if len(parts) == 0 {
		return ""
	}
	className := p.resolveRootReceiver(uri, text, pos, parts[0])
	for _, part := range parts[1:] {
		className = p.propertyClassName(className, part)
		if className == "" {
			return ""
		}
	}
	return className
}

func (p *CompletionProvider) resolveRootReceiver(uri, text string, pos lsp.Position, root string) string {
	switch strings.ToLower(strings.TrimSpace(root)) {
	case "$this", "self", "static":
		return p.enclosingClassName(uri, text, pos)
	case "parent":
		return p.enclosingParentClassName(uri, text, pos)
	}
	if strings.HasPrefix(root, "$") {
		return classNameFromType(p.variableTypeAt(uri, text, pos, strings.TrimPrefix(root, "$")))
	}
	if root == "" {
		return ""
	}
	resolver := workspaceSymbolResolver{idx: p.idx}
	if sym, ok := resolver.resolveClassSymbol(root); ok {
		return sym.FQN
	}
	return ensureLeadingSlash(root)
}

func (p *CompletionProvider) enclosingClassName(uri, text string, pos lsp.Position) string {
	if p.idx != nil {
		if sym := classLikeSymbolBeforePosition(p.idx.GetIndex().GetByURI(uri), pos); sym != nil {
			return sym.FQN
		}
	}
	snapshot := p.documentSnapshot(uri, text)
	line, col := int(pos.Line)+1, int(pos.Character)+1
	if name := enclosingClassLikeName(snapshot.nodes, line, col); name != "" {
		return ensureLeadingSlash(name)
	}
	return ""
}

func (p *CompletionProvider) enclosingParentClassName(uri, text string, pos lsp.Position) string {
	className := p.enclosingClassName(uri, text, pos)
	if className == "" {
		return ""
	}
	resolver := workspaceSymbolResolver{idx: p.idx}
	class, ok := resolver.ResolveClass(className)
	if !ok || len(class.Extends) == 0 {
		return ""
	}
	return class.Extends[0]
}

func (p *CompletionProvider) variableTypeAt(uri, text string, pos lsp.Position, name string) string {
	if name == "" {
		return ""
	}
	snapshot := p.documentSnapshot(uri, text)
	line := int(pos.Line) + 1
	col := identifierColumnOnLine(text, pos, name)
	ctx := p.analysisContext(uri, text, snapshot)
	if target, ok := analyse.InferHoverTargetAtPosition(snapshot.nodes, line, col, name, ctx); ok && target.Type != "" {
		return target.Type
	}
	if flowType, ok := inferVariableFlowHoverType(snapshot.nodes, line, name, ctx); ok {
		return flowType
	}
	return ""
}

func identifierColumnOnLine(text string, pos lsp.Position, name string) int {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return int(pos.Character) + 1
	}
	line := lines[pos.Line]
	col := int(pos.Character)
	if col > len(line) {
		col = len(line)
	}
	needle := "$" + name
	if idx := strings.LastIndex(line[:col], needle); idx >= 0 {
		return idx + 2
	}
	if idx := strings.LastIndex(line[:col], name); idx >= 0 {
		return idx + 1
	}
	return col + 1
}

func (p *CompletionProvider) propertyClassName(className, propertyName string) string {
	if className == "" || propertyName == "" || p.idx == nil {
		return ""
	}
	resolver := workspaceSymbolResolver{idx: p.idx}
	property, ok := resolver.ResolveProperty(className, propertyName)
	if !ok {
		return ""
	}
	return classNameFromType(property.Type)
}

func (p *CompletionProvider) classMemberSymbols(className string) []*indexer.Symbol {
	if p.idx == nil || className == "" {
		return nil
	}
	resolver := workspaceSymbolResolver{idx: p.idx}
	index := p.idx.GetIndex()
	seen := map[string]struct{}{}
	var members []*indexer.Symbol
	for _, ancestor := range resolver.classLineage(className) {
		classSym, ok := resolver.resolveClassSymbol(ancestor)
		if !ok {
			continue
		}
		prefix := strings.ToLower(classSym.FQN + "::")
		for _, sym := range index.GetByURI(classSym.URI) {
			if !strings.HasPrefix(strings.ToLower(sym.FQN), prefix) {
				continue
			}
			switch sym.Kind {
			case indexer.KindMethod, indexer.KindConstructor, indexer.KindProperty, indexer.KindField, indexer.KindConstant, indexer.KindEnumMember:
			default:
				continue
			}
			key := strings.ToLower(sym.Name)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			members = append(members, sym)
		}
	}
	return members
}

func (p *CompletionProvider) documentSnapshot(uri, text string) semanticSnapshot {
	if p.cache != nil {
		return p.cache.snapshot(uri, text)
	}
	return parseSemanticSnapshot(text)
}

func (p *CompletionProvider) analysisContext(uri, text string, snapshot semanticSnapshot) *analyse.AnalysisContext {
	if p.cache != nil {
		return p.cache.analysisContextForFile(p.idx, uri, uriToFilename(uri), text, snapshot.nodes)
	}
	return analysisContextFromSnapshot(nil, nil, p.idx)
}

func (p *CompletionProvider) maxItems() int {
	if p.cfg.MaxCompletionItems <= 0 {
		return 100
	}
	return p.cfg.MaxCompletionItems
}

func (p *CompletionProvider) atMaxItems(count int) bool {
	return count >= p.maxItems()
}

func memberMatchesAccess(sym *indexer.Symbol, staticAccess, fromThis bool) bool {
	if !fromThis && strings.EqualFold(sym.Visibility, "private") {
		return false
	}
	if !fromThis && !staticAccess && strings.EqualFold(sym.Visibility, "protected") {
		return false
	}
	switch sym.Kind {
	case indexer.KindMethod, indexer.KindConstructor:
		return true
	case indexer.KindProperty, indexer.KindField:
		if staticAccess {
			return sym.IsStatic
		}
		return !sym.IsStatic || fromThis
	case indexer.KindConstant, indexer.KindEnumMember:
		return staticAccess
	default:
		return false
	}
}

func memberKindRank(kind indexer.SymbolKind) string {
	switch kind {
	case indexer.KindMethod, indexer.KindConstructor:
		return "m_"
	case indexer.KindProperty, indexer.KindField:
		return "p_"
	default:
		return "z_"
	}
}

type memberAccess struct {
	receiver string
	static   bool
}

func (a memberAccess) receiverIsThis() bool {
	root := ""
	if parts := splitObjectChain(a.receiver); len(parts) > 0 {
		root = parts[0]
	}
	switch strings.ToLower(root) {
	case "$this", "self", "static", "parent":
		return true
	}
	return false
}

func memberAccessAt(text string, pos lsp.Position) (memberAccess, bool) {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return memberAccess{}, false
	}
	line := lines[pos.Line]
	col := int(pos.Character)
	if col > len(line) {
		col = len(line)
	}
	prefix := wordAt(text, pos)
	head := strings.TrimRightFunc(line[:col-len(prefix)], unicode.IsSpace)
	switch {
	case strings.HasSuffix(head, "?->"):
		return memberAccess{receiver: strings.TrimSpace(head[:len(head)-3])}, true
	case strings.HasSuffix(head, "->"):
		return memberAccess{receiver: strings.TrimSpace(head[:len(head)-2])}, true
	case strings.HasSuffix(head, "::"):
		return memberAccess{receiver: strings.TrimSpace(head[:len(head)-2]), static: true}, true
	default:
		return memberAccess{}, false
	}
}

func splitObjectChain(expr string) []string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	expr = strings.ReplaceAll(expr, "?->", "->")
	parts := strings.Split(expr, "->")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, strings.TrimSuffix(part, "()"))
	}
	return out
}

func enclosingClassLikeName(nodes []ast.Node, line, col int) string {
	var found string
	var walk func([]ast.Node, string)
	walk = func(current []ast.Node, namespace string) {
		for _, node := range current {
			switch n := node.(type) {
			case *ast.NamespaceNode:
				ns := n.Name
				if ns == "" {
					ns = namespace
				}
				walk(n.Body, ns)
			case *ast.ClassNode:
				if sourceContainsPos(n.GetPos(), n.GetEndPos(), line, col) {
					found = qualifyName(namespace, n.Name)
					walk(n.Methods, namespace)
				}
			case *ast.TraitNode:
				if n.Name != nil && sourceContainsPos(n.GetPos(), n.GetEndPos(), line, col) {
					found = qualifyName(namespace, n.Name.Name)
					walk(n.Body, namespace)
				}
			case *ast.EnumNode:
				if sourceContainsPos(n.GetPos(), n.GetEndPos(), line, col) {
					found = qualifyName(namespace, n.Name)
					walk(n.Methods, namespace)
				}
			case *ast.FunctionNode:
				if sourceContainsPos(n.GetPos(), n.GetEndPos(), line, col) {
					walk(n.Body, namespace)
				}
			}
		}
	}
	walk(nodes, "")
	return found
}

func qualifyName(namespace, name string) string {
	name = strings.TrimPrefix(name, `\`)
	if namespace == "" {
		return name
	}
	return strings.TrimPrefix(namespace, `\`) + `\` + name
}

func sourceContainsPos(start, end ast.Position, line, col int) bool {
	if start.Line == 0 {
		return false
	}
	if line < start.Line || (line == start.Line && col < start.Column) {
		return false
	}
	if end.Line == 0 {
		return true
	}
	if line > end.Line {
		return false
	}
	if line == end.Line && col > end.Column && end.Column > 0 {
		return false
	}
	return true
}

func classLikeSymbolBeforePosition(symbols []*indexer.Symbol, pos lsp.Position) *indexer.Symbol {
	var best *indexer.Symbol
	for _, sym := range symbols {
		if !isClassLikeKind(sym.Kind) {
			continue
		}
		if positionBeforeSymbolStart(pos, sym) {
			continue
		}
		if best == nil || symbolRankCloser(sym, best) {
			best = sym
		}
	}
	return best
}

func positionBeforeSymbolStart(pos lsp.Position, sym *indexer.Symbol) bool {
	startLine := sym.StartLine
	startChar := sym.StartChar
	if startLine == 0 && startChar == 0 && sym.Range.Start.Line > 0 {
		startLine = uint32(max(sym.Range.Start.Line-1, 0))
		startChar = uint32(max(sym.Range.Start.Column-1, 0))
	}
	return pos.Line < startLine || (pos.Line == startLine && pos.Character < startChar)
}

func symbolRankCloser(candidate, current *indexer.Symbol) bool {
	if candidate.StartLine > current.StartLine {
		return true
	}
	if candidate.StartLine == current.StartLine && candidate.StartChar >= current.StartChar {
		return true
	}
	return false
}

func classNameFromType(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "?")
	raw = strings.ReplaceAll(raw, "&", "|")
	for _, part := range strings.Split(raw, "|") {
		part = strings.TrimSpace(part)
		if i := strings.IndexByte(part, '<'); i >= 0 {
			part = part[:i]
		}
		part = strings.TrimSuffix(part, "[]")
		if part == "" || isCompletionScalarType(part) {
			continue
		}
		return ensureLeadingSlash(part)
	}
	return ""
}

func isCompletionScalarType(name string) bool {
	switch strings.ToLower(strings.TrimPrefix(name, `\`)) {
	case "array", "bool", "boolean", "callable", "false", "float", "int", "integer", "iterable", "mixed", "never", "null", "object", "resource", "string", "true", "void", "self", "static", "parent":
		return true
	default:
		return false
	}
}

func symbolToCompletionItem(sym *indexer.Symbol) lsp.CompletionItem {
	kind := lsp.CompletionItemKindText
	switch sym.Kind {
	case indexer.KindClass, indexer.KindStruct:
		kind = lsp.CompletionItemKindClass
	case indexer.KindInterface:
		kind = lsp.CompletionItemKindInterface
	case indexer.KindFunction:
		kind = lsp.CompletionItemKindFunction
	case indexer.KindMethod:
		kind = lsp.CompletionItemKindMethod
	case indexer.KindConstructor:
		kind = lsp.CompletionItemKindConstructor
	case indexer.KindProperty:
		kind = lsp.CompletionItemKindProperty
	case indexer.KindField:
		kind = lsp.CompletionItemKindField
	case indexer.KindConstant:
		kind = lsp.CompletionItemKindConstant
	case indexer.KindEnum:
		kind = lsp.CompletionItemKindEnum
	case indexer.KindEnumMember:
		kind = lsp.CompletionItemKindEnumMember
	case indexer.KindModule:
		kind = lsp.CompletionItemKindModule
	}
	return lsp.CompletionItem{
		Label:  sym.Name,
		Kind:   kind,
		Detail: sym.FQN,
		Documentation: &lsp.MarkupContent{
			Kind:  "markdown",
			Value: sym.DocComment,
		},
	}
}

func phpKeywordCompletions(prefix string) []lsp.CompletionItem {
	kws := []string{
		"abstract", "array", "as", "break", "callable", "case", "catch",
		"class", "clone", "const", "continue", "declare", "default", "do",
		"echo", "else", "elseif", "empty", "enum", "extends", "final",
		"finally", "fn", "for", "foreach", "function", "global", "goto",
		"if", "implements", "include", "include_once", "instanceof",
		"interface", "isset", "list", "match", "namespace", "new", "null",
		"print", "private", "protected", "public", "readonly", "require",
		"require_once", "return", "self", "static", "switch", "throw",
		"trait", "true", "false", "try", "unset", "use", "var", "while",
		"yield", "yield from",
	}
	prefix = strings.ToLower(prefix)
	items := make([]lsp.CompletionItem, 0, len(kws))
	for _, kw := range kws {
		if prefix != "" && !strings.HasPrefix(kw, prefix) {
			continue
		}
		items = append(items, lsp.CompletionItem{
			Label:    kw,
			Kind:     lsp.CompletionItemKindKeyword,
			SortText: completionSortKeywords + kw,
		})
	}
	return items
}

// wordAt returns the identifier fragment ending at pos.
func wordAt(text string, pos lsp.Position) string {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	col := int(pos.Character)
	if col > len(line) {
		col = len(line)
	}
	start := col
	for start > 0 && isIdentChar(rune(line[start-1])) {
		start--
	}
	return line[start:col]
}

func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '\\'
}
