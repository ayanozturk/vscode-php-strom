package providers

// This file contains stub implementations for all remaining LSP providers.
// Each provider has the method signatures required by the handler/registry.
// Real logic will be added incrementally.

import (
	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
	"github.com/ayanozturk/vscode-php-strom/parser"
)

// ─── Hover ────────────────────────────────────────────────────────────────────

type HoverProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *HoverProvider) Provide(uri, text string, pos lsp.Position) *lsp.Hover {
	word := wordAt(text, pos)
	if word == "" {
		return nil
	}
	sym := p.idx.GetIndex().GetByFQN(`\` + word)
	if sym == nil {
		syms := p.idx.GetIndex().Search(word)
		if len(syms) > 0 {
			sym = syms[0]
		}
	}
	if sym == nil {
		return nil
	}
	return &lsp.Hover{
		Contents: lsp.MarkupContent{Kind: "markdown", Value: "**" + sym.FQN + "**\n\n" + sym.DocComment},
	}
}

// ─── Definition ───────────────────────────────────────────────────────────────

type DefinitionProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *DefinitionProvider) Provide(uri, text string, pos lsp.Position) []lsp.Location {
	word := wordAt(text, pos)
	syms := p.idx.GetIndex().Search(word)
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
	return nil
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
			Range:          rangeToLSP(s.Range),
			SelectionRange: rangeToLSP(s.Range),
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

// ─── Diagnostics ──────────────────────────────────────────────────────────────

type DiagnosticsProvider struct{ idx *indexer.WorkspaceIndexer }

func (p *DiagnosticsProvider) Analyse(uri, text string) []lsp.Diagnostic {
	return nil
}

// ─── InlineValues ─────────────────────────────────────────────────────────────

type InlineValuesProvider struct{}

func (p *InlineValuesProvider) Provide(uri, text string, r lsp.Range) []lsp.InlineValueVariableLookup {
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func symToLocation(s *indexer.Symbol) lsp.Location {
	return lsp.Location{URI: s.URI, Range: rangeToLSP(s.Range)}
}

func rangeToLSP(r parser.Range) lsp.Range {
	return lsp.Range{} // byte offsets → line/char conversion done in a future pass
}
