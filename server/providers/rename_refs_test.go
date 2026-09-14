package providers

import (
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestRenameUsesBoundSymbolNotWordAt(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	a := "<?php\nnamespace A;\nclass Foo {}\n"
	b := "<?php\nnamespace B;\nclass Foo {}\n"
	idx.IndexDocument("file:///workspace/A.php", a)
	idx.IndexDocument("file:///workspace/B.php", b)

	provider := &RenameProvider{idx: idx}
	text := "<?php\nnamespace A;\nfunction f(Foo $x) {}\n"
	// Cursor on Foo in type hint.
	pos := lsp.Position{Line: 2, Character: 12}
	edit := provider.Provide("file:///workspace/Use.php", text, pos, "Bar")
	if edit == nil || len(edit.Changes) == 0 {
		t.Fatal("expected rename edits from bound A\\Foo")
	}
	for uri, edits := range edit.Changes {
		if uri == "file:///workspace/B.php" {
			t.Fatalf("rename must not touch B\\Foo via unqualified spelling; edits=%v", edits)
		}
	}
}

func TestReferencesUsesBoundSymbol(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/A.php", "<?php\nnamespace A;\nclass Foo {}\n")
	idx.IndexDocument("file:///workspace/B.php", "<?php\nnamespace B;\nclass Foo {}\n")

	provider := &ReferencesProvider{idx: idx}
	text := "<?php\nnamespace A;\nfunction f(Foo $x) {}\n"
	pos := lsp.Position{Line: 2, Character: 12}
	locs := provider.Provide("file:///workspace/Use.php", text, pos, true)
	if len(locs) == 0 {
		t.Fatal("expected references for A\\Foo")
	}
	for _, loc := range locs {
		if loc.URI == "file:///workspace/B.php" {
			t.Fatalf("refs must not include B\\Foo: %+v", locs)
		}
	}
}

func TestRenamePrepareUsesExactBoundSpan(t *testing.T) {
	provider := &RenameProvider{}
	text := "<?php\nclass Foo {}\n"
	pos := lsp.Position{Line: 1, Character: 7}
	r := provider.Prepare("file:///workspace/Foo.php", text, pos)
	if r == nil {
		t.Fatal("expected prepare range")
	}
	if r.Start.Line != 1 || r.End.Line != 1 {
		t.Fatalf("unexpected range lines: %+v", r)
	}
	if r.End.Character-r.Start.Character != 3 {
		t.Fatalf("expected exact Foo span width 3, got %+v", r)
	}
}
