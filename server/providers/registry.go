package providers

import (
	"github.com/ayanozturk/go-php-parser/overrides"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

// Config is the subset of server configuration needed by providers.
type Config struct {
	PHPVersion                string
	InsertUseDeclaration      bool
	MaxCompletionItems        int
	DocumentRoot              string
	BraceStyle                string
	InsertSpaces              bool
	TabSize                   int
	CodeLensReferences        bool
	CodeLensImplementations   bool
	CodeLensOverrides         bool
	CodeLensParent            bool
	InlayHintsParamNames      bool
	InlayHintsParamTypes      bool
	InlayHintsReturnTypes     bool
	DisableUndefinedSymbols   bool
	DisableUndefinedVariables bool
	DisableTypeErrors         bool
	DiagnosticsExclusions     DiagnosticsPathExclusions
	DiagnosticsOverrides      *overrides.Compiled
}

// Registry holds all LSP feature providers and is the single point of access
// for the LSP handler.
type Registry struct {
	idx *indexer.WorkspaceIndexer
	cfg Config

	Completion     *CompletionProvider
	Hover          *HoverProvider
	Definition     *DefinitionProvider
	Declaration    *DeclarationProvider
	TypeDefinition *TypeDefinitionProvider
	Implementation *ImplementationProvider
	References     *ReferencesProvider
	Highlight      *HighlightProvider
	Symbol         *SymbolProvider
	SignatureHelp  *SignatureHelpProvider
	Formatting     *FormattingProvider
	Rename         *RenameProvider
	Folding        *FoldingProvider
	SelectionRange *SelectionRangeProvider
	CodeAction     *CodeActionProvider
	CodeLens       *CodeLensProvider
	InlayHints     *InlayHintsProvider
	DocumentLinks  *DocumentLinksProvider
	TypeHierarchy  *TypeHierarchyProvider
	Diagnostics    *DiagnosticsProvider
	InlineValues   *InlineValuesProvider
}

// SemanticCacheTrace returns cumulative cache accounting shared by diagnostics,
// hover, definition, and declaration providers.
func (r *Registry) SemanticCacheTrace() SemanticCacheTraceSnapshot {
	if r == nil || r.Diagnostics == nil {
		return SemanticCacheTraceSnapshot{}
	}
	return r.Diagnostics.cache.traceSnapshot()
}

// NewRegistry creates a fully-initialised provider registry.
func NewRegistry(idx *indexer.WorkspaceIndexer, cfg Config) *Registry {
	r := &Registry{idx: idx, cfg: cfg}
	semanticCache := newSemanticDocumentCache()
	r.Completion = &CompletionProvider{idx: idx, cfg: cfg}
	r.Hover = &HoverProvider{idx: idx, cache: semanticCache}
	r.Definition = &DefinitionProvider{idx: idx, cache: semanticCache}
	r.Declaration = &DeclarationProvider{idx: idx, cache: semanticCache}
	r.TypeDefinition = &TypeDefinitionProvider{idx: idx}
	r.Implementation = &ImplementationProvider{idx: idx}
	r.References = &ReferencesProvider{idx: idx}
	r.Highlight = &HighlightProvider{idx: idx}
	r.Symbol = &SymbolProvider{idx: idx}
	r.SignatureHelp = &SignatureHelpProvider{idx: idx}
	r.Formatting = &FormattingProvider{cfg: cfg}
	r.Rename = &RenameProvider{idx: idx}
	r.Folding = &FoldingProvider{}
	r.SelectionRange = &SelectionRangeProvider{}
	r.CodeAction = &CodeActionProvider{idx: idx}
	r.CodeLens = &CodeLensProvider{idx: idx, cfg: cfg}
	r.InlayHints = &InlayHintsProvider{idx: idx, cfg: cfg}
	r.DocumentLinks = &DocumentLinksProvider{}
	r.TypeHierarchy = &TypeHierarchyProvider{idx: idx}
	r.Diagnostics = &DiagnosticsProvider{idx: idx, cfg: cfg, cache: semanticCache}
	r.InlineValues = &InlineValuesProvider{}
	return r
}
