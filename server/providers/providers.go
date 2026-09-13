package providers

// This file contains stub implementations for all remaining LSP providers.
// Each provider has the method signatures required by the handler/registry.
// Real logic will be added incrementally.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
	goplexer "github.com/ayanozturk/go-php-parser/lexer"
	goparser "github.com/ayanozturk/go-php-parser/parser"
	"github.com/ayanozturk/go-php-parser/syntax"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// ─── Hover ────────────────────────────────────────────────────────────────────

type HoverProvider struct {
	idx   *indexer.WorkspaceIndexer
	cache *semanticDocumentCache
}

func (p *HoverProvider) Provide(uri, text string, pos lsp.Position) *lsp.Hover {
	if p.idx != nil {
		p.idx.IndexDocument(uri, text)
	}

	var inferredType string
	var hoverTarget analyse.HoverTarget
	var hasHoverTarget bool
	ident := identifierAt(text, pos)
	if ident != "" {
		snapshot := p.cache.snapshot(uri, text)
		analysisCtx := p.cache.analysisContextForFile(p.idx, uri, uriToFilename(uri), text, snapshot.nodes)
		hoverTarget, hasHoverTarget = analyse.InferHoverTargetAtPosition(snapshot.nodes, int(pos.Line)+1, int(pos.Character)+1, unqualifiedName(ident), analysisCtx)
		if hasHoverTarget {
			inferredType = hoverTarget.Type
		}
		if flowType, ok := inferVariableFlowHoverType(snapshot.nodes, int(pos.Line)+1, unqualifiedName(ident), analysisCtx); ok {
			inferredType = flowType
			if !hasHoverTarget {
				hoverTarget = analyse.HoverTarget{Kind: analyse.HoverTargetVariable, Type: flowType}
				hasHoverTarget = true
			}
		}
	}

	var sym *indexer.Symbol
	if p.idx != nil {
		sym = resolveHoverSymbol(p.idx, uri, text, pos, ident, hoverTarget, hasHoverTarget)
	}
	if inferredType == "" || inferredType == "mixed" {
		if memberType := hoverTypeFromSymbol(sym); memberType != "" {
			inferredType = memberType
		}
	}

	value := formatHoverContents(inferredType, sym)
	if value == "" {
		return nil
	}
	return &lsp.Hover{
		Contents: lsp.MarkupContent{Kind: "markdown", Value: value},
	}
}

func formatHoverContents(inferredType string, sym *indexer.Symbol) string {
	parts := make([]string, 0, 2)
	if inferredType != "" {
		parts = append(parts, "```php\n"+inferredType+"\n```")
	}
	if sym != nil {
		doc := "**" + sym.FQN + "**"
		if sym.DocComment != "" {
			doc += "\n\n" + sym.DocComment
		}
		parts = append(parts, doc)
	}
	return strings.Join(parts, "\n\n")
}

func hoverTypeFromSymbol(sym *indexer.Symbol) string {
	if sym == nil {
		return ""
	}
	if sym.Kind == indexer.KindMethod && sym.ReturnType != "" {
		return sym.ReturnType
	}
	if sym.Kind == indexer.KindProperty && sym.Type != "" {
		return sym.Type
	}
	return ""
}

// ─── Definition ───────────────────────────────────────────────────────────────

type DefinitionProvider struct {
	idx   *indexer.WorkspaceIndexer
	cache *semanticDocumentCache
}

func (p *DefinitionProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	word := identifierAt(text, pos)

	// Member accesses (`$obj->member`, `Class::member`) must resolve against
	// the receiver's type, not by matching the bare member name against every
	// symbol in the workspace. Skipping this check let `$this->cache->set(...)`
	// resolve to unrelated classes literally named `Set` anywhere in the
	// index (including vendor/), instead of CacheInterface::set().
	if p.cache != nil && word != "" {
		snapshot := p.cache.snapshot(uri, text)
		analysisCtx := p.cache.analysisContextForFile(p.idx, uri, uriToFilename(uri), text, snapshot.nodes)
		if hoverTarget, ok := analyse.InferHoverTargetAtPosition(snapshot.nodes, int(pos.Line)+1, int(pos.Character)+1, unqualifiedName(word), analysisCtx); ok && hoverTarget.ReceiverClass != "" {
			switch hoverTarget.Kind {
			case analyse.HoverTargetMethod:
				if sym := resolveMethodSymbol(p.idx, hoverTarget.ReceiverClass, unqualifiedName(word)); sym != nil {
					return []lsp.Location{symToLocation(sym)}
				}
				return nil
			case analyse.HoverTargetProperty:
				if sym := resolvePropertySymbol(p.idx, hoverTarget.ReceiverClass, unqualifiedName(word)); sym != nil {
					return []lsp.Location{symToLocation(sym)}
				}
				return nil
			}
		}
	}

	if locs := resolveTypeDefinitionLocations(p.idx, text, pos); len(locs) > 0 {
		return locs
	}

	if word == "" {
		return nil
	}

	if sym := p.idx.GetIndex().GetByFQN(ensureLeadingSlash(word)); sym != nil {
		return []lsp.Location{symToLocation(sym)}
	}

	lookup := unqualifiedName(word)
	if lookup == "" {
		return nil
	}

	syms := prioritizeDefinitionMatches(p.idx.GetIndex().GetByName(lookup), lookup)
	var locs []lsp.Location
	for _, s := range syms {
		locs = append(locs, symToLocation(s))
	}
	return locs
}

// ─── Declaration ──────────────────────────────────────────────────────────────

type DeclarationProvider struct {
	idx   *indexer.WorkspaceIndexer
	cache *semanticDocumentCache
}

func (p *DeclarationProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	return (&DefinitionProvider{idx: p.idx, cache: p.cache}).Provide(uri, text, pos)
}

// ─── TypeDefinition ───────────────────────────────────────────────────────────

type TypeDefinitionProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *TypeDefinitionProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	return resolveTypeDefinitionLocations(p.idx, text, pos)
}

// ─── Implementation ───────────────────────────────────────────────────────────

type ImplementationProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *ImplementationProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	return nil
}

// ─── References ───────────────────────────────────────────────────────────────

type ReferencesProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *ReferencesProvider) Provide(uri, text string, pos lsp.Position, includeDecl bool) []lsp.Location {
	word := wordAt(text, pos)
	if word == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []lsp.Location
	add := func(loc lsp.Location) {
		key := loc.URI + ":" + strconv.FormatUint(uint64(loc.Range.Start.Line), 10) + ":" +
			strconv.FormatUint(uint64(loc.Range.Start.Character), 10)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, loc)
	}
	// Prefer project-scoped binder UsageGraph when the indexer has one.
	if p.idx != nil {
		if g := p.idx.UsageGraph(); g != nil {
			for _, use := range g.FindByName(word) {
				add(nameUseToLocation(use, text, uri))
			}
		}
	}
	for _, loc := range sameFileNameUses(uri, text, word) {
		add(loc)
	}
	for _, s := range prioritizeDefinitionMatches(p.idx.GetIndex().GetByName(word), word) {
		add(symToLocation(s))
	}
	if len(out) > 0 {
		return out
	}
	for _, s := range p.idx.GetIndex().Search(word) {
		add(symToLocation(s))
	}
	return out
}

func nameUseToLocation(use analyse.NameUse, openText, openURI string) lsp.Location {
	uri := use.URI
	if uri == "" {
		uri = openURI
	}
	start := lsp.Position{Line: uint32(use.StartLine), Character: uint32(use.StartChar)}
	end := lsp.Position{Line: uint32(use.EndLine), Character: uint32(use.EndChar)}
	if uri == openURI && (use.EndLine == 0 && use.EndChar == 0 && use.Span.End > use.Span.Start) {
		start = offsetToPosition(openText, use.Span.Start)
		end = offsetToPosition(openText, use.Span.End)
	}
	return lsp.Location{
		URI: uri,
		Range: lsp.Range{
			Start: start,
			End:   end,
		},
	}
}

func sameFileNameUses(uri, text, word string) []lsp.Location {
	res := syntax.Parse([]byte(text))
	if res == nil || res.File == nil || res.File.Root == nil {
		return nil
	}
	b := analyse.NewBinder("", nil)
	var walk func(*syntax.RedNode)
	walk = func(n *syntax.RedNode) {
		if n == nil {
			return
		}
		switch n.Kind() {
		case syntax.KindUnqualifiedName, syntax.KindQualifiedName,
			syntax.KindFullyQualifiedName, syntax.KindRelativeName:
			b.BindName(n, "name")
		}
		for _, c := range n.Children() {
			walk(c)
		}
	}
	walk(res.File.Root)

	var locs []lsp.Location
	lower := strings.ToLower(word)
	for _, use := range b.Graph.Uses {
		written := use.Written
		if i := strings.LastIndexByte(written, '\\'); i >= 0 {
			written = written[i+1:]
		}
		if !strings.EqualFold(written, word) && strings.ToLower(use.Resolved) != lower {
			continue
		}
		start := offsetToPosition(text, use.Span.Start)
		end := offsetToPosition(text, use.Span.End)
		locs = append(locs, lsp.Location{
			URI: uri,
			Range: lsp.Range{
				Start: start,
				End:   end,
			},
		})
	}
	return locs
}

// ─── DocumentHighlight ────────────────────────────────────────────────────────

type HighlightProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *HighlightProvider) Provide(uri, text string, pos lsp.Position) []lsp.DocumentHighlight {
	return nil
}

// ─── Symbol ───────────────────────────────────────────────────────────────────

type SymbolProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *SymbolProvider) ProvideDocument(uri, text string) []lsp.DocumentSymbol {
	syms := p.idx.GetIndex().GetByURI(uri)
	var ds []lsp.DocumentSymbol
	for _, s := range syms {
		ds = append(ds, lsp.DocumentSymbol{
			Name:           s.Name,
			Kind:           lsp.SymbolKind(s.Kind),
			Range:          rangeToLSP(s),
			SelectionRange: rangeToLSP(s),
		})
	}
	return ds
}

func (p *SymbolProvider) ProvideWorkspace(query string) []lsp.SymbolInformation {
	var syms []*indexer.Symbol
	if query == "" {
		syms = p.idx.GetIndex().AllSymbols()
	} else {
		syms = p.idx.GetIndex().Search(query)
	}
	var infos []lsp.SymbolInformation
	for _, s := range syms {
		infos = append(infos, lsp.SymbolInformation{
			Name:     s.Name,
			Kind:     lsp.SymbolKind(s.Kind),
			Location: symToLocation(s),
		})
	}
	return infos
}

// ─── SignatureHelp ────────────────────────────────────────────────────────────

type SignatureHelpProvider struct{ idx *indexer.WorkspaceIndexer }

// Provide is implemented in signature_help.go.

// ─── Formatting ───────────────────────────────────────────────────────────────

type FormattingProvider struct{ cfg Config }

func (p *FormattingProvider) Format(uri, text string, opts lsp.FormattingOptions) []lsp.TextEdit {
	// Identity reprint from the lossless token/green tree. Style rewrites come later.
	src := []byte(text)
	file := syntax.ParseTokens(src)
	printed := syntax.Print(file.Root)
	if printed == text {
		return nil
	}
	end := offsetToPosition(text, len(text))
	return []lsp.TextEdit{{
		Range: lsp.Range{
			Start: lsp.Position{Line: 0, Character: 0},
			End:   end,
		},
		NewText: printed,
	}}
}

func (p *FormattingProvider) FormatRange(uri, text string, r lsp.Range, opts lsp.FormattingOptions) []lsp.TextEdit {
	// Range formatting falls back to full-file identity until trivia edits exist.
	return p.Format(uri, text, opts)
}

func offsetToPosition(text string, offset int) lsp.Position {
	if offset > len(text) {
		offset = len(text)
	}
	line, col := 0, 0
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return lsp.Position{Line: uint32(line), Character: uint32(col)}
}

// ─── Rename ───────────────────────────────────────────────────────────────────

type RenameProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *RenameProvider) Provide(uri, text string, pos lsp.Position, newName string) *lsp.WorkspaceEdit {
	word := wordAt(text, pos)
	if word == "" || newName == "" {
		return nil
	}
	changes := map[string][]lsp.TextEdit{}
	addEdit := func(loc lsp.Location) {
		changes[loc.URI] = append(changes[loc.URI], lsp.TextEdit{Range: loc.Range, NewText: newName})
	}
	if p.idx != nil {
		if g := p.idx.UsageGraph(); g != nil {
			for _, use := range g.FindByName(word) {
				addEdit(nameUseToLocation(use, text, uri))
			}
		}
	}
	for _, loc := range sameFileNameUses(uri, text, word) {
		addEdit(loc)
	}
	for _, s := range prioritizeDefinitionMatches(p.idx.GetIndex().GetByName(word), word) {
		addEdit(symToLocation(s))
	}
	if len(changes) == 0 {
		return nil
	}
	return &lsp.WorkspaceEdit{Changes: changes}
}

func (p *RenameProvider) Prepare(uri, text string, pos lsp.Position) *lsp.Range {
	word, start, end := wordSpanAt(text, pos)
	if word == "" {
		return nil
	}
	_ = uri
	s := offsetToPosition(text, start)
	e := offsetToPosition(text, end)
	return &lsp.Range{Start: s, End: e}
}

func wordSpanAt(text string, pos lsp.Position) (string, int, int) {
	offset := 0
	line, col := 0, 0
	for i := 0; i < len(text); i++ {
		if line == int(pos.Line) && col == int(pos.Character) {
			offset = i
			break
		}
		if text[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
		offset = i + 1
	}
	isIdent := func(c byte) bool {
		return c == '_' || c == '\\' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
	}
	start, end := offset, offset
	for start > 0 && isIdent(text[start-1]) {
		start--
	}
	for end < len(text) && isIdent(text[end]) {
		end++
	}
	if start == end {
		return "", 0, 0
	}
	return text[start:end], start, end
}

// ─── FoldingRange ─────────────────────────────────────────────────────────────

type FoldingProvider struct{}

func (p *FoldingProvider) Provide(uri, text string) []lsp.FoldingRange {
	return nil
}

// ─── SelectionRange ───────────────────────────────────────────────────────────

type SelectionRangeProvider struct{}

func (p *SelectionRangeProvider) Provide(uri, text string, positions []lsp.Position) []lsp.SelectionRange {
	var results []lsp.SelectionRange
	for range positions {
		results = append(results, lsp.SelectionRange{})
	}
	return results
}

// ─── CodeAction ───────────────────────────────────────────────────────────────

type CodeActionProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *CodeActionProvider) Provide(uri, text string, r lsp.Range, ctx lsp.CodeActionContext) []lsp.CodeAction {
	return nil
}

func (p *CodeActionProvider) Resolve(a lsp.CodeAction) lsp.CodeAction {
	return a
}

// ─── CodeLens ─────────────────────────────────────────────────────────────────

type CodeLensProvider struct {
	idx *indexer.WorkspaceIndexer
	cfg Config
}

func (p *CodeLensProvider) Provide(uri, text string) []lsp.CodeLens {
	return nil
}

func (p *CodeLensProvider) Resolve(l lsp.CodeLens) lsp.CodeLens {
	return l
}

// ─── InlayHints ───────────────────────────────────────────────────────────────

type InlayHintsProvider struct {
	idx *indexer.WorkspaceIndexer
	cfg Config
}

func (p *InlayHintsProvider) Provide(uri, text string, r lsp.Range) []lsp.InlayHint {
	return nil
}

// ─── DocumentLinks ────────────────────────────────────────────────────────────

type DocumentLinksProvider struct{}

func (p *DocumentLinksProvider) Provide(uri, text string) []lsp.DocumentLink {
	return nil
}

// ─── TypeHierarchy ────────────────────────────────────────────────────────────

type TypeHierarchyProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *TypeHierarchyProvider) Prepare(uri, text string, pos lsp.Position) []lsp.TypeHierarchyItem {
	return nil
}

func (p *TypeHierarchyProvider) Supertypes(item lsp.TypeHierarchyItem) []lsp.TypeHierarchyItem {
	return nil
}

func (p *TypeHierarchyProvider) Subtypes(item lsp.TypeHierarchyItem) []lsp.TypeHierarchyItem {
	return nil
}

// ─── InlineValues ─────────────────────────────────────────────────────────────

type InlineValuesProvider struct{}

func (p *InlineValuesProvider) Provide(uri, text string, r lsp.Range) []lsp.InlineValueVariableLookup {
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func symToLocation(s *indexer.Symbol) lsp.Location {
	return lsp.Location{URI: s.URI, Range: rangeToLSP(s)}
}

func rangeToLSP(s *indexer.Symbol) lsp.Range {
	return lsp.Range{
		Start: lsp.Position{Line: s.StartLine, Character: s.StartChar},
		End:   lsp.Position{Line: s.EndLine, Character: s.EndChar},
	}
}

type documentTypeContext struct {
	namespace string
	aliases   map[string]string
}

func resolveTypeDefinitionLocations(idx *indexer.WorkspaceIndexer, text string, pos lsp.Position) []lsp.Location {
	ident := identifierAt(text, pos)
	if ident == "" {
		return nil
	}

	for _, candidate := range resolveTypeCandidates(text, ident) {
		sym := idx.GetIndex().GetByFQN(candidate)
		if sym == nil || !isClassLikeKind(sym.Kind) {
			continue
		}
		return []lsp.Location{symToLocation(sym)}
	}

	lookup := unqualifiedName(ident)
	if lookup == "" {
		return nil
	}

	matched := prioritizeDefinitionMatches(idx.GetIndex().GetByName(lookup), lookup)
	var locs []lsp.Location
	for _, sym := range matched {
		if isClassLikeKind(sym.Kind) {
			locs = append(locs, symToLocation(sym))
		}
	}
	return locs
}

func resolveTypeCandidates(text, ident string) []string {
	if ident == "" {
		return nil
	}

	ident = strings.TrimSpace(ident)
	if ident == "self" || ident == "static" || ident == "parent" {
		return nil
	}
	if strings.HasPrefix(ident, `\`) {
		return []string{ensureLeadingSlash(ident)}
	}

	ctx := parseDocumentTypeContext(text)
	firstSegment := ident
	remainder := ""
	if idx := strings.Index(ident, `\`); idx >= 0 {
		firstSegment = ident[:idx]
		remainder = ident[idx+1:]
	}

	seen := make(map[string]struct{})
	var candidates []string
	appendCandidate := func(candidate string) {
		candidate = ensureLeadingSlash(candidate)
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	if target, ok := ctx.aliases[strings.ToLower(firstSegment)]; ok {
		if remainder != "" {
			appendCandidate(target + `\` + remainder)
		} else {
			appendCandidate(target)
		}
	}
	if ctx.namespace != "" {
		appendCandidate(`\` + ctx.namespace + `\` + ident)
	}
	appendCandidate(`\` + ident)
	return candidates
}

func parseDocumentTypeContext(text string) documentTypeContext {
	ctx := documentTypeContext{aliases: make(map[string]string)}
	l := goplexer.New(text)
	p := goparser.New(l, false)
	nodes := p.Parse()
	collectTypeContext(nodes, "", &ctx)
	return ctx
}

func collectTypeContext(nodes []ast.Node, currentNS string, ctx *documentTypeContext) {
	namespace := currentNS
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.NamespaceNode:
			if len(n.Body) > 0 {
				if ctx.namespace == "" {
					ctx.namespace = n.Name
				}
				collectTypeContext(n.Body, n.Name, ctx)
				continue
			}
			namespace = n.Name
			if ctx.namespace == "" {
				ctx.namespace = n.Name
			}
		case *ast.UseNode:
			if n.Type != "class" {
				continue
			}
			alias := n.Alias
			if alias == "" {
				alias = unqualifiedName(n.Path)
			}
			ctx.aliases[strings.ToLower(alias)] = ensureLeadingSlash(n.Path)
		}
	}
	if ctx.namespace == "" {
		ctx.namespace = namespace
	}
}

func identifierAt(text string, pos lsp.Position) string {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	col := int(pos.Character)
	if col < 0 {
		col = 0
	}
	if col > len(line) {
		col = len(line)
	}

	start := col
	for start > 0 && isIdentChar(rune(line[start-1])) {
		start--
	}
	end := col
	for end < len(line) && isIdentChar(rune(line[end])) {
		end++
	}
	return line[start:end]
}

func prioritizeDefinitionMatches(symbols []*indexer.Symbol, lookup string) []*indexer.Symbol {
	exactClassLike := make([]*indexer.Symbol, 0)
	exact := make([]*indexer.Symbol, 0)
	seen := make(map[string]struct{})
	lowerLookup := strings.ToLower(lookup)

	for _, sym := range symbols {
		if strings.ToLower(sym.Name) != lowerLookup {
			continue
		}
		if _, ok := seen[sym.FQN]; ok {
			continue
		}
		seen[sym.FQN] = struct{}{}
		exact = append(exact, sym)
		if isClassLikeKind(sym.Kind) {
			exactClassLike = append(exactClassLike, sym)
		}
	}

	if len(exactClassLike) > 0 {
		sortSymbols(exactClassLike)
		return exactClassLike
	}
	if len(exact) > 0 {
		sortSymbols(exact)
		return exact
	}
	sortSymbols(symbols)
	return symbols
}

func resolveHoverSymbol(idx *indexer.WorkspaceIndexer, uri, text string, pos lsp.Position, ident string, hoverTarget analyse.HoverTarget, hasHoverTarget bool) *indexer.Symbol {
	lookup := unqualifiedName(ident)
	if lookup == "" {
		lookup = unqualifiedName(wordAt(text, pos))
	}
	if lookup == "" {
		return nil
	}

	if hasHoverTarget && hoverTarget.ReceiverClass != "" {
		switch hoverTarget.Kind {
		case analyse.HoverTargetMethod:
			if sym := resolveMethodSymbol(idx, hoverTarget.ReceiverClass, lookup); sym != nil {
				return sym
			}
		case analyse.HoverTargetProperty:
			if sym := resolvePropertySymbol(idx, hoverTarget.ReceiverClass, lookup); sym != nil {
				return sym
			}
		}
	}

	if sym := symbolDeclaredAtPosition(idx.GetIndex().GetByURI(uri), lookup, pos); sym != nil {
		return sym
	}

	if hasHoverTarget {
		switch hoverTarget.Kind {
		case analyse.HoverTargetLiteral, analyse.HoverTargetVariable, analyse.HoverTargetMethod, analyse.HoverTargetProperty:
			// Receiver-aware accesses should not degrade into arbitrary global
			// short-name matches when the hovered token is not a declaration-like
			// workspace symbol target.
			return nil
		}
	}

	for _, candidate := range resolveTypeCandidates(text, ident) {
		sym := idx.GetIndex().GetByFQN(candidate)
		if sym == nil {
			continue
		}
		if isClassLikeKind(sym.Kind) {
			return sym
		}
		return sym
	}

	if sym := idx.GetIndex().GetByFQN(ensureLeadingSlash(ident)); sym != nil {
		return sym
	}

	matched := prioritizeDefinitionMatches(idx.GetIndex().GetByName(lookup), lookup)
	if len(matched) == 0 {
		return nil
	}
	return matched[0]
}

func resolveMethodSymbol(idx *indexer.WorkspaceIndexer, className, methodName string) *indexer.Symbol {
	if idx == nil {
		return nil
	}
	resolver := workspaceSymbolResolver{idx: idx}
	classSym, ok := resolver.resolveClassSymbol(className)
	if !ok {
		return nil
	}
	index := idx.GetIndex()
	if sym := index.GetByFQN(classSym.FQN + "::" + methodName); sym != nil && sym.Kind == indexer.KindMethod {
		return sym
	}
	for _, sym := range prioritizeDefinitionMatches(index.GetByName(methodName), methodName) {
		if sym.Kind != indexer.KindMethod {
			continue
		}
		if !strings.HasPrefix(sym.FQN, classSym.FQN+"::") {
			continue
		}
		if strings.EqualFold(sym.Name, methodName) {
			return sym
		}
	}
	return nil
}

func resolvePropertySymbol(idx *indexer.WorkspaceIndexer, className, propertyName string) *indexer.Symbol {
	if idx == nil {
		return nil
	}
	resolver := workspaceSymbolResolver{idx: idx}
	classSym, ok := resolver.resolveClassSymbol(className)
	if !ok {
		return nil
	}
	index := idx.GetIndex()
	if sym := index.GetByFQN(classSym.FQN + "::$" + propertyName); sym != nil && sym.Kind == indexer.KindProperty {
		return sym
	}
	for _, sym := range prioritizeDefinitionMatches(index.GetByName(propertyName), propertyName) {
		if sym.Kind != indexer.KindProperty {
			continue
		}
		if !strings.HasPrefix(sym.FQN, classSym.FQN+"::$") {
			continue
		}
		if strings.EqualFold(sym.Name, propertyName) {
			return sym
		}
	}
	return nil
}

func symbolDeclaredAtPosition(symbols []*indexer.Symbol, lookup string, pos lsp.Position) *indexer.Symbol {
	var containing []*indexer.Symbol
	var sameLine []*indexer.Symbol
	var matches []*indexer.Symbol
	for _, sym := range symbols {
		if !strings.EqualFold(sym.Name, lookup) {
			continue
		}
		matches = append(matches, sym)
		if positionWithinRange(pos, rangeToLSP(sym)) {
			containing = append(containing, sym)
		}
		if sym.StartLine == pos.Line {
			sameLine = append(sameLine, sym)
		}
	}
	if len(containing) > 0 {
		sort.SliceStable(containing, func(i, j int) bool {
			left := symbolSpan(containing[i])
			right := symbolSpan(containing[j])
			if left != right {
				return left < right
			}
			return compareSymbols(containing[i], containing[j]) < 0
		})
		return containing[0]
	}
	if len(sameLine) > 0 {
		sort.SliceStable(sameLine, func(i, j int) bool {
			left := symbolDistanceFromPosition(sameLine[i], pos)
			right := symbolDistanceFromPosition(sameLine[j], pos)
			if left != right {
				return left < right
			}
			return compareSymbols(sameLine[i], sameLine[j]) < 0
		})
		return sameLine[0]
	}
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left := symbolDistanceFromPosition(matches[i], pos)
		right := symbolDistanceFromPosition(matches[j], pos)
		if left != right {
			return left < right
		}
		return compareSymbols(matches[i], matches[j]) < 0
	})
	return matches[0]
}

func positionWithinRange(pos lsp.Position, r lsp.Range) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	return true
}

func symbolSpan(sym *indexer.Symbol) uint64 {
	start := uint64(sym.StartLine)*1_000_000 + uint64(sym.StartChar)
	end := uint64(sym.EndLine)*1_000_000 + uint64(sym.EndChar)
	if end <= start {
		return 0
	}
	return end - start
}

func symbolDistanceFromPosition(sym *indexer.Symbol, pos lsp.Position) uint64 {
	lineDelta := absDiff(sym.StartLine, pos.Line)
	charDelta := absDiff(sym.StartChar, pos.Character)
	return uint64(lineDelta)*1_000_000 + uint64(charDelta)
}

func absDiff(left, right uint32) uint32 {
	if left > right {
		return left - right
	}
	return right - left
}

func sortSymbols(symbols []*indexer.Symbol) {
	sort.SliceStable(symbols, func(i, j int) bool {
		return compareSymbols(symbols[i], symbols[j]) < 0
	})
}

func compareSymbols(left, right *indexer.Symbol) int {
	if left == nil || right == nil {
		switch {
		case left == nil && right == nil:
			return 0
		case left == nil:
			return 1
		default:
			return -1
		}
	}
	if left.Kind != right.Kind {
		if kindRank(left.Kind) < kindRank(right.Kind) {
			return -1
		}
		return 1
	}
	if left.FQN != right.FQN {
		if left.FQN < right.FQN {
			return -1
		}
		return 1
	}
	if left.URI != right.URI {
		if left.URI < right.URI {
			return -1
		}
		return 1
	}
	if left.StartLine != right.StartLine {
		if left.StartLine < right.StartLine {
			return -1
		}
		return 1
	}
	if left.StartChar != right.StartChar {
		if left.StartChar < right.StartChar {
			return -1
		}
		return 1
	}
	return 0
}

func kindRank(kind indexer.SymbolKind) int {
	switch kind {
	case indexer.KindClass, indexer.KindInterface, indexer.KindModule, indexer.KindEnum:
		return 0
	case indexer.KindConstructor:
		return 1
	case indexer.KindMethod:
		return 2
	case indexer.KindProperty:
		return 3
	case indexer.KindFunction:
		return 4
	default:
		return 5
	}
}

func isClassLikeKind(kind indexer.SymbolKind) bool {
	switch kind {
	case indexer.KindClass, indexer.KindInterface, indexer.KindModule, indexer.KindEnum:
		return true
	default:
		return false
	}
}

func ensureLeadingSlash(name string) string {
	if name == "" || strings.HasPrefix(name, `\`) {
		return name
	}
	return `\` + name
}

func unqualifiedName(name string) string {
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, `\`)
	if idx := strings.LastIndex(name, `\`); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
