package providers

// This file contains stub implementations for all remaining LSP providers.
// Each provider has the method signatures required by the handler/registry.
// Real logic will be added incrementally.

import (
	"strings"

	"go-phpcs/analyse"
	"go-phpcs/ast"
	goplexer "go-phpcs/lexer"
	goparser "go-phpcs/parser"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// ─── Hover ────────────────────────────────────────────────────────────────────

type HoverProvider struct {
	idx   *indexer.WorkspaceIndexer
	cache *semanticDocumentCache
}

func (p *HoverProvider) Provide(uri, text string, pos lsp.Position) *lsp.Hover {
	var inferredType string
	ident := identifierAt(text, pos)
	if ident != "" {
		snapshot := p.cache.snapshot(uri, text)
		analysisCtx := p.cache.analysisContext(p.idx)
		if resolvedType, ok := analyse.InferTypeAtPosition(snapshot.nodes, int(pos.Line)+1, int(pos.Character)+1, unqualifiedName(ident), analysisCtx); ok {
			inferredType = resolvedType
		}
	}

	word := wordAt(text, pos)
	var sym *indexer.Symbol
	if word != "" && p.idx != nil {
		sym = p.idx.GetIndex().GetByFQN(`\` + word)
		if sym == nil {
			syms := p.idx.GetIndex().Search(word)
			if len(syms) > 0 {
				sym = syms[0]
			}
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

// ─── Definition ───────────────────────────────────────────────────────────────

type DefinitionProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *DefinitionProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	if locs := resolveTypeDefinitionLocations(p.idx, text, pos); len(locs) > 0 {
		return locs
	}

	word := identifierAt(text, pos)
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

	syms := prioritizeDefinitionMatches(p.idx.GetIndex().Search(lookup), lookup)
	var locs []lsp.Location
	for _, s := range syms {
		locs = append(locs, symToLocation(s))
	}
	return locs
}

// ─── Declaration ──────────────────────────────────────────────────────────────

type DeclarationProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *DeclarationProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	return (&DefinitionProvider{idx: p.idx}).Provide(uri, text, pos)
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
	// Simple: return definition location as placeholder
	syms := p.idx.GetIndex().Search(word)
	var locs []lsp.Location
	for _, s := range syms {
		locs = append(locs, symToLocation(s))
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

func (p *SignatureHelpProvider) Provide(uri, text string, pos lsp.Position) *lsp.SignatureHelp {
	return nil
}

// ─── Formatting ───────────────────────────────────────────────────────────────

type FormattingProvider struct{ cfg Config }

func (p *FormattingProvider) Format(uri, text string, opts lsp.FormattingOptions) []lsp.TextEdit {
	return nil
}

func (p *FormattingProvider) FormatRange(uri, text string, r lsp.Range, opts lsp.FormattingOptions) []lsp.TextEdit {
	return nil
}

// ─── Rename ───────────────────────────────────────────────────────────────────

type RenameProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *RenameProvider) Provide(uri, text string, pos lsp.Position, newName string) *lsp.WorkspaceEdit {
	return nil
}

func (p *RenameProvider) Prepare(uri, text string, pos lsp.Position) *lsp.Range {
	return nil
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

	matched := prioritizeDefinitionMatches(idx.GetIndex().Search(lookup), lookup)
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
		return exactClassLike
	}
	if len(exact) > 0 {
		return exact
	}
	return symbols
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
