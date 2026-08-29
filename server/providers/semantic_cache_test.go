package providers

import (
	"testing"

	"github.com/ayanozturk/go-php-parser/analyse"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func TestSemanticDocumentCacheAnalysisContextUsesSemanticSnapshotReaders(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(): void { $value = 'ready'; echo $value; }\n"
	)

	parsed := indexer.ParseSource(uri, text)
	cache := newSemanticDocumentCache()
	ctx := cache.analysisContextForFile(indexer.New(indexer.Config{}), uri, filename, text, parsed.Nodes)

	if ctx == nil {
		t.Fatal("expected an analysis context")
	}
	if ctx.Facts == nil {
		t.Fatal("expected semantic facts reader backed by the snapshot")
	}
	if ctx.Flow == nil {
		t.Fatal("expected flow reader backed by the snapshot")
	}
	if ctx.VariableFlow == nil {
		t.Fatal("expected variable-flow reader backed by the snapshot")
	}
	if _, ok := ctx.Facts.(*analyse.SemanticSnapshot); !ok {
		t.Fatalf("expected facts reader to be a semantic snapshot, got %T", ctx.Facts)
	}
	if _, ok := ctx.Flow.(*analyse.SemanticSnapshot); !ok {
		t.Fatalf("expected flow reader to be a semantic snapshot, got %T", ctx.Flow)
	}
	if _, ok := ctx.VariableFlow.(*analyse.SemanticSnapshot); !ok {
		t.Fatalf("expected variable-flow reader to be a semantic snapshot, got %T", ctx.VariableFlow)
	}
}

func TestSemanticDocumentCacheReusesAnalysisSnapshotForUnchangedRevision(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(): void { $value = 'ready'; echo $value; }\n"
	)

	idx := indexer.New(indexer.Config{})
	parsed := indexer.ParseSource(uri, text)
	cache := newSemanticDocumentCache()
	first := cache.analysisContextForFile(idx, uri, filename, text, parsed.Nodes)
	second := cache.analysisContextForFile(idx, uri, filename, text, parsed.Nodes)

	if first == second {
		t.Fatal("expected a fresh analysis context for each caller")
	}
	firstSnapshot := semanticSnapshotFromContext(t, first)
	secondSnapshot := semanticSnapshotFromContext(t, second)
	if firstSnapshot != secondSnapshot {
		t.Fatal("expected unchanged text and project revision to reuse the semantic snapshot")
	}
	if first.Facts != second.Facts || first.Flow != second.Flow || first.VariableFlow != second.VariableFlow {
		t.Fatal("expected all semantic readers to be reused with the cached snapshot")
	}
}

func TestSemanticDocumentCacheRebuildsAfterAnotherIndexedFileChanges(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(): void { echo 'ready'; }\n"
		otherURI = "file:///workspace/Dependency.php"
	)

	idx := indexer.New(indexer.Config{})
	idx.IndexDocument(otherURI, "<?php\nclass Dependency {}\n")
	parsed := indexer.ParseSource(uri, text)
	cache := newSemanticDocumentCache()
	first := cache.analysisContextForFile(idx, uri, filename, text, parsed.Nodes)

	idx.IndexDocument(otherURI, "<?php\nclass Dependency { public function changed(): void {} }\n")
	second := cache.analysisContextForFile(idx, uri, filename, text, parsed.Nodes)

	if firstSnapshot, secondSnapshot := semanticSnapshotFromContext(t, first), semanticSnapshotFromContext(t, second); firstSnapshot == secondSnapshot {
		t.Fatal("expected another indexed-file change to rebuild the semantic snapshot")
	}
}

func TestSemanticDocumentCacheRebuildsAfterDocumentTextChanges(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
	)

	idx := indexer.New(indexer.Config{})
	cache := newSemanticDocumentCache()
	firstText := "<?php\nfunction run(): void { echo 'ready'; }\n"
	firstParsed := indexer.ParseSource(uri, firstText)
	first := cache.analysisContextForFile(idx, uri, filename, firstText, firstParsed.Nodes)

	secondText := "<?php\nfunction run(): void { echo 'updated'; }\n"
	secondParsed := indexer.ParseSource(uri, secondText)
	second := cache.analysisContextForFile(idx, uri, filename, secondText, secondParsed.Nodes)

	if firstSnapshot, secondSnapshot := semanticSnapshotFromContext(t, first), semanticSnapshotFromContext(t, second); firstSnapshot == secondSnapshot {
		t.Fatal("expected changed document text to rebuild the semantic snapshot")
	}
}

func TestSemanticDocumentCacheForgetReleasesDocumentState(t *testing.T) {
	const (
		uri      = "file:///workspace/Example.php"
		filename = "/workspace/Example.php"
		text     = "<?php\nfunction run(): void { echo 'ready'; }\n"
	)

	cache := newSemanticDocumentCache()
	parsed := cache.snapshot(uri, text)
	cache.analysisContextForFile(indexer.New(indexer.Config{}), uri, filename, text, parsed.nodes)

	cache.forget(uri)
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if _, ok := cache.byURI[uri]; ok {
		t.Fatal("expected parsed document state to be released")
	}
	if _, ok := cache.analysis[uri]; ok {
		t.Fatal("expected semantic document state to be released")
	}
}

func semanticSnapshotFromContext(t *testing.T, ctx *analyse.AnalysisContext) *analyse.SemanticSnapshot {
	t.Helper()
	if ctx == nil {
		t.Fatal("expected non-nil analysis context")
	}
	snapshot, ok := ctx.Facts.(*analyse.SemanticSnapshot)
	if !ok || snapshot == nil {
		t.Fatalf("expected semantic snapshot facts reader, got %T", ctx.Facts)
	}
	return snapshot
}
