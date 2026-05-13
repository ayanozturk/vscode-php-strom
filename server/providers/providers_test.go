package providers

import (
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestDefinitionProviderFindsClassFromCursorInsideIdentifier(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Foo.php", "<?php\nclass Foo {}\n")

	provider := &DefinitionProvider{idx: idx}
	text := "<?php\nnew Foo();\n"
	locs := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 4})

	if len(locs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(locs))
	}
	if locs[0].URI != "file:///workspace/Foo.php" {
		t.Fatalf("expected definition in Foo.php, got %s", locs[0].URI)
	}
}

func TestDefinitionProviderPrefersClassLikeExactMatch(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Foo.php", "<?php\nclass Foo {}\n")
	idx.IndexDocument("file:///workspace/Other.php", "<?php\nclass Other { public function Foo() {} }\n")

	provider := &DefinitionProvider{idx: idx}
	text := "<?php\nnew Foo();\n"
	locs := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 5})

	if len(locs) != 1 {
		t.Fatalf("expected a single preferred definition, got %d", len(locs))
	}
	if locs[0].URI != "file:///workspace/Foo.php" {
		t.Fatalf("expected class definition in Foo.php, got %s", locs[0].URI)
	}
}
