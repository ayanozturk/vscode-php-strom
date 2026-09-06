package providers

import (
	"strings"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestCompletionAfterThisRanksClassMembersBeforeKeywords(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	source := `<?php
class Logger {
    public function info(string $message): void {}
}
class User {
    public Logger $logger;
    public function getUser(): self { return $this; }
    public function run(): void {
        $this->
    }
}
`
	uri := "file:///workspace/User.php"
	idx.IndexDocument(uri, source)

	provider := &CompletionProvider{idx: idx, cfg: Config{MaxCompletionItems: 100}, cache: newSemanticDocumentCache()}
	pos := positionAfter(source, "$this->")
	items := provider.Provide(uri, source, pos, nil)

	if hasCompletionLabel(items, "abstract") || hasCompletionLabel(items, "callable") {
		t.Fatalf("object access must not list PHP keywords, got %#v", completionLabels(items))
	}
	if !hasCompletionLabel(items, "logger") {
		t.Fatalf("expected property logger, got %#v", completionLabels(items))
	}
	if !hasCompletionLabel(items, "getUser") {
		t.Fatalf("expected method getUser, got %#v", completionLabels(items))
	}
	if !hasCompletionLabel(items, "run") {
		t.Fatalf("expected method run, got %#v", completionLabels(items))
	}
	logger := completionByLabel(items, "logger")
	getUser := completionByLabel(items, "getUser")
	if logger == nil || getUser == nil {
		t.Fatalf("missing member items")
	}
	if getUser.SortText >= completionSortKeywords || logger.SortText >= completionSortKeywords {
		t.Fatalf("members must sort before keywords: getUser=%q logger=%q", getUser.SortText, logger.SortText)
	}
	if getUser.Kind != lsp.CompletionItemKindMethod {
		t.Fatalf("getUser kind = %d, want method", getUser.Kind)
	}
	if logger.Kind != lsp.CompletionItemKindProperty {
		t.Fatalf("logger kind = %d, want property", logger.Kind)
	}
}

func TestCompletionAfterTypedPropertyContinuesMemberChain(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	source := `<?php
class Logger {
    public function info(string $message): void {}
}
class User {
    public Logger $logger;
    public function run(): void {
        $this->logger->
    }
}
`
	uri := "file:///workspace/User.php"
	idx.IndexDocument(uri, source)
	provider := &CompletionProvider{idx: idx, cfg: Config{MaxCompletionItems: 100}, cache: newSemanticDocumentCache()}
	items := provider.Provide(uri, source, positionAfter(source, "$this->logger->"), nil)
	if !hasCompletionLabel(items, "info") {
		t.Fatalf("expected Logger::info after $this->logger->, got %#v", completionLabels(items))
	}
	if hasCompletionLabel(items, "abstract") {
		t.Fatalf("did not expect keywords after member access, got %#v", completionLabels(items))
	}
}

func TestKeywordCompletionsSortAfterSymbols(t *testing.T) {
	items := phpKeywordCompletions("cl")
	if len(items) == 0 {
		t.Fatal("expected class keyword")
	}
	for _, item := range items {
		if !strings.HasPrefix(item.SortText, completionSortKeywords) {
			t.Fatalf("keyword sortText = %q, want prefix %q", item.SortText, completionSortKeywords)
		}
	}
}

func positionAfter(source, needle string) lsp.Position {
	idx := strings.Index(source, needle)
	if idx < 0 {
		return lsp.Position{}
	}
	idx += len(needle)
	line := strings.Count(source[:idx], "\n")
	lastNL := strings.LastIndex(source[:idx], "\n")
	col := idx
	if lastNL >= 0 {
		col = idx - lastNL - 1
	}
	return lsp.Position{Line: uint32(line), Character: uint32(col)}
}

func completionLabels(items []lsp.CompletionItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	return labels
}

func hasCompletionLabel(items []lsp.CompletionItem, label string) bool {
	return completionByLabel(items, label) != nil
}

func completionByLabel(items []lsp.CompletionItem, label string) *lsp.CompletionItem {
	for i := range items {
		if items[i].Label == label {
			return &items[i]
		}
	}
	return nil
}
