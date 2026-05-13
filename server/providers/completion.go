package providers

import (
	"strings"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// CompletionProvider produces completion items for a given position.
type CompletionProvider struct {
	idx *indexer.WorkspaceIndexer
	cfg Config
}

func (p *CompletionProvider) Provide(uri, text string, pos lsp.Position, ctx *lsp.CompletionContext) []lsp.CompletionItem {
	// Determine the word being completed at pos
	word := wordAt(text, pos)
	if word == "" {
		return phpKeywordCompletions()
	}

	idx := p.idx.GetIndex()
	var items []lsp.CompletionItem

	// Search indexed symbols
	syms := idx.Search(word)
	for _, sym := range syms {
		items = append(items, symbolToCompletionItem(sym))
		if len(items) >= p.cfg.MaxCompletionItems {
			break
		}
	}
	if len(items) == 0 {
		items = phpKeywordCompletions()
	}
	return items
}

func (p *CompletionProvider) Resolve(item lsp.CompletionItem) lsp.CompletionItem {
	// Enrich with full documentation when resolving
	return item
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
	case indexer.KindProperty, indexer.KindField:
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

func phpKeywordCompletions() []lsp.CompletionItem {
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
	items := make([]lsp.CompletionItem, len(kws))
	for i, kw := range kws {
		items[i] = lsp.CompletionItem{
			Label: kw,
			Kind:  lsp.CompletionItemKindKeyword,
		}
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
